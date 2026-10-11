package events

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// WatchFunc watches the folder or the regular file at path through the file system events of the platform. It calls changed
// when the folder gains, loses or renames an entry or when the file is written, and failed once when the watch fails and stops.
// The returned function ends the watch (platform.Platform.Watch).
type WatchFunc func(path string, changed func(), failed func(error)) (stop func() error, err error)

// policyReport is the text of an error record of the place `log policy`.
func policyReport(err error) string {
	return fmt.Sprintf("error: log policy: %v\n", err)
}

// readPolicyFile reads the policy file. It returns the policy in force after the read, the source of that policy, and whether
// the file changes the policy: a file that is missing puts the default policy in force, an empty file waits (no change), and a
// refused file leaves the policy as it was and returns the refusal.
func readPolicyFile(file string) (policy Policy, source string, change bool, err error) {
	text, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultPolicy(), "default", true, nil
	}
	if err != nil {
		return Policy{}, "", false, fmt.Errorf("%s: %v", file, err)
	}
	if PolicyBeingWritten(text) {
		return Policy{}, "", false, nil
	}
	policy, err = ParsePolicy(file, text)
	if err != nil {
		return Policy{}, "", false, err
	}
	return policy, "file", true, nil
}

// loadPolicyAtStart applies the first policy of the run: the policy of the file when it is complete and valid, otherwise the
// default policy. A file that is refused or cannot be read is reported.
func (l *Log) loadPolicyAtStart() {
	policy, source, change, err := readPolicyFile(l.options.PolicyFile)
	if err != nil {
		l.startRefused = err.Error()
		l.reportPolicy(err)
	}
	if !change {
		policy, source = DefaultPolicy(), "default"
	}
	l.ApplyPolicy(policy, source)
}

// reportPolicy writes an error record of the place `log policy` to the report of the options, which is the standard error of
// the sidecar and which the host stores as a record.
func (l *Log) reportPolicy(err error) {
	if _, writeErr := io.WriteString(l.options.PolicyReport, policyReport(err)); writeErr != nil {
		fmt.Fprintf(os.Stderr, "event writer: the refusal of the policy file cannot be reported: %v: %v\n", writeErr, err)
	}
}

// policyWatch is the watch of the policy file: the folder that holds it, which reports a creation, a replacement, a rename and a
// removal, and the file itself, which reports a write in place.
type policyWatch struct {
	log   *Log
	watch WatchFunc
	file  string

	mu      sync.Mutex
	stopped bool
	// stopFolder ends the watch of the folder. stopFile ends the watch of the file, and watched is the file it watches, so a
	// replacement (a new file under the same name) is watched from the next notification on.
	stopFolder func() error
	stopFile   func() error
	watched    os.FileInfo
	// refused is the text of the last refusal that was reported, so a notification that leaves the same file in place does
	// not report it again.
	refused string
}

// WatchPolicy watches the policy file of the options through the file system events of the platform, so no timer reads it. A
// creation, a write, a replacement and a rename put the policy of the file in force, and a removal puts the default policy in
// force; each change is the event log.policy. An empty file waits. A refused file is reported as an error record and the policy
// in force stays. The returned function ends the watch.
func (l *Log) WatchPolicy(watch WatchFunc) (func() error, error) {
	if l == nil {
		// silent: a nil log is a sidecar started without a log folder, and it has no policy file to watch.
		return func() error { return nil }, nil
	}
	if l.options.PolicyFile == "" {
		return nil, fmt.Errorf("the event log has no policy file to watch")
	}
	// The refusal of the file at the start is reported already, and the first read of the watch finds the same file.
	w := &policyWatch{log: l, watch: watch, file: l.options.PolicyFile, refused: l.startRefused}
	folder := filepath.Dir(w.file)
	stopFolder, err := watch(folder, w.reload, w.failed)
	if err != nil {
		return nil, fmt.Errorf("watch of the policy folder %s: %w", folder, err)
	}
	w.stopFolder = stopFolder
	// The file may have changed between the read at the start and the watch, so it is read again once the watch is armed.
	w.reload()
	return w.stop, nil
}

func (w *policyWatch) stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped = true
	return errors.Join(w.stopFolder(), w.endFile())
}

func (w *policyWatch) endFile() error {
	if w.stopFile == nil {
		return nil
	}
	err := w.stopFile()
	w.stopFile, w.watched = nil, nil
	return err
}

func (w *policyWatch) failed(err error) {
	w.log.reportPolicy(fmt.Errorf("watch of the policy file %s: %v", w.file, err))
}

// follow watches the file that is at the path now, and none when there is none.
func (w *policyWatch) follow() {
	info, err := os.Stat(w.file)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			w.log.reportPolicy(fmt.Errorf("%s: %v", w.file, err))
		}
		if endErr := w.endFile(); endErr != nil {
			w.log.reportPolicy(endErr)
		}
		return
	}
	if w.watched != nil && os.SameFile(w.watched, info) {
		return
	}
	if endErr := w.endFile(); endErr != nil {
		w.log.reportPolicy(endErr)
	}
	stop, err := w.watch(w.file, w.reload, w.failed)
	if err != nil {
		w.log.reportPolicy(fmt.Errorf("watch of the policy file %s: %w", w.file, err))
		return
	}
	w.stopFile, w.watched = stop, info
}

// reload reads the file after a notification. The same file again changes nothing.
func (w *policyWatch) reload() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.follow()
	policy, source, change, err := readPolicyFile(w.file)
	if err != nil {
		if message := err.Error(); message != w.refused {
			w.refused = message
			w.log.reportPolicy(err)
		}
		return
	}
	w.refused = ""
	if change {
		w.log.ApplyPolicy(policy, source)
	}
}
