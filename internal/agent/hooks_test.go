package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"runtime"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

func TestHookMatches(t *testing.T) {
	cases := []struct {
		matcher, tool string
		want          bool
	}{
		{"", "shell", true},      // empty = all
		{"shell", "shell", true}, // exact
		{"shell", "write_file", false},
		{"http_*", "http_get", true}, // glob
		{"http_*", "shell", false},
		{"*", "anything", true},
	}
	for _, c := range cases {
		if got := hookMatches(c.matcher, c.tool); got != c.want {
			t.Errorf("hookMatches(%q,%q)=%v want %v", c.matcher, c.tool, got, c.want)
		}
	}
}

// hookCmd builds a shell command that emits the given stdout on a clean exit,
// portable across the dev OS (PowerShell on Windows, sh elsewhere).
func emitCmd(stdout string) string {
	if runtime.GOOS == "windows" {
		// Single-quoted literal so JSON braces/quotes survive.
		return "Write-Output '" + stdout + "'"
	}
	return "printf '%s' '" + stdout + "'"
}

func blockCmd() string {
	if runtime.GOOS == "windows" {
		return "[Console]::Error.WriteLine('denied by test'); exit 2"
	}
	return "echo 'denied by test' 1>&2; exit 2"
}

func testRuntime(t *testing.T) *Runtime {
	t.Helper()
	return &Runtime{
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		workDir: t.TempDir(),
	}
}

func TestExecHook_Block_ExitCode2(t *testing.T) {
	r := testRuntime(t)
	dec, err := r.execHook(context.Background(), db.Hook{Command: blockCmd(), TimeoutSec: 10}, hookPayload{ToolName: "shell"})
	if err != nil {
		t.Fatalf("execHook err: %v", err)
	}
	if dec.Decision != "block" {
		t.Fatalf("expected block, got %q (reason %q)", dec.Decision, dec.Reason)
	}
}

func TestExecHook_UpdatedOutput(t *testing.T) {
	r := testRuntime(t)
	want := "compressed"
	payload, _ := json.Marshal(hookDecision{UpdatedOutput: &want})
	dec, err := r.execHook(context.Background(), db.Hook{Command: emitCmd(string(payload)), TimeoutSec: 10}, hookPayload{ToolName: "shell"})
	if err != nil {
		t.Fatalf("execHook err: %v", err)
	}
	if dec.UpdatedOutput == nil || *dec.UpdatedOutput != "compressed" {
		t.Fatalf("expected updatedOutput 'compressed', got %v", dec.UpdatedOutput)
	}
}

func TestExecHook_EmptyStdoutAllows(t *testing.T) {
	r := testRuntime(t)
	dec, err := r.execHook(context.Background(), db.Hook{Command: emitCmd(""), TimeoutSec: 10}, hookPayload{ToolName: "shell"})
	if err != nil {
		t.Fatalf("execHook err: %v", err)
	}
	if dec.Decision == "block" {
		t.Fatalf("empty stdout should not block")
	}
}
