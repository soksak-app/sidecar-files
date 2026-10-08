package files_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const bytesLimit = 33554432

func TestReadBytesAnswersTheBase64AndVersionOfAnyFile(t *testing.T) {
	root := t.TempDir()
	content := []byte{0xd0, 0xcf, 0x11, 0xe0, 0x00, 0xff, '\r', '\n'}
	if err := os.WriteFile(filepath.Join(root, "doc.hwp"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	reply := textReply(t, root, map[string]any{"operation": "readBytes", "id": "r", "path": "doc.hwp"})
	if reply["data"] != base64.StdEncoding.EncodeToString(content) || reply["version"] != version(content) || reply["error"] != nil {
		t.Fatalf("readBytes = %v", reply)
	}
	if err := os.Mkdir(filepath.Join(root, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	wantError(t, textReply(t, root, map[string]any{"operation": "readBytes", "id": "d", "path": "folder"}), "not a regular file: folder")
	large := filepath.Join(root, "large.bin")
	if err := os.WriteFile(large, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(large, bytesLimit+1); err != nil {
		t.Fatal(err)
	}
	wantError(t, textReply(t, root, map[string]any{"operation": "readBytes", "id": "l", "path": "large.bin"}),
		fmt.Sprintf("file is %d bytes, above the %d-byte limit: large.bin", bytesLimit+1, bytesLimit))
}

func TestWriteBytesWritesInPlaceWithTheVersionItRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.hwpx")
	first := []byte("PK\x03\x04first")
	if err := os.WriteFile(path, first, 0o640); err != nil {
		t.Fatal(err)
	}
	second := []byte("PK\x03\x04second\x00")
	data := base64.StdEncoding.EncodeToString(second)
	reply := textReply(t, root, map[string]any{"operation": "writeBytes", "id": "w", "path": "doc.hwpx", "data": data, "expect": version(first)})
	if reply["version"] != version(second) || reply["error"] != nil {
		t.Fatalf("writeBytes = %v", reply)
	}
	if written, _ := os.ReadFile(path); !bytes.Equal(written, second) {
		t.Fatalf("file holds %q", written)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	wantError(t, textReply(t, root, map[string]any{"operation": "writeBytes", "id": "c", "path": "doc.hwpx", "data": data, "expect": version(first)}),
		"changed on disk: doc.hwpx")
	wantError(t, textReply(t, root, map[string]any{"operation": "writeBytes", "id": "b", "path": "doc.hwpx", "data": "not base64!", "expect": version(second)}),
		"writeBytes data is not base64")
	wantError(t, textReply(t, root, map[string]any{"operation": "writeBytes", "id": "n", "path": "doc.hwpx", "data": data}),
		"writeBytes requires expect: a version or null")
	created := textReply(t, root, map[string]any{"operation": "writeBytes", "id": "new", "path": "new.hwp", "data": data, "expect": nil})
	if created["version"] != version(second) {
		t.Fatalf("writeBytes of a new file = %v", created)
	}
	wantError(t, textReply(t, root, map[string]any{"operation": "writeBytes", "id": "e", "path": "new.hwp", "data": data, "expect": nil}), "exists: new.hwp")
}
