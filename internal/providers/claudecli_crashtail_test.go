package providers

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestStdoutCrashTail(t *testing.T) {
	// Prefers plain (non-JSON) lines — those carry the real error text when the
	// CLI dies with empty stderr.
	mixed := []string{`{"type":"system"}`, "panic: boom", `{"type":"assistant"}`}
	if got := stdoutCrashTail(mixed); !strings.Contains(got, "panic: boom") || strings.Contains(got, `"type"`) {
		t.Fatalf("should surface the plain error line only, got: %s", got)
	}
	// Falls back to the raw tail when only JSON lines exist.
	if got := stdoutCrashTail([]string{`{"type":"result"}`}); !strings.Contains(got, "result") {
		t.Fatalf("should fall back to raw tail, got: %s", got)
	}
	// Empty input still yields an explicit note, never an empty string.
	if got := stdoutCrashTail(nil); got == "" {
		t.Fatal("empty tail should still return a note")
	}
	// A stream-json result/error event carries the real failure reason and must
	// be surfaced over benign startup events (e.g. SessionStart hooks).
	stream := []string{
		`{"type":"system","subtype":"hook_started","hook_name":"SessionStart:startup"}`,
		`{"type":"result","subtype":"error_during_execution","is_error":true,"error":"mcp server failed"}`,
	}
	got := stdoutCrashTail(stream)
	if !strings.Contains(got, "mcp server failed") || strings.Contains(got, "hook_started") {
		t.Fatalf("should surface the result/error event, not startup noise, got: %s", got)
	}
}

// TestDumpCLIFailure verifies the full-output dump writes a recoverable log file
// containing the invocation, full stdout and stderr — the diagnostic that the
// 12-line inline tail can miss (e.g. an early exit after SessionStart hooks).
func TestDumpCLIFailure(t *testing.T) {
	args := []string{"-p", "--output-format", "stream-json", "--mcp-config", "/tmp/tionharness-mcp-xyz.json"}
	stdout := []byte(`{"type":"system","subtype":"hook_started","hook_name":"SessionStart:startup"}` + "\n")
	stderr := []byte("the real fatal error line")

	path := dumpCLIFailure("/usr/bin/claude", args, "C:/ws", errors.New("exit status 1"), stdout, stderr)
	if path == "" {
		t.Fatal("dumpCLIFailure returned empty path")
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("log file not readable: %v", err)
	}
	body := string(b)
	for _, want := range []string{
		"exit status 1",        // the run error
		"--mcp-config",         // the invocation args
		"C:/ws",                // the work dir
		"the real fatal error", // full stderr
		"SessionStart:startup", // full stdout
		"===== STDOUT",         // section header
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dump missing %q\n--- body ---\n%s", want, body)
		}
	}
}

// TestCLIParserSalvage covers resilience: a stream cut off before its final
// "result" event (the CLI crashed at the end of the turn) still yields the
// assistant content via salvage(), so a long research turn isn't wholly lost.
func TestCLIParserSalvage(t *testing.T) {
	p := newCLIParser("claude-x", nil)
	p.feed(`{"type":"assistant","message":{"content":[{"type":"text","text":"Here is the research result."}]}}`)
	// No "result" event arrived → finish() must fail (the crash case).
	if _, err := p.finish(); err == nil {
		t.Fatal("finish() should fail without a result event")
	}
	got := p.salvage()
	if got == nil || !strings.Contains(got.Text, "research result") {
		t.Fatalf("salvage should recover the partial answer, got: %+v", got)
	}
}

// TestCLIParserSalvage_GenuineErrorNotMasked ensures an explicit error result is
// never masked as success by salvage — only a clean cut-off is recovered.
func TestCLIParserSalvage_GenuineErrorNotMasked(t *testing.T) {
	p := newCLIParser("claude-x", nil)
	p.feed(`{"type":"assistant","message":{"content":[{"type":"text","text":"partial"}]}}`)
	p.feed(`{"type":"result","is_error":true,"result":"rate limited"}`)
	if got := p.salvage(); got != nil {
		t.Fatalf("a genuine error result must NOT be salvaged, got: %+v", got)
	}
}

// TestCLIParserSalvage_NothingUsable returns nil when no assistant content exists.
func TestCLIParserSalvage_NothingUsable(t *testing.T) {
	p := newCLIParser("claude-x", nil)
	p.feed(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"WebSearch","input":{}}]}}`)
	if got := p.salvage(); got != nil {
		t.Fatalf("salvage with no assistant text should return nil, got: %+v", got)
	}
}

// TestCLIParserRanTool gates the crash-retry: a turn that reached a tool (possible
// side effects) is NOT retryable; a clean crash before any tool IS.
func TestCLIParserRanTool(t *testing.T) {
	// No events → nothing ran → retryable (ranTool false). Models the "crash right
	// after init" case (empty stdout body before the first assistant turn).
	clean := newCLIParser("claude-x", nil)
	if clean.ranTool() {
		t.Fatal("a parser with no trace must report ranTool=false (retry safe)")
	}
	// A tool was invoked → not safe to retry.
	withTool := newCLIParser("claude-x", nil)
	withTool.feed(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"WebSearch","input":{}}]}}`)
	if !withTool.ranTool() {
		t.Fatal("a parser that saw a tool_use must report ranTool=true (no retry)")
	}
}
