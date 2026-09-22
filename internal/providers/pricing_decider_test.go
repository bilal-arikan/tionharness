package providers

import "testing"

// Decision calls bill under the provider their request went to (internal/decider
// billing.go): OpenRouter, TypeSafe's own API, or a server on the user's own
// network, which costs nothing.
func TestDecisionModelPrices(t *testing.T) {
	cases := []struct {
		provider, model string
		input           float64
	}{
		{"openrouter", "typesafe/jev-1.13", 0.042},
		{"openrouter", "typesafe/jev-latest", 0.042},
		{"typesafe", "jev-latest", 0.042},
		{"typesafe", "jev-1.13", 0.042},
		{"local", "openjev-latest", 0},
		{"local", "qwen3-8b", 0},
	}
	for _, c := range cases {
		p, ok := PriceFor(c.provider, c.model)
		if !ok || p.InputPerMTok != c.input || p.OutputPerMTok != 0 {
			t.Errorf("PriceFor(%q, %q) = %+v, %v; want a known input price of %v", c.provider, c.model, p, ok, c.input)
		}
	}
}
