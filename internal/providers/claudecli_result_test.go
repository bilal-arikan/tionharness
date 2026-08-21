package providers

import (
	"strings"
	"testing"
)

// TestCLIAuthErrorTextClassification: CLI 2.1.238 reports an expired OAuth
// session as {"subtype":"success","is_error":true,...,"result":"Failed to
// authenticate: OAuth session expired and could not be refreshed"} — the
// wording changed, and missing it makes the turn look retryable.
func TestCLIAuthErrorTextClassification(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"observed 2.1.238 oauth expiry", "Failed to authenticate: OAuth session expired and could not be refreshed", true},
		{"legacy not logged in", "Not logged in · Please run /login", true},
		{"api key", "invalid x-api-key", true},
		{"non-auth failure", "Error: connection reset by peer while streaming", false},
		{"tool failure is not auth", "Tool ran without output or errors", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAuthErrorText(tc.text); got != tc.want {
				t.Fatalf("isAuthErrorText(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// TestCLIResultTerminalReason: a generic (non-auth, non-rate-limit) failure must
// carry terminal_reason in the error text, otherwise the caller only sees a bare
// sentence and cannot tell an api_error from a user abort.
func TestCLIResultTerminalReason(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"result","subtype":"success","is_error":true,"stop_reason":"stop_sequence","terminal_reason":"api_error","result":"Upstream failure"}`)

	if !p.hadError {
		t.Fatal("expected hadError")
	}
	if !strings.Contains(p.errText, "api_error") {
		t.Fatalf("errText = %q, want it to mention terminal_reason api_error", p.errText)
	}
	if !strings.Contains(p.errText, "Upstream failure") {
		t.Fatalf("errText = %q, want the original result text preserved", p.errText)
	}
}

// An auth failure keeps its actionable wording untouched — terminal_reason must
// not be appended there (the classification already tells the caller what to do).
func TestCLIResultAuthKeepsCleanMessage(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"result","subtype":"success","is_error":true,"stop_reason":"stop_sequence","terminal_reason":"api_error","result":"Failed to authenticate: OAuth session expired and could not be refreshed"}`)

	if !p.notLoggedIn {
		t.Fatal("expected the turn to be classified as an auth failure")
	}
	if strings.Contains(p.errText, "terminal_reason") {
		t.Fatalf("errText = %q, want no terminal_reason suffix on an auth failure", p.errText)
	}
}

// TestCLIResultPermissionDenials: blocked tool calls degrade the answer, so they
// get a visible trace note on the success path too.
func TestCLIResultPermissionDenials(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"result","subtype":"success","is_error":false,"result":"done","permission_denials":[{"tool_name":"Bash","tool_use_id":"t1"},{"tool_name":"Write","tool_use_id":"t2"}]}`)

	var note string
	for _, s := range p.resp.Trace {
		if strings.Contains(s.Text, "permission denial") {
			note = s.Text
		}
	}
	if note == "" {
		t.Fatalf("no permission-denial note in trace: %+v", p.resp.Trace)
	}
	if !strings.Contains(note, "2 permission denial(s)") || !strings.Contains(note, "Bash") || !strings.Contains(note, "Write") {
		t.Fatalf("note = %q, want the count and both tool names", note)
	}
}

// A clean result must not add any note.
func TestCLIResultNoPermissionDenialsIsSilent(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"result","subtype":"success","is_error":false,"result":"done"}`)
	for _, s := range p.resp.Trace {
		if strings.Contains(s.Text, "permission denial") {
			t.Fatalf("unexpected note in trace: %q", s.Text)
		}
	}
}

// TestCLIHookResponseFailure: a hook that exits non-zero can strip a tool call
// or block an edit while the turn still ends "successfully" — make it visible.
func TestCLIHookResponseFailure(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"system","subtype":"hook_response","hook_name":"guard","hook_event":"PreToolUse","exit_code":1,"outcome":"blocked","stderr":"boom: policy violated"}`)

	if len(p.resp.Trace) != 1 {
		t.Fatalf("trace = %+v, want exactly one warning step", p.resp.Trace)
	}
	step := p.resp.Trace[0]
	if step.Kind != "text" {
		t.Fatalf("step kind = %q, want text", step.Kind)
	}
	for _, want := range []string{"guard", "exit 1", "boom: policy violated"} {
		if !strings.Contains(step.Text, want) {
			t.Fatalf("step text = %q, want it to contain %q", step.Text, want)
		}
	}
}

// A hook's stderr can be arbitrarily long; the note is capped so one noisy hook
// cannot flood the trace.
func TestCLIHookResponseStderrTrimmed(t *testing.T) {
	p := newCLIParser("", nil)
	long := strings.Repeat("x", 4000)
	p.feed(`{"type":"system","subtype":"hook_response","hook_name":"noisy","exit_code":2,"stderr":"` + long + `"}`)

	if len(p.resp.Trace) != 1 {
		t.Fatalf("trace = %+v, want exactly one warning step", p.resp.Trace)
	}
	if got := len(p.resp.Trace[0].Text); got > 700 {
		t.Fatalf("note length = %d, want the stderr trimmed to ~500 chars", got)
	}
}

// A successful hook must stay silent — hooks fire on every step, so noting them
// would drown the activity trace.
func TestCLIHookResponseSuccessIsSilent(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":"system","subtype":"hook_response","hook_name":"guard","hook_event":"PreToolUse","exit_code":0,"outcome":"allow","stdout":"ok"}`)

	if len(p.resp.Trace) != 0 {
		t.Fatalf("trace = %+v, want no step for a successful hook", p.resp.Trace)
	}
}
