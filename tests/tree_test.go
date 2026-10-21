package files_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soksak-app/sidecar-files/src/files"
)

func serveTree(t *testing.T, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := files.Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, nil); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

func treeRequest(t *testing.T, surface, root, operation, id string, fields map[string]any, cids []string) string {
	t.Helper()
	body := map[string]any{"operation": operation, "id": id}
	for key, value := range fields {
		body[key] = value
	}
	line, err := json.Marshal(map[string]any{"surface": surface, "root": root, "cids": cids, "body": body})
	if err != nil {
		t.Fatal(err)
	}
	return string(line)
}

func makeGitRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "src", "changed.go")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", root}, {"-C", root, "add", "src/changed.go"}, {"-C", root, "-c", "user.name=Files Test", "-c", "user.email=files@example.invalid", "commit", "-q", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func treeOf(t *testing.T, event map[string]any) map[string]any {
	t.Helper()
	body, ok := event["body"].(map[string]any)
	if !ok {
		t.Fatalf("event body = %#v", event["body"])
	}
	tree, ok := body["tree"].(map[string]any)
	if !ok {
		t.Fatalf("tree snapshot missing from body: %#v", body)
	}
	return tree
}

func rowOf(rows []map[string]any, path string) map[string]any {
	for _, row := range rows {
		if row["path"] == path {
			return row
		}
	}
	return nil
}

func rowsOf(tree map[string]any) []map[string]any {
	rows, ok := tree["entries"].([]any)
	if !ok {
		panic(fmt.Sprintf("tree entries = %#v", tree["entries"]))
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.(map[string]any))
	}
	return out
}

func TestTreeSnapshotHasGitDetailForDirectoryAndEchoesCids(t *testing.T) {
	const route = "plugin:files:project-a:@soksak/sidecar-files"
	root := makeGitRoot(t)
	events := serveTree(t, treeRequest(t, route, root, "snapshot", "s1", nil, []string{"cid-snapshot"}))
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if got := events[0]["cids"]; fmt.Sprint(got) != "[cid-snapshot]" {
		t.Fatalf("reply cids = %#v", got)
	}
	tree := treeOf(t, events[0])
	rows := rowsOf(tree)
	if row := rowOf(rows, "src"); row == nil || row["detail"] != "modified" {
		t.Fatalf("directory detail = %#v", rows)
	}
	if tree["generation"] != float64(1) {
		t.Fatalf("first root generation = %#v", tree["generation"])
	}
}

func TestTreeOperationsReturnFreshDataAndKeepStatePerRoute(t *testing.T) {
	const routeA = "plugin:files:project-a:@soksak/sidecar-files"
	const routeB = "plugin:files:project-b:@soksak/sidecar-files"
	rootA := makeGitRoot(t)
	rootB := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootB, "other.txt"), []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	events := serveTree(t,
		treeRequest(t, routeA, rootA, "snapshot", "a1", nil, []string{"cid-1"}),
		treeRequest(t, routeA, rootA, "expand", "a2", map[string]any{"generation": 1, "path": "src"}, []string{"cid-2"}),
		treeRequest(t, routeA, rootA, "select", "a3", map[string]any{"generation": 1, "path": "src/changed.go"}, []string{"cid-3"}),
		treeRequest(t, routeA, rootA, "collapse", "a4", map[string]any{"generation": 1, "path": "src"}, []string{"cid-4"}),
		treeRequest(t, routeA, rootB, "snapshot", "a5", nil, []string{"cid-5"}),
		treeRequest(t, routeB, rootA, "snapshot", "b1", nil, []string{"cid-6"}),
	)
	if len(events) != 6 {
		t.Fatalf("events = %#v", events)
	}
	for i, event := range events {
		want := fmt.Sprintf("[cid-%d]", i+1)
		if fmt.Sprint(event["cids"]) != want {
			t.Fatalf("reply %d cids = %#v, want %s", i, event["cids"], want)
		}
	}
	rows := rowsOf(treeOf(t, events[1]))
	if row := rowOf(rows, "src/changed.go"); row == nil {
		t.Fatalf("expanded rows = %#v", rows)
	}
	rows = rowsOf(treeOf(t, events[2]))
	if row := rowOf(rows, "src/changed.go"); row == nil || row["selected"] != true {
		t.Fatalf("selected rows = %#v", rows)
	}
	rows = rowsOf(treeOf(t, events[3]))
	if rowOf(rows, "src/changed.go") != nil || treeOf(t, events[3])["selected"] != nil {
		t.Fatalf("collapsed selection = %#v", treeOf(t, events[3]))
	}
	rows = rowsOf(treeOf(t, events[4]))
	if treeOf(t, events[4])["generation"] != float64(2) || len(rows) != 1 || rows[0]["path"] != "other.txt" {
		t.Fatalf("root replacement state = %#v", treeOf(t, events[4]))
	}
	if treeOf(t, events[5])["generation"] != float64(1) {
		t.Fatalf("other project route generation = %#v", treeOf(t, events[5]))
	}
}

func TestTreeRestartAndClosedSessionResetExpansionAndSelection(t *testing.T) {
	const route = "plugin:files:project-a:@soksak/sidecar-files"
	root := makeGitRoot(t)
	closed := `{"surface":"` + route + `","root":"` + root + `","closed":true}`
	first := serveTree(t,
		treeRequest(t, route, root, "snapshot", "s1", nil, nil),
		treeRequest(t, route, root, "expand", "e1", map[string]any{"generation": 1, "path": "src"}, nil),
		treeRequest(t, route, root, "select", "p1", map[string]any{"generation": 1, "path": "src/changed.go"}, nil),
		closed,
		treeRequest(t, route, root, "snapshot", "s2", nil, nil),
	)
	if treeOf(t, first[4])["generation"] != float64(1) || treeOf(t, first[4])["selected"] != nil {
		t.Fatalf("closed route retained state: %#v", treeOf(t, first[4]))
	}
	rows := rowsOf(treeOf(t, first[4]))
	if row := rowOf(rows, "src"); row == nil || row["expanded"] != false {
		t.Fatalf("closed route retained expansion: %#v", rows)
	}
	// A new sidecar process has no memory of the old session, even when the host reuses the same session id.
	restarted := serveTree(t, treeRequest(t, route, root, "snapshot", "s3", nil, nil))
	if treeOf(t, restarted[0])["generation"] != float64(1) || treeOf(t, restarted[0])["selected"] != nil {
		t.Fatalf("restart retained state: %#v", treeOf(t, restarted[0]))
	}
}

func TestTreeOperationRejectsStaleGenerationAndRequiresSnapshot(t *testing.T) {
	const route = "plugin:files:project-a:@soksak/sidecar-files"
	root := t.TempDir()
	events := serveTree(t,
		treeRequest(t, route, root, "expand", "before", map[string]any{"generation": 1, "path": "src"}, nil),
		treeRequest(t, route, root, "snapshot", "s1", nil, nil),
		treeRequest(t, route, root, "select", "stale", map[string]any{"generation": 2, "path": ""}, nil),
	)
	body0 := events[0]["body"].(map[string]any)
	if body0["error"] == nil {
		t.Fatalf("expand before snapshot succeeded: %#v", body0)
	}
	body2 := events[2]["body"].(map[string]any)
	if body2["error"] == nil {
		t.Fatalf("stale generation succeeded: %#v", body2)
	}
}
