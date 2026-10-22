package files_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordTimeLayout is the form of the time of a text record (docs/spec/diagnostics.md#forms).
const recordTimeLayout = "2006-01-02T15:04:05.000Z07:00"

var (
	buildOnce  sync.Once
	builtPath  string
	buildError error
)

// TestMain removes the folder of the built sidecar after the tests, so no binary stays in the temporary folder.
func TestMain(m *testing.M) {
	code := m.Run()
	if builtPath != "" {
		if err := os.RemoveAll(filepath.Dir(builtPath)); err != nil {
			fmt.Fprintf(os.Stderr, "remove the built sidecar: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

// sidecarBinary builds the sidecar once for the tests that run it as a process.
func sidecarBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		folder, err := os.MkdirTemp("", "files-sidecar-test-")
		if err != nil {
			buildError = err
			return
		}
		builtPath = filepath.Join(folder, "soksak-files")
		output, err := exec.Command("go", "build", "-o", builtPath, "../src").CombinedOutput()
		if err != nil {
			buildError = errors.New(string(output))
		}
	})
	if buildError != nil {
		t.Fatalf("build: %v", buildError)
	}
	return builtPath
}

// runSidecar runs the sidecar with the arguments and the lines on its standard input, and returns its standard error and exit status.
func runSidecar(t *testing.T, input string, args ...string) (string, int) {
	t.Helper()
	command := exec.Command(sidecarBinary(t), args...)
	command.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	err := command.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return stderr.String(), 0
	case errors.As(err, &exit):
		return stderr.String(), exit.ExitCode()
	default:
		t.Fatal(err)
		return "", -1
	}
}

// onlyEventFile returns the events of the one event file in folder.
func onlyEventFile(t *testing.T, folder string) []loggedEvent {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(folder, "files-*.jsonl"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("event files %v: %v", matches, err)
	}
	return readEvents(t, matches[0])
}

func find(lines []loggedEvent, event string) *loggedEvent {
	for index := range lines {
		if lines[index].Event == event {
			return &lines[index]
		}
	}
	return nil
}

// startRecord asserts that stderr is exactly the start record of the process, in the form of a text record
// (docs/spec/diagnostics.md#forms), and returns the time that the record states. The host stores each line of the standard
// error of a standard input and output sidecar as a record of level info, so the record shows the time from the spawn of the
// process to its first own record (S31).
func startRecord(t *testing.T, stderr string) time.Time {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("the process wrote %d lines: %q", len(lines), stderr)
	}
	parts := strings.SplitN(lines[0], " ", 5)
	if len(parts) != 5 || parts[1] != "info" || parts[2] != "sidecar" || parts[3] != "files:" || parts[4] != "started" {
		t.Fatalf("the line is not the start record of the process: %q", lines[0])
	}
	at, err := time.Parse(recordTimeLayout, parts[0])
	if err != nil {
		t.Fatalf("the time of the start record %q: %v", parts[0], err)
	}
	return at
}

// contract: sidecar.log.the-process-starts-with-its-own-record
func TestTheProcessStartsWithItsOwnRecord(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "logs")
	stderr, status := runSidecar(t, "", "--log-dir", folder)
	if status != 0 {
		t.Fatalf("status %d, standard error %q", status, stderr)
	}
	at := startRecord(t, stderr)
	if elapsed := time.Since(at); elapsed < 0 || elapsed > time.Minute {
		t.Fatalf("the start record time %v is not the time of this run: %v before now", at, elapsed)
	}
	// The record is written before the setup of the process, so it precedes the log.open that the writer writes after the
	// setup.
	open := find(onlyEventFile(t, folder), "log.open")
	if open == nil {
		t.Fatal("the event file holds no log.open")
	}
	if opened := time.UnixMicro(open.TsMicros); opened.Before(at) {
		t.Fatalf("log.open at %v precedes the start record at %v", opened, at)
	}
}

// contract: log.sidecar.failure-ends-with-an-error-event
func TestAFailureThatEndsTheSidecarIsAnErrorEventAndAnExit(t *testing.T) {
	config := t.TempDir()
	replace(t, filepath.Join(config, "log-policy.json"), policyText(nil))
	folder := filepath.Join(config, "logs")
	// An invalid request line is refused and the sidecar keeps running, so the failure that ends it is a read failure: a line over the
	// line limit of the protocol (docs/spec/files.md).
	stderr, status := runSidecar(t, strings.Repeat("x", 67108864+1)+"\n", "--log-dir", folder)
	if status != 1 || !strings.Contains(stderr, "token too long") {
		t.Fatalf("status %d, standard error %q", status, stderr)
	}
	lines := onlyEventFile(t, folder)
	failure, exit := find(lines, "sidecar.files.failure"), find(lines, "sidecar.files.exit")
	if failure == nil || failure.Level != "error" || !strings.Contains(failure.Fields["error"].(string), "token too long") {
		t.Fatalf("failure %+v", failure)
	}
	if exit == nil || exit.Fields["status"] != float64(1) || exit.Seq <= failure.Seq {
		t.Fatalf("exit %+v", exit)
	}
}

// contract: log.sidecar.exit-states-the-status
func TestACleanEndStatesStatusZero(t *testing.T) {
	config := t.TempDir()
	replace(t, filepath.Join(config, "log-policy.json"), policyText(nil))
	folder := filepath.Join(config, "logs")
	stderr, status := runSidecar(t, "", "--log-dir", folder)
	// The start record is the only line of the standard error of a clean run.
	if status != 0 {
		t.Fatalf("status %d, standard error %q", status, stderr)
	}
	startRecord(t, stderr)
	exit := find(onlyEventFile(t, folder), "sidecar.files.exit")
	if exit == nil || exit.Fields["status"] != float64(0) || exit.Level != "info" {
		t.Fatalf("exit %+v", exit)
	}
}

// contract: log.sidecar.without-a-folder-says-so
func TestWithoutALogFolderTheSidecarSaysSoOnceAndRuns(t *testing.T) {
	stderr, status := runSidecar(t, "")
	if status != 0 || strings.Count(stderr, "no event file") != 1 {
		t.Fatalf("status %d, standard error %q", status, stderr)
	}
}

func TestAnUnknownArgumentIsRefused(t *testing.T) {
	stderr, status := runSidecar(t, "", "--log-folder", "x")
	if status != 2 || !strings.Contains(stderr, "log-folder") {
		t.Fatalf("status %d, standard error %q", status, stderr)
	}
}

// contract: log.policy.sidecar-reads-the-file-beside-the-log-folder
func TestTheSidecarAppliesThePolicyFileOfTheParentOfTheLogFolder(t *testing.T) {
	config := t.TempDir()
	if err := os.WriteFile(filepath.Join(config, "log-policy.json"), []byte(policyText(map[string]string{"call": "off"})), 0o600); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(config, "logs")
	root := t.TempDir()
	request := requestWith(t, []string{"b1.1"}, root, map[string]any{"operation": "list", "id": "l1", "path": ""})
	stderr, status := runSidecar(t, request+"\n", "--log-dir", folder)
	// The start record is the only line of the standard error of a clean run.
	if status != 0 {
		t.Fatalf("status %d, standard error %q", status, stderr)
	}
	startRecord(t, stderr)
	lines := onlyEventFile(t, folder)
	if len(eventsNamed(lines, "sidecar.files.request")) != 0 {
		t.Fatalf("a request was stored while the class call is off: %+v", lines)
	}
	if sourcesOf(lines) != "file" {
		t.Fatalf("log.policy events %+v", policies(lines))
	}
	gated := eventsNamed(lines, "log.gated")
	if len(gated) != 1 || gated[0].Fields["class"] != "call" || gated[0].Fields["level"] != "info" || gated[0].Fields["count"] != float64(1) {
		t.Fatalf("log.gated %+v", gated)
	}
}
