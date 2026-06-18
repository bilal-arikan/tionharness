package providers

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// TestCostDetailed_CacheTiers verifies fresh input, cache-read (cheap),
// cache-write (premium) and output are each priced at the right multiplier.
func TestCostDetailed_CacheTiers(t *testing.T) {
	p, ok := PriceFor("anthropic", "claude-opus-4-8") // 15 in / 75 out per Mtok
	if !ok {
		t.Fatal("opus price missing")
	}

	// 1M fresh input = $15; 1M output = $75.
	if got := p.Cost(1_000_000, 1_000_000); !approx(got, 90) {
		t.Errorf("plain cost = %v, want 90", got)
	}
	// 1M cache-read = 15 * 0.10 = $1.50.
	if got := p.CostDetailed(0, 0, 1_000_000, 0); !approx(got, 1.5) {
		t.Errorf("cache-read cost = %v, want 1.5", got)
	}
	// 1M cache-write = 15 * 1.25 = $18.75.
	if got := p.CostDetailed(0, 0, 0, 1_000_000); !approx(got, 18.75) {
		t.Errorf("cache-write cost = %v, want 18.75", got)
	}
	// Savings on 1M cache-read = 15 * 0.90 = $13.50.
	if got := p.CacheSavings(1_000_000); !approx(got, 13.5) {
		t.Errorf("savings = %v, want 13.5", got)
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
