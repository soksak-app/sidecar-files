package files_test

import (
	"testing"
	"time"

	"github.com/soksak-app/sidecar-files/src/files"
	"github.com/soksak-app/sidecar-files/src/platform"
)

// contract: log.native.platform-reads-the-clock-and-takes-the-boot
func TestThePlatformReadsTheMonotonicClock(t *testing.T) {
	// The files package registers the implementation of the operating system.
	_ = files.Serve
	current, err := platform.Current()
	if err != nil {
		t.Fatal(err)
	}
	first, err := current.MonotonicNanos()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	second, err := current.MonotonicNanos()
	if err != nil {
		t.Fatal(err)
	}
	if first == 0 || second < first+10_000_000 || second > first+10_000_000_000 {
		t.Fatalf("the clock read %d and then %d after 20 ms", first, second)
	}
}
