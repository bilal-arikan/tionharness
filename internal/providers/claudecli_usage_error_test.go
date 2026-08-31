package providers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// The parser recovering usage on an error result is only half the fix: the turn
// returns (nil, err), so unless the failure CARRIES that usage the caller bills
// the most expensive turns as zero. Assert the outward contract — what finish
// hands back — not the parser's internal fields.
func TestCLIErrorResultCarriesUsageOutOfTheProvider(t *testing.T) {
	p := newCLIParser("opus", nil)
	p.feed(`{"type":"result","is_error":true,"result":"Not logged in · Please run /login",` +
		`"num_turns":2,"usage":{"input_tokens":4321,"output_tokens":9,"cache_read_input_tokens":700}}`)

	resp, err := p.finish()
	if err == nil {
		t.Fatal("an is_error result must still fail the turn")
	}
	if resp != nil {
		t.Fatalf("a failed turn must not return a response: %+v", resp)
	}
	ue, ok := UsageFromError(err)
	if !ok {
		t.Fatalf("the failure dropped the turn's usage: %v", err)
	}
	if ue.Usage.InputTokens != 4321 || ue.Usage.OutputTokens != 9 || ue.Usage.CacheReadTokens != 700 {
		t.Fatalf("usage on the error = %+v, want the result envelope's totals", ue.Usage)
	}
	if ue.ProviderCalls != 2 {
		t.Fatalf("providerCalls = %d, want 2", ue.ProviderCalls)
	}
	if ue.Model != "opus" {
		t.Fatalf("model = %q, want opus", ue.Model)
	}
	// The failure must stay a failure: usage rides along, it does not soften the
	// classification the caller retries (or refuses to retry) on.
	if !strings.Contains(err.Error(), "Not logged in") {
		t.Fatalf("error text lost the CLI's reason: %v", err)
	}
	if !p.notLoggedIn {
		t.Fatal("auth classification was lost")
	}
}

// A turn that spent nothing must not fabricate a usage record.
func TestUsageErrorNotAttachedWithoutUsage(t *testing.T) {
	p := newCLIParser("opus", nil)
	p.feed(`{"type":"result","is_error":true,"result":"boom"}`)
	_, err := p.finish()
	if err == nil {
		t.Fatal("expected an error")
	}
	if ue, ok := UsageFromError(err); ok {
		t.Fatalf("zero-usage failure was wrapped anyway: %+v", ue)
	}
}

// WithUsage keeps the innermost measurement: re-wrapping at each layer would let
// an outer (staler) usage shadow the parser's.
func TestWithUsageDoesNotDoubleWrap(t *testing.T) {
	inner := WithUsage(errors.New("boom"), "opus", Usage{InputTokens: 10}, 1)
	outer := WithUsage(inner, "haiku", Usage{InputTokens: 999}, 7)
	ue, ok := UsageFromError(outer)
	if !ok {
		t.Fatal("usage lost")
	}
	if ue.Usage.InputTokens != 10 || ue.Model != "opus" || ue.ProviderCalls != 1 {
		t.Fatalf("outer wrap shadowed the inner measurement: %+v", ue)
	}
}

// is_error decides whether the turn succeeded. If salvage cannot decode it, the
// turn must fail — before the tolerant parser the malformed envelope was dropped
// whole and the turn failed hard, so keeping the event while losing this field
// would trade a hard failure for a silent FALSE SUCCESS.
func TestCLIUndecodableIsErrorFailsTheTurn(t *testing.T) {
	p := newCLIParser("", nil)
	// is_error as an object: unmodellable into bool, so salvage drops the field.
	p.feed(`{"type":"result","is_error":{"unexpected":"object"},"session_id":"s9",` +
		`"result":"partial answer","usage":{"input_tokens":50,"output_tokens":3}}`)

	if !p.hadError {
		t.Fatal("an unreadable is_error was treated as success")
	}
	resp, err := p.finish()
	if err == nil {
		t.Fatalf("finish reported success for an envelope with no readable is_error: %+v", resp)
	}
	if !strings.Contains(err.Error(), "is_error") {
		t.Fatalf("the failure does not name the dropped field: %v", err)
	}
	// The rest of the envelope still survives salvage — including the usage that
	// must reach the caller's accounting.
	if p.resp.SessionID != "s9" {
		t.Fatalf("session id lost: %q", p.resp.SessionID)
	}
	ue, ok := UsageFromError(err)
	if !ok || ue.Usage.InputTokens != 50 {
		t.Fatalf("usage lost on the fail-closed path: %v", err)
	}
	var note string
	for _, s := range p.resp.Trace {
		if strings.Contains(s.Text, "[claude-cli field drop]") {
			note = s.Text
		}
	}
	if !strings.Contains(note, "is_error") {
		t.Fatalf("the field drop was not reported by name; trace=%+v", p.resp.Trace)
	}
}

// A result envelope whose `type` cannot be decoded needs no special handling: the
// event matches no case, so the turn ends with "no result in stream" — it already
// fails closed. Pinned so a future change to the switch cannot turn that into a
// silent success.
func TestCLIUndecodableTypeStillFailsTheTurn(t *testing.T) {
	p := newCLIParser("", nil)
	p.feed(`{"type":{"unexpected":"object"},"is_error":false,"result":"done"}`)
	if _, err := p.finish(); err == nil {
		t.Fatal("an envelope with no readable type was accepted as a completed turn")
	}
}

// The startup watchdog was unreachable in tests while its window was a const, so
// the WatchdogReasonStartup branch shipped unexercised.
func TestCLISessionStartupWatchdogUsesInjectedWindow(t *testing.T) {
	s, _ := newTestCLISession(t)
	prev := cliStartupTimeout()
	SetCLIStartupTimeout(80 * time.Millisecond)
	t.Cleanup(func() { SetCLIStartupTimeout(prev) })

	var kills []WatchdogKill
	req := Request{OnWatchdog: func(k WatchdogKill) { kills = append(kills, k) }}

	done := make(chan error, 1)
	go func() {
		// No stdout is ever written: the turn never produces a first line.
		_, err := s.Turn(context.Background(), "hello", req, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a turn that never emitted anything returned success")
		}
		if !strings.Contains(err.Error(), "produced no output") {
			t.Fatalf("err = %v, want the startup-hang failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("startup watchdog never fired")
	}
	if len(kills) != 1 || kills[0].Reason != WatchdogReasonStartup {
		t.Fatalf("watchdog records = %+v, want one startup kill", kills)
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if !closed {
		t.Fatal("startup-killed session was left open")
	}
}
