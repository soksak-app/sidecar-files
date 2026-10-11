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
)

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

// contract: log.sidecar.failure-ends-with-an-error-event
func TestAFailureThatEndsTheSidecarIsAnErrorEventAndAnExit(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "logs")
	stderr, status := runSidecar(t, "not json\n", "--log-dir", folder)
	if status != 1 || !strings.Contains(stderr, "invalid request") {
		t.Fatalf("status %d, standard error %q", status, stderr)
	}
	lines := onlyEventFile(t, folder)
	failure, exit := find(lines, "sidecar.files.failure"), find(lines, "sidecar.files.exit")
	if failure == nil || failure.Level != "error" || !strings.Contains(failure.Fields["error"].(string), "invalid request") {
		t.Fatalf("failure %+v", failure)
	}
	if exit == nil || exit.Fields["status"] != float64(1) || exit.Seq <= failure.Seq {
		t.Fatalf("exit %+v", exit)
	}
}

// contract: log.sidecar.exit-states-the-status
func TestACleanEndStatesStatusZero(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "logs")
	if stderr, status := runSidecar(t, "", "--log-dir", folder); status != 0 || stderr != "" {
		t.Fatalf("status %d, standard error %q", status, stderr)
	}
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
