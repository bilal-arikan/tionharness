package providers

// Price is a model's API token price in USD per 1,000,000 tokens.
type Price struct {
	InputPerMTok  float64 `json:"inputPerMTok"`
	OutputPerMTok float64 `json:"outputPerMTok"`
}

// Cost returns the USD cost of a given input/output token count at this price.
func (p Price) Cost(inputTokens, outputTokens int) float64 {
	return float64(inputTokens)/1_000_000*p.InputPerMTok + float64(outputTokens)/1_000_000*p.OutputPerMTok
}

// priceTable holds APPROXIMATE list prices (USD per 1M tokens) for the metered
// providers, keyed by provider kind then model id. These are list-price
// estimates for the budget screen, not billing-grade figures — provider prices
// change, and prompt caching / batch discounts are not modelled. Treat the
// screen's cost as a ballpark. (A future settings panel can make these
// user-editable.)
//
// claude-cli is intentionally absent: it runs against a local Claude
// subscription (OAuth), so its calls are flat-rate, not per-token — PriceFor
// reports them as unpriced so the UI shows "abonelik" instead of a fake cost.
var priceTable = map[string]map[string]Price{
	"anthropic": {
		"claude-opus-4-8":            {InputPerMTok: 15, OutputPerMTok: 75},
		"claude-sonnet-4-6":          {InputPerMTok: 3, OutputPerMTok: 15},
		"claude-haiku-4-5-20251001":  {InputPerMTok: 1, OutputPerMTok: 5},
		"claude-fable-5":             {InputPerMTok: 3, OutputPerMTok: 15},
	},
	"minimax": {
		"MiniMax-M2.1":           {InputPerMTok: 0.30, OutputPerMTok: 1.20},
		"MiniMax-M2.1-lightning": {InputPerMTok: 0.20, OutputPerMTok: 0.80},
		"MiniMax-M2":             {InputPerMTok: 0.30, OutputPerMTok: 1.20},
	},
}

// PriceFor returns the list price for a provider+model and whether one is known.
// minimax-anthropic shares the minimax table (same models, Anthropic-protocol
// transport). Unknown provider/model and all claude-cli models return ok=false
// (unpriced — subscription or custom endpoint).
func PriceFor(provider, model string) (Price, bool) {
	switch provider {
	case "minimax-anthropic":
		provider = "minimax"
	}
	models, ok := priceTable[provider]
	if !ok {
		return Price{}, false
	}
	p, ok := models[model]
	return p, ok
}
