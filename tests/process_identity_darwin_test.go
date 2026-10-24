package files_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const filesDisplayName = "soksak://sidecar/files"

func launchServicesName(pid int) (string, bool, error) {
	command := exec.Command("/usr/bin/lsappinfo", "list")
	output, err := command.Output()
	if err != nil {
		return "", false, fmt.Errorf("lsappinfo list: %w", err)
	}
	lines := strings.Split(string(output), "\n")
	needle := fmt.Sprintf("pid = %d ", pid)
	for index, line := range lines {
		if !strings.Contains(line, needle) {
			continue
		}
		for previous := index; previous >= 0; previous-- {
			if strings.Contains(lines[previous], " ASN:") {
				parts := strings.SplitN(lines[previous], ") \"", 2)
				if len(parts) != 2 {
					return "", false, fmt.Errorf("lsappinfo ASN line has no quoted name: %q", lines[previous])
				}
				name := strings.SplitN(parts[1], "\" ASN:", 2)
				if len(name) != 2 {
					return "", false, fmt.Errorf("lsappinfo ASN line has no display name: %q", lines[previous])
				}
				return name[0], true, nil
			}
		}
		return "", false, fmt.Errorf("lsappinfo has no ASN entry for pid %d", pid)
	}
	return "", false, nil
}

func TestLiveProcessUsesItsDeclaredLaunchServicesDisplayName(t *testing.T) {
	testLiveProcessDisplayName(t, environmentWithoutDisplayName(os.Environ()))
}

func TestLiveProcessIgnoresHostDisplayNameEnvironment(t *testing.T) {
	environment := environmentWithoutDisplayName(os.Environ())
	environment = append(environment, "SOKSAK_SIDECAR_DISPLAY_NAME=soksak://sidecar/incorrect")
	testLiveProcessDisplayName(t, environment)
}

func testLiveProcessDisplayName(t *testing.T, environment []string) {
	t.Helper()
	command := exec.Command(sidecarBinary(t))
	command.Env = environment
	command.Stdout = io.Discard
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	ended := false
	defer func() {
		if !ended {
			if err := command.Process.Kill(); err != nil {
				t.Errorf("kill sidecar pid %d: %v", command.Process.Pid, err)
			}
			if err := command.Wait(); err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != -1 {
					t.Errorf("wait after kill: %v", err)
				}
			}
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	var name string
	for time.Now().Before(deadline) {
		var found bool
		name, found, err = launchServicesName(command.Process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if name != filesDisplayName {
		t.Fatalf("LaunchServices name for pid %d is %q, want %q; stderr %q", command.Process.Pid, name, filesDisplayName, stderr.String())
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	ended = true
}
