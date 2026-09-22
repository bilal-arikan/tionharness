package providers

import (
	"math"
	"testing"
)

// TestNewDeepSeekAndGLMPrices pins the 2026-09 list prices: V4.1 Flash and the
// routed V4 Flash name share one rate, the Anthropic-mode kind shares the
// DeepSeek table, and the free GLM-4.7-Flash is a KNOWN zero, not unpriced.
func TestNewDeepSeekAndGLMPrices(t *testing.T) {
	cases := []struct {
		provider, model string
		in, out, read   float64
	}{
		{"deepseek", "deepseek-flash", 0.15, 0.60, 0.02},
		{"deepseek", "deepseek-v4-flash", 0.15, 0.60, 0.02},
		{"deepseek-anthropic", "deepseek-flash", 0.15, 0.60, 0.02},
		{"deepseek", "deepseek-v4-pro", 0.66, 1.98, 0.033},
		{"zai", "glm-5.3", 1.40, 4.40, 0.19},
		{"zai", "glm-5.3-flash", 0.15, 0.50, 0.20},
		{"zai", "glm-5.3-flashx", 0.37, 1.25, 0.20},
		{"zai", "glm-5.1", 1.40, 4.40, 0.19},
		{"zai", "glm-5", 1.00, 3.20, 0.20},
		{"glm-anthropic", "glm-5.3", 1.40, 4.40, 0.19},
		{"zhipu-glm", "glm-5.3-flash", 0.15, 0.50, 0.20},
	}
	for _, c := range cases {
		p, ok := PriceFor(c.provider, c.model)
		if !ok {
			t.Errorf("%s/%s: unpriced", c.provider, c.model)
			continue
		}
		if p.InputPerMTok != c.in || p.OutputPerMTok != c.out || p.cacheReadMult() != c.read {
			t.Errorf("%s/%s = %+v, want in %.2f out %.2f read %.3f", c.provider, c.model, p, c.in, c.out, c.read)
		}
	}

	free, ok := PriceFor("zai", "glm-4.7-flash")
	if !ok {
		t.Fatal("glm-4.7-flash must be a known (free) price, not unpriced")
	}
	if cost := free.CostDetailed(1_000_000, 1_000_000, 1_000_000, 0); cost != 0 {
		t.Errorf("glm-4.7-flash cost = %v, want 0", cost)
	}

	// V4.1 Flash cache hits bill at $0.003/MTok off-peak.
	flash, _ := PriceFor("deepseek", "deepseek-flash")
	if got := flash.CostDetailed(0, 0, 1_000_000, 0); math.Abs(got-0.003) > 1e-9 {
		t.Errorf("deepseek-flash cache-hit cost = %v, want 0.003", got)
	}
}

// TestCuratedDeepSeekAndZAIModelsArePriced guards catalog ↔ price-table drift:
// every model the DeepSeek and Z.ai kinds offer has a list price, so the budget
// screen never shows a curated model as unpriced.
func TestCuratedDeepSeekAndZAIModelsArePriced(t *testing.T) {
	for _, kind := range []string{"deepseek", "deepseek-anthropic", "zai"} {
		k, ok := lookupKind(kind)
		if !ok {
			t.Fatalf("kind %q not registered", kind)
		}
		for _, m := range k.Manifest().Models {
			if _, ok := PriceFor(kind, m.ID); !ok {
				t.Errorf("%s/%s is offered but unpriced", kind, m.ID)
			}
		}
	}
}
