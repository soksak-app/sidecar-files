package files_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soksak-app/sidecar-files/src/events"
	"github.com/soksak-app/sidecar-files/src/routine"
)

// startedLog starts a writer with the clock of the platform in folder.
func startedLog(t *testing.T, folder string) *events.Log {
	t.Helper()
	log, err := events.Start(events.DefaultOptions(folder, "files", clock(1_000_000_000)))
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// A routine that panics is recorded with its stack and ends the process, so the child process of this test runs one.
func TestRoutinePanicChild(t *testing.T) {
	folder := os.Getenv("SIDECAR_TEST_PANIC_FOLDER")
	if folder == "" {
		t.Skip("runs only as the child of TestAPanicOfARoutineIsRecordedAndEndsTheProcess")
	}
	routine.Use(startedLog(t, folder))
	routine.Go("panicking", func() { panic("on purpose") })
	time.Sleep(time.Minute)
}

// contract: log.panic.routine-records-and-ends-the-process
func TestAPanicOfARoutineIsRecordedAndEndsTheProcess(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "logs")
	command := exec.Command(os.Args[0], "-test.run", "^TestRoutinePanicChild$")
	command.Env = append(os.Environ(), "SIDECAR_TEST_PANIC_FOLDER="+folder)
	err := command.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("the child ended with %v, want exit status 2", err)
	}
	matches, err := filepath.Glob(filepath.Join(folder, "files-*.jsonl"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("event files %v: %v", matches, err)
	}
	var panicked *loggedEvent
	lines := readEvents(t, matches[0])
	for index := range lines {
		if lines[index].Event == "sidecar.files.panic" {
			panicked = &lines[index]
		}
	}
	if panicked == nil || panicked.Level != "error" {
		t.Fatalf("no error event sidecar.files.panic in %+v", lines)
	}
	stack, _ := panicked.Fields["stack"].(string)
	if panicked.Fields["routine"] != "panicking" || panicked.Fields["panic"] != "on purpose" || !strings.Contains(stack, "TestRoutinePanicChild") {
		t.Fatalf("fields %v", panicked.Fields)
	}
}

func TestARoutineRunsItsFunctionOnce(t *testing.T) {
	routine.Use(nil)
	done := make(chan int, 2)
	routine.Go("counting", func() { done <- 1 })
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the routine did not run")
	}
}
