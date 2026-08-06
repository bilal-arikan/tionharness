// Package billing holds the cost/pricing domain math for usage rollups, kept out
// of the HTTP layer so the api package only shapes JSON. It maps a stored usage
// slice (db.KindStat) to USD via the provider price table, falling back to an
// equivalent-API estimate for subscription providers (e.g. claude-cli).
package billing

import (
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
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
//
// INVARIANT: model değişikliği geçmiş kayıtları asla yeniden fiyatlamaz.
// Usage.ByModel anahtarı çağrıyı gerçekten servis eden modeldir — agent.Model
// değişse bile geçmiş anahtarlar ("claude-sonnet-4-20250514") korunur ve
// PriceStat okuma anında o anahtara göre fiyat uygular. Bu davranış regresyon
// testiyle korunmaktadır (bkz. internal/billing/model_change_poc_test.go).
func PriceStat(provider, model string, st db.KindStat) (cost, save float64, priced, estimated bool) {
	if p, ok := providers.PriceFor(provider, model); ok {
		return p.CostDetailed(st.InputTokens, st.OutputTokens, st.CacheReadTokens, st.CacheWriteTokens),
			p.CacheSavings(st.CacheReadTokens), true, false
	}
	if st.InputTokens+st.OutputTokens+st.CacheReadTokens+st.CacheWriteTokens == 0 {
		return 0, 0, true, false // no spend → nothing unpriced
	}
	// Unpriced spend: try an equivalent-API estimate (e.g. claude-cli → anthropic).
	// The cache-read savings are estimated the SAME way the cost is — otherwise a
	// claude-cli workspace (the default keyless provider) shows a non-zero estimated
	// cost that already benefits from cache pricing, yet $0 savings everywhere (Budget
	// Savings Center, session usage, per-message debug). priced stays false so the UI
	// keeps flagging both figures as an estimate ("~").
	if ep, ok := providers.EstimateFor(provider, model); ok {
		return ep.CostDetailed(st.InputTokens, st.OutputTokens, st.CacheReadTokens, st.CacheWriteTokens),
			ep.CacheSavings(st.CacheReadTokens), false, true
	}
	return 0, 0, false, false
}

// NoCacheCost returns the counterfactual USD cost of a usage slice if prompt
// caching did not exist (cache read/write billed as fresh input, no discount, no
// write premium). Mirrors PriceStat's price resolution: real list price, else an
// equivalent-API estimate (subscription providers), else 0 (unpriced). This is the
// honest baseline for the "cost without caching" figure — NOT cost+savings, which
// leaves the cache-write premium in and overstates it.
func NoCacheCost(provider, model string, st db.KindStat) float64 {
	if p, ok := providers.PriceFor(provider, model); ok {
		return p.CostNoCaching(st.InputTokens, st.OutputTokens, st.CacheReadTokens, st.CacheWriteTokens)
	}
	if ep, ok := providers.EstimateFor(provider, model); ok {
		return ep.CostNoCaching(st.InputTokens, st.OutputTokens, st.CacheReadTokens, st.CacheWriteTokens)
	}
	return 0
}

// Row is one provider+model's priced usage slice: the stored token counters plus
// the USD cost/savings derived by PriceStat. Stat carries the raw counts so the
// caller can shape its own per-model DTO without re-reading the rollup map.
type Row struct {
	Provider   string
	Model      string
	Stat       db.KindStat
	CostUSD    float64
	SavingsUSD float64
	Priced     bool // a real list price applied (vs an estimate / unpriced)
	Estimated  bool // cost is an equivalent-API estimate (subscription provider)
}

// Rollup is the aggregate of a usage rollup's per-model breakdown: the priced
// rows (sorted costliest first) plus the workspace-level totals. It is the single
// merged primitive behind every budget surface — both the "just the cost" callers
// (per-agent rows, daily trend) and the "rows + totals" callers (the usage
// endpoints) read what they need from one computation.
type Rollup struct {
	Rows       []Row
	CostUSD    float64
	SavingsUSD float64
	// NoCacheCostUSD is the counterfactual total if caching did not exist (cache
	// read/write billed as fresh input). The honest "cost without caching" baseline
	// — always ≥ CostUSD, and NOT equal to CostUSD+SavingsUSD (that keeps the write
	// premium). Budget's "Tasarrufsuz maliyet" card reads this.
	NoCacheCostUSD   float64
	CacheReadTokens  int
	CacheWriteTokens int
	Priced           bool // false when ANY spend lacks a real list price
	Estimated        bool // true when ANY cost is an equivalent-API estimate
}

// RollupOf prices every entry of a "<provider>|<model>" → KindStat map and returns
// the per-model rows (costliest first, input+output as the tiebreak) plus the
// aggregate cost/savings/cache totals and priced/estimated flags. Replaces the old
// costOf + modelRowsFor pair: cost-only callers read Rollup.CostUSD/Priced/...,
// row callers map Rollup.Rows to their DTO.
func RollupOf(byModel map[string]db.KindStat) Rollup {
	roll := Rollup{Priced: true}
	for key, st := range byModel {
		provider, model, _ := strings.Cut(key, "|")
		cost, save, priced, estimated := PriceStat(provider, model, st)
		if !priced {
			roll.Priced = false
		}
		if estimated {
			roll.Estimated = true
		}
		roll.Rows = append(roll.Rows, Row{
			Provider: provider, Model: model, Stat: st,
			CostUSD: cost, SavingsUSD: save, Priced: priced, Estimated: estimated,
		})
		roll.CostUSD += cost
		roll.SavingsUSD += save
		roll.NoCacheCostUSD += NoCacheCost(provider, model, st)
		roll.CacheReadTokens += st.CacheReadTokens
		roll.CacheWriteTokens += st.CacheWriteTokens
	}
	sort.SliceStable(roll.Rows, func(i, j int) bool {
		if roll.Rows[i].CostUSD != roll.Rows[j].CostUSD {
			return roll.Rows[i].CostUSD > roll.Rows[j].CostUSD
		}
		return roll.Rows[i].Stat.InputTokens+roll.Rows[i].Stat.OutputTokens >
			roll.Rows[j].Stat.InputTokens+roll.Rows[j].Stat.OutputTokens
	})
	return roll
}
