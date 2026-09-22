package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/procwatch"
)

// findLedgerEntry returns the newest ledger entry whose command contains marker.
func findLedgerEntry(t *testing.T, marker string) (procwatch.Entry, bool) {
	t.Helper()
	for _, e := range procwatch.Default().List(procwatch.Filter{}) {
		if strings.Contains(e.Command, marker) {
			return e, true
		}
	}
	return procwatch.Entry{}, false
}

// TestForegroundShellIsRecordedInTheLedger covers the instrumentation contract of
// the shell tool: a foreground command reaches the process ledger with its own
// command line, an owner taken from the turn context, a pid and a terminal
// status — the data the workspace process panel renders.
func TestForegroundShellIsRecordedInTheLedger(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	if !sb.Ready() {
		t.Skip("sandbox not ready in this environment")
	}
	tool := NewShellTool(sb)
	if !tool.Available() {
		t.Skip("no POSIX shell available on this host")
	}
	ctx := procwatch.WithOwner(t.Context(), procwatch.Owner{SessionID: "SESLEDGER", AgentID: "AGTLEDGER"})

	if _, err := tool.CallStream(ctx, json.RawMessage(`{"command":"echo ledgerprobe"}`), nil); err != nil {
		t.Fatalf("shell call: %v", err)
	}

	e, ok := findLedgerEntry(t, "ledgerprobe")
	if !ok {
		t.Fatal("the foreground shell command never reached the ledger")
	}
	if e.Kind != procwatch.KindShell || e.Label != "Bash" {
		t.Errorf("kind/label = %q/%q, want shell/Bash", e.Kind, e.Label)
	}
	if e.Status != procwatch.StatusSucceeded || e.ExitCode != 0 {
		t.Errorf("status/exit = %q/%d, want succeeded/0", e.Status, e.ExitCode)
	}
	if e.Owner.SessionID != "SESLEDGER" || e.Owner.AgentID != "AGTLEDGER" {
		t.Errorf("owner = %+v, want the owner stamped on the context", e.Owner)
	}
	if e.PID == 0 {
		t.Error("no pid recorded — the panel would show a process it cannot identify")
	}
	if !strings.Contains(e.OutputTail, "ledgerprobe") {
		t.Errorf("output tail = %q, want the command's own output", e.OutputTail)
	}
}

// TestFailingForegroundShellIsRecordedAsFailed pins the outcome mapping: a
// non-zero exit is a failure with its code, not a success with empty output.
func TestFailingForegroundShellIsRecordedAsFailed(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	if !sb.Ready() {
		t.Skip("sandbox not ready in this environment")
	}
	tool := NewShellTool(sb)
	if !tool.Available() {
		t.Skip("no POSIX shell available on this host")
	}

	if _, err := tool.CallStream(t.Context(), json.RawMessage(`{"command":"echo failprobe; exit 7"}`), nil); err != nil {
		t.Fatalf("shell call: %v", err)
	}
	e, ok := findLedgerEntry(t, "failprobe")
	if !ok {
		t.Fatal("the failing command never reached the ledger")
	}
	if e.Status != procwatch.StatusFailed || e.ExitCode != 7 {
		t.Errorf("status/exit = %q/%d, want failed/7", e.Status, e.ExitCode)
	}
}

// TestBackgroundShellLedgerEntryEndsAsKilled covers the background half: the
// shell is tracked while detached, and a shell_manage kill lands as "killed"
// rather than as a plain non-zero exit.
func TestBackgroundShellLedgerEntryEndsAsKilled(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	if !sb.Ready() {
		t.Skip("sandbox not ready in this environment")
	}
	m := NewShellManager()
	id, err := m.Start(t.Context(), sb, "sleep bgledgerprobe", "Bash", portableSleep)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	e, ok := findLedgerEntry(t, "bgledgerprobe")
	if !ok {
		t.Fatal("the background shell never reached the ledger")
	}
	if e.Kind != procwatch.KindShellBackground || e.Status != procwatch.StatusRunning || !e.Stoppable {
		t.Fatalf("entry = %+v, want a running, stoppable background shell", e)
	}

	if _, err := m.Kill(id); err != nil {
		t.Fatalf("kill: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		e, ok = findLedgerEntry(t, "bgledgerprobe")
		if ok && e.Status != procwatch.StatusRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background shell never reached a terminal status (last: %+v)", e)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if e.Status != procwatch.StatusKilled {
		t.Errorf("status = %q, want killed — a requested stop must not read as a failure", e.Status)
	}
}

// portableSleep builds a command that stays alive long enough to be killed.
func portableSleep(ctx context.Context, _ string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/c", "ping -n 30 127.0.0.1 >NUL")
	}
	return exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30")
}
