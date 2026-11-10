//go:build !windows

package files_test

// Checks the end of a sidecar process on SIGTERM (S58): the stop of the host closes the input and signals the process group at
// almost the same time, and SIGTERM ends the sidecar through the same cleanup as the end of its input
// (docs/spec/files.md).

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// signalledSidecar is a sidecar process in its own process group whose input stays open until the test ends it.
type signalledSidecar struct {
	t       *testing.T
	command *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Scanner
	stderr  *bytes.Buffer
}

func startSignalledSidecar(t *testing.T, environment []string, args ...string) *signalledSidecar {
	t.Helper()
	command := exec.Command(sidecarBinary(t), args...)
	command.Env = environmentWithoutDisplayName(environment)
	// The host signals the process group of the sidecar, which holds the git processes that the sidecar starts.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	return &signalledSidecar{t: t, command: command, input: input, output: scanner, stderr: stderr}
}

func (s *signalledSidecar) send(line map[string]any) {
	s.t.Helper()
	text, err := json.Marshal(line)
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.input.Write(append(text, '\n')); err != nil {
		s.t.Fatal(err)
	}
}

// reply reads the next line of the standard output.
func (s *signalledSidecar) reply() map[string]any {
	s.t.Helper()
	if !s.output.Scan() {
		s.t.Fatalf("the output ended: %v; stderr %q", s.output.Err(), s.stderr.String())
	}
	var line map[string]any
	if err := json.Unmarshal(s.output.Bytes(), &line); err != nil {
		s.t.Fatalf("%q: %v", s.output.Text(), err)
	}
	return line
}

// terminate sends SIGTERM to the process group of the sidecar while its input is open, reads the rest of the output and
// requires exit status 0. The input closes after the sidecar ended.
func (s *signalledSidecar) terminate() []map[string]any {
	s.t.Helper()
	if err := syscall.Kill(-s.command.Process.Pid, syscall.SIGTERM); err != nil {
		s.t.Fatal(err)
	}
	var rest []map[string]any
	for s.output.Scan() {
		var line map[string]any
		if err := json.Unmarshal(s.output.Bytes(), &line); err != nil {
			s.t.Fatalf("%q: %v", s.output.Text(), err)
		}
		rest = append(rest, line)
	}
	err := s.command.Wait()
	if closeErr := s.input.Close(); closeErr != nil {
		s.t.Errorf("close the input: %v", closeErr)
	}
	if err != nil {
		s.t.Fatalf("the sidecar ended with %v after SIGTERM, want status 0; stderr %q", err, s.stderr.String())
	}
	return rest
}

func seqOf(t *testing.T, lines []loggedEvent, event string) uint64 {
	t.Helper()
	found := find(lines, event)
	if found == nil {
		t.Fatalf("no %s in %+v", event, lines)
	}
	return found.Seq
}

// tracedFolder is a log folder whose policy file stores every class at every level, so the events of the cleanup are in the file.
func tracedFolder(t *testing.T) string {
	t.Helper()
	config := t.TempDir()
	replace(t, filepath.Join(config, "log-policy.json"), policyText(nil))
	return filepath.Join(config, "logs")
}

func TestSIGTERMEndsTheSidecarThroughTheCleanupOfTheEndOfTheInput(t *testing.T) {
	folder := tracedFolder(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	sidecar := startSignalledSidecar(t, os.Environ(), "--log-dir", folder)
	sidecar.send(map[string]any{"surface": "state:files:p1", "root": root, "body": map[string]any{"operation": "watch", "id": "w1", "paths": []string{"src"}}})
	if reply := sidecar.reply(); reply["body"].(map[string]any)["id"] != "w1" {
		t.Fatalf("watch reply %+v", reply)
	}
	sidecar.terminate()
	lines := onlyEventFile(t, folder)
	exit := find(lines, "sidecar.files.exit")
	if exit == nil || exit.Fields["status"] != float64(0) || exit.Fields["end"] != "terminated" {
		t.Fatalf("exit event %+v in %+v", exit, lines)
	}
	ends := eventsNamed(lines, "sidecar.files.watch_end")
	if len(ends) != 1 || ends[0].Fields["path"] != "src" || ends[0].Fields["reason"] != "terminated" {
		t.Fatalf("watch ends %+v", ends)
	}
	// The order of the cleanup: the exit event, the end of the watches, then the flush, which wrote both.
	if exit.Seq > ends[0].Seq {
		t.Fatalf("the watch ended at seq %d before the exit event at seq %d", ends[0].Seq, exit.Seq)
	}
}

// standInGit is a git that tells the test it started through a named pipe and waits for SIGTERM; on the signal it writes the
// file ended and exits with status 1, so the sidecar sees a failed git call.
const standInGit = `#!/bin/sh
trap 'echo ended > "$STAND_IN_GIT/ended"; kill "$waiting" 2>/dev/null; exit 1' TERM
sleep 600 > /dev/null 2>&1 &
waiting=$!
echo started > "$STAND_IN_GIT/started"
wait "$waiting"
`

func TestSIGTERMWaitsForTheGitProcessesOfTheRequestInHand(t *testing.T) {
	folder := tracedFolder(t)
	root := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(standInGit), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(bin, "started"), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "STAND_IN_GIT="+bin)
	sidecar := startSignalledSidecar(t, environment, "--log-dir", folder)
	sidecar.send(map[string]any{"surface": "state:files:p1", "root": root, "body": map[string]any{"operation": "git", "id": "g1"}})
	// Opening the named pipe ends when the stand-in git opens it to write, so the signal finds git running.
	started, err := os.ReadFile(filepath.Join(bin, "started"))
	if err != nil || strings.TrimSpace(string(started)) != "started" {
		t.Fatalf("the stand-in git did not start: %q %v", started, err)
	}
	rest := sidecar.terminate()
	// The request in hand finished after its git process ended, and its reply is the failure of that call.
	if len(rest) != 1 || rest[0]["body"].(map[string]any)["id"] != "g1" || rest[0]["body"].(map[string]any)["error"] == nil {
		t.Fatalf("the output after SIGTERM is %+v, want the failed reply of g1", rest)
	}
	if _, err := os.Stat(filepath.Join(bin, "ended")); err != nil {
		t.Fatalf("the sidecar ended before its git process: %v", err)
	}
	lines := onlyEventFile(t, folder)
	called := find(lines, "sidecar.files.git")
	if called == nil || called.Fields["ok"] != false {
		t.Fatalf("git event %+v in %+v", called, lines)
	}
	exit := find(lines, "sidecar.files.exit")
	if exit == nil || exit.Fields["status"] != float64(0) || exit.Fields["end"] != "terminated" {
		t.Fatalf("exit event %+v in %+v", exit, lines)
	}
	if request := seqOf(t, lines, "sidecar.files.request"); called.Seq > request || request > exit.Seq {
		t.Fatalf("git at seq %d, request at seq %d, exit at seq %d: want git, request, exit", called.Seq, request, exit.Seq)
	}
}
