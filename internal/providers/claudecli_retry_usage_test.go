package providers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The fake-claude helper is selected by these two env vars. Plain names on
// purpose: cliBaseEnv strips anything CLAUDE_*/ANTHROPIC_* shaped before the
// subprocess is launched.
const (
	claudeRetryHelperEnv  = "TIONHARNESS_FAKE_CLAUDE_MODE"
	claudeRetryCounterEnv = "TIONHARNESS_FAKE_CLAUDE_COUNTER"
)

// TestClaudeHelperCrashesThenSucceeds impersonates the claude CLI across the
// retry loop's two attempts, counting them through a file because each attempt
// is a fresh process. Attempt 0 reports a measured failure and exits non-zero
// (the "clean crash" shape: usage, no model turn, no tool → retryable); attempt 1
// answers normally.
func TestClaudeHelperCrashesThenSucceeds(t *testing.T) {
	mode := os.Getenv(claudeRetryHelperEnv)
	if mode != "crash-then-succeed" && mode != "compact-crash-then-succeed" && mode != "compact-crash-after-text" {
		t.Skip("helper: only runs when re-executed as a fake claude binary")
	}
	if mode == "compact-crash-after-text" {
		fmt.Println(`{"type":"system","subtype":"hook_started","hook_id":"pre-salvage","hook_event":"PreCompact","session_id":"cli-in"}`)
		fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"usable partial"}]}}`)
		os.Exit(1)
	}
	counter := os.Getenv(claudeRetryCounterEnv)
	attempt := 0
	if raw, err := os.ReadFile(counter); err == nil {
		attempt, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			fmt.Println("helper: unreadable attempt counter:", err)
			os.Exit(2)
		}
	}
	if err := os.WriteFile(counter, []byte(strconv.Itoa(attempt+1)), 0o644); err != nil {
		fmt.Println("helper: cannot write attempt counter:", err)
		os.Exit(2)
	}
	if attempt == 0 {
		if mode == "compact-crash-then-succeed" {
			fmt.Println(`{"type":"system","subtype":"hook_started","hook_id":"pre-first","hook_event":"PreCompact","session_id":"cli-in"}`)
			os.Exit(1)
		}
		fmt.Println(`{"type":"result","is_error":true,"result":"transient crash right after init",` +
			`"num_turns":1,"usage":{"input_tokens":1000,"output_tokens":20,` +
			`"cache_read_input_tokens":300,"cache_creation_input_tokens":40}}`)
		os.Exit(1)
	}
	if mode == "compact-crash-then-succeed" {
		fmt.Println(`{"type":"system","subtype":"hook_started","hook_id":"pre-second","hook_event":"PreCompact","session_id":"cli-in"}`)
		fmt.Println(`{"type":"system","subtype":"compact_boundary","session_id":"cli-out"}`)
	}
	fmt.Println(`{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}`)
	fmt.Println(`{"type":"result","is_error":false,"result":"hello","num_turns":3,` +
		`"usage":{"input_tokens":7,"output_tokens":5,"cache_read_input_tokens":11,` +
		`"cache_creation_input_tokens":13}}`)
	os.Exit(0)
}

func TestClaudeRetryCompactionLifecycleClosesEachAttempt(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no test binary path: %v", err)
	}
	counter := filepath.Join(t.TempDir(), "attempts")
	t.Setenv(claudeRetryHelperEnv, "compact-crash-then-succeed")
	t.Setenv(claudeRetryCounterEnv, counter)
	var events []CLICompactionEvent
	req := Request{
		ResumeSessionID: "cli-in",
		OnCLICompaction: func(ev CLICompactionEvent) { events = append(events, ev) },
	}
	c := &ClaudeCLI{binPath: self}
	args := []string{"-test.run=TestClaudeHelperCrashesThenSucceeds", "-test.v=false"}
	if _, err := c.completeWithArgs(context.Background(), args, "prompt", "opus", req); err != nil {
		t.Fatal(err)
	}
	var terminals []CLICompactionEvent
	for _, ev := range events {
		if ev.Phase == CLICompactionError || ev.Phase == CLICompactionSuccess {
			terminals = append(terminals, ev)
		}
	}
	if len(terminals) != 2 || terminals[0].Phase != CLICompactionError || !terminals[0].Retryable || terminals[1].Phase != CLICompactionSuccess {
		t.Fatalf("terminals = %+v; all events=%+v", terminals, events)
	}
	if terminals[0].ExitCode != 1 || terminals[0].ErrorKind != "process_error" {
		t.Fatalf("first terminal classification = %+v", terminals[0])
	}
	if terminals[0].AttemptID == terminals[1].AttemptID || terminals[0].Attempt != 1 || terminals[1].Attempt != 2 {
		t.Fatalf("attempt correlation = %+v", terminals)
	}
}

func TestClaudeSalvagedCrashClosesOpenCompactionAttempt(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no test binary path: %v", err)
	}
	t.Setenv(claudeRetryHelperEnv, "compact-crash-after-text")
	var events []CLICompactionEvent
	req := Request{
		ResumeSessionID: "cli-in",
		OnCLICompaction: func(ev CLICompactionEvent) { events = append(events, ev) },
	}
	c := &ClaudeCLI{binPath: self}
	args := []string{"-test.run=TestClaudeHelperCrashesThenSucceeds", "-test.v=false"}
	resp, _, err := c.runAttempt(context.Background(), args, "prompt", "opus", req)
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.Text == "" {
		t.Fatalf("salvaged response = %+v", resp)
	}
	var terminal *CLICompactionEvent
	for i := range events {
		if events[i].Phase == CLICompactionError || events[i].Phase == CLICompactionCancelled || events[i].Phase == CLICompactionSuccess {
			terminal = &events[i]
		}
	}
	if terminal == nil || terminal.Phase != CLICompactionError || terminal.ErrorKind != "process_error" || terminal.ExitCode != 1 {
		t.Fatalf("terminal = %+v; events=%+v", terminal, events)
	}
}

// The retry loop used to return the successful attempt's response and drop the
// failed attempt's error on the floor — together with the usage it carried. Those
// tokens were really spent, and the successful Response is the only thing the
// caller bills, so the total must be the SUM of both attempts.
func TestClaudeRetrySumsDiscardedAttemptUsage(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Skipf("no test binary path: %v", err)
	}
	counter := filepath.Join(t.TempDir(), "attempts")
	t.Setenv(claudeRetryHelperEnv, "crash-then-succeed")
	t.Setenv(claudeRetryCounterEnv, counter)

	c := &ClaudeCLI{binPath: self}
	args := []string{"-test.run=TestClaudeHelperCrashesThenSucceeds", "-test.v=false"}
	resp, err := c.completeWithArgs(context.Background(), args, "prompt", "opus", Request{})
	if err != nil {
		t.Fatalf("the second attempt succeeded, so the turn must succeed: %v", err)
	}
	if raw, rerr := os.ReadFile(counter); rerr != nil || strings.TrimSpace(string(raw)) != "2" {
		t.Fatalf("want exactly two attempts, counter=%q err=%v", raw, rerr)
	}
	want := Usage{
		InputTokens:      1000 + 7,
		OutputTokens:     20 + 5,
		CacheReadTokens:  300 + 11,
		CacheWriteTokens: 40 + 13,
	}
	if resp.Usage != want {
		t.Fatalf("usage = %+v, want the sum of both attempts %+v", resp.Usage, want)
	}
	// num_turns is a call count, so the discarded attempt's calls add up too.
	if resp.ProviderCalls != 1+3 {
		t.Fatalf("providerCalls = %d, want 4 (both attempts)", resp.ProviderCalls)
	}
}

// addUsage must cover EVERY field: a summand that is silently dropped here is a
// token class that is never billed. Uses distinct values so a copy-paste in the
// helper cannot pass by coincidence.
func TestAddUsageSumsEveryField(t *testing.T) {
	a := Usage{InputTokens: 1, OutputTokens: 2, CacheReadTokens: 3, CacheWriteTokens: 4,
		CacheWrite5mTokens: 5, CacheWrite1hTokens: 6, ThinkingTokens: 7}
	b := Usage{InputTokens: 10, OutputTokens: 20, CacheReadTokens: 30, CacheWriteTokens: 40,
		CacheWrite5mTokens: 50, CacheWrite1hTokens: 60, ThinkingTokens: 70}
	want := Usage{InputTokens: 11, OutputTokens: 22, CacheReadTokens: 33, CacheWriteTokens: 44,
		CacheWrite5mTokens: 55, CacheWrite1hTokens: 66, ThinkingTokens: 77}
	if got := addUsage(a, b); got != want {
		t.Fatalf("addUsage = %+v, want %+v", got, want)
	}
	// ThinkingTokensMeasured is a flag, not a counter: it must survive if either
	// attempt measured, and must not be invented when neither did.
	for _, tc := range []struct{ a, b, want bool }{
		{false, false, false},
		{true, false, true},
		{false, true, true},
		{true, true, true},
	} {
		a.ThinkingTokensMeasured, b.ThinkingTokensMeasured = tc.a, tc.b
		if got := addUsage(a, b).ThinkingTokensMeasured; got != tc.want {
			t.Fatalf("addUsage(%v, %v).ThinkingTokensMeasured = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// An attempt that failed without spending anything carries no UsageError; the
// fold must leave the successful response untouched rather than zero it.
func TestFoldFailedAttemptsIgnoresUnbilledErrors(t *testing.T) {
	resp := &Response{Usage: Usage{InputTokens: 9}, ProviderCalls: 1}
	foldFailedAttempts(resp, []error{fmt.Errorf("no usage here")})
	if resp.Usage.InputTokens != 9 || resp.ProviderCalls != 1 {
		t.Fatalf("an unbilled failure changed the response: %+v", resp)
	}
}
