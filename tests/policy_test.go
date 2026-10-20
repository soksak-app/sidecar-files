package files_test

// Checks the policy of the event file against the policy contract of the soksak core specification (docs/spec/logging.md,
// "Classes and policy" and "Policy file"). tests/fixtures/policy/cases.json is owned by core
// (packages/log-contract/fixtures/policy/cases.json) and is copied unchanged; the reader of this repository must give the result
// of each case.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/soksak-app/sidecar-files/src/events"
)

type policyCases struct {
	File    string            `json:"file"`
	Classes []string          `json:"classes"`
	Default map[string]string `json:"default"`
	Values  []string          `json:"values"`
	Valid   []struct {
		Name    string            `json:"name"`
		Text    string            `json:"text"`
		Classes map[string]string `json:"classes"`
		Allows  [][2]string       `json:"allows"`
		Refuses [][2]string       `json:"refuses"`
	} `json:"valid"`
	Invalid []struct {
		Name        string `json:"name"`
		Text        string `json:"text"`
		Error       string `json:"error"`
		ErrorPrefix string `json:"error_prefix"`
	} `json:"invalid"`
	Unchanged []struct {
		Name string `json:"name"`
		Text string `json:"text"`
	} `json:"unchanged_by_watcher"`
}

func loadPolicyCases(t *testing.T) policyCases {
	t.Helper()
	text, err := os.ReadFile(filepath.Join("fixtures", "policy", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases policyCases
	if err := json.Unmarshal(text, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}

// catalogClasses reads the class of each event from the catalog that the manifest names.
func catalogClasses(t *testing.T) map[string]events.Class {
	t.Helper()
	text, err := os.ReadFile(filepath.Join("..", "events.json"))
	if err != nil {
		t.Fatal(err)
	}
	classes, err := events.ClassesOfCatalog(text)
	if err != nil {
		t.Fatal(err)
	}
	return classes
}

// contract: log.policy.reader-gives-the-golden-result
func TestThePolicyReaderGivesTheResultOfEveryGoldenCase(t *testing.T) {
	cases := loadPolicyCases(t)
	file := "/config/" + cases.File
	for _, valid := range cases.Valid {
		policy, err := events.ParsePolicy(file, []byte(valid.Text))
		if err != nil {
			t.Fatalf("%s: %v", valid.Name, err)
		}
		for class, level := range valid.Classes {
			if got := policy.Level(events.Class(class)); got != level {
				t.Fatalf("%s: the class %s is %q, want %q", valid.Name, class, got, level)
			}
		}
		for _, pair := range valid.Allows {
			if !policy.Allows(events.Class(pair[0]), events.Level(pair[1])) {
				t.Fatalf("%s: the policy refuses %v", valid.Name, pair)
			}
		}
		for _, pair := range valid.Refuses {
			if policy.Allows(events.Class(pair[0]), events.Level(pair[1])) {
				t.Fatalf("%s: the policy allows %v", valid.Name, pair)
			}
		}
		if events.PolicyBeingWritten([]byte(valid.Text)) {
			t.Fatalf("%s: a complete policy is not a file that is being written", valid.Name)
		}
	}
	for _, invalid := range cases.Invalid {
		_, err := events.ParsePolicy(file, []byte(invalid.Text))
		if err == nil {
			t.Fatalf("%s: the text was accepted", invalid.Name)
		}
		if invalid.ErrorPrefix != "" {
			if want := strings.ReplaceAll(invalid.ErrorPrefix, "<file>", file); !strings.HasPrefix(err.Error(), want) {
				t.Fatalf("%s: error %q does not start with %q", invalid.Name, err, want)
			}
			continue
		}
		if want := strings.ReplaceAll(invalid.Error, "<file>", file); err.Error() != want {
			t.Fatalf("%s: error %q, want %q", invalid.Name, err, want)
		}
	}
	for _, unchanged := range cases.Unchanged {
		if !events.PolicyBeingWritten([]byte(unchanged.Text)) {
			t.Fatalf("%s: the reader does not wait on %q", unchanged.Name, unchanged.Text)
		}
	}
}

// contract: log.policy.default-stores-only-errors
func TestTheDefaultPolicyStoresOnlyErrorsOfEveryClass(t *testing.T) {
	cases := loadPolicyCases(t)
	policy := events.DefaultPolicy()
	for _, class := range cases.Classes {
		if got := policy.Level(events.Class(class)); got != cases.Default[class] {
			t.Fatalf("the default policy holds %s at %q, want %q", class, got, cases.Default[class])
		}
		if !policy.Allows(events.Class(class), events.LevelError) {
			t.Fatalf("the default policy refuses %s error", class)
		}
		for _, level := range []events.Level{events.LevelWarn, events.LevelInfo, events.LevelDebug, events.LevelTrace} {
			if policy.Allows(events.Class(class), level) {
				t.Fatalf("the default policy stores %s %s", class, level)
			}
		}
	}
}

// contract: log.policy.default-without-a-file
func TestWithoutAPolicyFileOnlyErrorsAreStoredOfEveryClass(t *testing.T) {
	cases := loadPolicyCases(t)
	log := startGatedLog(t)
	for _, class := range cases.Classes {
		if !log.Enabled(events.Class(class), events.LevelError) {
			t.Fatalf("the writer without a policy file refuses %s error", class)
		}
		for _, level := range []events.Level{events.LevelWarn, events.LevelInfo, events.LevelDebug, events.LevelTrace} {
			if log.Enabled(events.Class(class), level) {
				t.Fatalf("the writer without a policy file stores %s %s", class, level)
			}
		}
	}
	policies := eventsNamed(finish(t, log), "log.policy")
	if len(policies) != 1 || policies[0].Fields["source"] != "default" {
		t.Fatalf("log.policy events %+v, want the default policy once", policies)
	}
	classes, _ := policies[0].Fields["classes"].(map[string]any)
	for _, class := range cases.Classes {
		if classes[class] != cases.Default[class] {
			t.Fatalf("log.policy holds %s at %v, want %q", class, classes[class], cases.Default[class])
		}
	}
}

// contract: log.policy.catalog-gives-the-class
func TestTheCatalogGivesTheClassOfEachDeclaredEvent(t *testing.T) {
	classes := catalogClasses(t)
	for name, want := range map[string]events.Class{
		"sidecar.files.request": "call", "sidecar.files.watch_change": "io", "sidecar.files.start": "lifecycle",
		"log.policy": "lifecycle", "log.gated": "lifecycle",
	} {
		if classes[name] != want {
			t.Fatalf("the class of %s is %q, want %q", name, classes[name], want)
		}
	}
}

func mustPolicy(t *testing.T, levels map[string]string) events.Policy {
	t.Helper()
	merged := map[string]string{"lifecycle": "trace", "input": "trace", "call": "trace", "io": "trace", "frame": "trace"}
	for class, level := range levels {
		merged[class] = level
	}
	text, err := json.Marshal(map[string]any{"schema": 1, "classes": merged})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := events.ParsePolicy("log-policy.json", text)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func startGatedLog(t *testing.T) *events.Log {
	t.Helper()
	classes := catalogClasses(t)
	return startLogWithDefaultPolicy(t, func(options *events.Options) { options.Classes = classes })
}

// contract: log.policy.change-is-an-event
func TestTheFirstPolicyAndEachChangeAreTheEventLogPolicyAndTheSamePolicyStoresNothing(t *testing.T) {
	log := startGatedLog(t)
	narrow := mustPolicy(t, map[string]string{"io": "info", "frame": "off"})
	log.ApplyPolicy(narrow, "file")
	log.ApplyPolicy(narrow, "file")
	log.ApplyPolicy(events.DefaultPolicy(), "default")
	lines := eventsNamed(finish(t, log), "log.policy")
	if len(lines) != 3 {
		t.Fatalf("log.policy events %+v, want the default, the file and the default again", lines)
	}
	for _, line := range lines {
		if line.Level != "info" || line.Layer != "sidecar" {
			t.Fatalf("log.policy %+v", line)
		}
	}
	if lines[0].Fields["source"] != "default" || lines[1].Fields["source"] != "file" || lines[2].Fields["source"] != "default" {
		t.Fatalf("sources %v %v %v", lines[0].Fields["source"], lines[1].Fields["source"], lines[2].Fields["source"])
	}
	classes, _ := lines[1].Fields["classes"].(map[string]any)
	if len(classes) != 5 || classes["io"] != "info" || classes["frame"] != "off" || classes["lifecycle"] != "trace" {
		t.Fatalf("classes of the file policy %v", classes)
	}
}

// contract: log.policy.closed-gate-builds-no-fields-and-counts
func TestAClosedGateBuildsNoFieldsAndLogGatedCountsTheExactNumber(t *testing.T) {
	log := startGatedLog(t)
	log.ApplyPolicy(mustPolicy(t, map[string]string{"io": "info"}), "file")
	var built atomic.Int64
	build := func() map[string]any {
		built.Add(1)
		return map[string]any{"surface": "s", "path": "p"}
	}
	for i := 0; i < 3; i++ {
		log.EmitHost(events.LevelDebug, "sidecar.files.watch_change", build)
	}
	log.EmitHost(events.LevelInfo, "sidecar.files.watch_change", build)
	if built.Load() != 1 {
		t.Fatalf("the builder ran %d times, want only for the event the policy stores", built.Load())
	}
	first := eventsNamed(finish(t, log), "log.gated")
	if len(first) != 1 || first[0].Level != "info" || first[0].Fields["class"] != "io" || first[0].Fields["level"] != "debug" || first[0].Fields["count"] != float64(3) {
		t.Fatalf("log.gated %+v, want io debug 3", first)
	}
	log.EmitHost(events.LevelDebug, "sidecar.files.watch_change", build)
	log.EmitHost(events.LevelTrace, "sidecar.files.watch_change", build)
	second := eventsNamed(finish(t, log), "log.gated")
	if len(second) != 3 || second[1].Fields["count"] != float64(1) || second[2].Fields["count"] != float64(1) {
		t.Fatalf("log.gated after the second report %+v, want the earlier one and one for each of debug and trace", second)
	}
	third := eventsNamed(finish(t, log), "log.gated")
	if len(third) != 3 {
		t.Fatalf("a report without a refusal stored log.gated again: %+v", third)
	}
}

// contract: log.policy.off-stores-no-event-of-the-class
func TestOffStoresNoEventOfTheClassAndTheEventsOfTheWriterPassEveryGate(t *testing.T) {
	log := startGatedLog(t)
	log.ApplyPolicy(mustPolicy(t, map[string]string{"lifecycle": "off", "call": "off", "io": "off", "input": "off", "frame": "off"}), "file")
	log.EmitHost(events.LevelInfo, "sidecar.files.start", map[string]any{"pid": 1, "goos": "x", "goarch": "y"})
	log.EmitHost(events.LevelError, "sidecar.files.failure", map[string]any{"error": "e"})
	log.EmitHostCids(events.LevelInfo, "sidecar.files.request", []string{"b.1"}, map[string]any{"surface": "s"})
	lines := finish(t, log)
	if len(eventsNamed(lines, "sidecar.files.start"))+len(eventsNamed(lines, "sidecar.files.failure"))+len(eventsNamed(lines, "sidecar.files.request")) != 0 {
		t.Fatalf("a class that is off stored events: %+v", lines)
	}
	if len(eventsNamed(lines, "log.policy")) != 2 || len(eventsNamed(lines, "log.open")) != 1 {
		t.Fatalf("the events of the writer did not pass the gate: %+v", lines)
	}
	counts := map[string]float64{}
	for _, gated := range eventsNamed(lines, "log.gated") {
		counts[gated.Fields["class"].(string)+" "+gated.Fields["level"].(string)] = gated.Fields["count"].(float64)
	}
	if counts["lifecycle info"] != 1 || counts["lifecycle error"] != 1 || counts["call info"] != 1 {
		t.Fatalf("counts %v", counts)
	}
}

// contract: log.policy.undeclared-event-has-no-gate
func TestAnEventTheCatalogDoesNotDeclareHasNoClassAndNoGate(t *testing.T) {
	log := startGatedLog(t)
	log.ApplyPolicy(mustPolicy(t, map[string]string{"lifecycle": "off", "call": "off", "io": "off", "input": "off", "frame": "off"}), "file")
	log.EmitHost(events.LevelInfo, "sidecar.files.undeclared", map[string]any{"x": 1})
	if len(eventsNamed(finish(t, log), "sidecar.files.undeclared")) != 1 {
		t.Fatal("an undeclared event was gated")
	}
}

// contract: log.policy.gate-answers-without-building
func TestTheGateAnswersAndCountsForAProducerThatSkipsTheBuilding(t *testing.T) {
	log := startGatedLog(t)
	log.ApplyPolicy(mustPolicy(t, map[string]string{"io": "warn"}), "file")
	if !log.Enabled("io", "warn") || log.Enabled("io", "info") {
		t.Fatal("Enabled does not follow the policy")
	}
	if !log.Gate("io", "error") || log.Gate("io", "debug") {
		t.Fatal("Gate does not follow the policy")
	}
	lines := finish(t, log)
	gated := eventsNamed(lines, "log.gated")
	if len(gated) != 1 || gated[0].Fields["count"] != float64(1) || gated[0].Fields["level"] != "debug" {
		t.Fatalf("Gate counted %+v, want one refusal at debug and none for Enabled", gated)
	}
	var nilLog *events.Log
	if nilLog.Enabled("io", "error") || nilLog.Gate("io", "error") {
		t.Fatal("a nil log stores nothing")
	}
}
