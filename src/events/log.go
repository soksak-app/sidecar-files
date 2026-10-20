// Package events writes the event file of the sidecar in the event contract of the soksak core specification
// (docs/spec/logging.md#writers).
//
// A producer pushes an event to a queue bounded by count and by bytes without waiting. One writer goroutine assigns seq,
// serializes the JSON line, writes batches, rotates the file and records the events that the queue refused and the batches
// that the disk refused as log.dropped.
package events

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"
)

// Schema is the schema number of the lines that this writer writes. Schema 2 adds `mono_ns` to every line.
const Schema = 2

// layer is the layer of every event of a sidecar.
const layer = "sidecar"

// The bounds of the writer (docs/spec/logging.md#bounds, docs/spec/logging.md#files).
const (
	queueEvents   = 8192
	queueBytes    = 16 << 20
	batchInterval = 100 * time.Millisecond
	rotateBytes   = 100 << 20
	generations   = 5
	fieldsLimit   = 1 << 20
	headBytes     = 4096
)

// Level is the level of an event.
type Level string

// The levels of an event.
const (
	LevelError Level = "error"
	LevelWarn  Level = "warn"
	LevelInfo  Level = "info"
	LevelDebug Level = "debug"
	LevelTrace Level = "trace"
)

// Options are the folder, the role, the clock and the bounds of a writer. DefaultOptions fills the bounds of the contract.
type Options struct {
	// Directory is the log folder; the file is Directory/<Role>-<boot>.jsonl.
	Directory string
	// Role is the name of the sidecar in the file name and in log.open.
	Role string
	// MonotonicNanos reads the system-wide monotonic clock of the platform (docs/spec/logging.md#event).
	MonotonicNanos func() (uint64, error)
	// QueueEvents and QueueBytes bound the events that wait for the batch interval.
	QueueEvents int
	QueueBytes  int64
	// BatchInterval is the time between two writes of the batch.
	BatchInterval time.Duration
	// RotateBytes is the size at which the file is closed, and Generations the number of earlier files that are kept.
	RotateBytes int64
	Generations int
	// Classes gives the class of each event name that the catalog declares (ClassesOfCatalog). The policy gates the events it
	// names; an event it does not name has no class and no gate.
	Classes map[string]Class
	// PolicyFile is the policy file log-policy.json, in the parent folder of the log folder (docs/spec/logging.md#policy-file).
	// Start reads it, WatchPolicy watches it, and an empty value gives the writer the default policy only. PolicyReport
	// receives an error record of the place `log policy` for a file that is refused.
	PolicyFile   string
	PolicyReport io.Writer
	// Write stores a batch in the file. A test replaces it to cause a failed write.
	Write func(file *os.File, data []byte) error
}

// DefaultOptions returns the options of the contract for the folder, the role and the clock.
func DefaultOptions(directory, role string, monotonicNanos func() (uint64, error)) Options {
	return Options{
		Directory: directory, Role: role, MonotonicNanos: monotonicNanos,
		QueueEvents: queueEvents, QueueBytes: queueBytes, BatchInterval: batchInterval,
		RotateBytes: rotateBytes, Generations: generations,
		Write: func(file *os.File, data []byte) error {
			_, err := file.Write(data)
			return err
		},
	}
}

// pendingEvent is one event waiting for the writer. seq is assigned when the writer serializes it.
type pendingEvent struct {
	tsMicros int64
	monoNs   uint64
	level    Level
	event    string
	// class is the class that the catalog declares for the event; it is empty, and undeclared is set, for a name the catalog
	// does not declare (docs/spec/logging.md#event).
	class      Class
	undeclared bool
	cids       []string
	// fields is the fields as JSON text, serialized once by the producer.
	fields json.RawMessage
	bytes  int
	// truncated is the size in bytes of the fields text before it was cut to its head, and 0 for an event that was not cut.
	truncated int
	// drop is the range of a log.dropped event; a batch that holds the event counts the range with it.
	drop *loss
}

// message is what the queue carries: an event, or a request of the owner. Two slots of the queue are reserved for requests,
// so a flush or a stop is never refused because events filled the queue.
type message struct {
	event *pendingEvent
	flush chan struct{}
	stop  bool
}

// ExitDeadline is the time a process waits for the event file to be flushed before it ends.
const ExitDeadline = 2 * time.Second

// reservedForRequests is the number of slots of the queue that only requests use.
const reservedForRequests = 2

// The channels of a loss: other holds the levels error, warn and info, and debug holds the levels debug and trace
// (docs/spec/logging.md#files).
const (
	channelOther = iota
	channelDebug
	channelCount
)

// channelNames are the names of the channels in log.dropped.
var channelNames = [channelCount]string{"other", "debug"}

// The reasons of a loss: the queue was full, or the caller gave an invalid value.
const (
	reasonFull = iota
	reasonRefused
	reasonCount
)

// reasonTexts are the reasons of the loss records.
var reasonTexts = [reasonCount]string{"the queue was full", "the event was refused as invalid"}

// lossCount is the number of events and bytes that one channel and reason lost, and the largest queue depth at those refusals.
// The writer resets it when it records the loss.
type lossCount struct {
	events, bytes, depth atomic.Uint64
}

// Log is the writer of one run of the sidecar. A nil *Log accepts no event.
type Log struct {
	options      Options
	messages     chan message
	queuedEvents atomic.Int64
	queuedBytes  atomic.Int64
	// losses counts the refusals of each channel and reason until the writer records them (docs/spec/logging.md#writers).
	losses       [channelCount][reasonCount]lossCount
	closed       atomic.Bool
	path         string
	debugPath    string
	boot         string
	done         chan struct{}
	closeOnce    sync.Once
	clockFailure sync.Once
	// policy is the policy in force. policyApplied is whether one was applied, and policyEvents are the log.policy events that
	// wait for the writer; policyMu guards the three. gated counts the events that the gate refused, by class and level.
	policy        atomic.Pointer[Policy]
	policyMu      sync.Mutex
	policyApplied bool
	policyEvents  []*pendingEvent
	// startRefused is the refusal of the policy file at the start, which Start reported.
	startRefused string
	gated        [5][5]atomic.Uint64
}

// Start creates the folder (mode 0700) and the file Directory/<Role>-<boot>.jsonl (mode 0600, created new), and starts the
// writer. The first line of the file is log.open, which the writer stores with the first batch. <boot> is the start time in
// seconds and the process identifier.
func Start(options Options) (*Log, error) {
	if options.Directory == "" || options.Role == "" || options.MonotonicNanos == nil || options.Write == nil {
		return nil, fmt.Errorf("event log options need a directory, a role, a clock and a write function")
	}
	if options.PolicyFile != "" && options.PolicyReport == nil {
		return nil, fmt.Errorf("event log options with a policy file need a report for its refusals")
	}
	if options.QueueEvents < 1 || options.QueueBytes < 1 || options.BatchInterval <= 0 || options.RotateBytes < 1 || options.Generations < 1 {
		return nil, fmt.Errorf("event log options need positive bounds, found %+v", options)
	}
	if err := os.MkdirAll(options.Directory, 0o700); err != nil {
		return nil, err
	}
	// A folder that existed before keeps its mode; the log folder of the contract is private.
	if err := os.Chmod(options.Directory, 0o700); err != nil {
		return nil, err
	}
	boot := fmt.Sprintf("%d-%d", time.Now().Unix(), os.Getpid())
	path := filepath.Join(options.Directory, fmt.Sprintf("%s-%s.jsonl", options.Role, boot))
	debugFile := filepath.Join(options.Directory, fmt.Sprintf("%s-%s.debug.jsonl", options.Role, boot))
	file, err := openFile(path, os.O_EXCL)
	if err != nil {
		return nil, err
	}
	log := &Log{
		options:   options,
		messages:  make(chan message, options.QueueEvents+reservedForRequests),
		path:      path,
		debugPath: debugFile,
		boot:      boot,
		done:      make(chan struct{}),
	}
	open := log.openEvent()
	// The default policy is in force until a policy file says otherwise, and it is the first log.policy of the run.
	if options.PolicyFile != "" {
		log.loadPolicyAtStart()
	} else {
		log.ApplyPolicy(DefaultPolicy(), "default")
	}
	// The writer is the one goroutine of this package. A panic in it cannot be recorded by the event file itself, so the
	// text goes to the standard error, which the host stores, and the process ends.
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				fmt.Fprintf(os.Stderr, "event writer: panic: %v\n%s", recovered, debug.Stack())
				os.Exit(2)
			}
		}()
		log.run(file, open)
	}()
	return log, nil
}

// openFile opens the event file for appending, close-on-exec, with mode 0600.
func openFile(path string, extra int) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY|syscall.O_CLOEXEC|extra, 0o600)
}

// Path is the main file of this run (error, warn and info).
func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// DebugPath is the debug file of this run (debug and trace), which exists once its first line was written.
func (l *Log) DebugPath() string {
	if l == nil {
		return ""
	}
	return l.debugPath
}

// monotonicNanos reads the monotonic clock of the records. A clock that cannot be read is a defect of the platform: it is
// reported on the standard error (once, because every later event repeats it) and the event stores 0, which the audit of the
// contract refuses.
func (l *Log) monotonicNanos() uint64 {
	nanos, err := l.options.MonotonicNanos()
	if err != nil {
		l.clockFailure.Do(func() { fmt.Fprintf(os.Stderr, "event writer: the monotonic clock cannot be read: %v\n", err) })
		return 0
	}
	return nanos
}

// openEvent is the log.open event of a file: the schema number, the process identifier, the role and the boot.
func (l *Log) openEvent() *pendingEvent {
	fields := fmt.Sprintf(`{"schema":%d,"pid":%d,"role":%q,"boot":%q}`, Schema, os.Getpid(), l.options.Role, l.boot)
	open := &pendingEvent{tsMicros: time.Now().UnixMicro(), monoNs: l.monotonicNanos(), level: LevelInfo, event: "log.open", fields: json.RawMessage(fields)}
	l.classify(open)
	return open
}

// checkCids refuses a list of correlation identifiers that the line of an event cannot carry: an identifier is not empty and
// holds no quote, no backslash and no control character, so a line writes it without escaping.
func checkCids(cids []string) error {
	for _, cid := range cids {
		if cid == "" {
			return fmt.Errorf("a cid is empty")
		}
		for _, c := range cid {
			if c < 0x20 || c == 0x7f || c == '"' || c == '\\' {
				return fmt.Errorf("the cid %q holds a quote, a backslash or a control character", cid)
			}
		}
	}
	return nil
}

// EmitHost queues an event. It returns false, and counts the loss, when the queue has no room or the event is invalid. It
// never waits. The fields are serialized once, and only after the queue is known to have room for another event. The name
// is the entrance name that the census of soksak-events recognizes. An event that the policy does not store is counted for
// log.gated and not queued; the fields may be a `func() map[string]any`, which runs only for an event that the policy stores.
func (l *Log) EmitHost(level Level, event string, fields any) bool {
	return l.emit(level, event, nil, fields)
}

// EmitHostCids queues an event like EmitHost, with the correlation identifiers of the inputs it belongs to. An identifier
// that a line cannot carry is a defect of the caller: it is reported on the standard error, counted as lost, and the event is
// refused.
func (l *Log) EmitHostCids(level Level, event string, cids []string, fields any) bool {
	return l.emit(level, event, cids, fields)
}

func (l *Log) emit(level Level, event string, cids []string, fields any) bool {
	if l == nil || l.closed.Load() {
		// silent: a nil log is a sidecar started without a log folder, and a closed log has flushed its file at the exit.
		return false
	}
	switch level {
	case LevelError, LevelWarn, LevelInfo, LevelDebug, LevelTrace:
	default:
		l.refuse(level, len(event))
		fmt.Fprintf(os.Stderr, "event writer: %s has the level %q, which the contract does not define\n", event, string(level))
		return false
	}
	if !l.gate(event, level) {
		return false
	}
	if err := checkCids(cids); err != nil {
		l.refuse(level, len(event))
		fmt.Fprintf(os.Stderr, "event writer: the cids of %s are refused: %v\n", event, err)
		return false
	}
	if l.queuedEvents.Load() >= int64(l.options.QueueEvents) {
		l.countLoss(level, len(event))
		return false
	}
	// A builder runs only for an event that the policy stores.
	if build, ok := fields.(func() map[string]any); ok {
		fields = build()
	}
	text, err := json.Marshal(fields)
	if err != nil {
		// A value that cannot be serialized is a defect of the caller; it is counted in place of the event.
		l.refuse(level, len(event))
		fmt.Fprintf(os.Stderr, "event writer: the fields of %s cannot be serialized: %v\n", event, err)
		return false
	}
	text, truncated := capFields(text)
	bytes := len(text) + len(event)
	for _, cid := range cids {
		bytes += len(cid)
	}
	if l.queuedBytes.Load()+int64(bytes) > l.options.QueueBytes {
		l.countLoss(level, bytes)
		return false
	}
	pending := &pendingEvent{tsMicros: time.Now().UnixMicro(), monoNs: l.monotonicNanos(), level: level, event: event, cids: cids, fields: text, bytes: bytes, truncated: truncated}
	l.classify(pending)
	l.queuedBytes.Add(int64(bytes))
	l.queuedEvents.Add(1)
	select {
	case l.messages <- message{event: pending}:
		return true
	default:
		l.queuedBytes.Add(-int64(bytes))
		l.queuedEvents.Add(-1)
		l.countLoss(level, bytes)
		return false
	}
}

// cutText returns text cut to at most limit bytes on a character boundary, and whether it was cut.
func cutText(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	end := limit
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end], true
}

// capFields returns the fields text of an event as the writer stores it. Fields over fieldsLimit are replaced by
// `{"head": <the first headBytes bytes of the text>}`, and the second result is the size of the text before the cut; it is 0
// for fields under the bound (docs/spec/logging.md#bounds).
func capFields(text json.RawMessage) (json.RawMessage, int) {
	if len(text) <= fieldsLimit {
		return text, 0
	}
	// silent: the text is over the bound here, so the second result (whether it was cut) is true by the condition above.
	head, _ := cutText(string(text), headBytes)
	cut, err := json.Marshal(map[string]string{"head": head})
	if err != nil {
		// A string serializes without failure, so this is a defect of the serializer; the event keeps its size and no head.
		fmt.Fprintf(os.Stderr, "event writer: the head of an event over the size bound cannot be serialized: %v\n", err)
		return json.RawMessage(`{}`), len(text)
	}
	return cut, len(text)
}

// resultErrorBytes is the bound of the text of an error of the result form (docs/spec/logging.md#result-form).
const resultErrorBytes = 1024

// ResultError returns the `error` of an event of the result form: `<code>: <text>`, cut to 1,024 bytes on a character
// boundary. A failure that has no code of its own has the internal code -32603.
func ResultError(code int, text string) string {
	// silent: the cut flag is not needed, the bound is the contract and the text is cut in place.
	cut, _ := cutText(fmt.Sprintf("%d: %s", code, text), resultErrorBytes)
	return cut
}

// channelOf is the channel of the events of a level; a level the contract does not define is counted in other.
func channelOf(level Level) int {
	if level == LevelDebug || level == LevelTrace {
		return channelDebug
	}
	return channelOther
}

// countLoss counts an event of the level that the queue refused because it was full.
func (l *Log) countLoss(level Level, bytes int) {
	l.count(&l.losses[channelOf(level)][reasonFull], bytes)
}

// refuse counts an event of the level that the writer refused because the caller gave it an invalid value.
func (l *Log) refuse(level Level, bytes int) {
	l.count(&l.losses[channelOf(level)][reasonRefused], bytes)
}

// count adds one event of bytes to a loss and keeps the largest queue depth at its refusals.
func (l *Log) count(loss *lossCount, bytes int) {
	loss.events.Add(1)
	loss.bytes.Add(uint64(bytes))
	queued := uint64(l.queuedEvents.Load())
	for {
		current := loss.depth.Load()
		if queued <= current || loss.depth.CompareAndSwap(current, queued) {
			return
		}
	}
}

// classify gives the event the class that the catalog declares for its name. A name the catalog does not declare is stored
// without a class and marked undeclared (docs/spec/logging.md#event).
func (l *Log) classify(e *pendingEvent) {
	e.class, e.undeclared = l.options.Classes[e.event], false
	if e.class == "" {
		e.undeclared = true
	}
}

// Flush writes everything that is queued and waits until it is on disk, at most deadline.
func (l *Log) Flush(deadline time.Duration) error {
	if l == nil {
		// silent: a nil log has no file to flush.
		return nil
	}
	answer := make(chan struct{})
	// A request has a reserved slot, so this send is not refused by a full queue of events.
	select {
	case l.messages <- message{flush: answer}:
	default:
		return fmt.Errorf("the event writer did not take the flush request")
	}
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	select {
	case <-answer:
		return nil
	case <-timer.C:
		return fmt.Errorf("the event writer did not flush within %s", deadline)
	}
}

// Close writes everything that is queued and ends the writer. An event after Close is refused.
func (l *Log) Close() {
	if l == nil {
		// silent: a nil log has no writer to end.
		return
	}
	l.closeOnce.Do(func() {
		l.closed.Store(true)
		select {
		case l.messages <- message{stop: true}:
			<-l.done
		default:
			fmt.Fprintln(os.Stderr, "event writer: the writer did not take the stop request")
		}
	})
}

// eventRecord is the line of one event with the seq that the writer gives it. The fields are already JSON text, so the line
// is assembled without parsing them again.
type eventRecord struct {
	TsMicros int64           `json:"ts_us"`
	MonoNs   uint64          `json:"mono_ns"`
	Seq      uint64          `json:"seq"`
	Level    Level           `json:"level"`
	Class    Class           `json:"class,omitempty"`
	Layer    string          `json:"layer"`
	Event    string          `json:"event"`
	Cids     []string        `json:"cids,omitempty"`
	Fields   json.RawMessage `json:"fields"`
	// Undeclared is true for an event whose name the catalog does not declare; such a line has no class.
	Undeclared bool `json:"undeclared,omitempty"`
	// Truncated is the size in bytes of the fields before they were cut; it is absent for an event that was not cut.
	Truncated int `json:"truncated,omitempty"`
}

func eventLine(seq uint64, e *pendingEvent) ([]byte, error) {
	line, err := json.Marshal(eventRecord{TsMicros: e.tsMicros, MonoNs: e.monoNs, Seq: seq, Level: e.level, Class: e.class, Layer: layer, Event: e.event, Cids: e.cids, Fields: e.fields, Undeclared: e.undeclared, Truncated: e.truncated})
	if err != nil {
		return nil, err
	}
	return append(line, '\n'), nil
}

// loss is a range of seq that no line of the file holds, with the events and the bytes it stands for. queued is the largest
// queue depth at the refusals that the range counts; a write that failed refused no event, so its queued is 0. channel is
// the channel of the lost events.
type loss struct {
	from, to, count, bytes, queued uint64
	channel, reason                string
}

// dropped is the log.dropped event that names the range. It is stored in the main file (docs/spec/logging.md#writers).
func (l *Log) dropped(lost loss) *pendingEvent {
	fields := fmt.Sprintf(`{"from_seq":%d,"to_seq":%d,"count":%d,"bytes":%d,"reason":%q,"channel":%q,"queued":%d}`, lost.from, lost.to, lost.count, lost.bytes, lost.reason, lost.channel, lost.queued)
	dropped := &pendingEvent{tsMicros: time.Now().UnixMicro(), monoNs: l.monotonicNanos(), level: LevelWarn, event: "log.dropped", fields: json.RawMessage(fields), drop: &lost}
	l.classify(dropped)
	return dropped
}

// rotateFiles moves the full file to generation 1 and the older generations one step up, removes the generation beyond the
// kept ones, and opens a new file. It returns the new file and the name of the removed generation (empty when none was
// removed).
func (l *Log) rotateFiles(path string, file *os.File) (*os.File, string, error) {
	generation := func(n int) string { return fmt.Sprintf("%s.%d", path, n) }
	removed := ""
	if err := os.Remove(generation(l.options.Generations)); err == nil {
		removed = filepath.Base(generation(l.options.Generations))
	} else if !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("rotation could not remove %s: %v", generation(l.options.Generations), err)
	}
	for n := l.options.Generations - 1; n >= 1; n-- {
		if err := os.Rename(generation(n), generation(n+1)); err != nil && !os.IsNotExist(err) {
			return nil, "", fmt.Errorf("rotation could not move %s: %v", generation(n), err)
		}
	}
	if err := os.Rename(path, generation(1)); err != nil {
		return nil, "", fmt.Errorf("rotation could not move %s: %v", path, err)
	}
	// The new file is opened before the old one is closed, so a failure leaves the writer with a file to write.
	next, err := openFile(path, 0)
	if err != nil {
		return nil, "", fmt.Errorf("rotation could not open %s: %v", path, err)
	}
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "event writer: rotation could not close the full file: %v\n", err)
	}
	return next, removed, nil
}

// channelFile is one file of the writer: the main file of the levels error, warn and info, or the debug file of the levels
// debug and trace (docs/spec/logging.md#files). The debug file is created when its first line is written.
type channelFile struct {
	// name is the channel of the lines of the file, which names the loss of a write of the file.
	name string
	path string
	file *os.File
	// batch holds the lines that wait for the next write, batchLines their number and batchFirst the seq of the first.
	batch      []byte
	batchLines uint64
	batchFirst uint64
	// batchDrops are the ranges of the log.dropped lines in the batch, which a failed write of the batch loses with its lines.
	batchDrops []loss
	// size is the number of bytes of the file, written or waiting in the batch.
	size int64
	// onDisk is whether the file holds its log.open line; inBatchOpen is whether the batch holds it.
	onDisk, inBatchOpen bool
	// pending is the range that a write refused; its log.dropped is stored in the main file with the next batch of the file.
	pending loss
	// rotated is the note of a rotation, stored in the line after the log.open of the new file.
	rotated *pendingEvent
	// rotateAt is the size at which the file is rotated; a failed rotation moves it by one bound.
	rotateAt int64
}

// run is the writer goroutine: it takes the messages in order, assigns seq and writes the batches. The levels error, warn
// and info go to the main file and the levels debug and trace to the debug file, with one seq across both. Every log.dropped
// is stored in the main file.
func (l *Log) run(file *os.File, first *pendingEvent) {
	defer close(l.done)
	otherChannel := &channelFile{name: channelNames[channelOther], path: l.path, file: file, rotateAt: l.options.RotateBytes}
	debugChannel := &channelFile{name: channelNames[channelDebug], path: l.debugPath, rotateAt: l.options.RotateBytes}
	defer func() {
		for _, channel := range []*channelFile{otherChannel, debugChannel} {
			if channel.file != nil {
				if err := channel.file.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "event writer: %v\n", err)
				}
			}
		}
	}()
	var seq uint64
	// heldEvents and heldBytes are the events of the queue that the batches hold; the queue is charged for them until they
	// are written, so the batches are bounded like the queue.
	var heldEvents, heldBytes int64
	appendLine := func(f *channelFile, e *pendingEvent) {
		seq++
		line, err := eventLine(seq, e)
		if err != nil {
			// The line is not written; the seq stays taken, so the audit shows the gap next to this report.
			fmt.Fprintf(os.Stderr, "event writer: %s cannot be written: %v\n", e.event, err)
			return
		}
		if f.batchLines == 0 {
			f.batchFirst = seq
		}
		f.batchLines++
		f.batch = append(f.batch, line...)
		f.size += int64(len(line))
		if e.drop != nil {
			f.batchDrops = append(f.batchDrops, *e.drop)
		}
	}
	// begin stores the log.open of a file that has none in its batch, with the note of the rotation that made the file. Then it
	// stores the log.dropped of a write that the disk refused, in the main file.
	var begin func(f *channelFile)
	begin = func(f *channelFile) {
		if !f.onDisk && !f.inBatchOpen {
			open := first
			if open == nil {
				open = l.openEvent()
			}
			// The first policy of the run follows the log.open of the first file, so the file says what it stores from its
			// second line.
			policies := []*pendingEvent(nil)
			if first != nil {
				policies = l.takePolicyEvents()
			}
			if f == otherChannel {
				first = nil
			}
			appendLine(f, open)
			f.inBatchOpen = true
			for _, e := range policies {
				appendLine(f, e)
			}
			if f.rotated != nil {
				appendLine(f, f.rotated)
				f.rotated = nil
			}
		}
		// A write that the disk refused left a range that no line holds. Its log.dropped goes to the main file.
		if f.pending.count > 0 {
			lost := f.pending
			f.pending = loss{}
			begin(otherChannel)
			appendLine(otherChannel, l.dropped(lost))
		}
	}
	flush := func(f *channelFile) {
		// A file with no line to write opens with its first line: begin would write the log.open of a file that no event of
		// its channel reached, and the debug file would hold an open line without a writer.
		if len(f.batch) == 0 {
			return
		}
		begin(f)
		// A failed write is reported on the standard error and counted: the lines of the batch have seq that the file does
		// not hold, so the next lines name their range in a log.dropped.
		if err := l.options.Write(f.file, f.batch); err != nil {
			fmt.Fprintf(os.Stderr, "event writer: %v\n", err)
			lost := loss{from: f.batchFirst, to: seq, count: f.batchLines, bytes: uint64(len(f.batch)), channel: f.name, reason: "the write failed: " + err.Error()}
			// The log.dropped lines of the batch are lost with it: each adds the events it named in place of its own line.
			for _, drop := range f.batchDrops {
				lost.from = min(lost.from, drop.from)
				lost.count += drop.count - 1
				lost.bytes += drop.bytes
			}
			f.pending = lost
			f.size -= int64(len(f.batch))
		} else if f.inBatchOpen {
			f.onDisk = true
		}
		f.inBatchOpen = false
		f.batch = f.batch[:0]
		f.batchLines = 0
		f.batchDrops = nil
	}
	// push stores an event. A file that this line would take past the bound is closed first; the new file opens with log.open
	// and the record of the rotation, so every file of the contract starts with log.open and seq continues.
	push := func(e *pendingEvent) {
		f := otherChannel
		if e.level == LevelDebug || e.level == LevelTrace {
			f = debugChannel
		}
		begin(f)
		line, err := eventLine(seq+1, e)
		if err == nil && f.size > 0 && f.size+int64(len(line)) > f.rotateAt {
			full := f.size
			flush(f)
			next, removed, rotateErr := l.rotateFiles(f.path, f.file)
			if rotateErr != nil {
				// A failed rotation leaves the full file in use; the next try is one bound later, so the failure is
				// reported once for each bound instead of for each event.
				fmt.Fprintf(os.Stderr, "event writer: %v\n", rotateErr)
				f.rotateAt = f.size + l.options.RotateBytes
			} else {
				f.file = next
				f.size, f.onDisk, f.rotateAt = 0, false, l.options.RotateBytes
				name := "null"
				if removed != "" {
					name = fmt.Sprintf("%q", removed)
				}
				f.rotated = &pendingEvent{tsMicros: time.Now().UnixMicro(), monoNs: l.monotonicNanos(), level: LevelInfo, event: "log.rotated",
					fields: json.RawMessage(fmt.Sprintf(`{"bytes":%d,"kept":%d,"removed":%s}`, full, l.options.Generations, name))}
				l.classify(f.rotated)
				begin(f)
			}
		}
		// The debug file is created with its first line of its channel.
		if f.file == nil {
			opened, openErr := openFile(f.path, os.O_EXCL)
			if openErr != nil {
				fmt.Fprintf(os.Stderr, "event writer: the debug file could not be created: %v\n", openErr)
				return
			}
			f.file = opened
		}
		appendLine(f, e)
	}
	// recordLoss stores one log.dropped for each channel and reason that lost events, each after the range of seq that its
	// refused events would have had. The refused events had no seq. Their numbers are skipped, and log.dropped is stored after
	// the range that it names (docs/spec/logging.md#event).
	recordLoss := func() {
		for channel := range channelCount {
			for reason := range reasonCount {
				counted := &l.losses[channel][reason]
				events, bytes, queued := counted.events.Swap(0), counted.bytes.Swap(0), counted.depth.Swap(0)
				if events == 0 {
					continue
				}
				from := seq + 1
				seq += events
				push(l.dropped(loss{from: from, to: seq, count: events, bytes: bytes, queued: queued, channel: channelNames[channel], reason: reasonTexts[reason]}))
			}
		}
	}
	// recordPolicy stores the log.policy events and the counts of log.gated. They are events of the writer and pass every gate.
	recordPolicy := func() {
		for _, e := range l.takePolicyEvents() {
			push(e)
		}
		for _, count := range l.takeGated() {
			push(l.gatedEvent(count))
		}
	}
	write := func() {
		recordLoss()
		recordPolicy()
		// The debug file is flushed first: a write of it that the disk refuses stores its log.dropped in the main file, which
		// the main file then writes.
		flush(debugChannel)
		flush(otherChannel)
		// Every event that the batches held is written or counted as lost now, so the queue is no longer charged for it.
		l.queuedEvents.Add(-heldEvents)
		l.queuedBytes.Add(-heldBytes)
		heldEvents, heldBytes = 0, 0
	}
	take := func(e *pendingEvent) {
		heldEvents++
		heldBytes += int64(e.bytes)
		push(e)
	}
	answer := func(request *message) bool {
		if request.flush != nil {
			close(request.flush)
		}
		return request.stop
	}
	// drain takes every event that is queued now, writes them, and returns the request that it met, if any.
	drain := func() *message {
		for {
			select {
			case next := <-l.messages:
				if next.event != nil {
					take(next.event)
					continue
				}
				write()
				return &next
			default:
				write()
				return nil
			}
		}
	}
	timer := time.NewTimer(l.options.BatchInterval)
	defer timer.Stop()
	for {
		select {
		case next := <-l.messages:
			if next.event != nil {
				urgent := next.event.level == LevelError
				take(next.event)
				if urgent {
					write()
				}
				continue
			}
			// The queue is in order, so every event before the request is already taken; the request writes them and answers.
			write()
			if answer(&next) {
				return
			}
		case <-timer.C:
			if request := drain(); request != nil && answer(request) {
				return
			}
			timer.Reset(l.options.BatchInterval)
		}
	}
}
