package files_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/soksak-app/sidecar-files/src/files"
)

// failingWriteFile wraps a real file and injects failures at selected operations.
type failingWriteFile struct {
	file        *os.File
	writeLimit  int
	writeErr    error
	truncateErr error
	syncErr     error
	closeErr    error
	calls       []string
	closed      bool
}

func (file *failingWriteFile) WriteAt(content []byte, offset int64) (int, error) {
	file.calls = append(file.calls, fmt.Sprintf("write:%d", offset))
	if file.writeLimit >= 0 {
		content = content[:file.writeLimit]
	}
	written, err := file.file.WriteAt(content, offset)
	return written, errors.Join(err, file.writeErr)
}

func (file *failingWriteFile) Truncate(size int64) error {
	file.calls = append(file.calls, fmt.Sprintf("truncate:%d", size))
	if file.truncateErr != nil {
		return file.truncateErr
	}
	return file.file.Truncate(size)
}

func (file *failingWriteFile) Sync() error {
	file.calls = append(file.calls, "sync")
	if file.syncErr != nil {
		return file.syncErr
	}
	return file.file.Sync()
}

func (file *failingWriteFile) Close() error {
	file.calls = append(file.calls, "close")
	file.closed = true
	return errors.Join(file.file.Close(), file.closeErr)
}

// writeFailureCase bounds each file-operation case independently, including its cleanup.
func writeFailureCase(t *testing.T) {
	t.Helper()
	name := t.Name()
	timer := time.AfterFunc(10*time.Second, func() { panic("file-operation case timed out: " + name) })
	t.Cleanup(func() { timer.Stop() })
}

func openedWriteFile(t *testing.T, content string) (*failingWriteFile, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	opened, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	file := &failingWriteFile{file: opened, writeLimit: -1}
	t.Cleanup(func() {
		if !file.closed {
			if err := file.file.Close(); err != nil {
				t.Errorf("close fixture: %v", err)
			}
		}
	})
	return file, path
}

func TestWriteOpenedFileFailures(t *testing.T) {
	operationErr := errors.New("injected operation failure")
	closeErr := errors.New("injected close failure")
	for _, each := range []struct {
		name, content, wantContent, prefix string
		writeLimit                         int
		writeErr, truncateErr, syncErr     error
		closeErr                           error
		calls                              []string
	}{
		{"partial_write", "NEW", "NEiginal-tail", "write incomplete: f.txt: ", 2, operationErr, nil, nil, nil, []string{"write:0", "close"}},
		{"write_before_any_byte", "NEW", "original-tail", "write f.txt: ", 0, operationErr, nil, nil, nil, []string{"write:0", "close"}},
		{"truncate", "NEW", "NEWginal-tail", "write incomplete: f.txt: ", -1, nil, operationErr, nil, nil, []string{"write:0", "truncate:3", "close"}},
		{"sync", "NEW", "NEW", "write incomplete: f.txt: ", -1, nil, nil, operationErr, nil, []string{"write:0", "truncate:3", "sync", "close"}},
		{"close", "NEW", "NEW", "write incomplete: f.txt: ", -1, nil, nil, nil, closeErr, []string{"write:0", "truncate:3", "sync", "close"}},
		{"partial_write_and_close", "NEW", "NEiginal-tail", "write incomplete: f.txt: ", 2, operationErr, nil, nil, closeErr, []string{"write:0", "close"}},
		{"empty_content_sync", "", "", "write f.txt: ", -1, nil, nil, operationErr, nil, []string{"write:0", "truncate:0", "sync", "close"}},
	} {
		t.Run(each.name, func(t *testing.T) {
			writeFailureCase(t)
			file, path := openedWriteFile(t, "original-tail")
			file.writeLimit, file.writeErr = each.writeLimit, each.writeErr
			file.truncateErr, file.syncErr, file.closeErr = each.truncateErr, each.syncErr, each.closeErr
			written, err := files.WriteOpenedFile(file, "f.txt", []byte(each.content))
			if err == nil {
				t.Fatal("failed file operation returned no error")
			}
			if written != "" {
				t.Fatalf("failed file operation returned version %q", written)
			}
			if !strings.HasPrefix(err.Error(), each.prefix) {
				t.Fatalf("error = %q, want prefix %q", err, each.prefix)
			}
			for _, cause := range []error{each.writeErr, each.truncateErr, each.syncErr, each.closeErr} {
				if cause != nil && !errors.Is(err, cause) {
					t.Fatalf("error %v lost cause %v", err, cause)
				}
			}
			if !reflect.DeepEqual(file.calls, each.calls) {
				t.Fatalf("operations = %v, want %v", file.calls, each.calls)
			}
			if got := contentOf(t, path); got != each.wantContent {
				t.Fatalf("content = %q, want %q", got, each.wantContent)
			}
			if _, err := file.file.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("file remained open after failure: %v", err)
			}
		})
	}
}

func TestWriteOpenedFileCompletesBeforeReturningAVersion(t *testing.T) {
	for _, content := range []string{"NEW", ""} {
		t.Run(fmt.Sprintf("length_%d", len(content)), func(t *testing.T) {
			writeFailureCase(t)
			file, path := openedWriteFile(t, "original-tail")
			written, err := files.WriteOpenedFile(file, "f.txt", []byte(content))
			if err != nil || written != version([]byte(content)) {
				t.Fatalf("result = %q, %v, want the content version", written, err)
			}
			want := []string{"write:0", fmt.Sprintf("truncate:%d", len(content)), "sync", "close"}
			if !reflect.DeepEqual(file.calls, want) {
				t.Fatalf("operations = %v, want %v", file.calls, want)
			}
			if got := contentOf(t, path); got != content {
				t.Fatalf("content = %q, want %q", got, content)
			}
			if _, err := file.file.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("file remained open after success: %v", err)
			}
		})
	}
}
