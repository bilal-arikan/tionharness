package providers

// Price is a model's API token price in USD per 1,000,000 tokens.
type Price struct {
	InputPerMTok  float64 `json:"inputPerMTok"`
	OutputPerMTok float64 `json:"outputPerMTok"`
	// Optional per-model prompt-cache multipliers (relative to InputPerMTok). Zero
	// falls back to the package defaults below. Providers differ: Anthropic bills
	// cache reads at 0.10× while OpenRouter's general (non-Anthropic) tier is 0.25×,
	// so OpenAI/DeepSeek-routed models carry a higher read multiplier here.
	CacheReadMultOverride  float64 `json:"cacheReadMult,omitempty"`
	CacheWriteMultOverride float64 `json:"cacheWriteMult,omitempty"`
}

// Prompt-cache price multipliers relative to the base input price. Cache reads
// are ~10× cheaper than fresh input; cache writes carry a modest premium. These
// match Anthropic's standard (5-minute) cache tier; the 1-hour extended tier
// writes at 2× — not separately modelled here (treated as the standard premium).
// A Price may override these per model (see CacheReadMultOverride).
const (
	CacheReadMult  = 0.10
	CacheWriteMult = 1.25
)

// cacheReadMult / cacheWriteMult return the effective multipliers for this price,
// honouring a per-model override when set and otherwise the package defaults.
func (p Price) cacheReadMult() float64 {
	if p.CacheReadMultOverride > 0 {
		return p.CacheReadMultOverride
	}
	return CacheReadMult
}

func (p Price) cacheWriteMult() float64 {
	if p.CacheWriteMultOverride > 0 {
		return p.CacheWriteMultOverride
	}
	return CacheWriteMult
}

// Cost returns the USD cost of a plain input/output token count at this price
// (no caching). Equivalent to CostDetailed(in, out, 0, 0).
func (p Price) Cost(inputTokens, outputTokens int) float64 {
	return p.CostDetailed(inputTokens, outputTokens, 0, 0)
}

// CostDetailed returns the USD cost including prompt-cache tiers: fresh input at
// the base rate, cache reads at CacheReadMult, cache writes at CacheWriteMult,
// and output at the output rate.
func (p Price) CostDetailed(inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) float64 {
	in := float64(inputTokens) * p.InputPerMTok
	cr := float64(cacheReadTokens) * p.InputPerMTok * p.cacheReadMult()
	cw := float64(cacheWriteTokens) * p.InputPerMTok * p.cacheWriteMult()
	out := float64(outputTokens) * p.OutputPerMTok
	return (in + cr + cw + out) / 1_000_000
}

// CacheSavings returns the USD saved by serving cacheReadTokens from cache
// instead of paying the full input rate for them (the cache discount realised).
func (p Price) CacheSavings(cacheReadTokens int) float64 {
	return float64(cacheReadTokens) * p.InputPerMTok * (1 - p.cacheReadMult()) / 1_000_000
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
		"claude-opus-4-8":           {InputPerMTok: 15, OutputPerMTok: 75},
		"claude-sonnet-4-6":         {InputPerMTok: 3, OutputPerMTok: 15},
		"claude-haiku-4-5-20251001": {InputPerMTok: 1, OutputPerMTok: 5},
		"claude-fable-5":            {InputPerMTok: 3, OutputPerMTok: 15},
	},
	"minimax": {
		"MiniMax-M2.1":           {InputPerMTok: 0.30, OutputPerMTok: 1.20},
		"MiniMax-M2.1-lightning": {InputPerMTok: 0.20, OutputPerMTok: 0.80},
		"MiniMax-M2":             {InputPerMTok: 0.30, OutputPerMTok: 1.20},
	},
	// OpenRouter is keyed by its namespaced model ids. Anthropic-routed models keep
	// Anthropic's pass-through pricing, including the 0.10× cache-read / 1.25× write
	// tier. Other vendors (OpenAI/DeepSeek/…) use OpenRouter's general 0.25× cache
	// tier — add them with CacheReadMultOverride: 0.25 (e.g.
	// "openai/gpt-…": {InputPerMTok: …, OutputPerMTok: …, CacheReadMultOverride: 0.25}).
	// Unlisted models fall through to unpriced (the screen is explicitly ballpark).
	"openrouter": {
		"anthropic/claude-opus-4.8":   {InputPerMTok: 15, OutputPerMTok: 75, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-sonnet-4.6": {InputPerMTok: 3, OutputPerMTok: 15, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-haiku-4.5":  {InputPerMTok: 1, OutputPerMTok: 5, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
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

// EstimateFor returns an equivalent-API cost estimate for subscription providers
// where there is no direct per-token billing. For claude-cli the Anthropic list
// price for the same model is returned so the budget screen can display an
// "estimated equivalent API cost" figure clearly labelled as non-billable.
// Returns ok=false when the model has no equivalent list price.
func EstimateFor(provider, model string) (Price, bool) {
	switch provider {
	case "claude-cli":
		// claude-cli runs via OAuth/subscription; reuse the Anthropic list price for
		// the same model id as an informational estimate.
		if p, ok := priceTable["anthropic"][model]; ok {
			return p, true
		}
	}
	return Price{}, false
}
