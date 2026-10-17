package files_test

// Checks that the policy file is read when the writer starts and when it changes, through the file system events of the
// platform (docs/spec/logging.md, "Policy file"): the writer waits on an empty file, refuses a bad file once and keeps the
// policy, and a removal restores the default policy.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/soksak-app/sidecar-files/src/events"
	"github.com/soksak-app/sidecar-files/src/files"
	"github.com/soksak-app/sidecar-files/src/platform"
)

// lockedBuffer is the standard error of a watcher test: the watcher writes from the goroutine of the platform watch.
type lockedBuffer struct {
	mu   sync.Mutex
	text bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.String()
}

// watchedLog starts a writer in <config>/logs with the policy file <config>/log-policy.json, and returns the writer, the
// policy file, what the writer reported on its standard error, and the function that ends the watch.
func watchedLog(t *testing.T, initial string) (*events.Log, string, *lockedBuffer, func()) {
	t.Helper()
	// The files package registers the implementation of the operating system.
	_ = files.Serve
	current, err := platform.Current()
	if err != nil {
		t.Fatal(err)
	}
	config := t.TempDir()
	file := filepath.Join(config, "log-policy.json")
	if initial != "" {
		replace(t, file, initial)
	}
	report := &lockedBuffer{}
	classes := catalogClasses(t)
	log := startLogIn(t, filepath.Join(config, "logs"), func(options *events.Options) {
		options.Classes = classes
		options.PolicyFile = file
		options.PolicyReport = report
	})
	stop, err := log.WatchPolicy(current.Watch)
	if err != nil {
		t.Fatal(err)
	}
	ended := false
	end := func() {
		if ended {
			return
		}
		ended = true
		if err := stop(); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(end)
	return log, file, report, end
}

// startLogIn starts a writer in the folder.
func startLogIn(t *testing.T, folder string, change func(*events.Options)) *events.Log {
	t.Helper()
	options := events.DefaultOptions(folder, "files", clock(9_000_000_000))
	change(&options)
	log, err := events.Start(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)
	return log
}

// replace writes the file the way the host does: as a new file in the same folder that is renamed over the path.
func replace(t *testing.T, path, text string) {
	t.Helper()
	next := path + ".next"
	if err := os.WriteFile(next, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, path); err != nil {
		t.Fatal(err)
	}
}

func policyText(levels map[string]string) string {
	merged := map[string]string{"lifecycle": "trace", "input": "trace", "call": "trace", "io": "trace", "frame": "trace"}
	for class, level := range levels {
		merged[class] = level
	}
	text := `{"schema":1,"classes":{`
	for index, class := range []string{"lifecycle", "input", "call", "io", "frame"} {
		if index > 0 {
			text += ","
		}
		text += `"` + class + `":"` + merged[class] + `"`
	}
	return text + "}}"
}

// await reads the event file until the condition holds. The change of the file reaches the writer from the goroutine of the
// platform watch, and the event file is the only record of it, so the test reads that record until it shows the change.
func await(t *testing.T, log *events.Log, what string, condition func([]loggedEvent) bool) []loggedEvent {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		lines := finish(t, log)
		if condition(lines) {
			return lines
		}
		if time.Now().After(deadline) {
			t.Fatalf("waited 10 seconds for %s: %+v", what, lines)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func policies(lines []loggedEvent) []loggedEvent { return eventsNamed(lines, "log.policy") }

func sourcesOf(lines []loggedEvent) string {
	var sources []string
	for _, line := range policies(lines) {
		sources = append(sources, line.Fields["source"].(string))
	}
	return strings.Join(sources, ",")
}

// contract: log.policy.file-at-start-is-the-first-policy
func TestAPolicyFileThatExistsAtStartIsTheFirstPolicy(t *testing.T) {
	log, _, report, _ := watchedLog(t, policyText(map[string]string{"io": "info"}))
	lines := finish(t, log)
	if sourcesOf(lines) != "file" || policies(lines)[0].Seq != 2 {
		t.Fatalf("log.policy events %+v, want one from the file after log.open", policies(lines))
	}
	if log.Enabled("io", "debug") || !log.Enabled("io", "info") {
		t.Fatal("the policy of the file is not in force")
	}
	if report.String() != "" {
		t.Fatalf("standard error %q", report.String())
	}
}

// contract: log.policy.creation-and-removal
func TestACreatedFileChangesThePolicyAndItsRemovalRestoresTheDefault(t *testing.T) {
	log, file, _, _ := watchedLog(t, "")
	if sources := sourcesOf(finish(t, log)); sources != "default" {
		t.Fatalf("log.policy sources %q at start, want default", sources)
	}
	replace(t, file, policyText(map[string]string{"io": "info", "frame": "off"}))
	lines := await(t, log, "the policy of the created file", func(lines []loggedEvent) bool { return len(policies(lines)) == 2 })
	if sourcesOf(lines) != "default,file" || policies(lines)[1].Fields["classes"].(map[string]any)["io"] != "info" {
		t.Fatalf("log.policy events %+v", policies(lines))
	}
	if log.Enabled("io", "debug") {
		t.Fatal("the created file is not in force")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	lines = await(t, log, "the default policy after the removal", func(lines []loggedEvent) bool { return len(policies(lines)) == 3 })
	if sourcesOf(lines) != "default,file,default" || !log.Enabled("io", "error") || log.Enabled("io", "debug") || log.Enabled("frame", "trace") {
		t.Fatalf("log.policy events %+v", policies(lines))
	}
}

// contract: log.policy.write-in-place
func TestAWriteInPlaceAndAReplacementReachTheGate(t *testing.T) {
	log, file, _, _ := watchedLog(t, policyText(map[string]string{"io": "info"}))
	if err := os.WriteFile(file, []byte(policyText(map[string]string{"io": "warn"})), 0o600); err != nil {
		t.Fatal(err)
	}
	await(t, log, "the policy written in place", func(lines []loggedEvent) bool { return len(policies(lines)) == 2 })
	if log.Enabled("io", "info") || !log.Enabled("io", "warn") {
		t.Fatal("the write in place is not in force")
	}
	replace(t, file, policyText(map[string]string{"io": "error"}))
	await(t, log, "the replaced policy", func(lines []loggedEvent) bool { return len(policies(lines)) == 3 })
	if log.Enabled("io", "warn") || !log.Enabled("io", "error") {
		t.Fatal("the replacement is not in force")
	}
	// The file that replaced the first one is watched too: a write in place to it reaches the gate.
	if err := os.WriteFile(file, []byte(policyText(map[string]string{"io": "debug"})), 0o600); err != nil {
		t.Fatal(err)
	}
	await(t, log, "the write in place to the replacement", func(lines []loggedEvent) bool { return len(policies(lines)) == 4 })
	if !log.Enabled("io", "debug") {
		t.Fatal("the write in place to the replacement is not in force")
	}
}

// contract: log.policy.empty-file-waits-and-bad-file-is-refused-once
func TestAnEmptyFileWaitsAndABadFileIsRefusedOnceAndTheEarlierPolicyStays(t *testing.T) {
	log, file, report, _ := watchedLog(t, policyText(map[string]string{"io": "info"}))
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	bad := `{"schema":1,"classes":{"lifecycle":"trace","input":null,"call":"trace","io":"trace","frame":"trace"}}`
	replace(t, file, bad)
	want := "error: log policy: " + file + ": the level of the class input must be text, found null\n"
	await(t, log, "the refusal", func([]loggedEvent) bool { return report.String() != "" })
	// Another entry in the folder notifies the folder watch again; the same refused text is not reported again.
	if err := os.WriteFile(filepath.Join(filepath.Dir(file), "other"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	replace(t, file, policyText(map[string]string{"io": "warn"}))
	lines := await(t, log, "the next good policy", func(lines []loggedEvent) bool { return len(policies(lines)) == 2 })
	if report.String() != want {
		t.Fatalf("standard error %q, want %q", report.String(), want)
	}
	if sourcesOf(lines) != "file,file" || policies(lines)[1].Fields["classes"].(map[string]any)["io"] != "warn" {
		t.Fatalf("log.policy events %+v: the empty file and the bad file changed the policy", policies(lines))
	}
}

// contract: log.policy.refusal-of-the-file-at-start
func TestABadFileAtStartLeavesTheDefaultPolicyAndIsReported(t *testing.T) {
	log, file, report, _ := watchedLog(t, `{"schema":2,"classes":{}}`)
	lines := finish(t, log)
	if sourcesOf(lines) != "default" || log.Enabled("io", "warn") || !log.Enabled("io", "error") {
		t.Fatalf("log.policy events %+v", policies(lines))
	}
	if want := "error: log policy: " + file + ": schema 2 is not 1\n"; report.String() != want {
		t.Fatalf("standard error %q, want %q", report.String(), want)
	}
}

// contract: log.policy.watch-needs-a-file
func TestAWatchWithoutAPolicyFileIsRefused(t *testing.T) {
	log := startLogWithDefaultPolicy(t, nil)
	current, err := platform.Current()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.WatchPolicy(current.Watch); err == nil || !strings.Contains(err.Error(), "policy file") {
		t.Fatalf("error %v, want a refusal that names the policy file", err)
	}
}
