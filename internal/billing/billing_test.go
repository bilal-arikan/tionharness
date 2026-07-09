package billing

import (
	"math"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func approxUSD(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestPriceStat covers the single pricing primitive every budget surface routes
// through: real list price, equivalent-API estimate for subscription providers,
// the no-spend short-circuit, and a fully unpriced/unknown model.
func TestPriceStat(t *testing.T) {
	// Real price (anthropic opus: 5 in / 25 out per Mtok), with a 1M cache read.
	cost, save, priced, est := PriceStat("anthropic", "claude-opus-4-8",
		db.KindStat{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000})
	if !priced || est {
		t.Errorf("anthropic: priced=%v est=%v, want true/false", priced, est)
	}
	// 5 (input) + 25 (output) + 5*0.10 (cache read) = 30.5
	if !approxUSD(cost, 30.5) {
		t.Errorf("anthropic cost = %v, want 30.5", cost)
	}
	if !approxUSD(save, 4.5) { // 5 * 0.90 per 1M
		t.Errorf("anthropic savings = %v, want 4.5", save)
	}

	// Subscription provider (claude-cli) → unpriced but equivalent-API estimated.
	// Cache read/write are estimated too: cost includes the discounted cache tiers
	// AND the savings figure is non-zero (regression guard: estimated savings used
	// to be dropped to 0, hiding cache ROI for the default keyless provider).
	cost, save, priced, est = PriceStat("claude-cli", "claude-sonnet-4-6",
		db.KindStat{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000, CacheWriteTokens: 1_000_000})
	if priced || !est {
		t.Errorf("claude-cli: priced=%v est=%v, want false/true", priced, est)
	}
	// sonnet 3 in + 15 out + cache-read 3*0.10 (0.3) + cache-write 3*1.25 (3.75).
	// The write uses the STANDARD 5-min tier (1.25×), NOT the anthropic table's
	// 1-hour override (2×) — that premium is specific to the native client.
	if !approxUSD(cost, 22.05) {
		t.Errorf("claude-cli estimate = %v, want 22.05 (5-min cache-write tier)", cost)
	}
	if !approxUSD(save, 2.7) { // 3 * 0.90 per 1M cache read
		t.Errorf("claude-cli estimated savings = %v, want 2.7", save)
	}

	// No spend → priced=true, cost=0 (must not flag the rollup as unpriced).
	cost, _, priced, est = PriceStat("claude-cli", "whatever", db.KindStat{})
	if !priced || est || cost != 0 {
		t.Errorf("empty stat: cost=%v priced=%v est=%v, want 0/true/false", cost, priced, est)
	}

	// Unknown model with real spend and no estimate → unpriced, not estimated.
	cost, _, priced, est = PriceStat("openrouter", "some/unlisted-model",
		db.KindStat{InputTokens: 100, OutputTokens: 10})
	if priced || est || cost != 0 {
		t.Errorf("unlisted: cost=%v priced=%v est=%v, want 0/false/false", cost, priced, est)
	}

	// No-caching baseline resolves like the cost: real price for anthropic, the
	// equivalent-API estimate for claude-cli, 0 for unpriced/unknown.
	if got := NoCacheCost("anthropic", "claude-opus-4-8",
		db.KindStat{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000, CacheWriteTokens: 1_000_000}); !approxUSD(got, 40) {
		t.Errorf("anthropic no-cache = %v, want 40", got)
	}
	// claude-cli sonnet estimate: (1M in + 1M write)*3 + 1M out*15 = 6 + 15 = 21.
	if got := NoCacheCost("claude-cli", "claude-sonnet-4-6",
		db.KindStat{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheWriteTokens: 1_000_000}); !approxUSD(got, 21) {
		t.Errorf("claude-cli no-cache = %v, want 21", got)
	}
	if got := NoCacheCost("openrouter", "some/unlisted-model", db.KindStat{InputTokens: 100}); got != 0 {
		t.Errorf("unlisted no-cache = %v, want 0", got)
	}
}

// TestRollupOf verifies the merged costOf/modelRowsFor primitive: rows sorted
// costliest first, aggregate cost/cache totals, and the priced/estimated flags
// reflecting a mix of priced + unpriced spend.
func TestRollupOf(t *testing.T) {
	roll := RollupOf(map[string]db.KindStat{
		// Cheap haiku (1 in / 5 out): 1M+1M = 6 USD.
		"anthropic|claude-haiku-4-5-20251001": {Calls: 1, InputTokens: 1_000_000, OutputTokens: 1_000_000},
		// Pricey opus (5 in / 25 out): 1M+1M = 30 USD.
		"anthropic|claude-opus-4-8": {Calls: 1, InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 500_000},
		// Unpriced/unknown model with real spend → flips Priced false.
		"openrouter|some/unknown": {Calls: 1, InputTokens: 100, OutputTokens: 10},
	})

	if len(roll.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(roll.Rows))
	}
	// Costliest first: opus (30) > haiku (6) > unknown (0).
	if roll.Rows[0].Model != "claude-opus-4-8" || roll.Rows[2].Model != "some/unknown" {
		t.Errorf("sort order wrong: %s ... %s", roll.Rows[0].Model, roll.Rows[2].Model)
	}
	// Aggregate cost = 30 + 6 + opus cache read (5 * 0.10 * 0.5M/1M = 0.25) = 36.25.
	if !approxUSD(roll.CostUSD, 36.25) {
		t.Errorf("total cost = %v, want 36.25", roll.CostUSD)
	}
	if roll.CacheReadTokens != 500_000 {
		t.Errorf("cacheRead = %d, want 500000", roll.CacheReadTokens)
	}
	// No-caching baseline: cache tokens billed as fresh input, no discount/premium.
	// haiku (1/5): 1M in + 1M out = 6. opus (5/25): (1M in + 0.5M read)*5 + 1M out*25
	// = 7.5 + 25 = 32.5. unknown: unpriced → 0. Total = 38.5 (≥ CostUSD 36.25).
	if !approxUSD(roll.NoCacheCostUSD, 38.5) {
		t.Errorf("no-cache cost = %v, want 38.5", roll.NoCacheCostUSD)
	}
	if roll.Priced {
		t.Error("Priced should be false (unknown model has real spend)")
	}
	if roll.Estimated {
		t.Error("Estimated should be false (no subscription provider here)")
	}
}
