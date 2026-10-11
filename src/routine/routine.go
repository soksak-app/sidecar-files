// Package routine is the one entrance that starts a goroutine of the sidecar. A goroutine that panics ends the process, so the
// entrance records the panic with its stack as an error event before it ends the process with a non-zero status
// (docs/spec/logging.md#writers in the soksak core repository).
package routine

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync/atomic"

	"github.com/soksak-app/sidecar-files/src/events"
)

// panicStatus is the exit status of a process that a panic ended.
const panicStatus = 2

var current atomic.Pointer[events.Log]

// Use sets the event log that records the panics of the routines of this process. A nil log records none, and the panic is
// written to the standard error only.
func Use(log *events.Log) { current.Store(log) }

// Go runs fn in a new goroutine named name. A panic of fn is stored as `sidecar.files.panic` (`routine`, `panic`, `stack`) and
// ends the process with status 2.
func Go(name string, fn func()) {
	go func() {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			stack := string(debug.Stack())
			// The text goes to the standard error as well, which the host stores as a record, because the event file may be off.
			fmt.Fprintf(os.Stderr, "files sidecar: panic in %s: %v\n%s", name, recovered, stack)
			log := current.Load()
			log.EmitHost(events.LevelError, "sidecar.files.panic", map[string]any{"routine": name, "panic": fmt.Sprint(recovered), "stack": stack})
			if err := log.Flush(events.ExitDeadline); err != nil {
				fmt.Fprintf(os.Stderr, "files sidecar: %v\n", err)
			}
			os.Exit(panicStatus)
		}()
		fn()
	}()
}
