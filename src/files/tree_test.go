package files

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestMissingExpandedDirectoryIsPrunedWithoutMaskingListErrors(t *testing.T) {
	root := t.TempDir()
	list := func(_ string, relative string) ([]Entry, error) {
		if relative == "" {
			return []Entry{}, nil
		}
		t.Fatalf("missing path %q should not be listed after its parent omits it", relative)
		return nil, nil
	}
	route := &treeRoute{root: root, generation: 1, expanded: map[string]bool{"": true, "src": true}, listings: map[string][]Entry{}}
	var request Request
	request.Root = root
	request.Body.Operation = "snapshot"
	request.Body.ID = "snapshot-missing"
	snapshot, err := route.update(root, "plugin:files:p1:@soksak/sidecar-files", request.Body.Operation, request, Trace{}, list,
		func(string, string, []string, string, Trace) error { return nil })
	if err != nil {
		t.Fatalf("missing expanded directory was not pruned: %v", err)
	}
	if route.expanded["src"] || len(snapshot.Entries) != 0 {
		t.Fatalf("missing path state = expanded:%v snapshot:%#v", route.expanded["src"], snapshot)
	}
}

func TestExpandedDirectoryListFailureIsReturnedInErrorReply(t *testing.T) {
	const surface = "plugin:files:project-a:@soksak/sidecar-files"
	root := t.TempDir()
	list := func(_ string, relative string) ([]Entry, error) {
		if relative == "" {
			return []Entry{{Name: "src", Directory: true}}, nil
		}
		if relative == "src" {
			return nil, errors.New("permission denied")
		}
		t.Fatalf("unexpected list path %q", relative)
		return nil, nil
	}
	route := &treeRoute{root: root, generation: 1, expanded: map[string]bool{"": true, "src": true}, listings: map[string][]Entry{}}
	var request Request
	request.Surface = surface
	request.Root = root
	request.Cids = []string{"tree-action-1"}
	request.Body.Operation = "snapshot"
	request.Body.ID = "snapshot-1"

	// A previously expanded directory is now unreadable. Inject the error and avoid native watch setup.
	snapshot, err := route.update(root, surface, request.Body.Operation, request, Trace{}, list,
		func(string, string, []string, string, Trace) error { return nil })
	body := errorReplyBody(request, EventBody{ID: request.Body.ID, Tree: snapshot}, err)
	line, marshalErr := json.Marshal(Event{Surface: surface, Cids: request.Cids, Body: &body})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var reply Event
	if err := json.Unmarshal(line, &reply); err != nil {
		t.Fatalf("decode reply %q: %v", line, err)
	}
	if reply.Body == nil || reply.Body.ID != request.Body.ID || !strings.Contains(reply.Body.Error, "permission denied") {
		t.Fatalf("reply = %s, want correlated {id,error} with the listing failure", line)
	}
	if len(reply.Cids) != 1 || reply.Cids[0] != request.Cids[0] {
		t.Fatalf("reply cids = %#v, want %#v", reply.Cids, request.Cids)
	}
}
