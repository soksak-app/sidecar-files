//go:build darwin

package darwin

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// MonotonicNanos reads CLOCK_UPTIME_RAW, which is mach_absolute_time in nanoseconds: the clock of the events of the other
// processes of the application.
func (implementation) MonotonicNanos() (uint64, error) {
	var now unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_UPTIME_RAW, &now); err != nil {
		return 0, fmt.Errorf("clock_gettime(CLOCK_UPTIME_RAW): %w", err)
	}
	return uint64(now.Sec)*1_000_000_000 + uint64(now.Nsec), nil
}
