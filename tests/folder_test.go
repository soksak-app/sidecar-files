package files_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// processUmask reads the umask of the process. The call sets it twice and restores it; no test runs in parallel.
func processUmask() os.FileMode {
	previous := syscall.Umask(0)
	syscall.Umask(previous)
	return os.FileMode(previous)
}

func TestCreateFolderCreatesMissingParentsWithUmaskMode(t *testing.T) {
	root := t.TempDir()
	reply := textReply(t, root, map[string]any{"operation": "createFolder", "id": "f", "path": ".moai/config"})
	if got, _ := reply["error"].(string); got != "" {
		t.Fatalf("createFolder = %v", reply)
	}
	if reply["id"] != "f" {
		t.Fatalf("createFolder reply = %v, want id f", reply)
	}
	info, err := os.Stat(filepath.Join(root, ".moai", "config"))
	if err != nil || !info.IsDir() {
		t.Fatalf("the folder .moai/config is not a folder: %v", err)
	}
	if want := os.FileMode(0o777) &^ processUmask(); info.Mode().Perm() != want {
		t.Fatalf("mode %o, want %o (0777 masked by the umask)", info.Mode().Perm(), want)
	}
}

func TestCreateFolderAnswersSuccessForAnExistingFolder(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "kept.txt"), []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "second"} {
		reply := textReply(t, root, map[string]any{"operation": "createFolder", "id": id, "path": "docs"})
		if got, _ := reply["error"].(string); got != "" || reply["id"] != id {
			t.Fatalf("createFolder %s = %v, want success with id %s", id, reply, id)
		}
	}
	if got := contentOf(t, filepath.Join(root, "docs", "kept.txt")); got != "kept" {
		t.Fatalf("the existing folder lost its content: %q", got)
	}
}

func TestCreateFolderRefusesAnExistingNonFolder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	wantError(t, textReply(t, root, map[string]any{"operation": "createFolder", "id": "n", "path": "a.txt"}),
		"not a folder: a.txt")
	wantError(t, textReply(t, root, map[string]any{"operation": "createFolder", "id": "p", "path": "a.txt/sub"}),
		"not a folder: a.txt/sub")
	if got := contentOf(t, filepath.Join(root, "a.txt")); got != "file" {
		t.Fatalf("the refused folder changed the file: %q", got)
	}
}

func TestCreateFolderRejectsPathsThatLeaveTheRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{root, outside} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "missing"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	wantErrorContaining(t, textReply(t, root, map[string]any{"operation": "createFolder", "id": "a", "path": filepath.Join(outside, "made")}),
		"must be relative to the root")
	wantErrorContaining(t, textReply(t, root, map[string]any{"operation": "createFolder", "id": "b", "path": "../escaped"}),
		"leaves the root")
	wantErrorContaining(t, textReply(t, root, map[string]any{"operation": "createFolder", "id": "c", "path": "link"}),
		"leaves the root")
	wantErrorContaining(t, textReply(t, root, map[string]any{"operation": "createFolder", "id": "d", "path": "link/made"}),
		"leaves the root")
	if reply := textReply(t, root, map[string]any{"operation": "createFolder", "id": "e", "path": "dangling/made"}); reply["error"] == nil {
		t.Fatalf("createFolder through a dangling link = %v, want an error", reply)
	}
	for _, created := range []string{filepath.Join(base, "escaped"), filepath.Join(outside, "made"), filepath.Join(base, "missing")} {
		if _, err := os.Lstat(created); err == nil {
			t.Fatalf("a refused createFolder created %s", created)
		}
	}
}

func TestCreateFolderFollowsASymbolicLinkInsideTheRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if reply := textReply(t, root, map[string]any{"operation": "createFolder", "id": "s", "path": "alias/made"}); reply["id"] != "s" {
		t.Fatalf("createFolder through a link = %v", reply)
	}
	if info, err := os.Stat(filepath.Join(root, "real", "made")); err != nil || !info.IsDir() {
		t.Fatalf("the folder was not created in the link target: %v", err)
	}
}
