package files_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// setMode sets the mode of path exactly; a mode given to os.WriteFile is masked by the umask.
func setMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func wantMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("mode %o, want %o", info.Mode().Perm(), want)
	}
}

func TestWriteExecutableCreatesAFileWithModeExactly0755(t *testing.T) {
	// The umask would clear group and other bits of a 0755 request made without a chmod; the mode must not depend on it.
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)
	root := t.TempDir()
	reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "run.sh", "text": "#!/bin/sh\n", "expect": nil, "bom": false, "executable": true})
	if got := reply["version"]; got != version([]byte("#!/bin/sh\n")) {
		t.Fatalf("reply = %v, want the version of the written bytes", reply)
	}
	wantMode(t, filepath.Join(root, "run.sh"), 0o755)
	if got := contentOf(t, filepath.Join(root, "run.sh")); got != "#!/bin/sh\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestWriteExecutableSetsTheModeOfAnExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "run.sh")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	setMode(t, path, 0o600)
	reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "run.sh", "text": "new", "expect": version([]byte("old")), "bom": false, "executable": true})
	if got := reply["version"]; got != version([]byte("new")) {
		t.Fatalf("reply = %v, want the version of the written bytes", reply)
	}
	wantMode(t, path, 0o755)
	if got := contentOf(t, path); got != "new" {
		t.Fatalf("content = %q", got)
	}
}

func TestWriteWithoutExecutableKeepsTheModeOfAnExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	setMode(t, path, 0o640)
	for _, member := range []map[string]any{{}, {"executable": false}} {
		body := map[string]any{"operation": "write", "id": "w", "path": "notes.txt", "text": "new", "expect": version([]byte("old")), "bom": false}
		for key, value := range member {
			body[key] = value
		}
		if reply := textReply(t, root, body); reply["error"] != nil {
			t.Fatalf("write %v = %v", member, reply)
		}
		wantMode(t, path, 0o640)
		setMode(t, path, 0o640)
		if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWriteRefusesANonBooleanExecutableBeforeOpeningTheFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "run.sh")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	setMode(t, path, 0o600)
	for _, value := range []any{"true", 1, nil} {
		reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "run.sh", "text": "new", "expect": version([]byte("old")), "bom": false, "executable": value})
		wantError(t, reply, "write executable must be a boolean")
	}
	reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "made.sh", "text": "new", "expect": nil, "bom": false, "executable": "yes"})
	wantError(t, reply, "write executable must be a boolean")
	if _, err := os.Stat(filepath.Join(root, "made.sh")); !os.IsNotExist(err) {
		t.Fatalf("a refused request created made.sh: %v", err)
	}
	if got := contentOf(t, path); got != "old" {
		t.Fatalf("content = %q, want it unchanged", got)
	}
	wantMode(t, path, 0o600)
}

func TestWriteExecutableRefusesAChangeOnDiskWithoutChangingTheMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "run.sh")
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	setMode(t, path, 0o600)
	reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "run.sh", "text": "new", "expect": version([]byte("old")), "bom": false, "executable": true})
	wantError(t, reply, "changed on disk: run.sh")
	wantMode(t, path, 0o600)
	if got := contentOf(t, path); got != "changed" {
		t.Fatalf("content = %q, want it unchanged", got)
	}
}
