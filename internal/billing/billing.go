// Package billing holds the cost/pricing domain math for usage rollups, kept out
// of the HTTP layer so the api package only shapes JSON. It maps a stored usage
// slice (db.KindStat) to USD via the provider price table, falling back to an
// equivalent-API estimate for subscription providers (e.g. claude-cli).
package billing

import (
	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// PriceStat prices one provider+model usage slice — the single source of truth
// for "what does this spend cost". It uses the real list price when known,
// otherwise an equivalent-API estimate (subscription providers like claude-cli →
// the Anthropic list price for the same model). Returns the USD cost, the
// prompt-cache USD savings (cache reads vs full input price), whether a real price
// applied (priced), and whether the cost is an estimate. A slice with no tokens is
// priced=true / cost=0 (there is nothing to price, so it never flags the rollup as
// "contains unpriced spend"). Every budget surface routes through here so the
// screens compute cost identically.
func PriceStat(provider, model string, st db.KindStat) (cost, save float64, priced, estimated bool) {
	if p, ok := providers.PriceFor(provider, model); ok {
		return p.CostDetailed(st.InputTokens, st.OutputTokens, st.CacheReadTokens, st.CacheWriteTokens),
			p.CacheSavings(st.CacheReadTokens), true, false
	}
	if st.InputTokens+st.OutputTokens+st.CacheReadTokens+st.CacheWriteTokens == 0 {
		return 0, 0, true, false // no spend → nothing unpriced
	}
	// Unpriced spend: try an equivalent-API estimate (e.g. claude-cli → anthropic).
	if ep, ok := providers.EstimateFor(provider, model); ok {
		return ep.CostDetailed(st.InputTokens, st.OutputTokens, st.CacheReadTokens, st.CacheWriteTokens),
			0, false, true
	}
	return 0, 0, false, false
}
