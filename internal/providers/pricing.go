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

// CostNoCaching returns the counterfactual USD cost if prompt caching did not
// exist: every cache-read AND cache-write token is billed as fresh input at the
// base rate — no read discount (cacheReadMult) and no write premium
// (cacheWriteMult). This is the honest "what you'd pay without any caching"
// baseline, so a savings display can subtract the real cost from it. It is NOT
// simply cost+CacheSavings: that keeps the cache-write premium, overstating the
// baseline by (cacheWriteMult−1)×cacheWrite×InputPerMTok.
func (p Price) CostNoCaching(inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) float64 {
	in := float64(inputTokens+cacheReadTokens+cacheWriteTokens) * p.InputPerMTok
	out := float64(outputTokens) * p.OutputPerMTok
	return (in + out) / 1_000_000
}

// CoolingWasteUSD returns the AVOIDABLE USD overpay when a warm prompt-cache
// prefix was lost to TTL expiry / server eviction and had to be re-written. The
// reWrittenTokens are native Anthropic's cache_creation count for that cold call:
// they were billed at the write multiplier, but had the cache stayed warm they
// would have been served as a cache READ at the (far cheaper) read multiplier.
// The difference (writeMult − readMult) is the money a timely turn would have
// saved. It is NOT the whole re-write — seeding a cache always costs at least the
// read tier — only the premium paid for having gone cold. Zero for a non-positive
// count. (OpenRouter folds a cold prefix into plain input with no write counter,
// so its re-paid prefix cannot be separated from genuinely-new input; there
// reWrittenTokens is ~0 and this reports 0 rather than over-count.)
func (p Price) CoolingWasteUSD(reWrittenTokens int) float64 {
	if reWrittenTokens <= 0 {
		return 0
	}
	delta := p.cacheWriteMult() - p.cacheReadMult()
	if delta <= 0 {
		return 0
	}
	return float64(reWrittenTokens) * p.InputPerMTok * delta / 1_000_000
}

// CoolingWaste resolves the price for provider+model (real list price, else a
// subscription equivalent-API estimate, mirroring PriceFor/EstimateFor) and
// returns the avoidable cooling overpay for a cold re-written prefix, whether the
// figure is an estimate (subscription provider), and whether any price applied.
// ok=false leaves the caller with no figure to record (unpriced/custom endpoint).
func CoolingWaste(provider, model string, reWrittenTokens int) (usd float64, estimated, ok bool) {
	if p, k := PriceFor(provider, model); k {
		return p.CoolingWasteUSD(reWrittenTokens), false, true
	}
	if ep, k := EstimateFor(provider, model); k {
		return ep.CoolingWasteUSD(reWrittenTokens), true, true
	}
	return 0, false, false
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
		// Opus 5 (2026-07-24) keeps Opus-tier pricing unchanged ($5/$25 per MTok).
		"claude-opus-5":   {InputPerMTok: 5, OutputPerMTok: 25, CacheWriteMultOverride: CacheWrite1hMult},
		"claude-opus-4-8": {InputPerMTok: 5, OutputPerMTok: 25, CacheWriteMultOverride: CacheWrite1hMult},
		// Sonnet 5 standard list price ($3/$15). Introductory $2/$10 runs through
		// 2026-08-31; the table tracks the standard rate as a stable ballpark.
		"claude-sonnet-5":           {InputPerMTok: 3, OutputPerMTok: 15, CacheWriteMultOverride: CacheWrite1hMult},
		"claude-sonnet-4-6":         {InputPerMTok: 3, OutputPerMTok: 15, CacheWriteMultOverride: CacheWrite1hMult},
		"claude-haiku-4-5-20251001": {InputPerMTok: 1, OutputPerMTok: 5, CacheWriteMultOverride: CacheWrite1hMult},
		// Fable 5 sits ABOVE Opus-tier pricing ($10/$50 per MTok).
		"claude-fable-5": {InputPerMTok: 10, OutputPerMTok: 50, CacheWriteMultOverride: CacheWrite1hMult},
	},
	// codex-cli is intentionally absent from this table for the SAME reason as
	// claude-cli above: it runs against a ChatGPT subscription login (CODEX_HOME),
	// so its calls are flat-rate, not per-token. EstimateFor supplies an
	// equivalent-API estimate using OpenAI's first-party per-token prices instead
	// (see the "openai" table below and the codex-cli case in EstimateFor).
	//
	// OpenAI first-party API prices (USD per 1M tokens), verified 2026-08-18
	// against two independent sources (devtk.ai, cloudzero.com/apidog) that agree
	// on GPT-5.5 and GPT-5.4-mini. All OpenAI cache reads are a flat 0.10× of
	// input (verified per-model below); OpenAI has no separate cache-WRITE
	// premium (caching is automatic, not opt-in like Anthropic's), so
	// CacheWriteMultOverride is left unset (falls back to CacheWriteMult, which
	// is never charged for codex-cli since it has no priceTable entry — this
	// table exists only to feed EstimateFor).
	//
	// gpt-5.4 and gpt-5.2 are deliberately OMITTED: gpt-5.4 has a published price
	// but codex-cli's own catalog notes it "ChatGPT hesabıyla kullanılamaz" (see
	// kind_codexcli.go) — not worth pricing a model this transport can't run on a
	// ChatGPT login. gpt-5.2 has NO published per-token price in either source
	// checked — guessing one is worse than omitting it (PriceFor/EstimateFor
	// correctly report unpriced for anything absent here).
	"openai": {
		"gpt-5.6-sol":   {InputPerMTok: 5.00, OutputPerMTok: 30.00, CacheReadMultOverride: 0.10},
		"gpt-5.6-terra": {InputPerMTok: 2.00, OutputPerMTok: 12.00, CacheReadMultOverride: 0.10},
		"gpt-5.6-luna":  {InputPerMTok: 0.20, OutputPerMTok: 1.20, CacheReadMultOverride: 0.10},
		"gpt-5.5":       {InputPerMTok: 5.00, OutputPerMTok: 30.00, CacheReadMultOverride: 0.10},
		"gpt-5.4-mini":  {InputPerMTok: 0.75, OutputPerMTok: 4.50, CacheReadMultOverride: 0.10},
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
		"anthropic/claude-opus-5":     {InputPerMTok: 5, OutputPerMTok: 25, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-opus-4.8":   {InputPerMTok: 5, OutputPerMTok: 25, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-sonnet-5":   {InputPerMTok: 3, OutputPerMTok: 15, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-sonnet-4.6": {InputPerMTok: 3, OutputPerMTok: 15, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-haiku-4.5":  {InputPerMTok: 1, OutputPerMTok: 5, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
		"anthropic/claude-fable-5":    {InputPerMTok: 10, OutputPerMTok: 50, CacheReadMultOverride: 0.10, CacheWriteMultOverride: 1.25},
	},
	// Z.ai GLM family (Anthropic-mode transport). Official Z.ai list prices per 1M
	// tokens (2026-08; GLM-5.2 = $1.40/$4.40, cached input $0.26 → read mult ~0.19).
	// Anthropic-protocol endpoint bills cache_control breakpoints, hence the write
	// mult. IDs evolve → unlisted models fall through to unpriced.
	"zai": {
		"glm-5.2":       {InputPerMTok: 1.40, OutputPerMTok: 4.40, CacheReadMultOverride: 0.19, CacheWriteMultOverride: 1.25},
		"glm-5.1":       {InputPerMTok: 0.97, OutputPerMTok: 3.04, CacheReadMultOverride: 0.19, CacheWriteMultOverride: 1.25},
		"glm-5":         {InputPerMTok: 0.60, OutputPerMTok: 1.92, CacheReadMultOverride: 0.19, CacheWriteMultOverride: 1.25},
		"glm-4.7":       {InputPerMTok: 0.60, OutputPerMTok: 2.20, CacheReadMultOverride: 0.18, CacheWriteMultOverride: 1.25},
		"glm-4.7-flash": {InputPerMTok: 0.06, OutputPerMTok: 0.40, CacheReadMultOverride: 0.19, CacheWriteMultOverride: 1.25},
	},
	// DeepSeek V4 family (first-party OpenAI-compatible endpoint). DeepSeek's
	// context caching is automatic with no write premium and a deep read discount,
	// reported via prompt_cache_hit_tokens (see oaiUsage.toUsage). IDs evolve →
	// unlisted models fall through to unpriced.
	//
	// Prices per 1M tokens, effective 2026-08-16 16:00 UTC (the price hike that
	// ended the flat rate). Billing is now peak/off-peak: peak is 01:00–04:00 and
	// 06:00–10:00 UTC (7h/day), off-peak is everything else and costs half.
	// This table is time-independent, so it tracks the OFF-PEAK (regular) rate —
	// the majority of the day. Turns that land in a peak window are therefore
	// under-reported by 2×; a time-aware Price would be needed to fix that.
	//
	//	          off-peak (this table)   peak (2×)     cache-hit input (off-peak)
	//	V4 Flash  $0.22 / $0.66           $0.44/$1.32   $0.007   (~0.032× input)
	//	V4 Pro    $0.66 / $1.98           $1.32/$3.96   $0.022   (~0.033× input)
	//
	// The deepseek-anthropic kind shares this table via PriceFor.
	"deepseek": {
		"deepseek-v4-flash": {InputPerMTok: 0.22, OutputPerMTok: 0.66, CacheReadMultOverride: 0.032},
		"deepseek-v4-pro":   {InputPerMTok: 0.66, OutputPerMTok: 1.98, CacheReadMultOverride: 0.033},
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
	case "deepseek-anthropic":
		provider = "deepseek"
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
		// the same model id as an informational estimate. BUT the 1-hour extended
		// cache-write premium (CacheWrite1hMult, 2×) the anthropic table carries is
		// specific to TionSwarm's OWN native anthropic client, which always requests
		// ttl:"1h". Claude Code CLI manages its own cache_control at the default
		// 5-minute TTL (1.25×), so clear the override here → the estimate uses the
		// standard write tier. Cache-read (0.10×) is unchanged. Without this a
		// claude-cli agent's cache-write cost is over-estimated by ~60%.
		if p, ok := priceTable["anthropic"][model]; ok {
			p.CacheWriteMultOverride = 0 // fall back to CacheWriteMult (1.25×), the 5-minute tier
			return p, true
		}
	case "codex-cli":
		// codex-cli runs via ChatGPT/Codex subscription login; reuse OpenAI's own
		// first-party list price for the same model id as an informational
		// estimate. No cache-write premium to strip here (unlike claude-cli/
		// anthropic) — the "openai" table never set one; caching is automatic on
		// OpenAI's side, not an opt-in TTL choice.
		if p, ok := priceTable["openai"][model]; ok {
			return p, true
		}
	}
	return Price{}, false
}
