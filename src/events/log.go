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
	cids     []string
	// fields is the fields as JSON text, serialized once by the producer.
	fields json.RawMessage
	bytes  int
	// truncated is the size in bytes of the fields text before it was cut to its head, and 0 for an event that was not cut.
	truncated int
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

// Log is the writer of one run of the sidecar. A nil *Log accepts no event.
type Log struct {
	options       Options
	messages      chan message
	queuedEvents  atomic.Int64
	queuedBytes   atomic.Int64
	lostEvents    atomic.Uint64
	lostBytes     atomic.Uint64
	refusedEvents atomic.Uint64
	refusedBytes  atomic.Uint64
	closed        atomic.Bool
	path          string
	boot          string
	done          chan struct{}
	closeOnce     sync.Once
	clockFailure  sync.Once
}

// Start creates the folder (mode 0700) and the file Directory/<Role>-<boot>.jsonl (mode 0600, created new), and starts the
// writer. The first line of the file is log.open, which the writer stores with the first batch. <boot> is the start time in
// seconds and the process identifier.
func Start(options Options) (*Log, error) {
	if options.Directory == "" || options.Role == "" || options.MonotonicNanos == nil || options.Write == nil {
		return nil, fmt.Errorf("event log options need a directory, a role, a clock and a write function")
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
	file, err := openFile(path, os.O_EXCL)
	if err != nil {
		return nil, err
	}
	log := &Log{
		options:  options,
		messages: make(chan message, options.QueueEvents+reservedForRequests),
		path:     path,
		boot:     boot,
		done:     make(chan struct{}),
	}
	open := log.openEvent()
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

// Path is the file of this run.
func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
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
	return &pendingEvent{tsMicros: time.Now().UnixMicro(), monoNs: l.monotonicNanos(), level: LevelInfo, event: "log.open", fields: json.RawMessage(fields)}
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
// is the entrance name that the census of soksak-events recognizes.
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
		l.refuse(len(event))
		fmt.Fprintf(os.Stderr, "event writer: %s has the level %q, which the contract does not define\n", event, string(level))
		return false
	}
	if err := checkCids(cids); err != nil {
		l.refuse(len(event))
		fmt.Fprintf(os.Stderr, "event writer: the cids of %s are refused: %v\n", event, err)
		return false
	}
	if l.queuedEvents.Load() >= int64(l.options.QueueEvents) {
		l.countLoss(len(event))
		return false
	}
	text, err := json.Marshal(fields)
	if err != nil {
		// A value that cannot be serialized is a defect of the caller; it is counted in place of the event.
		l.refuse(len(event))
		fmt.Fprintf(os.Stderr, "event writer: the fields of %s cannot be serialized: %v\n", event, err)
		return false
	}
	text, truncated := capFields(text)
	bytes := len(text) + len(event)
	for _, cid := range cids {
		bytes += len(cid)
	}
	if l.queuedBytes.Load()+int64(bytes) > l.options.QueueBytes {
		l.countLoss(bytes)
		return false
	}
	pending := &pendingEvent{tsMicros: time.Now().UnixMicro(), monoNs: l.monotonicNanos(), level: level, event: event, cids: cids, fields: text, bytes: bytes, truncated: truncated}
	l.queuedBytes.Add(int64(bytes))
	l.queuedEvents.Add(1)
	select {
	case l.messages <- message{event: pending}:
		return true
	default:
		l.queuedBytes.Add(-int64(bytes))
		l.queuedEvents.Add(-1)
		l.countLoss(bytes)
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

// countLoss counts an event that the queue refused because it was full.
func (l *Log) countLoss(bytes int) {
	l.lostEvents.Add(1)
	l.lostBytes.Add(uint64(bytes))
}

// refuse counts an event that the writer refused because the caller gave it an invalid value.
func (l *Log) refuse(bytes int) {
	l.refusedEvents.Add(1)
	l.refusedBytes.Add(uint64(bytes))
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
	Layer    string          `json:"layer"`
	Event    string          `json:"event"`
	Cids     []string        `json:"cids,omitempty"`
	Fields   json.RawMessage `json:"fields"`
	// Truncated is the size in bytes of the fields before they were cut; it is absent for an event that was not cut.
	Truncated int `json:"truncated,omitempty"`
}

func eventLine(seq uint64, e *pendingEvent) ([]byte, error) {
	line, err := json.Marshal(eventRecord{TsMicros: e.tsMicros, MonoNs: e.monoNs, Seq: seq, Level: e.level, Layer: layer, Event: e.event, Cids: e.cids, Fields: e.fields, Truncated: e.truncated})
	if err != nil {
		return nil, err
	}
	return append(line, '\n'), nil
}

// loss is a range of seq that no line of the file holds, with the events and the bytes it stands for.
type loss struct {
	from, to, count, bytes uint64
	reason                 string
}

// dropped is the log.dropped event that names the range.
func (l *Log) dropped(lost loss) *pendingEvent {
	fields := fmt.Sprintf(`{"from_seq":%d,"to_seq":%d,"count":%d,"bytes":%d,"reason":%q}`, lost.from, lost.to, lost.count, lost.bytes, lost.reason)
	return &pendingEvent{tsMicros: time.Now().UnixMicro(), monoNs: l.monotonicNanos(), level: LevelWarn, event: "log.dropped", fields: json.RawMessage(fields)}
}

// rotateFiles moves the full file to generation 1 and the older generations one step up, removes the generation beyond the
// kept ones, and opens a new file. It returns the new file and the name of the removed generation (empty when none was
// removed).
func (l *Log) rotateFiles(file *os.File) (*os.File, string, error) {
	generation := func(n int) string { return fmt.Sprintf("%s.%d", l.path, n) }
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
	if err := os.Rename(l.path, generation(1)); err != nil {
		return nil, "", fmt.Errorf("rotation could not move %s: %v", l.path, err)
	}
	// The new file is opened before the old one is closed, so a failure leaves the writer with a file to write.
	next, err := openFile(l.path, 0)
	if err != nil {
		return nil, "", fmt.Errorf("rotation could not open %s: %v", l.path, err)
	}
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "event writer: rotation could not close the full file: %v\n", err)
	}
	return next, removed, nil
}

// run is the writer goroutine: it takes the messages in order, assigns seq and writes the batches.
func (l *Log) run(file *os.File, first *pendingEvent) {
	defer close(l.done)
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "event writer: %v\n", err)
		}
	}()
	var (
		seq uint64
		// batch holds the lines that wait for the next write, batchLines their number and batchFirst the seq of the first.
		batch      []byte
		batchLines uint64
		batchFirst uint64
		// batchDrop is the range named by the log.dropped line in the batch.
		batchDrop *loss
		// size is the number of bytes of the current file, written or waiting in the batch.
		size int64
		// onDisk is whether the current file holds its log.open line; inBatchOpen is whether the batch holds it.
		onDisk, inBatchOpen bool
		// pending is the range that the next line must name, after a batch that the disk refused.
		pending loss
		// rotated is the note of a rotation, stored in the line after the log.open of the new file.
		rotated *pendingEvent
		// rotateAt is the size at which the file is rotated; a failed rotation moves it by one bound.
		rotateAt = l.options.RotateBytes
		// heldEvents and heldBytes are the events of the queue that the batch holds; the queue is charged for them until the
		// batch is written, so the batch is bounded like the queue.
		heldEvents, heldBytes int64
	)
	appendLine := func(e *pendingEvent) {
		seq++
		line, err := eventLine(seq, e)
		if err != nil {
			// The line is not written; the seq stays taken, so the audit shows the gap next to this report.
			fmt.Fprintf(os.Stderr, "event writer: %s cannot be written: %v\n", e.event, err)
			return
		}
		if batchLines == 0 {
			batchFirst = seq
		}
		batchLines++
		batch = append(batch, line...)
		size += int64(len(line))
	}
	// begin stores the lines that a file needs before the next event: its log.open (with the note of the rotation that made
	// the file) and the log.dropped of a batch that the disk refused.
	begin := func() {
		if !onDisk && !inBatchOpen {
			open := first
			if open == nil {
				open = l.openEvent()
			}
			first = nil
			appendLine(open)
			inBatchOpen = true
			if rotated != nil {
				appendLine(rotated)
				rotated = nil
			}
		}
		if pending.count > 0 {
			lost := pending
			pending = loss{}
			batchDrop = &lost
			appendLine(l.dropped(lost))
		}
	}
	flush := func() {
		begin()
		l.queuedEvents.Add(-heldEvents)
		l.queuedBytes.Add(-heldBytes)
		heldEvents, heldBytes = 0, 0
		if len(batch) == 0 {
			return
		}
		// A failed write is reported on the standard error and counted: the lines of the batch have seq that the file does
		// not hold, so the next lines name their range in a log.dropped.
		if err := l.options.Write(file, batch); err != nil {
			fmt.Fprintf(os.Stderr, "event writer: %v\n", err)
			lost := loss{from: batchFirst, to: seq, count: batchLines, bytes: uint64(len(batch)), reason: "the write failed: " + err.Error()}
			if batchDrop != nil {
				lost.from = min(lost.from, batchDrop.from)
				lost.count += batchDrop.count - 1
				lost.bytes += batchDrop.bytes
			}
			pending = lost
			size -= int64(len(batch))
		} else if inBatchOpen {
			onDisk = true
		}
		inBatchOpen = false
		batch = batch[:0]
		batchLines = 0
		batchDrop = nil
	}
	// push stores an event. A file that this line would take past the bound is closed first; the new file opens with log.open
	// and the record of the rotation, so every file of the contract starts with log.open and seq continues.
	push := func(e *pendingEvent) {
		begin()
		line, err := eventLine(seq+1, e)
		if err == nil && size > 0 && size+int64(len(line)) > rotateAt {
			full := size
			flush()
			next, removed, rotateErr := l.rotateFiles(file)
			if rotateErr != nil {
				// A failed rotation leaves the full file in use; the next try is one bound later, so the failure is
				// reported once for each bound instead of for each event.
				fmt.Fprintf(os.Stderr, "event writer: %v\n", rotateErr)
				rotateAt = size + l.options.RotateBytes
			} else {
				file = next
				size, onDisk, rotateAt = 0, false, l.options.RotateBytes
				name := "null"
				if removed != "" {
					name = fmt.Sprintf("%q", removed)
				}
				rotated = &pendingEvent{tsMicros: time.Now().UnixMicro(), monoNs: l.monotonicNanos(), level: LevelInfo, event: "log.rotated",
					fields: json.RawMessage(fmt.Sprintf(`{"bytes":%d,"kept":%d,"removed":%s}`, full, l.options.Generations, name))}
				begin()
			}
		}
		appendLine(e)
	}
	// recordLoss stores one log.dropped after the range of seq that the refused events would have had. The refused events
	// had no seq. Their numbers are skipped, and log.dropped is stored after the range that it names
	// (docs/spec/logging.md#event).
	record := func(events, bytes uint64, reason string) {
		if events == 0 {
			return
		}
		begin()
		from := seq + 1
		seq += events
		push(l.dropped(loss{from: from, to: seq, count: events, bytes: bytes, reason: reason}))
	}
	recordLoss := func() {
		record(l.lostEvents.Swap(0), l.lostBytes.Swap(0), "the queue was full")
		record(l.refusedEvents.Swap(0), l.refusedBytes.Swap(0), "the event was refused as invalid")
	}
	write := func() {
		recordLoss()
		flush()
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
