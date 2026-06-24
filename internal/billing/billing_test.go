package billing

import (
	"math"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

func approxUSD(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestPriceStat covers the single pricing primitive every budget surface routes
// through: real list price, equivalent-API estimate for subscription providers,
// the no-spend short-circuit, and a fully unpriced/unknown model.
func TestPriceStat(t *testing.T) {
	// Real price (anthropic opus: 15 in / 75 out per Mtok), with a 1M cache read.
	cost, save, priced, est := PriceStat("anthropic", "claude-opus-4-8",
		db.KindStat{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000})
	if !priced || est {
		t.Errorf("anthropic: priced=%v est=%v, want true/false", priced, est)
	}
	// 15 (input) + 75 (output) + 15*0.10 (cache read) = 91.5
	if !approxUSD(cost, 91.5) {
		t.Errorf("anthropic cost = %v, want 91.5", cost)
	}
	if !approxUSD(save, 13.5) { // 15 * 0.90 per 1M
		t.Errorf("anthropic savings = %v, want 13.5", save)
	}

	// Subscription provider (claude-cli) → unpriced but equivalent-API estimated.
	cost, _, priced, est = PriceStat("claude-cli", "claude-sonnet-4-6",
		db.KindStat{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	if priced || !est {
		t.Errorf("claude-cli: priced=%v est=%v, want false/true", priced, est)
	}
	if !approxUSD(cost, 18) { // sonnet 3 in + 15 out
		t.Errorf("claude-cli estimate = %v, want 18", cost)
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
}
