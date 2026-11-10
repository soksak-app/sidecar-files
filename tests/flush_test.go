package files_test

// Checks the end of a flush (S58): a flush has no time limit and ends on the end of the write of the queued events, or on the
// end of the writer (docs/spec/files.md).

import (
	"bytes"
	"os"
	"sync"
	"testing"

	"github.com/soksak-app/sidecar-files/src/events"
)

func TestAFlushEndsWhenTheWriteOfTheQueuedEventsEnds(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var held sync.Once
	log := startLog(t, func(options *events.Options) {
		write := options.Write
		options.Write = func(file *os.File, data []byte) error {
			// The write of the batch that holds the event waits until the test releases it.
			if bytes.Contains(data, []byte("sidecar.files.start")) {
				held.Do(func() {
					entered <- struct{}{}
					<-release
				})
			}
			return write(file, data)
		}
	})
	// The cleanup releases the write before the cleanup of startLog closes the log, so a failed test does not hold the writer.
	var released sync.Once
	free := func() { released.Do(func() { close(release) }) }
	t.Cleanup(free)
	if !log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"pid": 1, "goos": "test", "goarch": "test"}) {
		t.Fatal("the event was refused")
	}
	flushed := make(chan error, 1)
	go func() { flushed <- log.Flush() }()
	<-entered
	select {
	case err := <-flushed:
		t.Fatalf("the flush ended with %v while its batch was not written", err)
	default:
	}
	free()
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}
	if find(readEvents(t, log.Path()), "sidecar.files.start") == nil {
		t.Fatal("the flush ended before the event was in the file")
	}
}

func TestAFlushOfAClosedLogEndsWithTheWriter(t *testing.T) {
	log := startLog(t, nil)
	if !log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"pid": 1, "goos": "test", "goarch": "test"}) {
		t.Fatal("the event was refused")
	}
	log.Close()
	// The writer wrote every queued event before it ended, so the end of the writer ends the flush.
	if err := log.Flush(); err != nil {
		t.Fatal(err)
	}
	if find(readEvents(t, log.Path()), "sidecar.files.start") == nil {
		t.Fatal("the event of the closed log is not in the file")
	}
}
