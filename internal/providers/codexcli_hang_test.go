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

func TestCodexRunAttemptIdleOutputTimeoutWhileGrandchildHoldsPipe(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "" {
		return
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

	originalTimeout := codexIdleOutputWindow()
	const window = 100 * time.Millisecond
	SetCodexIdleOutputTimeout(window)
	t.Cleanup(func() { SetCodexIdleOutputTimeout(originalTimeout) })

	var kills []WatchdogKill
	req := Request{OnWatchdog: func(k WatchdogKill) { kills = append(kills, k) }}

	c := &CodexCLI{binPath: self}
	args := []string{"-test.run=TestCodexHelperExitsLeavingChild", "-test.v=false"}
	_, retryable, err := c.runAttempt(context.Background(), args, "prompt", "gpt-test", req, "")
	if err == nil {
		t.Fatal("runAttempt returned no idle timeout error")
	}
	// The helper ran no tool before stalling, so the attempt had no side effects
	// and re-running it once is safe.
	if !retryable {
		t.Fatal("idle timeout error with no tool executed was not retryable")
	}
	if !isCodexIdleHang(err) {
		t.Fatalf("idle timeout error is not classified as an idle hang: %v", err)
	}
	if message := err.Error(); !strings.Contains(message, window.String()) || !strings.Contains(message, "idle-test-output") {
		t.Fatalf("idle timeout error lacks timeout or output tail: %v", err)
	}
	if len(kills) != 1 {
		t.Fatalf("watchdog kills reported = %d, want 1", len(kills))
	}
	if kills[0].Reason != WatchdogReasonIdle || kills[0].Window != window {
		t.Fatalf("watchdog kill = %+v, want idle/%s", kills[0], window)
	}
	if !strings.Contains(kills[0].Detail, "idle-test-output") {
		t.Fatalf("watchdog kill detail lacks the stdout tail: %q", kills[0].Detail)
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
	fmt.Println("idle-test-output")
	// Deliberately no Wait: the grandchild outlives this process.
}

// TestCodexHelperSleeps is the grandchild body — see the helper above.
func TestCodexHelperSleeps(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "sleep" {
		t.Skip("helper: only runs under the re-exec in TestCodexRunAttemptReturnsOnCancelWhileGrandchildHoldsPipe")
	}
	time.Sleep(60 * time.Second)
}

// Regression (SES2570): codex announced a native context_compaction and then
// went silent for the whole compaction model call. The stdout-silence watchdog
// read that as a hang and killed a turn that was making progress. It must now
// forgive codexCompactionIdleGrace windows first.
func TestCodexIdleWatchdogWaitsOutNativeCompaction(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "" {
		return
	}
	const window = 150 * time.Millisecond
	err, elapsed, retryable := runCodexStallHelper(t, window,
		`{"type":"item.started","item":{"id":"c1","type":"context_compaction"}}`)
	if err == nil {
		t.Fatal("runAttempt returned no idle timeout error")
	}
	// grace windows + the final one that actually kills.
	if min := time.Duration(codexCompactionIdleGrace+1) * window; elapsed < min-20*time.Millisecond {
		t.Fatalf("killed after %s, want at least %s: the compaction grace window did not apply", elapsed, min)
	}
	if !strings.Contains(err.Error(), "compaction grace window") {
		t.Fatalf("idle error does not report the grace windows: %v", err)
	}
	if !retryable {
		t.Fatal("a stalled compaction with no tool executed was not retryable")
	}
}

// A stall AFTER a tool ran may have side effects on disk, so re-running the turn
// is not free: it must come back non-retryable even though the diagnosis is the
// same idle watchdog.
func TestCodexIdleHangAfterToolIsNotRetryable(t *testing.T) {
	if os.Getenv("CODEX_TEST_HELPER") != "" {
		return
	}
	err, _, retryable := runCodexStallHelper(t, 150*time.Millisecond,
		`{"type":"item.completed","item":{"id":"t1","type":"command_execution","command":"touch x","exit_code":0,"status":"completed"}}`)
	if err == nil {
		t.Fatal("runAttempt returned no idle timeout error")
	}
	if retryable {
		t.Fatal("idle timeout after a tool call was retryable")
	}
}

// runCodexStallHelper runs a fake codex that prints line, then stalls until it is
// killed. It returns the attempt's error, how long the attempt took, and whether
// the error was reported as retryable.
func runCodexStallHelper(t *testing.T, window time.Duration, line string) (error, time.Duration, bool) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no test binary path: %v", err)
	}
	t.Setenv("CODEX_TEST_STALL_LINE", line)

	originalTimeout := codexIdleOutputWindow()
	SetCodexIdleOutputTimeout(window)
	t.Cleanup(func() { SetCodexIdleOutputTimeout(originalTimeout) })

	c := &CodexCLI{binPath: self}
	args := []string{"-test.run=TestCodexHelperEmitsThenStalls", "-test.v=false"}
	start := time.Now()
	_, retryable, runErr := c.runAttempt(context.Background(), args, "prompt", "gpt-test", Request{}, "")
	return runErr, time.Since(start), retryable
}

// TestCodexHelperEmitsThenStalls is the fake codex binary for the stall tests: it
// prints the one line it was given, then produces nothing until it is killed.
func TestCodexHelperEmitsThenStalls(t *testing.T) {
	line := os.Getenv("CODEX_TEST_STALL_LINE")
	if line == "" {
		t.Skip("helper: only runs under the re-exec in runCodexStallHelper")
	}
	fmt.Println(line)
	time.Sleep(60 * time.Second)
}
