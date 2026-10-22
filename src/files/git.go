package files

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GitEntry 는 git 상태가 있는 경로 하나다. 경로는 root 기준이다.
type GitEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// notRepository 는 git 이 저장소 밖에서 실행되었을 때의 종료 코드다. git 은 다른 실패(예: 깨진 설정)에도 같은 코드를 쓰므로
// 저장소 밖인지는 notRepositoryText 로 가린다.
const notRepository = 128

// notRepositoryText is the text that git prints on stderr when the directory is outside a repository. git runs with LC_ALL=C,
// so the text is the English one whatever the locale of the process.
const notRepositoryText = "not a git repository"

// GitStatus 는 root 의 git 상태를 root 기준 경로로 반환한다. root 가 저장소 안에 있지 않거나 git 이
// 설치되지 않았으면 빈 목록이다(docs/spec/sidecars.md#files). 다른 실패는 오류다.
func GitStatus(root string, trace Trace) ([]GitEntry, error) {
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	top, err := git(trace, base, "rev-parse", "--show-toplevel")
	if err != nil {
		var failure *gitFailure
		if errors.Is(err, exec.ErrNotFound) || (errors.As(err, &failure) && failure.code == notRepository && strings.Contains(failure.stderr, notRepositoryText)) {
			return []GitEntry{}, nil
		}
		return nil, err
	}
	topDir, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil {
		return nil, err
	}
	out, err := git(trace, base, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	entries := []GitEntry{}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if len(field) < 4 {
			continue
		}
		code, path := field[:2], field[3:]
		// 이름 바꾸기와 복사는 다음 필드에 원래 경로를 둔다.
		if code[0] == 'R' || code[0] == 'C' {
			i++
		}
		status := statusOf(code)
		if status == "" {
			continue
		}
		rel, err := filepath.Rel(base, filepath.Join(topDir, path))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		entries = append(entries, GitEntry{Path: filepath.ToSlash(rel), Status: status})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// statusOf 는 porcelain v1 의 두 글자 상태를 트리 상태로 바꾼다. 무시된 파일은 빈 문자열이다.
func statusOf(code string) string {
	switch {
	case code == "??":
		return "untracked"
	case code == "!!":
		return ""
	case code[0] == 'R':
		return "renamed"
	case code[0] == 'A':
		return "added"
	case code[0] == 'D' || code[1] == 'D':
		return "deleted"
	default:
		return "modified"
	}
}

// gitFailure is a git call that exited with a status. stderr is the text that git printed, which tells the causes of one
// status apart.
type gitFailure struct {
	code   int
	stderr string
	err    error
}

func (f *gitFailure) Error() string {
	return fmt.Sprintf("%v: %s", f.err, f.stderr)
}

func (f *gitFailure) Unwrap() error {
	return f.err
}

// git runs git in dir and stores the call as an event of trace.
func git(trace Trace, dir string, args ...string) ([]byte, error) {
	started := time.Now()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			failure := &gitFailure{code: exit.ExitCode(), stderr: strings.TrimSpace(stderr.String()), err: err}
			err = fmt.Errorf("git %s: %w", strings.Join(args, " "), failure)
			trace.git(strings.Join(args, " "), exit.ExitCode(), started, err)
		} else {
			trace.git(strings.Join(args, " "), internalCode, started, err)
		}
		return nil, err
	}
	trace.git(strings.Join(args, " "), 0, started, nil)
	return out, nil
}
