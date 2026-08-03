package providers

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestCostDetailed_CacheTiers verifies fresh input, cache-read (cheap),
// cache-write (premium) and output are each priced at the right multiplier.
func TestCostDetailed_CacheTiers(t *testing.T) {
	p, ok := PriceFor("anthropic", "claude-opus-4-8") // 5 in / 25 out per Mtok
	if !ok {
		t.Fatal("opus price missing")
	}

	// 1M fresh input = $5; 1M output = $25.
	if got := p.Cost(1_000_000, 1_000_000); !approx(got, 30) {
		t.Errorf("plain cost = %v, want 30", got)
	}
	// 1M cache-read = 5 * 0.10 = $0.50.
	if got := p.CostDetailed(0, 0, 1_000_000, 0); !approx(got, 0.5) {
		t.Errorf("cache-read cost = %v, want 0.5", got)
	}
	// 1M cache-write = 5 * 2.0 = $10 — the native anthropic client always
	// requests the 1-hour extended TTL, whose write premium is 2× (not the
	// standard 5-minute tier's 1.25×).
	if got := p.CostDetailed(0, 0, 0, 1_000_000); !approx(got, 10) {
		t.Errorf("cache-write cost = %v, want 10 (1h TTL premium)", got)
	}
	// Savings on 1M cache-read = 5 * 0.90 = $4.50.
	if got := p.CacheSavings(1_000_000); !approx(got, 4.5) {
		t.Errorf("savings = %v, want 4.5", got)
	}
	// No-caching baseline: cache read+write billed as fresh input, no discount/premium.
	// (1M in + 1M read + 1M write)*5 + 1M out*25 = 15 + 25 = $40. Note this is LESS
	// than the real cost (40.5) here because the 2× write premium on a cold write
	// exceeds full-price input — caching only wins once the prefix is re-read.
	if got := p.CostNoCaching(1_000_000, 1_000_000, 1_000_000, 1_000_000); !approx(got, 40) {
		t.Errorf("no-cache cost = %v, want 40", got)
	}
}

// TestCoolingWaste verifies the avoidable-overpay figure for a cold re-write:
// the write-tier price minus the read tier a timely turn would have paid.
func TestCoolingWaste(t *testing.T) {
	p, ok := PriceFor("anthropic", "claude-opus-4-8") // $5/Mtok in, 1h write 2×, read 0.10×
	if !ok {
		t.Fatal("opus price missing")
	}
	// 1M re-written prefix: paid 2× ($10), would have read at 0.10× ($0.50) warm.
	// Avoidable waste = 5 * (2.0 - 0.10) = $9.50 per 1M.
	if got := p.CoolingWasteUSD(1_000_000); !approx(got, 9.5) {
		t.Errorf("cooling waste = %v, want 9.5", got)
	}
	if got := p.CoolingWasteUSD(0); got != 0 {
		t.Errorf("zero tokens should waste nothing, got %v", got)
	}

	// Resolver: real list price → not estimated.
	usd, est, ok := CoolingWaste("anthropic", "claude-opus-4-8", 1_000_000)
	if !ok || est || !approx(usd, 9.5) {
		t.Errorf("CoolingWaste(anthropic) = %v est=%v ok=%v, want 9.5/false/true", usd, est, ok)
	}
	// claude-cli is a subscription → estimate via the Anthropic list price, but the
	// write premium falls back to the 5-minute 1.25× tier (Claude Code's own TTL),
	// so waste = 5 * (1.25 - 0.10) = $5.75 per 1M, flagged estimated.
	usd, est, ok = CoolingWaste("claude-cli", "claude-opus-4-8", 1_000_000)
	if !ok || !est || !approx(usd, 5.75) {
		t.Errorf("CoolingWaste(claude-cli) = %v est=%v ok=%v, want 5.75/true/true", usd, est, ok)
	}
	// Unpriced/custom endpoint → no figure.
	if _, _, ok := CoolingWaste("custom", "whatever", 1_000_000); ok {
		t.Error("unpriced provider should report ok=false")
	}
}

// TestCacheMultOverride verifies a per-model cache multiplier overrides the
// package default (OpenRouter's general 0.25× read tier vs Anthropic's 0.10×).
func TestCacheMultOverride(t *testing.T) {
	// Base input $10/Mtok, cache-read override 0.25 → 1M read = $2.50 (not $1.00).
	p := Price{InputPerMTok: 10, OutputPerMTok: 30, CacheReadMultOverride: 0.25}
	if got := p.CostDetailed(0, 0, 1_000_000, 0); !approx(got, 2.5) {
		t.Errorf("override cache-read cost = %v, want 2.5", got)
	}
	// Savings = 10 * (1 - 0.25) = $7.50 per 1M (vs $9 at the 0.10 default).
	if got := p.CacheSavings(1_000_000); !approx(got, 7.5) {
		t.Errorf("override savings = %v, want 7.5", got)
	}
	// Zero override falls back to the package default (0.10 → $1.00).
	d := Price{InputPerMTok: 10, OutputPerMTok: 30}
	if got := d.CostDetailed(0, 0, 1_000_000, 0); !approx(got, 1.0) {
		t.Errorf("default cache-read cost = %v, want 1.0", got)
	}
}

// TestPriceFor_OpenRouter confirms the namespaced OpenRouter models are priced
// with Anthropic's pass-through cache tier.
func TestPriceFor_OpenRouter(t *testing.T) {
	p, ok := PriceFor("openrouter", "anthropic/claude-sonnet-4.6")
	if !ok {
		t.Fatal("openrouter default model should be priced")
	}
	if p.InputPerMTok != 3 || p.OutputPerMTok != 15 {
		t.Errorf("unexpected price: in=%v out=%v", p.InputPerMTok, p.OutputPerMTok)
	}
	if p.cacheReadMult() != 0.10 {
		t.Errorf("anthropic-routed cache-read mult = %v, want 0.10", p.cacheReadMult())
	}
	if _, ok := PriceFor("openrouter", "some/unlisted-model"); ok {
		t.Error("unlisted openrouter model should be unpriced (ballpark screen)")
	}
}

// TestPriceFor_Sonnet5 pins the Sonnet 5 list price on both the anthropic and
// openrouter tables (standard $3/$15, Anthropic pass-through cache tier).
func TestPriceFor_Sonnet5(t *testing.T) {
	p, ok := PriceFor("anthropic", "claude-sonnet-5")
	if !ok || p.InputPerMTok != 3 || p.OutputPerMTok != 15 {
		t.Errorf("anthropic sonnet-5: ok=%v in=%v out=%v, want true/3/15", ok, p.InputPerMTok, p.OutputPerMTok)
	}
	q, ok := PriceFor("openrouter", "anthropic/claude-sonnet-5")
	if !ok || q.InputPerMTok != 3 || q.OutputPerMTok != 15 || q.cacheReadMult() != 0.10 {
		t.Errorf("openrouter sonnet-5: ok=%v in=%v out=%v read=%v, want true/3/15/0.10", ok, q.InputPerMTok, q.OutputPerMTok, q.cacheReadMult())
	}
}

// TestPriceFor_Subscription confirms claude-cli is unpriced (subscription).
func TestPriceFor_Subscription(t *testing.T) {
	if _, ok := PriceFor("claude-cli", "opus"); ok {
		t.Error("claude-cli should be unpriced (subscription)")
	}
	if _, ok := PriceFor("minimax-anthropic", "MiniMax-M2.1"); !ok {
		t.Error("minimax-anthropic should share the minimax price table")
	}
}

// TestEstimateFor_ClaudeCLI verifies that EstimateFor maps known claude-cli
// models to their Anthropic equivalent-API list price, and returns ok=false
// for unknown models (so no fake cost is shown for them).
func TestEstimateFor_ClaudeCLI(t *testing.T) {
	// Known anthropic model via claude-cli should get an estimate.
	p, ok := EstimateFor("claude-cli", "claude-sonnet-4-6")
	if !ok {
		t.Fatal("EstimateFor: claude-cli/claude-sonnet-4-6 should return a price")
	}
	if p.InputPerMTok != 3 || p.OutputPerMTok != 15 {
		t.Errorf("unexpected sonnet price: in=%v out=%v", p.InputPerMTok, p.OutputPerMTok)
	}

	// Unknown model should not get an estimate.
	if _, ok := EstimateFor("claude-cli", "unknown-model-xyz"); ok {
		t.Error("EstimateFor: unknown model should return ok=false")
	}

	// Non-subscription provider should not go through EstimateFor.
	if _, ok := EstimateFor("anthropic", "claude-sonnet-4-6"); ok {
		t.Error("EstimateFor: anthropic (metered) should not have an estimate")
	}

	// Cost of 1M input + 1M output at sonnet price = $3 + $15 = $18.
	if got := p.Cost(1_000_000, 1_000_000); !approx(got, 18) {
		t.Errorf("sonnet cost = %v, want 18", got)
	}

	// The 1-hour extended cache-write premium (2×) that the anthropic table carries
	// must NOT leak into the claude-cli estimate: Claude Code CLI caches at the
	// default 5-minute TTL (1.25×). 1M cache-write = 3 * 1.25 = $3.75 (not $6 at 2×).
	if got := p.CostDetailed(0, 0, 0, 1_000_000); !approx(got, 3.75) {
		t.Errorf("claude-cli cache-write = %v, want 3.75 (5-min tier, not 2× override)", got)
	}
	// Cache-read tier is unchanged at 0.10× → 1M read = $0.30.
	if got := p.CostDetailed(0, 0, 1_000_000, 0); !approx(got, 0.3) {
		t.Errorf("claude-cli cache-read = %v, want 0.3", got)
	}
}
