package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestAddUsageKind_BreaksDownByOrigin verifies that per-kind sub-counters and the
// grand total stay in sync across several origins, and that the plain AddUsage
// shim buckets under "other".
func TestAddUsageKind_BreaksDownByOrigin(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	const agent = "agent-1"

	if err := d.AddUsageKind(ctx, agent, UsageKindChat, "anthropic", "claude-opus-4-8", UsageDelta{Calls: 1, InputTokens: 100, OutputTokens: 30, CacheWriteTokens: 120, CacheWrite5mTokens: 20, CacheWrite1hTokens: 100}); err != nil {
		t.Fatal(err)
	}
	if err := d.AddUsageKind(ctx, agent, UsageKindCompact, "anthropic", "claude-haiku-4-5-20251001", UsageDelta{Calls: 1, InputTokens: 200, OutputTokens: 10}); err != nil {
		t.Fatal(err)
	}
	if err := d.AddUsageKind(ctx, agent, UsageKindChat, "anthropic", "claude-opus-4-8", UsageDelta{Calls: 1, InputTokens: 50, OutputTokens: 20, CacheReadTokens: 500}); err != nil {
		t.Fatal(err)
	}
	if err := d.AddUsage(ctx, agent, 1, 5, 5); err != nil { // no kind → "other"
		t.Fatal(err)
	}

	u, err := d.GetUsageToday(ctx, agent)
	if err != nil {
		t.Fatal(err)
	}

	// Grand totals = sum of every call.
	if u.Calls != 4 || u.InputTokens != 355 || u.OutputTokens != 65 {
		t.Fatalf("totals: calls=%d in=%d out=%d, want 4/355/65", u.Calls, u.InputTokens, u.OutputTokens)
	}
	if u.CacheWriteTokens != 120 || u.CacheWrite5mTokens != 20 || u.CacheWrite1hTokens != 100 {
		t.Fatalf("cache write totals=%d/%d/%d, want 120/20/100", u.CacheWriteTokens, u.CacheWrite5mTokens, u.CacheWrite1hTokens)
	}
	if m := u.ByModel[ModelKey("anthropic", "claude-opus-4-8")]; m.CacheWrite5mTokens != 20 || m.CacheWrite1hTokens != 100 {
		t.Fatalf("cache write model breakdown lost: %+v", m)
	}
	// Chat bucket aggregates its two calls.
	if c := u.ByKind[UsageKindChat]; c.Calls != 2 || c.InputTokens != 150 || c.OutputTokens != 50 {
		t.Errorf("chat bucket=%+v, want 2/150/50", c)
	}
	// Compaction — previously invisible — is now attributed.
	if c := u.ByKind[UsageKindCompact]; c.Calls != 1 || c.InputTokens != 200 {
		t.Errorf("compact bucket=%+v, want 1/200/10", c)
	}
	// Untagged calls fall back to "other".
	if c := u.ByKind[UsageKindOther]; c.Calls != 1 {
		t.Errorf("other bucket=%+v, want 1 call", c)
	}

	// Per-model breakdown: the two opus chat calls aggregate under one key.
	if m := u.ByModel[ModelKey("anthropic", "claude-opus-4-8")]; m.Calls != 2 || m.InputTokens != 150 {
		t.Errorf("opus model bucket=%+v, want 2/150/50", m)
	}
	if m := u.ByModel[ModelKey("anthropic", "claude-haiku-4-5-20251001")]; m.Calls != 1 || m.InputTokens != 200 {
		t.Errorf("haiku model bucket=%+v, want 1/200/10", m)
	}

	// The per-kind input tokens must sum back to the grand total.
	var sum int
	for _, c := range u.ByKind {
		sum += c.InputTokens
	}
	if sum != u.InputTokens {
		t.Errorf("sum of per-kind input=%d != total input=%d", sum, u.InputTokens)
	}
}

func TestAddUsageKind_SystemAndOperationKindsDoNotDoubleCount(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	const agent = "agent-1"

	if got := SystemUsageKind("system:title"); got != "system:title" {
		t.Fatalf("SystemUsageKind(system:title) = %q, want %q", got, "system:title")
	}

	systemDelta := UsageDelta{Calls: 1, InputTokens: 120}
	if err := d.AddUsageKind(ctx, agent, "system:title", "anthropic", "system-model", systemDelta); err != nil {
		t.Fatalf("add system usage: %v", err)
	}
	assertUsageKindTotals(t, d, ctx, agent, map[string]UsageDelta{
		"system:title": systemDelta,
	}, "title")

	titleDelta := UsageDelta{Calls: 2, InputTokens: 80}
	if err := d.AddUsageKind(ctx, agent, UsageKindTitle, "anthropic", "regular-model", titleDelta); err != nil {
		t.Fatalf("add regular usage: %v", err)
	}
	assertUsageKindTotals(t, d, ctx, agent, map[string]UsageDelta{
		"system:title": systemDelta,
		UsageKindTitle: titleDelta,
	})
}

func assertUsageKindTotals(t *testing.T, d *DB, ctx context.Context, agent string, expected map[string]UsageDelta, absentKinds ...string) {
	t.Helper()
	u, err := d.GetUsageToday(ctx, agent)
	if err != nil {
		t.Fatalf("get usage: %v", err)
	}
	for kind, want := range expected {
		got, ok := u.ByKind[kind]
		if !ok {
			t.Fatalf("ByKind[%q] missing", kind)
		}
		if got.Calls != want.Calls || got.InputTokens != want.InputTokens {
			t.Fatalf("ByKind[%q] calls/input = %d/%d, want %d/%d", kind, got.Calls, got.InputTokens, want.Calls, want.InputTokens)
		}
	}
	for _, kind := range absentKinds {
		if _, ok := u.ByKind[kind]; ok {
			t.Fatalf("ByKind[%q] unexpectedly present", kind)
		}
	}
	var calls, inputTokens int
	for _, stat := range u.ByKind {
		calls += stat.Calls
		inputTokens += stat.InputTokens
	}
	if calls != u.Calls {
		t.Fatalf("ByKind calls sum = %d, usage calls = %d", calls, u.Calls)
	}
	if inputTokens != u.InputTokens {
		t.Fatalf("ByKind input token sum = %d, usage input tokens = %d", inputTokens, u.InputTokens)
	}
}
