package providers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Regression (SES953): codex finished its turn and exited, but a process it had
// spawned — a Gradle daemon started by a tool call — outlived it holding the
// inherited stdout pipe. The read loop therefore never saw EOF and the turn hung
// forever, past the idle watchdog, because cancellation was only observable
// through the pipe closing. runAttempt must return when its context is cancelled
// no matter who still holds the pipe.
func TestCodexRunAttemptReturnsOnCancelWhileGrandchildHoldsPipe(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "" {
		return // helper re-exec; see the helper tests below
	}

	self, err := os.Executable()
	if err != nil {
		t.Skipf("no test binary path: %v", err)
	}
	t.Setenv("CODEX_TEST_SELF", self)
	grandchildPIDFile := filepath.Join(t.TempDir(), "grandchild.pid")
	t.Setenv("CODEX_TEST_GRANDCHILD_PID_FILE", grandchildPIDFile)
	t.Cleanup(func() {
		data, err := os.ReadFile(grandchildPIDFile)
		if err != nil {
			t.Errorf("read grandchild PID for cleanup: %v", err)
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			t.Errorf("parse grandchild PID for cleanup: %v", err)
			return
		}
		if runtime.GOOS == "windows" {
			cleanup := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
			if output, err := cleanup.CombinedOutput(); err != nil {
				t.Errorf("kill leaked test grandchild %d: %v: %s", pid, err, strings.TrimSpace(string(output)))
			}
			return
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			t.Errorf("find leaked test grandchild %d: %v", pid, err)
			return
		}
		if err := process.Kill(); err != nil {
			t.Errorf("kill leaked test grandchild %d: %v", pid, err)
		}
	})

	c := &CodexCLI{binPath: self}
	args := []string{"-test.run=TestCodexHelperExitsLeavingChild", "-test.v=false"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The helper exits within a second; cancel shortly after, standing in for the
	// idle watchdog firing on a turn whose process is already gone.
	go func() {
		time.Sleep(3 * time.Second)
		cancel()
	}()

	done := make(chan struct{})
	go func() {
		_, _, _ = c.runAttempt(ctx, args, "prompt", "gpt-test", Request{}, "")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(45 * time.Second):
		t.Fatal("runAttempt did not return after context cancellation: the read loop is still waiting on a pipe a grandchild holds open")
	}
}

// TestCodexHelperExitsLeavingChild is not a real test: it is the fake codex
// binary the regression above runs. It emits one line so the startup guard is
// satisfied, hands stdout to a grandchild that keeps running, then exits.
func TestCodexHelperExitsLeavingChild(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "" {
		t.Skip("helper: only runs under the re-exec in TestCodexRunAttemptReturnsOnCancelWhileGrandchildHoldsPipe")
	}
	self := os.Getenv("CODEX_TEST_SELF")
	if self == "" {
		t.Skip("helper: only runs under the re-exec in TestCodexRunAttemptReturnsOnCancelWhileGrandchildHoldsPipe")
	}
	child := exec.Command(self, "-test.run=TestCodexHelperSleeps", "-test.v=false")
	child.Env = append(os.Environ(), "CODEX_TEST_HELPER=sleep")
	child.Stdout = os.Stdout // inherit the pipe — this is what wedges the reader
	if err := child.Start(); err != nil {
		t.Fatalf("helper: start grandchild: %v", err)
	}
	pidFile := os.Getenv("CODEX_TEST_GRANDCHILD_PID_FILE")
	if pidFile == "" {
		t.Fatal("helper: CODEX_TEST_GRANDCHILD_PID_FILE is empty")
	}
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
		_ = child.Process.Kill()
		t.Fatalf("helper: write grandchild PID: %v", err)
	}
	fmt.Println(`{"type":"thread.started"}`)
	// Deliberately no Wait: the grandchild outlives this process.
}

// TestCodexHelperSleeps is the grandchild body — see the helper above.
func TestCodexHelperSleeps(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "sleep" {
		t.Skip("helper: only runs under the re-exec in TestCodexRunAttemptReturnsOnCancelWhileGrandchildHoldsPipe")
	}
	time.Sleep(60 * time.Second)
}
