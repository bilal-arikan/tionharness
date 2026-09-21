package tools

import (
	"context"
	"strings"
	"testing"
)

// newTestShell registers a bgProc directly, so the source can be exercised without
// launching a real process.
func newTestShell(t *testing.T) (*ShellManager, *bgProc) {
	t.Helper()
	m := NewShellManager()
	p := &bgProc{id: "bg1", command: "test", shell: "Bash", w: &bgWriter{max: bgShellRingBytes}}
	m.procs["bg1"] = p
	return m, p
}

// TestShellSourceCursorIndependentOfShellManage is the reason drainFrom exists: a
// monitor reading output must NOT consume it from shell_manage's cursor, and vice
// versa. Both readers see every byte.
func TestShellSourceCursorIndependentOfShellManage(t *testing.T) {
	m, p := newTestShell(t)
	src, err := NewShellSource(m, "bg1")
	if err != nil {
		t.Fatalf("NewShellSource: %v", err)
	}
	p.w.Write([]byte("first line\nsecond line\n"))

	// The monitor reads first.
	evs, done, _, err := src.Poll(context.Background())
	if err != nil || done {
		t.Fatalf("Poll: err=%v done=%v", err, done)
	}
	if len(evs) != 2 || evs[0].Payload != "first line" || evs[1].Payload != "second line" {
		t.Fatalf("monitor saw %+v", evs)
	}

	// shell_manage must STILL see the same output: the monitor's read did not
	// advance the delivered cursor.
	out, err := m.Output("bg1")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if !strings.Contains(out, "first line") || !strings.Contains(out, "second line") {
		t.Fatalf("shell_manage lost output the monitor had read: %q", out)
	}

	// And the reverse: shell_manage's read did not advance the monitor's cursor, so
	// the monitor sees only genuinely new output next time.
	evs, _, _, err = src.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(evs) != 0 {
		t.Fatalf("monitor re-read already-seen output: %+v", evs)
	}
	p.w.Write([]byte("third line\n"))
	evs, _, _, err = src.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(evs) != 1 || evs[0].Payload != "third line" {
		t.Fatalf("monitor missed new output: %+v", evs)
	}
}

// TestShellSourceStartsAtCurrentEnd pins that a monitor reports what happens from
// the moment it was armed, not the backlog the agent has already read.
func TestShellSourceStartsAtCurrentEnd(t *testing.T) {
	m, p := newTestShell(t)
	p.w.Write([]byte("old output\n"))
	src, err := NewShellSource(m, "bg1")
	if err != nil {
		t.Fatalf("NewShellSource: %v", err)
	}
	evs, _, _, err := src.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(evs) != 0 {
		t.Fatalf("monitor replayed the backlog: %+v", evs)
	}
}

// TestShellSourceReportsExitAfterFinalOutput guards that the LAST line is still
// delivered in the same poll that reports the terminal state — a match on the
// final line must not be lost.
func TestShellSourceReportsExitAfterFinalOutput(t *testing.T) {
	m, p := newTestShell(t)
	src, err := NewShellSource(m, "bg1")
	if err != nil {
		t.Fatalf("NewShellSource: %v", err)
	}
	p.w.Write([]byte("done: all tests passed\n"))
	p.finish(0)

	evs, done, reason, err := src.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if !done {
		t.Fatal("exited shell must report done")
	}
	if len(evs) != 1 || evs[0].Payload != "done: all tests passed" {
		t.Fatalf("final output lost on exit: %+v", evs)
	}
	if !strings.Contains(reason, "exited") {
		t.Fatalf("reason %q does not explain the terminal state", reason)
	}
}

// TestNewShellSourceUnknownIDIsError keeps an unknown shell a REAL error rather
// than an inert monitor that can never fire.
func TestNewShellSourceUnknownIDIsError(t *testing.T) {
	m := NewShellManager()
	if _, err := NewShellSource(m, "bg404"); err == nil {
		t.Fatal("unknown shell id must be an error")
	}
	// A nil manager (catalog/preview build) is the same kind of failure.
	if _, err := NewShellSource(nil, "bg1"); err == nil {
		t.Fatal("nil shell manager must be an error")
	}
}

// TestDrainFromDoesNotMoveDeliveredCursor is the unit-level guard on the split:
// drainFrom reports the next cursor to its caller and leaves w.delivered alone.
func TestDrainFromDoesNotMoveDeliveredCursor(t *testing.T) {
	w := &bgWriter{max: bgShellRingBytes}
	w.Write([]byte("hello"))
	out, next, lost := w.drainFrom(0)
	if out != "hello" || next != 5 || lost {
		t.Fatalf("drainFrom(0) = %q, %d, %v", out, next, lost)
	}
	if w.delivered != 0 {
		t.Fatalf("drainFrom moved the delivered cursor to %d", w.delivered)
	}
	// drain still returns everything, because its own cursor never moved.
	got, _ := w.drain()
	if got != "hello" {
		t.Fatalf("drain after drainFrom = %q", got)
	}
	if w.delivered != 5 {
		t.Fatalf("drain left delivered at %d, want 5", w.delivered)
	}
}
