package files_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/soksak-app/sidecar-files/src/files"
	"golang.org/x/sys/unix"
)

const limit = 8388608

// textReply 는 요청 하나를 처리하고 답의 body 를 JSON 그대로 반환한다.
func textReply(t *testing.T, root string, body map[string]any) map[string]any {
	t.Helper()
	line, err := json.Marshal(map[string]any{"surface": "state:files:p1", "root": root, "body": body})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := files.Serve(bytes.NewReader(append(line, '\n')), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var event map[string]any
	if err := json.Unmarshal(out.Bytes(), &event); err != nil {
		t.Fatalf("invalid event %q: %v", out.String(), err)
	}
	reply, ok := event["body"].(map[string]any)
	if !ok {
		t.Fatalf("event without body: %s", out.String())
	}
	return reply
}

func version(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func contentOf(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func wantError(t *testing.T, reply map[string]any, message string) {
	t.Helper()
	if got, _ := reply["error"].(string); got != message {
		t.Fatalf("reply = %v, want the error %q", reply, message)
	}
}

func wantErrorContaining(t *testing.T, reply map[string]any, part string) {
	t.Helper()
	if got, _ := reply["error"].(string); !strings.Contains(got, part) {
		t.Fatalf("reply = %v, want an error containing %q", reply, part)
	}
}

func TestServeReadsRequestLinesUpTo64MiB(t *testing.T) {
	root := t.TempDir()
	// 제어 문자는 JSON 에서 \u0001 의 6 바이트가 되므로 3.5 MiB 의 text 가 21 MiB 의 줄이 된다.
	text := strings.Repeat("\x01", 7<<19)
	reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "wide.txt", "text": text, "expect": nil, "bom": false})
	if reply["version"] != version([]byte(text)) || reply["error"] != nil {
		t.Fatalf("reply to a 21 MiB line = %v", reply["error"])
	}
	if contentOf(t, filepath.Join(root, "wide.txt")) != text {
		t.Fatal("the written file differs from the text")
	}
	// JSON 공백으로 줄을 정확히 67108864 바이트로 채운다. 한 바이트 더 긴 줄은 Serve 를 끝낸다.
	padded := func(length int) string {
		line, err := json.Marshal(map[string]any{"surface": "state:files:p1", "root": root, "body": map[string]any{"operation": "list", "id": "l", "path": ""}})
		if err != nil {
			t.Fatal(err)
		}
		return string(line[:len(line)-1]) + strings.Repeat(" ", length-len(line)) + "}"
	}
	var out bytes.Buffer
	if err := files.Serve(strings.NewReader(padded(67108864)+"\n"), &out); err != nil {
		t.Fatalf("serve of a 67108864-byte line: %v", err)
	}
	if !strings.Contains(out.String(), `"id":"l","entries"`) {
		t.Fatalf("reply to a 67108864-byte line = %.200s", out.String())
	}
	if err := files.Serve(strings.NewReader(padded(67108865)+"\n"), &out); err == nil {
		t.Fatal("a 67108865-byte line was accepted")
	}
}

func TestReadReportsTextVersionNewlineAndByteOrderMark(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		content, text, newline string
		bom                    bool
	}{
		{"a\nb\n", "a\nb\n", "lf", false},
		{"a\r\nb\r\n", "a\r\nb\r\n", "crlf", false},
		{"a\rb", "a\rb", "cr", false},
		{"a\nb\r\nc\r", "a\nb\r\nc\r", "mixed", false},
		{"one line <&>", "one line <&>", "none", false},
		{"", "", "none", false},
		{"\xef\xbb\xbfx\ny\n", "x\ny\n", "lf", true},
		{"\xef\xbb\xbf", "", "none", true},
	}
	for i, each := range cases {
		path := filepath.Join(root, "f.txt")
		if err := os.WriteFile(path, []byte(each.content), 0o644); err != nil {
			t.Fatal(err)
		}
		reply := textReply(t, root, map[string]any{"operation": "read", "id": "r", "path": "f.txt"})
		want := map[string]any{"id": "r", "text": each.text, "version": version([]byte(each.content)), "newline": each.newline, "bom": each.bom}
		got, _ := json.Marshal(reply)
		expected, _ := json.Marshal(want)
		if string(got) != string(expected) {
			t.Fatalf("case %d: reply = %s, want %s", i, got, expected)
		}
	}
}

func TestReadRejectsInvalidTextLargeFilesAndOtherFileTypes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bad.txt"), []byte("a\xffb"), 0o644); err != nil {
		t.Fatal(err)
	}
	big, err := os.Create(filepath.Join(root, "big.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := big.Truncate(limit + 1); err != nil {
		t.Fatal(err)
	}
	if err := big.Close(); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	read := func(path string) map[string]any {
		return textReply(t, root, map[string]any{"operation": "read", "id": "r", "path": path})
	}
	wantError(t, read("bad.txt"), "not UTF-8 text: bad.txt")
	wantError(t, read("big.txt"), "file is 8388609 bytes, above the 8388608-byte limit: big.txt")
	wantError(t, read("pipe"), "not a regular file: pipe")
	wantError(t, read("dir"), "not a regular file: dir")
}

func TestReadAndWriteRejectPathsOutsideTheRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "away")); err != nil {
		t.Fatal(err)
	}
	read := func(path string) map[string]any {
		return textReply(t, root, map[string]any{"operation": "read", "id": "r", "path": path})
	}
	wantErrorContaining(t, read("../x.txt"), "leaves the root")
	wantErrorContaining(t, read("/etc/hosts"), "must be relative")
	wantErrorContaining(t, read("link.txt"), "leaves the root")
	wantErrorContaining(t, read("away/secret.txt"), "leaves the root")
	write := func(path string, expect any) map[string]any {
		return textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": path, "text": "x", "expect": expect, "bom": false})
	}
	wantErrorContaining(t, write("../new.txt", nil), "leaves the root")
	wantErrorContaining(t, write("away/new.txt", nil), "leaves the root")
	wantErrorContaining(t, write("link.txt", version([]byte("secret"))), "leaves the root")
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("a write outside the root created a file: %v", err)
	}
	if contentOf(t, filepath.Join(outside, "secret.txt")) != "secret" {
		t.Fatal("a write outside the root changed a file")
	}
}

func inode(t *testing.T, path string) *syscall.Stat_t {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Sys().(*syscall.Stat_t)
}

func TestWriteWithExpectReplacesTheContentInPlace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	old := "a longer first content\n"
	if err := os.WriteFile(path, []byte(old), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(path, "user.soksak", []byte("kept"), 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(root, "hard.txt")); err != nil {
		t.Fatal(err)
	}
	before := inode(t, path)
	reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "f.txt", "text": "short", "expect": version([]byte(old)), "bom": false})
	if got, _ := json.Marshal(reply); string(got) != `{"id":"w","version":"`+version([]byte("short"))+`"}` {
		t.Fatalf("reply = %s", got)
	}
	if got := contentOf(t, path); got != "short" {
		t.Fatalf("content = %q, want the shorter text without the old tail", got)
	}
	if got := contentOf(t, filepath.Join(root, "hard.txt")); got != "short" {
		t.Fatalf("hard link content = %q", got)
	}
	after := inode(t, path)
	if after.Ino != before.Ino || after.Nlink != 2 || after.Mode != before.Mode {
		t.Fatalf("inode %d nlink %d mode %o, want inode %d nlink 2 mode %o", after.Ino, after.Nlink, after.Mode, before.Ino, before.Mode)
	}
	attribute := make([]byte, 16)
	n, err := unix.Getxattr(path, "user.soksak", attribute)
	if err != nil || string(attribute[:n]) != "kept" {
		t.Fatalf("extended attribute = %q, %v", attribute[:n], err)
	}
	// bom 이면 BOM 과 text 를 쓰고, 그 바이트의 SHA-256 을 돌려준다.
	reply = textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "f.txt", "text": "x\r\n", "expect": version([]byte("short")), "bom": true})
	if want := "\xef\xbb\xbfx\r\n"; reply["version"] != version([]byte(want)) || contentOf(t, path) != want {
		t.Fatalf("reply = %v, content = %q, want %q", reply, contentOf(t, path), want)
	}
}

func TestWriteThroughASymbolicLinkKeepsTheLinkAndChangesItsTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(root, "alias.txt")); err != nil {
		t.Fatal(err)
	}
	reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "alias.txt", "text": "new", "expect": version([]byte("old")), "bom": false})
	if reply["version"] != version([]byte("new")) {
		t.Fatalf("reply = %v", reply)
	}
	info, err := os.Lstat(filepath.Join(root, "alias.txt"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("alias.txt is no longer a symbolic link: %v %v", info, err)
	}
	if got := contentOf(t, filepath.Join(root, "real.txt")); got != "new" {
		t.Fatalf("target content = %q", got)
	}
}

func TestWriteRefusesAChangeOnDiskWithoutWriting(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	if err := os.WriteFile(path, []byte("changed elsewhere"), 0o644); err != nil {
		t.Fatal(err)
	}
	reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "f.txt", "text": "mine", "expect": version([]byte("read before")), "bom": false})
	wantError(t, reply, "changed on disk: f.txt")
	if got := contentOf(t, path); got != "changed elsewhere" {
		t.Fatalf("content = %q, want it unchanged", got)
	}
}

func TestWriteWithoutExpectCreatesOnlyANewFile(t *testing.T) {
	root := t.TempDir()
	reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "new.txt", "text": "made", "expect": nil, "bom": false})
	if got, _ := json.Marshal(reply); string(got) != `{"id":"w","version":"`+version([]byte("made"))+`"}` {
		t.Fatalf("reply = %s", got)
	}
	if got := contentOf(t, filepath.Join(root, "new.txt")); got != "made" {
		t.Fatalf("content = %q", got)
	}
	// 0666 을 프로세스 umask 로 가린 권한이다. 같은 방식으로 만든 파일과 비교한다.
	reference, err := os.OpenFile(filepath.Join(t.TempDir(), "reference"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	referenceInfo, err := reference.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := reference.Close(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "new.txt")); err != nil || info.Mode() != referenceInfo.Mode() {
		t.Fatalf("mode = %v (%v), want %v", info.Mode(), err, referenceInfo.Mode())
	}
	reply = textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "new.txt", "text": "again", "expect": nil, "bom": false})
	wantError(t, reply, "exists: new.txt")
	if got := contentOf(t, filepath.Join(root, "new.txt")); got != "made" {
		t.Fatalf("content = %q, want it unchanged", got)
	}
}

func TestWriteRejectsInvalidRequestsAndLargeText(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "f.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	reply := textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "f.txt", "text": "x", "bom": false})
	wantError(t, reply, "write requires expect: a version or null")
	reply = textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "f.txt", "text": "x", "expect": "old", "bom": false})
	wantError(t, reply, `write expect "old" is not a SHA-256 version`)
	reply = textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "f.txt", "expect": version([]byte("old")), "bom": false})
	wantError(t, reply, "write requires text")
	reply = textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "f.txt", "text": "x", "expect": version([]byte("old"))})
	wantError(t, reply, "write requires bom")
	reply = textReply(t, root, map[string]any{"operation": "write", "id": "w", "path": "f.txt", "text": strings.Repeat("a", limit+1), "expect": version([]byte("old")), "bom": false})
	wantError(t, reply, "file is 8388609 bytes, above the 8388608-byte limit: f.txt")
	if got := contentOf(t, path); got != "old" {
		t.Fatalf("content = %q, want it unchanged", got)
	}
}
