package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSessionUsage verifies per-session lifetime attribution: calls accumulate
// across origins, the per-model breakdown aggregates, compaction byte meters are
// independent of token counters, a blank session id is a no-op, and the rollup
// survives a reload from disk.
func TestSessionUsage(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store")
	d, err := Open(storePath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	const sid = "SES1"
	const agent = "AGT1"

	if err := d.AddSessionUsageKind(ctx, sid, agent, UsageKindChat, "anthropic", "claude-opus-4-8", UsageDelta{Calls: 1, InputTokens: 100, OutputTokens: 30, CacheWriteTokens: 120, CacheWrite5mTokens: 20, CacheWrite1hTokens: 100}); err != nil {
		t.Fatal(err)
	}
	if err := d.AddSessionUsageKind(ctx, sid, agent, UsageKindCompact, "anthropic", "claude-haiku-4-5", UsageDelta{Calls: 1, InputTokens: 200, OutputTokens: 10, CacheReadTokens: 500}); err != nil {
		t.Fatal(err)
	}
	// Blank session id must be a no-op (not attributed anywhere).
	if err := d.AddSessionUsageKind(ctx, "", agent, UsageKindChat, "anthropic", "x", UsageDelta{Calls: 9, InputTokens: 9}); err != nil {
		t.Fatal(err)
	}

	u, err := d.GetSessionUsage(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if u.Calls != 2 || u.InputTokens != 300 || u.OutputTokens != 40 {
		t.Fatalf("totals: calls=%d in=%d out=%d, want 2/300/40", u.Calls, u.InputTokens, u.OutputTokens)
	}
	if u.AgentID != agent {
		t.Errorf("agentID=%q, want %q", u.AgentID, agent)
	}
	if u.CacheWriteTokens != 120 || u.CacheWrite5mTokens != 20 || u.CacheWrite1hTokens != 100 {
		t.Fatalf("cache write totals=%d/%d/%d, want 120/20/100", u.CacheWriteTokens, u.CacheWrite5mTokens, u.CacheWrite1hTokens)
	}
	if c := u.ByKind[UsageKindChat]; c.Calls != 1 || c.InputTokens != 100 {
		t.Errorf("chat bucket=%+v, want 1/100/30", c)
	}
	if m := u.ByModel[ModelKey("anthropic", "claude-haiku-4-5")]; m.Calls != 1 || m.CacheReadTokens != 500 {
		t.Errorf("haiku model bucket=%+v, want 1 call / 500 cacheRead", m)
	}

	// Survives reload.
	d2, err := Open(storePath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	u2, err := d2.GetSessionUsage(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if u2.Calls != 2 || u2.CacheWrite5mTokens != 20 || u2.CacheWrite1hTokens != 100 {
		t.Fatalf("after reload: %+v", u2)
	}
}

// TestSessionUsageProviderCalls verifies the cumulative internal-round-trip
// counter: a claude-cli turn reports ProviderCalls=num_turns, a recorder that
// omits it (native, compaction) is floored to its Calls, and the total lets a
// consumer divide cumulative tokens down to a per-call figure.
func TestSessionUsageProviderCalls(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	const sid, agent = "SES1", "AGT1"

	// claude-cli turn: one TionSwarm call (Calls=1) = 3 internal round-trips.
	if err := d.AddSessionUsageKind(ctx, sid, agent, UsageKindChat, "claude-cli", "claude-opus-4-8",
		UsageDelta{Calls: 1, InputTokens: 10, CacheReadTokens: 90000, CacheWriteTokens: 30000, ProviderCalls: 3}); err != nil {
		t.Fatal(err)
	}
	// A recorder that doesn't report ProviderCalls (0) must floor to Calls (1).
	if err := d.AddSessionUsageKind(ctx, sid, agent, UsageKindCompact, "anthropic", "claude-haiku-4-5",
		UsageDelta{Calls: 1, InputTokens: 200}); err != nil {
		t.Fatal(err)
	}

	u, err := d.GetSessionUsage(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if u.Calls != 2 {
		t.Fatalf("Calls=%d, want 2 (TionSwarm turns)", u.Calls)
	}
	if u.ProviderCalls != 4 { // 3 (cli) + 1 (floored)
		t.Fatalf("ProviderCalls=%d, want 4 (3 + floored 1)", u.ProviderCalls)
	}
	// Per-call context from the cumulative totals ÷ ProviderCalls.
	perCall := (u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens) / u.ProviderCalls
	if perCall != (210+90000+30000)/4 {
		t.Fatalf("per-call=%d, want %d", perCall, (210+90000+30000)/4)
	}
}
