// Package files 는 파일 사이드카의 요청 처리를 구현한다.
//
// 한 줄에 JSON 메시지 하나를 사용한다. 형식은 docs/spec/sidecars.md 의 files 에 정의한다.
//
//	입력  {"surface": id, "root": 경로, "body": {"operation": "list", "id": 요청, "path": 상대 경로}}
//	      {"surface": id, "root": 경로, "body": {"operation": "watch", "id": 요청, "paths": [상대 경로]}}
//	      {"surface": id, "root": 경로, "body": {"operation": "git", "id": 요청}}
//	      {"surface": id, "root": 경로, "body": {"operation": "snapshot", "id": 요청}}
//	      {"surface": id, "root": 경로, "body": {"operation": "expand"|"collapse", "id": 요청, "generation": 세대, "path": 상대 경로}}
//	      {"surface": id, "root": 경로, "body": {"operation": "select", "id": 요청, "generation": 세대, "path": 경로|""}}
//	      {"surface": id, "root": 경로, "body": {"operation": "read", "id": 요청, "path": 상대 경로}}
//	      {"surface": id, "root": 경로, "body": {"operation": "write", "id": 요청, "path": 상대 경로, "text": 내용, "expect": 판|null, "bom": 참거짓}}
//	      {"surface": id, "root": 경로, "body": {"operation": "readBytes", "id": 요청, "path": 상대 경로}}
//	      {"surface": id, "root": 경로, "body": {"operation": "writeBytes", "id": 요청, "path": 상대 경로, "data": base64, "expect": 판|null}}
//	      {"surface": id, "closed": true}
//	출력  {"surface": id, "body": {"id": 요청, "entries": [{"name": 이름, "directory": 참거짓}]}}
//	      {"surface": id, "body": {"id": 요청, "entries": [{"path": 상대 경로, "status": 상태}]}}
//	      {"surface": id, "body": {"id": 요청, "tree": {"generation": 세대, "entries": [{"path": 경로, "name": 이름, "directory": 참거짓, "depth": 깊이, "expanded": 참거짓, "selected": 참거짓, "detail"?: git상태}], "selected": 경로|null}}}
//	      {"surface": id, "body": {"id": 요청, "text": 내용, "version": 판, "newline": 줄바꿈, "bom": 참거짓}}
//	      {"surface": id, "body": {"id": 요청, "version": 판}}
//	      {"surface": id, "body": {"id": 요청}}
//	      {"surface": id, "body": {"changed": 상대 경로}}
//	      {"surface": id, "body": {"id": 요청, "error": 메시지}}
//	      {"surface": id, "closed": true}
//	      {"surface": id, "closed": true, "error": 메시지}
//
// A request may carry `cids`, the correlation identifiers of the inputs it continues; the reply lines that answer it carry the
// same `cids` beside `surface` and `body`, and a failed request is the member `error` of the body.
//
// 세션은 감시하는 경로만 상태로 갖는다. Serve 는 입력이 닫히면 모든 감시를 끝내고 반환한다.
package files

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/soksak-app/sidecar-files/src/events"
	"github.com/soksak-app/sidecar-files/src/platform"
	_ "github.com/soksak-app/sidecar-files/src/platform/darwin"
	_ "github.com/soksak-app/sidecar-files/src/platform/linux"
	_ "github.com/soksak-app/sidecar-files/src/platform/windows"
)

// Request 는 호스트가 보낸 메시지 하나다.
type Request struct {
	Surface string `json:"surface"`
	Root    string `json:"root,omitempty"`
	// Cids are the correlation identifiers of the inputs that the request continues. A reply carries them back.
	Cids   []string `json:"cids,omitempty"`
	Closed bool     `json:"closed,omitempty"`
	Body   struct {
		Operation  string   `json:"operation"`
		ID         string   `json:"id"`
		Path       string   `json:"path"`
		Paths      []string `json:"paths"`
		Generation uint64   `json:"generation"`
		// Text, Expect, BOM 은 write 에만 있다. Expect 는 빠진 필드와 null 을 구별하려고 원문 그대로 받는다.
		Text   *string         `json:"text"`
		Expect json.RawMessage `json:"expect"`
		BOM    *bool           `json:"bom"`
		// Data holds the base64 bytes of writeBytes.
		Data *string `json:"data"`
	} `json:"body"`
}

// Entry 는 디렉터리 항목 하나다.
type Entry struct {
	Name      string `json:"name"`
	Directory bool   `json:"directory"`
}

// EventBody 는 요청의 답(목록, 내용, 쓰기, 감시 확인, 실패)이나 감시한 경로의 변경 하나를 담는다.
type EventBody struct {
	ID string `json:"id,omitempty"`
	// Entries 는 목록([]Entry)과 git 상태([]GitEntry)의 답에만 있다. 항목이 없으면 빈 배열이다.
	Entries any `json:"entries,omitempty"`
	// Text, Newline, BOM 은 read 의 답에만, Version 은 read 와 write 의 답에만 있다.
	Text *string `json:"text,omitempty"`
	// Data holds the base64 bytes of the readBytes reply.
	Data    *string `json:"data,omitempty"`
	Version string  `json:"version,omitempty"`
	Newline string  `json:"newline,omitempty"`
	BOM     *bool   `json:"bom,omitempty"`
	// Changed 는 항목이 바뀐 감시 디렉터리나 내용이 바뀐 감시 파일의 상대 경로다. 프로젝트 폴더는 빈 문자열이다.
	Changed *string       `json:"changed,omitempty"`
	Error   string        `json:"error,omitempty"`
	Tree    *TreeSnapshot `json:"tree,omitempty"`
}

// Event 는 호스트에 보내는 메시지 하나다. 요청의 답과 변경은 Body 를, closed 의 답은 Closed 와 닫지 못한 까닭
// Error 를 담는다(core 의 docs/spec/sidecars.md#messages).
type Event struct {
	Surface string `json:"surface"`
	// Cids are the correlation identifiers of the request that the message answers; a change of a watch has none.
	Cids   []string   `json:"cids,omitempty"`
	Body   *EventBody `json:"body,omitempty"`
	Closed bool       `json:"closed,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// Serve 는 in 이 닫힐 때까지 요청을 처리하고 답과 변경을 out 에 기록한다. 요청, 감시, git 호출은 log 에 event 로 남는다.
// log 가 nil 이면 event 를 남기지 않는다.
func Serve(in io.Reader, out io.Writer, log *events.Log) error {
	var mu sync.Mutex
	encoder := json.NewEncoder(out)
	// 8 MiB 의 text 가 JSON 에서 커지는 것은 HTML 문자 escape 를 빼야 최대 6 배로 줄의 한도 안에 든다.
	encoder.SetEscapeHTML(false)
	var writeErr error
	send := func(event Event) {
		mu.Lock()
		defer mu.Unlock()
		if err := encoder.Encode(event); err != nil && writeErr == nil {
			writeErr = fmt.Errorf("write event: %w", err)
		}
	}
	written := func() error {
		mu.Lock()
		defer mu.Unlock()
		return writeErr
	}
	watches := newWatches(send, log)
	trees := newTreeRoutes()
	defer watches.closeAll()
	scanner := bufio.NewScanner(in)
	// 요청 줄은 sidecar 출력 줄의 한도인 67108864 바이트까지다. 버퍼는 그 줄과 줄바꿈을 함께 담는다.
	scanner.Buffer(make([]byte, 0, 64*1024), lineLimit+1)
	for scanner.Scan() {
		var request Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return fmt.Errorf("invalid request: %w", err)
		}
		if request.Surface == "" {
			return fmt.Errorf("request without surface: %s", scanner.Text())
		}
		trace := Trace{Log: log, Surface: request.Surface, Cids: request.Cids}
		started := time.Now()
		if request.Closed {
			trees.close(request.Surface)
			// 감시를 끝낸 뒤 모든 closed 에 답한다.
			answer := Event{Surface: request.Surface, Cids: request.Cids, Closed: true}
			err := watches.set(request.Surface, "", nil, "closed", trace)
			if err != nil {
				answer.Error = err.Error()
			}
			trace.request(request, "closed", started, err)
			send(answer)
		} else {
			body := EventBody{ID: request.Body.ID}
			err := handle(watches, trees, request, &body, trace)
			if err != nil {
				body = EventBody{ID: request.Body.ID, Error: err.Error()}
			}
			trace.request(request, request.Body.Operation, started, err)
			send(Event{Surface: request.Surface, Cids: request.Cids, Body: &body})
		}
		if err := written(); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func handle(watches *watches, trees *treeRoutes, request Request, body *EventBody, trace Trace) error {
	if request.Root == "" {
		return fmt.Errorf("%s requires a root", request.Body.Operation)
	}
	switch request.Body.Operation {
	case "list":
		entries, err := List(request.Root, request.Body.Path)
		if err != nil {
			return err
		}
		body.Entries = entries
		return nil
	case "git":
		entries, err := GitStatus(request.Root, trace)
		if err != nil {
			return err
		}
		body.Entries = entries
		return nil
	case "read":
		return Read(request.Root, request.Body.Path, body)
	case "write":
		return writeRequest(request, body)
	case "readBytes":
		return ReadBytes(request.Root, request.Body.Path, body)
	case "writeBytes":
		return writeBytesRequest(request, body)
	case "watch":
		return watches.set(request.Surface, request.Root, request.Body.Paths, "replaced", trace)
	case "snapshot":
		route := trees.snapshot(request.Surface, request.Root)
		snapshot, err := route.update(request.Root, request.Surface, request.Body.Operation, request, trace, watches)
		if err != nil {
			return err
		}
		body.Tree = snapshot
		return nil
	case "expand", "collapse", "select":
		route, err := trees.route(request.Surface, request.Root, request.Body.Generation)
		if err != nil {
			return err
		}
		snapshot, err := route.update(request.Root, request.Surface, request.Body.Operation, request, trace, watches)
		if err != nil {
			return err
		}
		body.Tree = snapshot
		return nil
	default:
		return fmt.Errorf("unknown operation %q", request.Body.Operation)
	}
}

// lineLimit 은 요청 줄의 최대 바이트 수다.
const lineLimit = 67108864

// watch 는 감시 중인 경로 하나다. path 는 요청이 쓴 상대 경로다.
type watch struct {
	path string
	stop func() error
}

// watches 는 세션마다 감시 중인 경로다.
type watches struct {
	mu       sync.Mutex
	send     func(Event)
	log      *events.Log
	sessions map[string][]watch
}

func newWatches(send func(Event), log *events.Log) *watches {
	return &watches{send: send, log: log, sessions: map[string][]watch{}}
}

// end stops the watches of a surface and stores the end of each with the reason that ended it.
func (w *watches) end(surface, reason string) error {
	var errs []error
	for _, started := range w.sessions[surface] {
		err := started.stop()
		errs = append(errs, err)
		w.log.EmitHost(events.LevelInfo, "sidecar.files.watch_end", map[string]any{"surface": surface, "path": started.path, "reason": reason})
	}
	delete(w.sessions, surface)
	return errors.Join(errs...)
}

// set 은 세션의 감시를 paths 로 바꾼다. 경로 하나라도 실패하면 새 감시를 모두 끝내고 실패한다. 이전 감시는 reason 때문에 끝난다.
func (w *watches) set(surface, root string, paths []string, reason string, trace Trace) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.end(surface, reason); err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	current, err := platform.Current()
	if err != nil {
		return err
	}
	started := make([]watch, 0, len(paths))
	for _, path := range paths {
		stop, err := w.watch(current, surface, root, path)
		if err != nil {
			for _, earlier := range started {
				err = errors.Join(err, earlier.stop())
				w.log.EmitHostCids(events.LevelInfo, "sidecar.files.watch_end", trace.Cids, map[string]any{"surface": surface, "path": earlier.path, "reason": "start failed"})
			}
			return err
		}
		started = append(started, watch{path: path, stop: stop})
		w.log.EmitHostCids(events.LevelInfo, "sidecar.files.watch_start", trace.Cids, map[string]any{"surface": surface, "path": path})
	}
	w.sessions[surface] = started
	return nil
}

func (w *watches) watch(current platform.Platform, surface, root, path string) (func() error, error) {
	target, err := resolve(root, path)
	if err != nil {
		return nil, err
	}
	// FIFO 같은 다른 종류는 감시하려고 열면 멈출 수 있으므로 열기 전에 거른다.
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file or directory: %s", path)
	}
	changed := path
	return current.Watch(target, func() {
		w.log.EmitHost(events.LevelDebug, "sidecar.files.watch_change", func() map[string]any { return map[string]any{"surface": surface, "path": changed} })
		w.send(Event{Surface: surface, Body: &EventBody{Changed: &changed}})
	}, func(failure error) {
		w.log.EmitHost(events.LevelError, "sidecar.files.watch_failed", map[string]any{"surface": surface, "path": changed, "error": failure.Error()})
		w.send(Event{Surface: surface, Body: &EventBody{Error: failure.Error()}})
	})
}

// closeAll ends every watch because the input closed. A watch that does not stop is reported to its surface and stored.
func (w *watches) closeAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for surface := range w.sessions {
		if err := w.end(surface, "input closed"); err != nil {
			w.log.EmitHost(events.LevelError, "sidecar.files.watch_failed", map[string]any{"surface": surface, "path": "", "error": err.Error()})
			w.send(Event{Surface: surface, Body: &EventBody{Error: err.Error()}})
		}
	}
}

// resolve 는 root 안의 상대 경로 path 를 심볼릭 링크를 따라간 절대 경로로 반환한다. root 를 벗어나면 실패한다.
func resolve(root, path string) (string, error) {
	return resolveIn(root, path, false)
}

// resolveIn 은 resolve 와 같다. missing 이면 path 가 없을 때 그 부모 디렉터리를 따라간 경로에 마지막 이름을 붙인다.
// 새 파일을 만들 경로다. 대상이 없는 심볼릭 링크도 이 경로가 되며, 만들기의 O_EXCL 이 그 링크를 있는 것으로 거절한다.
func resolveIn(root, path string, missing bool) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path %q must be relative to the root", path)
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(base, path)
	// 이름만으로 root 를 벗어나는 경로는 그 바깥 경로를 조회하기 전에 거절한다.
	if !within(base, joined) {
		return "", fmt.Errorf("path %q leaves the root", path)
	}
	target, err := filepath.EvalSymlinks(joined)
	if err != nil && missing && errors.Is(err, fs.ErrNotExist) {
		var parent string
		parent, err = filepath.EvalSymlinks(filepath.Dir(joined))
		target = filepath.Join(parent, filepath.Base(joined))
	}
	if err != nil {
		return "", err
	}
	if !within(base, target) {
		return "", fmt.Errorf("path %q leaves the root", path)
	}
	return target, nil
}

// within 은 절대 경로 target 이 base 이거나 그 안에 있는지 반환한다.
func within(base, target string) bool {
	inside, err := filepath.Rel(base, target)
	return err == nil && inside != ".." && !strings.HasPrefix(inside, ".."+string(filepath.Separator))
}

// List 는 root 안의 상대 경로 path 에 있는 디렉터리의 항목을 디렉터리 먼저, 각 묶음은 이름순으로 반환한다.
// 심볼릭 링크를 따라간 경로가 root 를 벗어나면 실패한다.
func List(root, path string) ([]Entry, error) {
	target, err := resolve(root, path)
	if err != nil {
		return nil, err
	}
	read, err := os.ReadDir(target)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(read))
	for _, item := range read {
		directory := item.IsDir()
		// 디렉터리를 가리키는 심볼릭 링크도 디렉터리로 보인다. 대상이 없는 링크는 디렉터리가 아니다.
		if item.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(filepath.Join(target, item.Name()))
			directory = err == nil && info.IsDir()
		}
		entries = append(entries, Entry{Name: item.Name(), Directory: directory})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Directory != entries[j].Directory {
			return entries[i].Directory
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}
