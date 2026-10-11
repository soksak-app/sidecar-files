package files_test

// Checks the events of a run of Serve: each request, watch and git call is an event with the cids of its request, and each reply
// line carries those cids beside surface and body.

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soksak-app/sidecar-files/src/files"
)

// serveWithLog serves the lines with an event log and returns the raw reply lines and the events of the file.
func serveWithLog(t *testing.T, lines ...string) ([]string, []loggedEvent) {
	t.Helper()
	log := startLog(t, nil)
	var out bytes.Buffer
	if err := files.Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, log); err != nil {
		t.Fatalf("serve: %v", err)
	}
	return strings.Split(strings.TrimSpace(out.String()), "\n"), finish(t, log)
}

func requestWith(t *testing.T, cids []string, root string, body map[string]any) string {
	t.Helper()
	line, err := json.Marshal(map[string]any{"surface": "state:files:p1", "root": root, "cids": cids, "body": body})
	if err != nil {
		t.Fatal(err)
	}
	return string(line)
}

func eventsNamed(lines []loggedEvent, name string) []loggedEvent {
	var found []loggedEvent
	for _, line := range lines {
		if line.Event == name {
			found = append(found, line)
		}
	}
	return found
}

// contract: log.request.records-the-result-and-the-cids
func TestEachRequestIsAnEventWithItsResultAndTheCidsOfTheRequest(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a.txt"), "x")
	_, lines := serveWithLog(t,
		requestWith(t, []string{"b1.1", "b1.2"}, root, map[string]any{"operation": "list", "id": "l1", "path": ""}),
		requestWith(t, []string{"b1.3"}, root, map[string]any{"operation": "list", "id": "l2", "path": "missing"}),
		requestWith(t, nil, root, map[string]any{"operation": "frobnicate"}),
	)
	requests := eventsNamed(lines, "sidecar.files.request")
	if len(requests) != 3 {
		t.Fatalf("%d request events in %+v", len(requests), lines)
	}
	first, second, third := requests[0], requests[1], requests[2]
	if first.Level != "info" || first.Fields["ok"] != true || first.Fields["operation"] != "list" || first.Fields["id"] != "l1" || first.Fields["surface"] != "state:files:p1" || first.Fields["error"] != nil {
		t.Fatalf("first request %+v", first)
	}
	if strings.Join(first.Cids, ",") != "b1.1,b1.2" {
		t.Fatalf("cids %v", first.Cids)
	}
	if micros, ok := first.Fields["micros"].(float64); !ok || micros < 0 || micros != float64(int64(micros)) {
		t.Fatalf("micros %v", first.Fields["micros"])
	}
	if second.Level != "warn" || second.Fields["ok"] != false || !strings.HasPrefix(second.Fields["error"].(string), "-32603: ") || strings.Join(second.Cids, ",") != "b1.3" {
		t.Fatalf("second request %+v", second)
	}
	if third.Fields["error"] != "-32603: unknown operation \"frobnicate\"" || len(third.Cids) != 0 || third.Fields["id"] != nil {
		t.Fatalf("third request %+v", third)
	}
}

// contract: log.request.reply-carries-the-cids
func TestEachReplyLineCarriesTheCidsOfItsRequestBesideSurfaceAndBody(t *testing.T) {
	root := t.TempDir()
	replies, _ := serveWithLog(t,
		requestWith(t, []string{"b1.1"}, root, map[string]any{"operation": "list", "id": "l1", "path": ""}),
		requestWith(t, []string{"b1.2"}, root, map[string]any{"operation": "list", "id": "l2", "path": "missing"}),
		requestWith(t, nil, root, map[string]any{"operation": "list", "id": "l3", "path": ""}),
		`{"surface":"state:files:p1","cids":["b1.9"],"closed":true}`,
	)
	if len(replies) != 4 {
		t.Fatalf("replies %q", replies)
	}
	if got := keysOf(t, replies[0]); strings.Join(got, ",") != "surface,cids,body" {
		t.Fatalf("keys %v of %s", got, replies[0])
	}
	var failed struct {
		Cids []string       `json:"cids"`
		Body map[string]any `json:"body"`
	}
	if err := json.Unmarshal([]byte(replies[1]), &failed); err != nil {
		t.Fatal(err)
	}
	if strings.Join(failed.Cids, ",") != "b1.2" || failed.Body["error"] == nil || failed.Body["id"] != "l2" {
		t.Fatalf("failed reply %s", replies[1])
	}
	if strings.Contains(replies[2], "cids") {
		t.Fatalf("a request without cids has a reply with cids: %s", replies[2])
	}
	if !strings.Contains(replies[3], `"cids":["b1.9"]`) || !strings.Contains(replies[3], `"closed":true`) {
		t.Fatalf("closed reply %s", replies[3])
	}
}

// contract: log.watch.records-start-change-and-end
func TestAWatchIsStartedChangedAndEndedInTheEventFile(t *testing.T) {
	root := t.TempDir()
	log := startLog(t, nil)
	session := startSessionWith(t, log)
	session.send(root, map[string]any{"operation": "watch", "id": "w1", "paths": []string{""}})
	if reply := session.next("watch answer"); reply.Body == nil || reply.Body.ID != "w1" {
		t.Fatalf("reply %+v", reply)
	}
	write(t, filepath.Join(root, "new.txt"), "x")
	if changed := session.next("change"); changed.Body == nil || changed.Body.Changed == nil {
		t.Fatalf("change %+v", changed)
	}
	// A second watch request replaces the first watch.
	session.send(root, map[string]any{"operation": "watch", "id": "w2", "paths": []string{""}})
	session.next("second watch answer")
	session.send(root, map[string]any{"operation": "watch", "id": "w3", "paths": []string{}})
	session.next("watch removal answer")
	session.input.Write([]byte(`{"surface":"state:files:p1","closed":true}` + "\n"))
	session.next("closed answer")
	lines := finish(t, log)
	starts, changes, ends := eventsNamed(lines, "sidecar.files.watch_start"), eventsNamed(lines, "sidecar.files.watch_change"), eventsNamed(lines, "sidecar.files.watch_end")
	if len(starts) != 2 || starts[0].Fields["surface"] != "state:files:p1" || starts[0].Fields["path"] != "" {
		t.Fatalf("starts %+v", starts)
	}
	if len(changes) < 1 || changes[0].Fields["path"] != "" || changes[0].Level != "debug" {
		t.Fatalf("changes %+v", changes)
	}
	if len(ends) != 2 || ends[0].Fields["reason"] != "replaced" || ends[1].Fields["reason"] != "replaced" {
		t.Fatalf("ends %+v", ends)
	}
}

// contract: log.watch.records-the-end-by-close-and-by-the-end-of-input
func TestAWatchEndsWithTheReasonThatEndedIt(t *testing.T) {
	root := t.TempDir()
	log := startLog(t, nil)
	session := startSessionWith(t, log)
	session.send(root, map[string]any{"operation": "watch", "id": "w1", "paths": []string{""}})
	session.next("watch answer")
	session.input.Write([]byte(`{"surface":"state:files:p1","closed":true}` + "\n"))
	session.next("closed answer")
	session.send(root, map[string]any{"operation": "watch", "id": "w2", "paths": []string{""}})
	session.next("second watch answer")
	// Closing the input ends Serve, which ends the watch.
	session.input.Close()
	deadline := time.Now().Add(10 * time.Second)
	for {
		ends := eventsNamed(finish(t, log), "sidecar.files.watch_end")
		if len(ends) == 2 {
			if ends[0].Fields["reason"] != "closed" || ends[1].Fields["reason"] != "input closed" {
				t.Fatalf("ends %+v", ends)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ends %+v", ends)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// contract: log.git.records-each-call-with-its-result
func TestEachGitCallIsAnEventWithItsResult(t *testing.T) {
	repository := t.TempDir()
	run(t, repository, "init", "-q")
	write(t, filepath.Join(repository, "a.txt"), "x")
	plain := t.TempDir()
	// A folder outside any repository answers 128 to rev-parse, which is an empty status and not a failure of the request.
	_, lines := serveWithLog(t,
		requestWith(t, []string{"b2.1"}, repository, map[string]any{"operation": "git", "id": "g1"}),
		requestWith(t, []string{"b2.2"}, plain, map[string]any{"operation": "git", "id": "g2"}),
	)
	calls := eventsNamed(lines, "sidecar.files.git")
	if len(calls) != 3 {
		t.Fatalf("%d git events in %+v", len(calls), calls)
	}
	if calls[0].Fields["args"] != "rev-parse --show-toplevel" || calls[0].Fields["ok"] != true || calls[0].Level != "info" || strings.Join(calls[0].Cids, ",") != "b2.1" {
		t.Fatalf("rev-parse %+v", calls[0])
	}
	if calls[1].Fields["args"] != "status --porcelain=v1 -z --untracked-files=all" || calls[1].Fields["ok"] != true {
		t.Fatalf("status %+v", calls[1])
	}
	outside := calls[2]
	if outside.Fields["ok"] != false || outside.Level != "warn" || !strings.HasPrefix(outside.Fields["error"].(string), "128: ") || strings.Join(outside.Cids, ",") != "b2.2" {
		t.Fatalf("outside %+v", outside)
	}
	for _, request := range eventsNamed(lines, "sidecar.files.request") {
		if request.Fields["ok"] != true {
			t.Fatalf("request %+v", request)
		}
	}
}

// contract: log.policy.producers-ask-the-gate
func TestAPolicyThatTurnsTheCallClassOffStoresNoRequestAndCountsEachOne(t *testing.T) {
	root := t.TempDir()
	log := startGatedLog(t)
	log.ApplyPolicy(mustPolicy(t, map[string]string{"call": "off"}), "file")
	var out bytes.Buffer
	lines := []string{
		requestWith(t, []string{"b1.1"}, root, map[string]any{"operation": "list", "id": "l1", "path": ""}),
		requestWith(t, nil, root, map[string]any{"operation": "list", "id": "l2", "path": "missing"}),
	}
	if err := files.Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, log); err != nil {
		t.Fatal(err)
	}
	stored := finish(t, log)
	if len(eventsNamed(stored, "sidecar.files.request")) != 0 {
		t.Fatalf("a request was stored while the class call is off: %+v", stored)
	}
	counts := map[string]float64{}
	for _, gated := range eventsNamed(stored, "log.gated") {
		counts[gated.Fields["class"].(string)+" "+gated.Fields["level"].(string)] = gated.Fields["count"].(float64)
	}
	if counts["call info"] != 1 || counts["call warn"] != 1 {
		t.Fatalf("counts %v, want one info and one warn", counts)
	}
	if got := len(strings.Split(strings.TrimSpace(out.String()), "\n")); got != 2 {
		t.Fatalf("%d reply lines, want 2: the policy gates events and not replies", got)
	}
}
