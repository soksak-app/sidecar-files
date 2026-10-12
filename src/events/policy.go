package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Class is the class of an event. The catalog declares the class of each event (docs/spec/logging.md#classes-and-policy).
type Class string

// The classes of the contract, in the order of the contract.
const (
	ClassLifecycle Class = "lifecycle"
	ClassInput     Class = "input"
	ClassCall      Class = "call"
	ClassIO        Class = "io"
	ClassFrame     Class = "frame"
)

var classOrder = []Class{ClassLifecycle, ClassInput, ClassCall, ClassIO, ClassFrame}

// levelOrder lists the values of a class from the one that stores nothing to the one that stores everything. The rank of a
// level is its index: an event is stored when its rank is not above the rank of the value of its class.
var levelOrder = []string{"off", "error", "warn", "info", "debug", "trace"}

func classIndex(class Class) int {
	for i, each := range classOrder {
		if each == class {
			return i
		}
	}
	return -1
}

func levelRank(value string) int {
	for i, each := range levelOrder {
		if each == value {
			return i
		}
	}
	return -1
}

// Policy is the least severe level that is stored for each class. The zero value stores nothing; DefaultPolicy stores every
// level of every class.
type Policy struct {
	ranks [5]int
}

// DefaultPolicy is the policy in force while there is no policy file: every class at `trace`.
func DefaultPolicy() Policy {
	var policy Policy
	for i := range policy.ranks {
		policy.ranks[i] = levelRank("trace")
	}
	return policy
}

// Allows reports whether the policy stores an event of the class at the level. A class or a level outside the contract is
// stored: such an event is refused elsewhere, and a gate does not hide it.
func (p Policy) Allows(class Class, level Level) bool {
	index, rank := classIndex(class), levelRank(string(level))
	if index < 0 || rank < 1 {
		return true
	}
	return rank <= p.ranks[index]
}

// Level is the value of the class: off, error, warn, info, debug or trace.
func (p Policy) Level(class Class) string {
	index := classIndex(class)
	if index < 0 {
		return ""
	}
	return levelOrder[p.ranks[index]]
}

// classes is the value of every class as the fields of log.policy carry it.
func (p Policy) classes() map[string]string {
	result := make(map[string]string, len(classOrder))
	for i, class := range classOrder {
		result[string(class)] = levelOrder[p.ranks[i]]
	}
	return result
}

// PolicyBeingWritten reports whether the text is empty or holds only white space: a writer truncated the file and is writing
// it, so a reader waits for the next notification and reports nothing (docs/spec/logging.md#policy-file).
func PolicyBeingWritten(text []byte) bool {
	return len(bytes.TrimSpace(text)) == 0
}

// ParsePolicy reads the text of the policy file `file` with the rules of docs/spec/logging.md#policy-file. A refusal names the
// file and what was found, in the order of the contract.
func ParsePolicy(file string, text []byte) (Policy, error) {
	fail := func(format string, args ...any) (Policy, error) {
		return Policy{}, fmt.Errorf("%s: "+format, append([]any{file}, args...)...)
	}
	trimmed := bytes.TrimSpace(text)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		message := "the text is not a JSON object"
		var value json.RawMessage
		if err := json.Unmarshal(text, &value); err != nil {
			message = err.Error()
		}
		return fail("not JSON: %s", message)
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(text, &members); err != nil {
		return fail("not JSON: %s", err.Error())
	}
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name != "schema" && name != "classes" {
			return fail("the member %s is not one of schema, classes", name)
		}
	}
	schema, present := members["schema"]
	number, isNumber := 0.0, false
	if present {
		if parsed, err := strconv.ParseFloat(string(bytes.TrimSpace(schema)), 64); err == nil && !math.IsInf(parsed, 0) {
			number, isNumber = parsed, true
		}
	}
	if !isNumber || number != math.Trunc(number) {
		return fail("schema must be the number 1, found %s", compact(schema))
	}
	if number != 1 {
		return fail("schema %s is not 1", strconv.FormatFloat(number, 'f', -1, 64))
	}
	var levels map[string]json.RawMessage
	if raw, ok := members["classes"]; !ok || json.Unmarshal(raw, &levels) != nil || levels == nil {
		return fail("classes must be an object that names every class")
	}
	given := make([]string, 0, len(levels))
	for name := range levels {
		given = append(given, name)
	}
	sort.Strings(given)
	for _, name := range given {
		if classIndex(Class(name)) < 0 {
			return fail("the class %s is not one of lifecycle, input, call, io, frame", name)
		}
	}
	for _, class := range classOrder {
		if _, ok := levels[string(class)]; !ok {
			return fail("the class %s is missing", class)
		}
	}
	var policy Policy
	for i, class := range classOrder {
		raw := levels[string(class)]
		var value string
		// A JSON null leaves a string unchanged without an error, so the first byte decides whether the level is text.
		if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte(`"`)) || json.Unmarshal(raw, &value) != nil {
			return fail("the level of the class %s must be text, found %s", class, compact(raw))
		}
		rank := levelRank(value)
		if rank < 0 {
			return fail("the level %q of the class %s is not one of %s", value, class, strings.Join(levelOrder, ", "))
		}
		policy.ranks[i] = rank
	}
	return policy, nil
}

// compact writes a JSON value on one line without white space; a missing value is empty.
func compact(raw json.RawMessage) string {
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return string(raw)
	}
	return out.String()
}

// ClassesOfCatalog reads the class of each event from the catalog `events.json`. An event whose class is not one of the five is
// a defect of the catalog and is refused.
func ClassesOfCatalog(catalog []byte) (map[string]Class, error) {
	var parsed struct {
		Events map[string]struct {
			Class string `json:"class"`
		} `json:"events"`
	}
	if err := json.Unmarshal(catalog, &parsed); err != nil {
		return nil, fmt.Errorf("the event catalog is not JSON: %v", err)
	}
	classes := make(map[string]Class, len(parsed.Events))
	for name, event := range parsed.Events {
		if classIndex(Class(event.Class)) < 0 {
			return nil, fmt.Errorf("the event catalog gives %s the class %q, which is not one of lifecycle, input, call, io, frame", name, event.Class)
		}
		classes[name] = Class(event.Class)
	}
	return classes, nil
}

// gatedCount is one report of log.gated.
type gatedCount struct {
	class Class
	level Level
	count uint64
}

var levelsInOrder = []Level{LevelError, LevelWarn, LevelInfo, LevelDebug, LevelTrace}

func levelIndex(level Level) int {
	for i, each := range levelsInOrder {
		if each == level {
			return i
		}
	}
	return -1
}

// ApplyPolicy puts the policy in force. The first policy that a writer applies and each change of it is stored as log.policy
// (`source` is `default` or `file`); the same policy again stores nothing. The event is stored with the next batch.
func (l *Log) ApplyPolicy(policy Policy, source string) {
	if l == nil {
		// silent: a nil log is a sidecar started without a log folder, and it has no policy to apply.
		return
	}
	l.policyMu.Lock()
	defer l.policyMu.Unlock()
	if l.policyApplied && l.policy.Load().ranks == policy.ranks {
		return
	}
	l.policyApplied = true
	l.policy.Store(&policy)
	fields := map[string]any{"source": source, "classes": policy.classes()}
	text, err := json.Marshal(fields)
	if err != nil {
		// A map of strings serializes without failure, so this is a defect of the serializer.
		fmt.Fprintf(os.Stderr, "event writer: log.policy cannot be serialized: %v\n", err)
		return
	}
	reported := &pendingEvent{tsMicros: time.Now().UnixMicro(), monoNs: l.monotonicNanos(), level: LevelInfo, event: "log.policy", fields: text}
	l.classify(reported)
	l.policyEvents = append(l.policyEvents, reported)
}

// takePolicyEvents returns the log.policy events that wait for the writer.
func (l *Log) takePolicyEvents() []*pendingEvent {
	l.policyMu.Lock()
	defer l.policyMu.Unlock()
	taken := l.policyEvents
	l.policyEvents = nil
	return taken
}

// Enabled reports whether the policy in force stores an event of the class at the level. It does not count.
func (l *Log) Enabled(class Class, level Level) bool {
	if l == nil || l.closed.Load() {
		return false
	}
	return l.policy.Load().Allows(class, level)
}

// Gate is Enabled for a producer that skips the building of an event: a refusal is counted for the class and the level and
// leaves as log.gated.
func (l *Log) Gate(class Class, level Level) bool {
	if l == nil || l.closed.Load() {
		return false
	}
	if l.policy.Load().Allows(class, level) {
		return true
	}
	l.countGated(class, level)
	return false
}

func (l *Log) countGated(class Class, level Level) {
	if i, j := classIndex(class), levelIndex(level); i >= 0 && j >= 0 {
		l.gated[i][j].Add(1)
	}
}

// takeGated returns the refusals since the earlier report and resets them.
func (l *Log) takeGated() []gatedCount {
	var counts []gatedCount
	for i, class := range classOrder {
		for j, level := range levelsInOrder {
			if count := l.gated[i][j].Swap(0); count > 0 {
				counts = append(counts, gatedCount{class: class, level: level, count: count})
			}
		}
	}
	return counts
}

// gatedEvent is the log.gated event of one count.
func (l *Log) gatedEvent(count gatedCount) *pendingEvent {
	fields := fmt.Sprintf(`{"class":%q,"level":%q,"count":%d}`, string(count.class), string(count.level), count.count)
	gated := &pendingEvent{tsMicros: time.Now().UnixMicro(), monoNs: l.monotonicNanos(), level: LevelInfo, event: "log.gated", fields: json.RawMessage(fields)}
	l.classify(gated)
	return gated
}

// gate reports whether the event is stored. An event that the catalog does not declare has no class and no gate.
func (l *Log) gate(event string, level Level) bool {
	class, declared := l.options.Classes[event]
	if !declared {
		return true
	}
	return l.Gate(class, level)
}
