package providers

import "testing"

// TestLongContextSurchargeThreshold pins the boundary behaviour: the published
// rule is "requests EXCEEDING the threshold", so a request sitting exactly on it
// stays on standard rates and one token more flips the whole request.
func TestLongContextSurchargeThreshold(t *testing.T) {
	p, ok := PriceFor("openai", "gpt-6-astra")
	if !ok {
		t.Fatal("gpt-6-astra price missing")
	}
	const th = longContextThresholdGPT

	// Exactly at the threshold: standard $10/MTok input.
	if got := p.Cost(th, 0); !approx(got, float64(th)*10/1_000_000) {
		t.Errorf("at threshold = %v, want standard rate", got)
	}
	// One token past it: the WHOLE request doubles, not just the excess.
	want := float64(th+1) * 10 * 2 / 1_000_000
	if got := p.Cost(th+1, 0); !approx(got, want) {
		t.Errorf("past threshold = %v, want %v (2x on the full request)", got, want)
	}
}

// TestLongContextSurchargeAppliesToEveryTier pins that the surcharge scales each
// billing tier the way OpenAI documents: 2x on input, cache reads and cache
// writes, 1.5x on output.
func TestLongContextSurchargeAppliesToEveryTier(t *testing.T) {
	p, ok := PriceFor("openai", "gpt-6-astra")
	if !ok {
		t.Fatal("gpt-6-astra price missing")
	}
	// 300K fresh input (over the threshold) + 100K each cache tier + 1M output.
	const in, cr, cw, out = 300_000, 100_000, 100_000, 1_000_000
	want := (float64(in)*10*2 +
		float64(cr)*10*0.10*2 +
		float64(cw)*10*1.25*2 +
		float64(out)*50*1.5) / 1_000_000
	if got := p.CostDetailed(in, out, cr, cw); !approx(got, want) {
		t.Errorf("surcharged cost = %v, want %v", got, want)
	}
}

// TestLongContextThresholdCountsWholeInputSide pins that cache reads count toward
// the threshold. A mostly-cached long request has crossed it just as much as a
// fresh one, so billing only the fresh portion would silently under-report.
func TestLongContextThresholdCountsWholeInputSide(t *testing.T) {
	p, _ := PriceFor("openai", "gpt-6-astra")
	// 10K fresh, 300K served from cache: fresh alone is far under the threshold.
	got := p.CostDetailed(10_000, 0, 300_000, 0)
	want := (float64(10_000)*10*2 + float64(300_000)*10*0.10*2) / 1_000_000
	if !approx(got, want) {
		t.Errorf("cached long request = %v, want %v (surcharge applies)", got, want)
	}
}

// TestLongContextSurchargeOnAllGPTTiers pins the surcharge on every tier that
// carries it, using each tier's own published long-context rates as the oracle.
func TestLongContextSurchargeOnAllGPTTiers(t *testing.T) {
	for _, tc := range []struct {
		model           string
		longIn, longOut float64 // published long-context $/MTok
	}{
		{"gpt-6-astra", 20, 75},
		{"gpt-5.6-sol", 10, 45},
		{"gpt-5.6-terra", 4, 18},
		{"gpt-5.6-luna", 0.40, 1.80},
	} {
		p, ok := PriceFor("openai", tc.model)
		if !ok {
			t.Errorf("%s: price missing", tc.model)
			continue
		}
		const in, out = 1_000_000, 1_000_000
		want := (float64(in)*tc.longIn + float64(out)*tc.longOut) / 1_000_000
		if got := p.Cost(in, out); !approx(got, want) {
			t.Errorf("%s surcharged = %v, want %v", tc.model, got, want)
		}
	}
}

// TestLongContextSurchargeIsOptIn pins that models without a threshold are
// untouched — a huge Claude request must not pick up a GPT surcharge.
func TestLongContextSurchargeIsOptIn(t *testing.T) {
	p, ok := PriceFor("anthropic", "claude-fable-5-1")
	if !ok {
		t.Fatal("fable price missing")
	}
	if got := p.Cost(900_000, 0); !approx(got, float64(900_000)*10/1_000_000) {
		t.Errorf("non-surcharged model = %v, want plain input rate", got)
	}
}

// TestCostNoCachingCarriesSurcharge pins that the counterfactual baseline is
// surcharged too. If it were not, a long cached request would look like it "saved"
// the surcharge, which caching never does.
func TestCostNoCachingCarriesSurcharge(t *testing.T) {
	p, _ := PriceFor("openai", "gpt-6-astra")
	got := p.CostNoCaching(10_000, 1_000, 300_000, 0)
	want := (float64(310_000)*10*2 + float64(1_000)*50*1.5) / 1_000_000
	if !approx(got, want) {
		t.Errorf("no-caching baseline = %v, want %v", got, want)
	}
}
