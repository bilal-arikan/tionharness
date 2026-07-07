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
// are ~10× cheaper than fresh input; cache writes carry a premium. The package
// defaults match Anthropic's standard (5-minute) cache tier; the 1-hour
// extended tier writes at 2× — the native anthropic table carries that as a
// per-model CacheWriteMultOverride because the native client always requests
// ttl:"1h" when caching is on. A Price may override either multiplier per
// model (see CacheReadMultOverride).
const (
	CacheReadMult  = 0.10
	CacheWriteMult = 1.25
	// CacheWrite1hMult is the extended (1-hour TTL) cache-write premium.
	CacheWrite1hMult = 2.0
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
	// The native anthropic client always requests the 1-hour extended cache TTL
	// when caching is on, so writes carry the 2× extended premium here (not the
	// standard 1.25×).
	"anthropic": {
		"claude-opus-4-8":           {InputPerMTok: 5, OutputPerMTok: 25, CacheWriteMultOverride: CacheWrite1hMult},
		// Sonnet 5 standard list price ($3/$15). Introductory $2/$10 runs through
		// 2026-08-31; the table tracks the standard rate as a stable ballpark.
		"claude-sonnet-5":           {InputPerMTok: 3, OutputPerMTok: 15, CacheWriteMultOverride: CacheWrite1hMult},
		"claude-sonnet-4-6":         {InputPerMTok: 3, OutputPerMTok: 15, CacheWriteMultOverride: CacheWrite1hMult},
		"claude-haiku-4-5-20251001": {InputPerMTok: 1, OutputPerMTok: 5, CacheWriteMultOverride: CacheWrite1hMult},
		// Fable 5 sits ABOVE Opus-tier pricing ($10/$50 per MTok).
		"claude-fable-5":            {InputPerMTok: 10, OutputPerMTok: 50, CacheWriteMultOverride: CacheWrite1hMult},
	},
	"minimax": {
		"MiniMax-M2.1":           {InputPerMTok: 0.30, OutputPerMTok: 1.20, CacheReadMultOverride: 0.25},
		"MiniMax-M2.1-lightning": {InputPerMTok: 0.20, OutputPerMTok: 0.80, CacheReadMultOverride: 0.25},
		"MiniMax-M2":             {InputPerMTok: 0.30, OutputPerMTok: 1.20, CacheReadMultOverride: 0.25},
		"MiniMax-M1":             {InputPerMTok: 0.30, OutputPerMTok: 1.20, CacheReadMultOverride: 0.25},
		"MiniMax-Text-01":        {InputPerMTok: 0.20, OutputPerMTok: 1.10, CacheReadMultOverride: 0.25},
	},
	// OpenRouter is keyed by its namespaced model ids. Anthropic-routed models keep
	// Anthropic's pass-through pricing, including the 0.10× cache-read / 1.25× write
	// tier. Other vendors (OpenAI/DeepSeek/…) use OpenRouter's general 0.25× cache
	// tier — add them with CacheReadMultOverride: 0.25 (e.g.
	// "openai/gpt-…": {InputPerMTok: …, OutputPerMTok: …, CacheReadMultOverride: 0.25}).
	// Unlisted models fall through to unpriced (the screen is explicitly ballpark).
	"openrouter": {
		"anthropic/claude-opus-4.8":   {InputPerMTok: 5, OutputPerMTok: 25, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-sonnet-5":   {InputPerMTok: 3, OutputPerMTok: 15, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-sonnet-4.6": {InputPerMTok: 3, OutputPerMTok: 15, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-haiku-4.5":  {InputPerMTok: 1, OutputPerMTok: 5, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-fable-5":    {InputPerMTok: 10, OutputPerMTok: 50, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
	},
	// NOTE: market provider-pack prices (xai, mistral, gemini, … ~25 providers, up to
	// 15 models each) live in the generated pricing_market.go (var marketPrices,
	// merged into priceTable at init). Single source: data/gen_providers.py.
}

// AllPrices returns a copy of the full list-price table (provider id → model →
// Price). Used by the API to surface ballpark $/1M-token figures in the UI (e.g.
// the market provider preview), so the screen can show prices without a request.
func AllPrices() map[string]map[string]Price {
	out := make(map[string]map[string]Price, len(priceTable))
	for prov, models := range priceTable {
		m := make(map[string]Price, len(models))
		for id, p := range models {
			m[id] = p
		}
		out[prov] = m
	}
	return out
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
