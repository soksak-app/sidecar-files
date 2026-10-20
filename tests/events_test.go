package files_test

// Checks the writer of the event file against the event contract of the soksak core specification (docs/spec/logging.md):
// the golden lines in tests/fixtures/log-contract fix the shape of a line, of log.open and of log.dropped.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	catalog "github.com/soksak-app/sidecar-files"
	"unicode/utf8"

	"github.com/soksak-app/sidecar-files/src/events"
)

// loggedEvent is one line of an event file.
type loggedEvent struct {
	TsMicros   int64          `json:"ts_us"`
	MonoNs     uint64         `json:"mono_ns"`
	Seq        uint64         `json:"seq"`
	Level      string         `json:"level"`
	Layer      string         `json:"layer"`
	Event      string         `json:"event"`
	Class      string         `json:"class"`
	Undeclared bool           `json:"undeclared"`
	Cids       []string       `json:"cids"`
	Fields     map[string]any `json:"fields"`
	Truncated  int            `json:"truncated"`
}

// keysOf returns the keys of a JSON object in the order they are written.
func keysOf(t *testing.T, text string) []string {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(text))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		t.Fatalf("%q is not an object: %v", text, err)
	}
	var keys []string
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key.(string))
		var skipped json.RawMessage
		if err := decoder.Decode(&skipped); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

// fixtureLines reads a golden file of tests/fixtures/log-contract.
func fixtureLines(t *testing.T, name string) []string {
	t.Helper()
	text, err := os.ReadFile(filepath.Join("fixtures", "log-contract", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(text)), "\n")
}

func decodeEvent(t *testing.T, line string) loggedEvent {
	t.Helper()
	var event loggedEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("%q: %v", line, err)
	}
	return event
}

// readEvents returns the events of an event file in the order of its lines.
func readEvents(t *testing.T, path string) []loggedEvent {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result []loggedEvent
	for _, line := range strings.Split(strings.TrimSuffix(string(text), "\n"), "\n") {
		if line != "" {
			result = append(result, decodeEvent(t, line))
		}
	}
	return result
}

// clock returns a monotonic clock that advances by one microsecond at each reading, starting at start.
func clock(start uint64) func() (uint64, error) {
	var now atomic.Uint64
	now.Store(start)
	return func() (uint64, error) { return now.Add(1000), nil }
}

// startLog starts a writer with the bounds of the contract in a disposable folder. The policy file of its configuration folder
// stores every level of every class, so a test that is not about the policy sees each event that it emits; the default policy
// is started by startLogWithDefaultPolicy.
func startLog(t *testing.T, change func(*events.Options)) *events.Log {
	t.Helper()
	config := t.TempDir()
	file := filepath.Join(config, "log-policy.json")
	replace(t, file, policyText(nil))
	return startLogWithOptions(t, filepath.Join(config, "logs"), func(options *events.Options) {
		options.PolicyFile = file
		// The file holds a valid policy, so the writer reports nothing to this report.
		options.PolicyReport = io.Discard
		if change != nil {
			change(options)
		}
	})
}

// startLogWithDefaultPolicy starts a writer without a policy file, so the default policy is in force.
func startLogWithDefaultPolicy(t *testing.T, change func(*events.Options)) *events.Log {
	t.Helper()
	return startLogWithOptions(t, filepath.Join(t.TempDir(), "logs"), change)
}

func startLogWithOptions(t *testing.T, folder string, change func(*events.Options)) *events.Log {
	t.Helper()
	options := events.DefaultOptions(folder, "files", clock(9_000_000_000))
	// The writer of the sidecar always starts with the classes of its catalog, so the tests do too.
	classes, err := events.ClassesOfCatalog(catalog.Events)
	if err != nil {
		t.Fatal(err)
	}
	options.Classes = classes
	if change != nil {
		change(&options)
	}
	log, err := events.Start(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)
	return log
}

func finish(t *testing.T, log *events.Log) []loggedEvent {
	t.Helper()
	if err := log.Flush(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	// The levels of the debug channel are in the debug file; the order of the two files is the order of their seq.
	lines := readEvents(t, log.Path())
	if _, err := os.Stat(log.DebugPath()); err == nil {
		lines = append(lines, readEvents(t, log.DebugPath())...)
		lines = slices.SortedStableFunc(slices.Values(lines), func(a, b loggedEvent) int {
			return int(a.Seq) - int(b.Seq)
		})
	}
	return lines
}

// contract: log.writer.assigns-seq-and-opens-with-log-open
func TestTheFileOpensWithLogOpenAndTheNameFollowsTheContract(t *testing.T) {
	log := startLog(t, nil)
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"pid": 1, "platform": "x"})
	lines := finish(t, log)
	if matched, _ := regexp.MatchString(`^files-\d+-\d+\.jsonl$`, filepath.Base(log.Path())); !matched {
		t.Fatalf("the file is named %s", filepath.Base(log.Path()))
	}
	golden := fixtureLines(t, "key.jsonl")
	text, err := os.ReadFile(log.Path())
	if err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(string(text), "\n", 2)[0]
	if got, want := keysOf(t, first), keysOf(t, golden[0]); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("keys of log.open %v, golden %v", got, want)
	}
	open, goldenOpen := lines[0], decodeEvent(t, golden[0])
	if open.Event != "log.open" || open.Seq != 1 || open.Fields["schema"] != goldenOpen.Fields["schema"] || open.Fields["role"] != "files" || len(open.Fields) != len(goldenOpen.Fields) {
		t.Fatalf("log.open %+v, golden %+v", open, goldenOpen)
	}
	if open.Fields["pid"] != float64(os.Getpid()) || open.Fields["boot"] != strings.TrimSuffix(strings.TrimPrefix(filepath.Base(log.Path()), "files-"), ".jsonl") {
		t.Fatalf("log.open fields %v", open.Fields)
	}
	if lines[1].Seq != 2 || lines[1].Layer != "sidecar" {
		t.Fatalf("event %+v", lines[1])
	}
}

// contract: log.writer.debug-file
func TestTheLevelsOfTheDebugChannelGoToTheDebugFileWithOneSeqAcrossBoth(t *testing.T) {
	log := startLog(t, nil)
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"pid": 1, "platform": "x"})
	log.EmitHost(events.LevelDebug, "sidecar.files.watch_change", map[string]any{"surface": "s", "path": "a"})
	log.EmitHost(events.LevelWarn, "sidecar.files.watch_failed", map[string]any{"surface": "s", "path": "a", "error": "e"})
	log.EmitHost(events.LevelTrace, "an.unknown.trace", map[string]any{})
	if err := log.Flush(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log.DebugPath()); err != nil {
		t.Fatalf("the debug file holds no levels of the debug channel: %v", err)
	}
	lines := readEvents(t, log.Path())
	debugLines := readEvents(t, log.DebugPath())
	if debugLines[0].Event != "log.open" {
		t.Fatalf("the debug file opens with %s", debugLines[0].Event)
	}
	in := func(lines []loggedEvent, name string) *loggedEvent {
		for at := range lines {
			if lines[at].Event == name {
				return &lines[at]
			}
		}
		return nil
	}
	if in(lines, "sidecar.files.start") == nil || in(lines, "sidecar.files.watch_failed") == nil {
		t.Fatalf("the main file holds %v", lines)
	}
	if in(lines, "sidecar.files.watch_change") != nil || in(lines, "sidecar.files.start").Level == "debug" {
		t.Fatalf("the main file holds a level of the debug channel: %v", lines)
	}
	if in(debugLines, "sidecar.files.watch_change") == nil || in(debugLines, "an.unknown.trace") == nil {
		t.Fatalf("the debug file holds %v", debugLines)
	}
	if in(debugLines, "sidecar.files.start") != nil {
		t.Fatalf("the debug file holds a level of the main channel: %v", debugLines)
	}
	all := append(append([]loggedEvent{}, lines...), debugLines...)
	seen := map[uint64]bool{}
	for _, line := range all {
		if line.Seq == 0 || seen[line.Seq] {
			t.Fatalf("seq %d is zero or appears twice", line.Seq)
		}
		seen[line.Seq] = true
	}
	for at := uint64(1); at <= uint64(len(all)); at++ {
		if !seen[at] {
			t.Fatalf("seq %d of the two files is missing", at)
		}
	}
}

// contract: log.event.class-follows-the-catalog
func TestEveryLineCarriesTheClassThatTheCatalogDeclares(t *testing.T) {
	log := startLog(t, func(options *events.Options) {
		classes, err := events.ClassesOfCatalog(catalog.Events)
		if err != nil {
			t.Fatal(err)
		}
		options.Classes = classes
		if classes["sidecar.files.start"] != events.ClassLifecycle {
			t.Fatalf("the catalog gives sidecar.files.start the class %q", classes["sidecar.files.start"])
		}
	})
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"pid": 1, "platform": "x"})
	log.EmitHost(events.LevelInfo, "an.unknown.event", map[string]any{})
	lines := finish(t, log)
	for _, at := range []int{0, 1, 2} {
		if lines[at].Class != "lifecycle" {
			t.Fatalf("%s carries %q", lines[at].Event, lines[at].Class)
		}
	}
	if lines[3].Undeclared != true || lines[3].Class != "" {
		t.Fatalf("an event outside the catalog carries class %q and undeclared %v", lines[3].Class, lines[3].Undeclared)
	}
}

// contract: log.writer.output-follows-the-contract
func TestAnEventLineKeepsTheKeyOrderOfTheGoldenLine(t *testing.T) {
	log := startLog(t, nil)
	log.EmitHostCids(events.LevelInfo, "sidecar.files.request", []string{"b1.1"}, map[string]any{"hex": "0d"})
	if err := log.Flush(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(log.Path())
	if err != nil {
		t.Fatal(err)
	}
	own := keysOf(t, strings.Split(strings.TrimSpace(string(text)), "\n")[2])
	var kept []string
	for _, key := range keysOf(t, fixtureLines(t, "key.jsonl")[3]) {
		for _, name := range own {
			if key == name {
				kept = append(kept, key)
			}
		}
	}
	if strings.Join(kept, ",") != strings.Join(own, ",") || !strings.Contains(strings.Join(own, ","), "cids") {
		t.Fatalf("keys of an event %v, golden order %v", own, kept)
	}
}

// contract: log.writer.stores-both-clocks
func TestEveryLineCarriesTheWallClockInMicrosecondsAndTheMonotonicClock(t *testing.T) {
	log := startLog(t, nil)
	before := time.Now().UnixMicro()
	for i := 0; i < 50; i++ {
		log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"pid": i})
	}
	lines := finish(t, log)
	after := time.Now().UnixMicro()
	odd := false
	var previous uint64
	for _, line := range lines {
		if line.TsMicros < before-1_000_000 || line.TsMicros > after {
			t.Fatalf("ts_us %d is outside [%d, %d]", line.TsMicros, before, after)
		}
		if line.TsMicros%1000 != 0 {
			odd = true
		}
		// The injected clock advances by one microsecond at each reading, so the stored value is the reading.
		if line.MonoNs <= previous {
			t.Fatalf("mono_ns %d does not follow %d", line.MonoNs, previous)
		}
		previous = line.MonoNs
	}
	if !odd {
		t.Fatal("every ts_us ends in three zeros, so the clock has the resolution of a millisecond")
	}
	if lines[0].MonoNs != 9_000_001_000 {
		t.Fatalf("log.open mono_ns %d, the clock read 9000001000", lines[0].MonoNs)
	}
}

// contract: log.writer.stores-cids
func TestCidsAreStoredAndAnInvalidListIsRefusedAndCounted(t *testing.T) {
	log := startLog(t, nil)
	if !log.EmitHostCids(events.LevelInfo, "sidecar.files.request", []string{"b3.1042", "b3.1043"}, map[string]any{"a": 1}) {
		t.Fatal("a valid list was refused")
	}
	for _, bad := range [][]string{{""}, {`a"b`}, {`a\b`}, {"a\nb"}} {
		if log.EmitHostCids(events.LevelInfo, "sidecar.files.request", bad, map[string]any{"a": 1}) {
			t.Fatalf("the list %q was accepted", bad)
		}
	}
	lines := finish(t, log)
	if len(lines) < 4 || strings.Join(lines[2].Cids, ",") != "b3.1042,b3.1043" {
		t.Fatalf("lines %+v", lines)
	}
	dropped := lines[len(lines)-1]
	if dropped.Event != "log.dropped" || dropped.Fields["count"] != float64(4) {
		t.Fatalf("log.dropped %+v", dropped)
	}
}

// contract: log.writer.output-follows-the-contract
func TestFieldsAreEscapedAndAnOversizeEventIsCut(t *testing.T) {
	text := "quote \" backslash \\ newline \n tab \t unicode é 한"
	log := startLog(t, nil)
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"args": text})
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"args": strings.Repeat("a", 1<<20)})
	lines := finish(t, log)
	if len(lines) != 4 {
		t.Fatalf("%d lines", len(lines))
	}
	if lines[2].Fields["args"] != text {
		t.Fatalf("args %q", lines[2].Fields["args"])
	}
	head, _ := lines[3].Fields["head"].(string)
	if lines[3].Truncated <= 1<<20 || len(head) != 4096 || len(lines[3].Fields) != 1 {
		t.Fatalf("truncated %d, head %d bytes, %d fields", lines[3].Truncated, len(head), len(lines[3].Fields))
	}
}

// contract: log.writer.never-blocks-and-counts-loss
func TestARefusedEventIsCountedAndExplainedByLogDropped(t *testing.T) {
	log := startLog(t, nil)
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"name": "first"})
	// A value that JSON cannot carry is refused and counted in place of the event.
	if log.EmitHost(events.LevelInfo, "sidecar.files.start", make(chan int)) {
		t.Fatal("an event with fields that cannot be serialized was accepted")
	}
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"name": "last"})
	lines := finish(t, log)
	golden := decodeEvent(t, fixtureLines(t, "dropped.jsonl")[2])
	var dropped loggedEvent
	covered := map[uint64]bool{}
	for _, line := range lines {
		if covered[line.Seq] {
			t.Fatalf("seq %d appears twice", line.Seq)
		}
		covered[line.Seq] = true
		if line.Event == "log.dropped" {
			dropped = line
		}
	}
	for key := range golden.Fields {
		if _, ok := dropped.Fields[key]; !ok {
			t.Fatalf("log.dropped lacks the golden field %s: %+v", key, dropped)
		}
	}
	if dropped.Level != golden.Level || dropped.Fields["count"] != float64(1) || dropped.Fields["from_seq"] != dropped.Fields["to_seq"] {
		t.Fatalf("log.dropped = %+v", dropped)
	}
	covered[uint64(dropped.Fields["from_seq"].(float64))] = true
	for seq := uint64(1); seq <= uint64(len(covered)); seq++ {
		if !covered[seq] {
			t.Fatalf("seq %d is neither stored nor named by log.dropped", seq)
		}
	}
	if dropped.Seq <= uint64(dropped.Fields["to_seq"].(float64)) {
		t.Fatalf("log.dropped has seq %d before the end of its range %v", dropped.Seq, dropped.Fields["to_seq"])
	}
}

// The fields of log.dropped that the catalog declares are the fields of the core declaration in base-dropped.json.
func TestTheLogDroppedFieldsEqualTheCoreDeclaration(t *testing.T) {
	var catalogText struct {
		Events map[string]struct {
			Fields map[string]string `json:"fields"`
		} `json:"events"`
	}
	if err := json.Unmarshal(catalog.Events, &catalogText); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join("fixtures", "log-contract", "base-dropped.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(text, &golden); err != nil {
		t.Fatal(err)
	}
	declared := catalogText.Events["log.dropped"].Fields
	if len(declared) != len(golden.Fields) {
		t.Fatalf("events.json declares %v for log.dropped, the core declaration is %v", declared, golden.Fields)
	}
	for name, kind := range golden.Fields {
		if declared[name] != kind {
			t.Fatalf("events.json declares %q as %q, the core declaration is %q", name, declared[name], kind)
		}
	}
}

// A loss record states the largest queue depth at the refusals that it counts.
func TestALossRecordCarriesTheQueueDepthOfItsRefusals(t *testing.T) {
	log := startLog(t, func(options *events.Options) { options.QueueEvents = 4; options.BatchInterval = time.Hour })
	for i := 0; i < 4; i++ {
		if !log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": i}) {
			t.Fatalf("event %d was refused although the queue has room for it", i)
		}
	}
	if log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": 4}) {
		t.Fatal("an event over the queue bound was accepted")
	}
	lines := finish(t, log)
	var dropped *loggedEvent
	for at := range lines {
		if lines[at].Event == "log.dropped" {
			dropped = &lines[at]
		}
	}
	if dropped == nil {
		t.Fatalf("no log.dropped in %+v", lines)
	}
	if dropped.Fields["count"] != float64(1) || dropped.Fields["reason"] != "the queue was full" {
		t.Fatalf("log.dropped %+v", dropped.Fields)
	}
	if dropped.Fields["queued"] != float64(4) {
		t.Fatalf("log.dropped has queued %v, want the 4 events that waited in the queue", dropped.Fields["queued"])
	}
}

// Events that the writer has written no longer wait in the queue, so the queue accepts events again.
func TestTheQueueAcceptsEventsAgainAfterTheWriterWritesThem(t *testing.T) {
	log := startLog(t, func(options *events.Options) { options.QueueEvents = 4; options.BatchInterval = time.Hour })
	for i := 0; i < 4; i++ {
		if !log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": i}) {
			t.Fatalf("event %d was refused although the queue has room for it", i)
		}
	}
	if err := log.Flush(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	for i := 4; i < 8; i++ {
		if !log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": i}) {
			t.Fatalf("event %d was refused after the writer wrote the queue", i)
		}
	}
}

// contract: log.writer.never-blocks-and-counts-loss
func TestAFullQueueRefusesWithoutBlockingAndEveryEventIsStoredOrCounted(t *testing.T) {
	log := startLog(t, func(options *events.Options) { options.QueueEvents = 4; options.BatchInterval = time.Hour })
	const emitted = 5000
	accepted := 0
	for i := 0; i < emitted; i++ {
		if log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": i}) {
			accepted++
		}
	}
	if accepted == emitted || accepted == 0 {
		t.Fatalf("%d of %d events were accepted", accepted, emitted)
	}
	lines := finish(t, log)
	stored, counted := 0, 0
	for _, line := range lines {
		switch line.Event {
		case "sidecar.files.start":
			stored++
		case "log.dropped":
			counted += int(line.Fields["count"].(float64))
			if line.Fields["reason"] != "the queue was full" {
				t.Fatalf("reason %v", line.Fields["reason"])
			}
		}
	}
	if stored != accepted || stored+counted != emitted {
		t.Fatalf("emitted %d, accepted %d, stored %d, counted %d", emitted, accepted, stored, counted)
	}
}

// contract: log.writer.error-wakes-the-writer
func TestAnErrorEventIsWrittenWithoutWaitingForTheBatchInterval(t *testing.T) {
	log := startLog(t, func(options *events.Options) { options.BatchInterval = time.Hour })
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": 1})
	log.EmitHost(events.LevelError, "sidecar.files.failure", map[string]any{"error": "x"})
	deadline := time.Now().Add(10 * time.Second)
	for {
		text, err := os.ReadFile(log.Path())
		if err == nil && strings.Contains(string(text), "sidecar.files.failure") {
			if !strings.Contains(string(text), "sidecar.files.start") {
				t.Fatal("the error line was written without the event before it")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the error event was not written within 10 seconds: %q", text)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// contract: log.writer.files-are-private
func TestTheFileAndTheFolderArePrivate(t *testing.T) {
	log := startLog(t, nil)
	info, err := os.Stat(log.Path())
	if err != nil {
		t.Fatal(err)
	}
	folder, err := os.Stat(filepath.Dir(log.Path()))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 || folder.Mode().Perm() != 0o700 {
		t.Fatalf("file %v, folder %v", info.Mode().Perm(), folder.Mode().Perm())
	}
}

// contract: log.writer.flush-at-exit
func TestCloseWritesWhatIsQueued(t *testing.T) {
	log := startLog(t, func(options *events.Options) { options.BatchInterval = time.Hour })
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": 1})
	log.Close()
	lines := readEvents(t, log.Path())
	if len(lines) != 3 || lines[2].Event != "sidecar.files.start" {
		t.Fatalf("lines %+v", lines)
	}
	if log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": 2}) {
		t.Fatal("an event was accepted after the close")
	}
}

// contract: log.writer.rotates-at-the-bound-and-keeps-generations
func TestTheFileRotatesAtTheBoundAndKeepsTheGenerations(t *testing.T) {
	log := startLog(t, func(options *events.Options) { options.RotateBytes = 1500; options.Generations = 2 })
	for i := 0; i < 60; i++ {
		log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": i, "pad": strings.Repeat("p", 100)})
		if err := log.Flush(5 * time.Second); err != nil {
			t.Fatal(err)
		}
	}
	base := log.Path()
	if _, err := os.Stat(base + ".3"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the generation 3 exists beyond the 2 that are kept: %v", err)
	}
	var all []loggedEvent
	for _, path := range []string{base + ".2", base + ".1", base} {
		lines := readEvents(t, path)
		all = append(all, lines...)
		if lines[0].Event != "log.open" {
			t.Fatalf("%s starts with %s", path, lines[0].Event)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > 1500 {
			t.Fatalf("%s holds %d bytes, over the bound", path, info.Size())
		}
		if lines[1].Event != "log.rotated" || lines[1].Fields["kept"] != float64(2) {
			t.Fatalf("%s: second line %+v", path, lines[1])
		}
	}
	// seq continues across the generations, so the generations ordered by their first seq are consecutive.
	for index := 1; index < len(all); index++ {
		if all[index].Seq != all[index-1].Seq+1 {
			t.Fatalf("seq %d follows seq %d", all[index].Seq, all[index-1].Seq)
		}
	}
	rotated := readEvents(t, base)[1]
	if rotated.Event != "log.rotated" || rotated.Fields["removed"] == nil {
		t.Fatalf("the last rotation removed an older generation, found %+v", rotated)
	}
}

// contract: log.writer.refuses-an-unbounded-file
func TestAFailedWriteIsCountedAndExplainedByLogDropped(t *testing.T) {
	var failures atomic.Int32
	log := startLog(t, func(options *events.Options) {
		options.Write = func(file *os.File, data []byte) error {
			if failures.Add(-1) >= 0 {
				return fmt.Errorf("write failed on purpose")
			}
			_, err := file.Write(data)
			return err
		}
	})
	failures.Store(1)
	// The batch with log.open, the first log.policy and the first event fails.
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": 1})
	if err := log.Flush(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"i": 2})
	lines := finish(t, log)
	var dropped *loggedEvent
	for index := range lines {
		if lines[index].Event == "log.dropped" {
			dropped = &lines[index]
		}
	}
	if dropped == nil {
		t.Fatalf("no log.dropped in %+v", lines)
	}
	// The file holds no line of the failed batch, so it starts with a new log.open, which the log.dropped follows.
	if dropped.Fields["from_seq"] != float64(1) || dropped.Fields["to_seq"] != float64(3) || dropped.Fields["count"] != float64(3) || dropped.Seq != 5 {
		t.Fatalf("log.dropped %+v", dropped)
	}
	if lines[0].Event != "log.open" || lines[0].Seq != 4 {
		t.Fatalf("the file starts with %+v", lines[0])
	}
	if reason, _ := dropped.Fields["reason"].(string); !strings.Contains(reason, "write failed on purpose") {
		t.Fatalf("reason %q", reason)
	}
	if lines[len(lines)-1].Fields["i"] != float64(2) || lines[len(lines)-1].Seq != 6 {
		t.Fatalf("the event after the loss %+v", lines[len(lines)-1])
	}
}

func TestANilLogAcceptsNothingAndFailsNothing(t *testing.T) {
	var log *events.Log
	if log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{}) || log.EmitHostCids(events.LevelInfo, "sidecar.files.start", []string{"a"}, map[string]any{}) {
		t.Fatal("a nil log accepted an event")
	}
	if err := log.Flush(time.Second); err != nil {
		t.Fatal(err)
	}
	log.Close()
}

func TestALevelOutsideTheContractIsRefused(t *testing.T) {
	log := startLog(t, nil)
	if log.EmitHost(events.Level("fatal"), "sidecar.files.start", map[string]any{}) {
		t.Fatal("a level outside the contract was accepted")
	}
	lines := finish(t, log)
	if lines[len(lines)-1].Event != "log.dropped" {
		t.Fatalf("lines %+v", lines)
	}
}

// contract: log.result.error-is-a-code-and-a-text-cut-to-the-bound
func TestResultErrorIsTheCodeAndTheTextCutToTheBound(t *testing.T) {
	if got := events.ResultError(-32603, "refused"); got != "-32603: refused" {
		t.Fatalf("got %q", got)
	}
	long := events.ResultError(128, strings.Repeat("é", 2000))
	if len(long) > 1024 || !strings.HasPrefix(long, "128: é") || !utf8.ValidString(long) {
		t.Fatalf("got %d bytes, valid %v", len(long), utf8.ValidString(long))
	}
}
