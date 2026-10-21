package files

import (
	"time"

	"github.com/soksak-app/sidecar-files/src/events"
)

// internalCode is the code of a failure that has no code of its own: the internal code of the entrance (core
// docs/spec/logging.md#result-form).
const internalCode = -32603

// Trace names the request that the events of an operation belong to: the event log, the surface of the request and its
// correlation identifiers. The zero Trace records nothing.
type Trace struct {
	Log     *events.Log
	Surface string
	Cids    []string
}

// level is the level of an event of the result form: info for a success and warn for a failure.
func level(err error) events.Level {
	if err != nil {
		return events.LevelWarn
	}
	return events.LevelInfo
}

// request stores the result of a request: the operation, the identifiers it names and the time since started. The fields are
// built by a function that runs only when the policy stores the event.
func (t Trace) request(request Request, operation string, started time.Time, err error) {
	t.Log.EmitHostCids(level(err), "sidecar.files.request", t.Cids, func() map[string]any {
		return requestFields(request, operation, started, err)
	})
}

func requestFields(request Request, operation string, started time.Time, err error) map[string]any {
	fields := map[string]any{"surface": request.Surface, "operation": operation, "ok": err == nil, "micros": time.Since(started).Microseconds()}
	if request.Body.ID != "" {
		fields["id"] = request.Body.ID
	}
	if request.Body.Path != "" {
		fields["path"] = request.Body.Path
	}
	if len(request.Body.Paths) > 0 {
		fields["paths"] = request.Body.Paths
	}
	if request.Body.Generation > 0 {
		fields["generation"] = request.Body.Generation
	}
	if err != nil {
		fields["error"] = events.ResultError(internalCode, err.Error())
	}
	return fields
}

// git stores the result of one call of git: its arguments, the time it took and, for a failure, the exit status as the code.
func (t Trace) git(args string, code int, started time.Time, err error) {
	t.Log.EmitHostCids(level(err), "sidecar.files.git", t.Cids, func() map[string]any {
		fields := map[string]any{"surface": t.Surface, "args": args, "ok": err == nil, "micros": time.Since(started).Microseconds()}
		if err != nil {
			fields["error"] = events.ResultError(code, err.Error())
		}
		return fields
	})
}
