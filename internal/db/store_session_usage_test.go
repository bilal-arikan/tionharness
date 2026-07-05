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

	if err := d.AddSessionUsageKind(ctx, sid, agent, UsageKindChat, "anthropic", "claude-opus-4-8", UsageDelta{Calls: 1, InputTokens: 100, OutputTokens: 30}); err != nil {
		t.Fatal(err)
	}
	if err := d.AddSessionUsageKind(ctx, sid, agent, UsageKindCompact, "anthropic", "claude-haiku-4-5", UsageDelta{Calls: 1, InputTokens: 200, OutputTokens: 10, CacheReadTokens: 500}); err != nil {
		t.Fatal(err)
	}
	// Blank session id must be a no-op (not attributed anywhere).
	if err := d.AddSessionUsageKind(ctx, "", agent, UsageKindChat, "anthropic", "x", UsageDelta{Calls: 9, InputTokens: 9}); err != nil {
		t.Fatal(err)
	}
	if err := d.AddSessionCompactionSavings(ctx, sid, agent, 1200); err != nil {
		t.Fatal(err)
	}
	if err := d.AddSessionLLMCompactionSavings(ctx, sid, agent, 800); err != nil {
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
	if c := u.ByKind[UsageKindChat]; c.Calls != 1 || c.InputTokens != 100 {
		t.Errorf("chat bucket=%+v, want 1/100/30", c)
	}
	if m := u.ByModel[ModelKey("anthropic", "claude-haiku-4-5")]; m.Calls != 1 || m.CacheReadTokens != 500 {
		t.Errorf("haiku model bucket=%+v, want 1 call / 500 cacheRead", m)
	}
	if u.CompactSavedBytes != 1200 || u.CompactSavedBytesLLM != 800 {
		t.Errorf("savings A=%d B=%d, want 1200/800", u.CompactSavedBytes, u.CompactSavedBytesLLM)
	}
	// Savings are standalone — token counters untouched by them.
	if u.InputTokens != 300 {
		t.Errorf("savings must not touch token counters: in=%d", u.InputTokens)
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
	if u2.Calls != 2 || u2.CompactSavedBytes != 1200 || u2.CompactSavedBytesLLM != 800 {
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

// TestAddLLMCompactionSavings verifies System B's byte savings accumulate into
// today's per-agent rollup independently of token counters and System A's meter.
func TestAddLLMCompactionSavings(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	const agent = "AGT1"

	if err := d.AddLLMCompactionSavings(ctx, agent, 500); err != nil {
		t.Fatal(err)
	}
	if err := d.AddLLMCompactionSavings(ctx, agent, 300); err != nil {
		t.Fatal(err)
	}
	if err := d.AddCompactionSavings(ctx, agent, 1000); err != nil { // System A meter, separate
		t.Fatal(err)
	}
	// Non-positive deltas are no-ops.
	if err := d.AddLLMCompactionSavings(ctx, agent, 0); err != nil {
		t.Fatal(err)
	}

	u, err := d.GetUsageToday(ctx, agent)
	if err != nil {
		t.Fatal(err)
	}
	if u.CompactSavedBytesLLM != 800 {
		t.Fatalf("CompactSavedBytesLLM=%d, want 800", u.CompactSavedBytesLLM)
	}
	if u.CompactSavedBytes != 1000 {
		t.Fatalf("System A meter leaked: CompactSavedBytes=%d, want 1000", u.CompactSavedBytes)
	}
}
