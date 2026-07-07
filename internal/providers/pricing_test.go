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
}
