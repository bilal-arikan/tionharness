package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBGWriterDrain(t *testing.T) {
	w := &bgWriter{max: 8}
	w.Write([]byte("abc"))
	if out, lost := w.drain(); out != "abc" || lost {
		t.Fatalf("first drain = %q lost=%v", out, lost)
	}
	// Nothing new since last drain.
	if out, lost := w.drain(); out != "" || lost {
		t.Fatalf("empty drain = %q lost=%v", out, lost)
	}
	// Overflow the ring: total 12 bytes written, max 8 → oldest 4 dropped. Since the
	// reader already consumed "abc" (3), the delivered cursor is behind the ring
	// start (4), so 1 byte is lost.
	w.Write([]byte("defghijkl")) // total now 12, buf keeps last 8: "efghijkl"
	out, lost := w.drain()
	if !lost {
		t.Fatalf("expected lost=true after overflow, out=%q", out)
	}
	if out != "efghijkl" {
		t.Fatalf("post-overflow drain = %q", out)
	}
}

func TestShellManagerErrors(t *testing.T) {
	m := NewShellManager()
	if _, err := m.Output("nope"); err == nil {
		t.Fatal("Output on unknown id should error")
	}
	if _, err := m.Kill("nope"); err == nil {
		t.Fatal("Kill on unknown id should error")
	}
	if m.List() != "(no background shells)" {
		t.Fatalf("empty List = %q", m.List())
	}

	// A nil manager reports background unavailable rather than panicking.
	var nilMgr *ShellManager
	if _, err := nilMgr.Start(NewSandbox(t.TempDir()), "x", "Bash", nil); err == nil {
		t.Fatal("nil manager Start should error")
	}
}

// portableEcho builds a command that prints "hello" on the host shell.
func portableEcho(ctx context.Context, _ string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/c", "echo hello")
	}
	return exec.CommandContext(ctx, "/bin/sh", "-c", "echo hello")
}

func TestShellManagerRunBackground(t *testing.T) {
	m := NewShellManager()
	sb := NewSandbox(t.TempDir())
	id, err := m.Start(sb, "echo hello", "Bash", portableEcho)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// Poll shell_output until the process reports it exited (bounded). Output drains
	// destructively (each read returns only NEW bytes), so accumulate across polls —
	// exactly how a caller reads a background shell over several turns.
	deadline := time.Now().Add(5 * time.Second)
	var acc strings.Builder
	for time.Now().Before(deadline) {
		out, oerr := m.Output(id)
		if oerr != nil {
			t.Fatalf("output: %v", oerr)
		}
		acc.WriteString(out)
		if strings.Contains(out, "exited") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(acc.String(), "hello") {
		t.Fatalf("expected echoed output, got %q", acc.String())
	}
	// List shows the (now finished) shell.
	if !strings.Contains(m.List(), id) {
		t.Fatalf("List missing %s: %q", id, m.List())
	}
}

// mustJSONbg is a local marshal helper (mustJSON lives in another _test file in
// this package; keep this self-contained for the shell tests).
func mustJSONbg(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestShellOutputToolValidation(t *testing.T) {
	tool := NewShellOutputTool(NewShellManager())
	if _, err := tool.Call(context.Background(), mustJSONbg(t, map[string]any{"shell_id": ""})); err == nil {
		t.Fatal("empty shell_id should error")
	}
}
