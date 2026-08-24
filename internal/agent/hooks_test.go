package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
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
		// comma-separated alternatives (Bash,PowerShell after the shell split)
		{"Bash,PowerShell", "Bash", true},
		{"Bash,PowerShell", "PowerShell", true},
		{"Bash,PowerShell", "Write", false},
		{"Bash, PowerShell", "PowerShell", true}, // whitespace-tolerant
		{"http_*,Bash", "http_get", true},        // glob alt + exact alt
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

// syntaxErrorCmd is a command the platform's hook interpreter cannot PARSE. Both
// shells exit 2 for this — the same code the Claude Code contract reserves for a
// deliberate "block" — which is exactly why the two must be told apart.
func syntaxErrorCmd() string {
	if runtime.GOOS == "windows" {
		return "if( -and ) {"
	}
	return "if [ -z"
}

// A hook whose command is not valid syntax for the interpreter that runs it must
// surface as an ERROR (fail-open: the caller logs and lets the tool call through),
// never as a block decision. Conflating the two is how a PowerShell-authored hook
// silently disabled Bash for entire sessions.
func TestExecHook_InterpreterSyntaxErrorIsErrorNotBlock(t *testing.T) {
	r := testRuntime(t)
	dec, err := r.execHook(context.Background(), db.Hook{Command: syntaxErrorCmd(), TimeoutSec: 10}, hookPayload{ToolName: "Bash"})
	if err == nil {
		t.Fatalf("a syntax error must return an error, got decision %q", dec.Decision)
	}
	if dec.Decision == "block" {
		t.Fatal("a syntax error must not be reported as a deny decision")
	}
	// The error must name the fault, so debug.jsonl is diagnosable without a re-run.
	if !strings.Contains(err.Error(), "hook interpreter failed") {
		t.Errorf("error does not identify the interpreter failure: %v", err)
	}
}

// The inverse guard: a hook that deliberately exits 2 with an ordinary reason on
// stderr must still BLOCK. Hardening the interpreter case must not weaken deny.
func TestExecHook_DeliberateExit2StillBlocks(t *testing.T) {
	r := testRuntime(t)
	dec, err := r.execHook(context.Background(), db.Hook{Command: blockCmd(), TimeoutSec: 10}, hookPayload{ToolName: "Bash"})
	if err != nil {
		t.Fatalf("a deliberate deny must not become an error: %v", err)
	}
	if dec.Decision != "block" {
		t.Fatalf("expected block, got %q", dec.Decision)
	}
	if !strings.Contains(dec.Reason, "denied by test") {
		t.Errorf("deny reason lost: %q", dec.Reason)
	}
}

func TestInterpreterFailure(t *testing.T) {
	// Real stderr captured from the live failure this fix targets.
	if got := interpreterFailure("bash: line 1: syntax error near unexpected token '|'"); got == "" {
		t.Error("bash syntax error not recognised as an interpreter failure")
	}
	if got := interpreterFailure("ParserError: Missing closing ')'"); got == "" {
		t.Error("powershell ParserError not recognised as an interpreter failure")
	}
	// Multi-line stderr reports only the first line.
	got := interpreterFailure("syntax error near '|'\nstack line 2\nstack line 3")
	if strings.Contains(got, "stack line") {
		t.Errorf("interpreterFailure should report only the first line, got %q", got)
	}
	// An ordinary deny reason is NOT an interpreter failure.
	for _, s := range []string{"", "denied by policy", "blocked: file is protected"} {
		if got := interpreterFailure(s); got != "" {
			t.Errorf("ordinary stderr %q misread as an interpreter failure: %q", s, got)
		}
	}
}
