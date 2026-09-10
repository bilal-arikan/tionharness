package api

import (
	"context"
	"errors"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// fakeCounter is a TokenCounter that answers a fixed count (or error) and
// records how often it was asked.
type fakeCounter struct {
	n     int
	err   error
	calls int
	last  providers.Request
}

func (f *fakeCounter) CountTokens(ctx context.Context, req providers.Request) (int, error) {
	f.calls++
	f.last = req
	return f.n, f.err
}

func TestExactPrefixTokensCountsOnceAndReusesCachedCount(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	agent := db.Agent{Provider: "anthropic", Model: "claude-sonnet-5"}
	defs := []providers.ToolDef{{Name: "bash", Description: "Run"}}
	counter := &fakeCounter{n: 4321}

	// Read-only path never counts on a miss.
	if _, ok := exactPrefixTokens(ctx, database, counter, agent, "sys", defs, false); ok || counter.calls != 0 {
		t.Fatalf("read-only miss counted: ok=%v calls=%d", ok, counter.calls)
	}
	// Turn path counts exactly what the provider would receive as prefix.
	got, ok := exactPrefixTokens(ctx, database, counter, agent, "sys", defs, true)
	if !ok || got != 4321 || counter.calls != 1 {
		t.Fatalf("first count = %d ok=%v calls=%d", got, ok, counter.calls)
	}
	if counter.last.System != "sys" || len(counter.last.Tools) != 1 || counter.last.Model != "claude-sonnet-5" {
		t.Fatalf("count request = %+v", counter.last)
	}
	// Every later call, read-only included, is served from the cache.
	if got, ok = exactPrefixTokens(ctx, database, counter, agent, "sys", defs, false); !ok || got != 4321 || counter.calls != 1 {
		t.Fatalf("cached read = %d ok=%v calls=%d", got, ok, counter.calls)
	}
	if got, ok = exactPrefixTokens(ctx, database, counter, agent, "sys", defs, true); !ok || got != 4321 || counter.calls != 1 {
		t.Fatalf("cached turn read = %d ok=%v calls=%d", got, ok, counter.calls)
	}
	// A changed prefix is a new key → one more count.
	counter.n = 5000
	if got, ok = exactPrefixTokens(ctx, database, counter, agent, "sys v2", defs, true); !ok || got != 5000 || counter.calls != 2 {
		t.Fatalf("changed prefix = %d ok=%v calls=%d", got, ok, counter.calls)
	}
	// Nothing to count.
	if _, ok = exactPrefixTokens(ctx, database, counter, agent, "", nil, true); ok {
		t.Fatal("empty prefix reported a count")
	}
	if _, ok = exactPrefixTokens(ctx, database, nil, agent, "sys", defs, true); ok {
		t.Fatal("nil counter reported a count")
	}
}

func TestExactPrefixTokensBacksOffAfterFailure(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	agent := db.Agent{Provider: "anthropic", Model: "claude-opus-5"}
	counter := &fakeCounter{err: errors.New("boom")}
	if _, ok := exactPrefixTokens(ctx, database, counter, agent, "failing prefix", nil, true); ok || counter.calls != 1 {
		t.Fatalf("failure reported ok=%v calls=%d", ok, counter.calls)
	}
	// Within the back-off window the same prefix is not retried.
	counter.err, counter.n = nil, 10
	if _, ok := exactPrefixTokens(ctx, database, counter, agent, "failing prefix", nil, true); ok || counter.calls != 1 {
		t.Fatalf("retried inside back-off: ok=%v calls=%d", ok, counter.calls)
	}
}

func TestApplyPrefixCalibrationRescalesOnlyPrefixBuckets(t *testing.T) {
	fillers := []contextFiller{
		{Role: "system", Tokens: 600},
		{Role: "skills", Tokens: 300},
		{Role: "tools", Tokens: 100},
		{Role: "artifacts", Tokens: 50},
		{Role: "tool-activity", Tokens: 25},
	}
	out := applyPrefixCalibration(fillers, 2001)
	sum := 0
	for _, f := range out {
		switch f.Role {
		case "system", "skills", "tools":
			if !f.Calibrated {
				t.Fatalf("%s not marked calibrated", f.Role)
			}
			sum += f.Tokens
		default:
			if f.Calibrated {
				t.Fatalf("%s must stay heuristic", f.Role)
			}
		}
	}
	if sum != 2001 {
		t.Fatalf("prefix buckets sum to %d, want exactly 2001", sum)
	}
	// Shares are preserved: system is still 2x skills, 6x tools.
	if out[0].Tokens < out[1].Tokens*2-2 || out[0].Tokens > out[1].Tokens*2+2 {
		t.Fatalf("system/skills ratio drifted: %d vs %d", out[0].Tokens, out[1].Tokens)
	}
	if out[3].Tokens != 50 || out[4].Tokens != 25 {
		t.Fatalf("non-prefix buckets changed: %+v", out[3:])
	}
	// No prefix buckets / non-positive exact: untouched.
	only := []contextFiller{{Role: "artifacts", Tokens: 7}}
	if got := applyPrefixCalibration(only, 100); got[0].Tokens != 7 || got[0].Calibrated {
		t.Fatalf("bucket without prefix roles changed: %+v", got[0])
	}
	if got := applyPrefixCalibration([]contextFiller{{Role: "system", Tokens: 9}}, 0); got[0].Tokens != 9 {
		t.Fatalf("zero exact rescaled: %+v", got[0])
	}
}

func TestScaleToExactSumsExactlyAndKeepsShares(t *testing.T) {
	a, b, c := 700, 200, 100
	scaleToExact(1003, &a, &b, &c)
	if a+b+c != 1003 {
		t.Fatalf("sum = %d, want 1003", a+b+c)
	}
	if a < b*3 || c > b {
		t.Fatalf("shares drifted: %d %d %d", a, b, c)
	}
	z1, z2 := 0, 0
	scaleToExact(50, &z1, &z2)
	if z1 != 0 || z2 != 0 {
		t.Fatal("zero-sum parts must stay untouched")
	}
}
