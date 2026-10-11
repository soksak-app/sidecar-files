//go:build linux

package linux

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// MonotonicNanos reads CLOCK_MONOTONIC.
func (implementation) MonotonicNanos() (uint64, error) {
	var now unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &now); err != nil {
		return 0, fmt.Errorf("clock_gettime(CLOCK_MONOTONIC): %w", err)
	}
	return uint64(now.Sec)*1_000_000_000 + uint64(now.Nsec), nil
}
