package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/billing"
	"github.com/bilal-arikan/swarmgo/internal/db"
)

// kindStat is one origin's slice of consumption in a budget response.
type kindStat struct {
	Calls        int `json:"calls"`
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// agentBudgetRow is one agent's today usage plus its caps, joined with the
// agent's display identity so the screen renders without extra round-trips.
type agentBudgetRow struct {
	AgentID         string              `json:"agentId"`
	Name            string              `json:"name"`
	Avatar          string              `json:"avatar,omitempty"`
	Color           string              `json:"color,omitempty"`
	Provider        string              `json:"provider,omitempty"`
	Calls           int                 `json:"calls"`
	InputTokens     int                 `json:"inputTokens"`
	OutputTokens    int                 `json:"outputTokens"`
	ByKind          map[string]kindStat `json:"byKind,omitempty"`
	CostUSD         float64             `json:"costUSD"`
	Priced          bool                `json:"priced"`    // false when any of this agent's spend is unpriced (e.g. claude-cli)
	Estimated       bool                `json:"estimated"` // true when cost is an equivalent-API estimate (subscription provider)
	DailyCallLimit  int                 `json:"dailyCallLimit"`
	DailyTokenLimit int                 `json:"dailyTokenLimit"`
	// Tool-output compaction savings (bytes) for this agent today — System A
	// (deterministic) and System B (LLM summary), standalone meters with no cost.
	CompactSavedBytes    int `json:"compactSavedBytes"`
	CompactSavedBytesLLM int `json:"compactSavedBytesLLM"`
}

// tokenTotals is the shared token-counter block carried by every spend slice
// (per-model, per-provider, per-day). Embedded anonymously so its fields promote
// to the parent and marshal inline — the JSON shape is unchanged, but the five
// counters are declared once instead of repeated in each DTO.
type tokenTotals struct {
	Calls            int `json:"calls"`
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	CacheReadTokens  int `json:"cacheReadTokens"`
	CacheWriteTokens int `json:"cacheWriteTokens"`
}

// modelStat is one provider+model's slice of the spend — the detail row under a
// provider. CacheRead/Write are the prompt-cache token tiers, SavingsUSD the
// amount cache reads saved versus paying full input price. Estimated is true
// when the cost is an equivalent-API estimate (subscription provider such as
// claude-cli) rather than a real billed amount.
type modelStat struct {
	Model string `json:"model"`
	tokenTotals
	CostUSD    float64 `json:"costUSD"`
	SavingsUSD float64 `json:"savingsUSD"`
	Priced     bool    `json:"priced"`
	Estimated  bool    `json:"estimated"` // equivalent-API estimate (subscription)
}

// providerStat is one provider's slice of the workspace's spend, with the USD
// cost where the models are priced and a per-model detail list. Priced is false
// when any of the provider's spend carries no list price (claude-cli
// subscription, or a custom/unknown model). Estimated is true when the cost
// shown is an equivalent-API estimate (subscription provider).
type providerStat struct {
	Provider string `json:"provider"`
	tokenTotals
	CostUSD    float64     `json:"costUSD"`
	SavingsUSD float64     `json:"savingsUSD"`
	Priced     bool        `json:"priced"`
	Estimated  bool        `json:"estimated"` // equivalent-API estimate (subscription)
	Models     []modelStat `json:"models"`
}

// costOf sums the USD cost (including cache tiers) of a usage rollup's per-model
// breakdown. priced is false when any spend lacks a real list price
// (subscription/custom); estimated is true when an unpriced entry had an
// equivalent-API estimate. Pricing math lives in the billing package.
func costOf(byModel map[string]db.KindStat) (cost float64, priced bool, estimated bool) {
	priced = true
	for key, st := range byModel {
		provider, model, _ := strings.Cut(key, "|")
		c, _, p, e := billing.PriceStat(provider, model, st)
		cost += c
		if !p {
			priced = false
		}
		if e {
			estimated = true
		}
	}
	return cost, priced, estimated
}

// modelRowsFor turns a usage rollup's per-model breakdown into sorted detail
// rows (costliest first) plus aggregate cost/savings/cache totals. Mirrors the
// workspace screen's per-model logic (real price, else equivalent-API estimate)
// so the per-agent usage endpoint computes model cost identically.
func modelRowsFor(byModel map[string]db.KindStat) (rows []modelStat, totalCost, totalSavings float64, priced, estimated bool, cacheRead, cacheWrite int) {
	priced = true
	for key, st := range byModel {
		provider, model, _ := strings.Cut(key, "|")
		cost, save, modelPriced, modelEstimated := billing.PriceStat(provider, model, st)
		if !modelPriced {
			priced = false
		}
		if modelEstimated {
			estimated = true
		}
		rows = append(rows, modelStat{
			Model:       model,
			tokenTotals: tokenTotals{Calls: st.Calls, InputTokens: st.InputTokens, OutputTokens: st.OutputTokens, CacheReadTokens: st.CacheReadTokens, CacheWriteTokens: st.CacheWriteTokens},
			CostUSD:     cost, SavingsUSD: save, Priced: modelPriced, Estimated: modelEstimated,
		})
		totalCost += cost
		totalSavings += save
		cacheRead += st.CacheReadTokens
		cacheWrite += st.CacheWriteTokens
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].CostUSD != rows[j].CostUSD {
			return rows[i].CostUSD > rows[j].CostUSD
		}
		return rows[i].InputTokens+rows[i].OutputTokens > rows[j].InputTokens+rows[j].OutputTokens
	})
	return rows, totalCost, totalSavings, priced, estimated, cacheRead, cacheWrite
}

// dayPoint is one day's workspace-wide totals for the trend chart. Cache and
// cost/savings are carried per day so the trend can plot caching ROI over time
// (not just token volume) and so the window-cumulative totals can be summed
// straight off the trend.
type dayPoint struct {
	Day string `json:"day"`
	tokenTotals
	CostUSD              float64 `json:"costUSD"`
	SavingsUSD           float64 `json:"savingsUSD"`
	CompactSavedBytes    int     `json:"compactSavedBytes"`
	CompactSavedBytesLLM int     `json:"compactSavedBytesLLM"`
}

// handleWorkspaceUsage returns the data behind the Budget screen: today's
// workspace-wide totals (with a per-origin breakdown), a per-agent table joined
// with each agent's display identity and caps, and a daily trend over the last
// ?days= days (default 7, clamped 1..90). All numbers come from the already
// loaded usage rollups — no extra disk reads beyond what's in memory.
func (s *Server) handleWorkspaceUsage(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()

	days := 7
	if q := r.URL.Query().Get("days"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n >= 1 && n <= 90 {
			days = n
		}
	}

	agents, err := wsp.DB.ListAgents(ctx)
	if writeDBError(w, err, "") {
		return
	}

	today := db.Today()
	totals := kindStat{}
	byKind := map[string]kindStat{}
	byProvider := map[string]*providerStat{}
	// provider -> model -> accumulating detail row.
	byModel := map[string]map[string]*modelStat{}
	var totalCost, totalSavings float64
	var totalCacheRead, totalCacheWrite int
	var totalCompactBytes, totalCompactBytesLLM int
	totalPriced := true
	totalEstimated := false
	rows := make([]agentBudgetRow, 0, len(agents))

	for _, a := range agents {
		u, uerr := wsp.DB.GetUsageToday(ctx, a.ID)
		if uerr != nil {
			continue
		}
		cost, priced, estimated := costOf(u.ByModel)
		row := agentBudgetRow{
			AgentID:              a.ID,
			Name:                 a.Name,
			Avatar:               a.Avatar,
			Color:                a.Color,
			Provider:             a.Provider,
			Calls:                u.Calls,
			InputTokens:          u.InputTokens,
			OutputTokens:         u.OutputTokens,
			CostUSD:              cost,
			Priced:               priced,
			Estimated:            estimated,
			DailyCallLimit:       a.DailyCallLimit,
			DailyTokenLimit:      a.DailyTokenLimit,
			CompactSavedBytes:    u.CompactSavedBytes,
			CompactSavedBytesLLM: u.CompactSavedBytesLLM,
		}
		totalCompactBytes += u.CompactSavedBytes
		totalCompactBytesLLM += u.CompactSavedBytesLLM
		if len(u.ByKind) > 0 {
			row.ByKind = map[string]kindStat{}
			for k, st := range u.ByKind {
				row.ByKind[k] = kindStat{Calls: st.Calls, InputTokens: st.InputTokens, OutputTokens: st.OutputTokens}
				agg := byKind[k]
				agg.Calls += st.Calls
				agg.InputTokens += st.InputTokens
				agg.OutputTokens += st.OutputTokens
				byKind[k] = agg
			}
		}
		// Aggregate this agent's per-model rows up to per-provider totals + cost,
		// and into the per-model detail grouped under each provider.
		for key, st := range u.ByModel {
			provider, model, _ := strings.Cut(key, "|")
			mCost, mSave, mPriced, mEstimated := billing.PriceStat(provider, model, st)

			ps := byProvider[provider]
			if ps == nil {
				ps = &providerStat{Provider: provider, Priced: true}
				byProvider[provider] = ps
			}
			ps.Calls += st.Calls
			ps.InputTokens += st.InputTokens
			ps.OutputTokens += st.OutputTokens
			ps.CacheReadTokens += st.CacheReadTokens
			ps.CacheWriteTokens += st.CacheWriteTokens
			ps.CostUSD += mCost
			ps.SavingsUSD += mSave
			if !mPriced {
				ps.Priced = false
			}
			if mEstimated {
				ps.Estimated = true
			}

			mm := byModel[provider]
			if mm == nil {
				mm = map[string]*modelStat{}
				byModel[provider] = mm
			}
			ms := mm[model]
			if ms == nil {
				ms = &modelStat{Model: model, Priced: true}
				mm[model] = ms
			}
			ms.Calls += st.Calls
			ms.InputTokens += st.InputTokens
			ms.OutputTokens += st.OutputTokens
			ms.CacheReadTokens += st.CacheReadTokens
			ms.CacheWriteTokens += st.CacheWriteTokens
			ms.CostUSD += mCost
			ms.SavingsUSD += mSave
			if !mPriced {
				ms.Priced = false
			}
			if mEstimated {
				ms.Estimated = true
			}

			totalSavings += mSave
			totalCacheRead += st.CacheReadTokens
			totalCacheWrite += st.CacheWriteTokens
		}
		totals.Calls += u.Calls
		totals.InputTokens += u.InputTokens
		totals.OutputTokens += u.OutputTokens
		totalCost += cost
		if !priced {
			totalPriced = false
		}
		if estimated {
			totalEstimated = true
		}
		rows = append(rows, row)
	}

	// Heaviest spenders first so the table leads with what matters.
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].InputTokens+rows[i].OutputTokens > rows[j].InputTokens+rows[j].OutputTokens
	})

	// Providers as a slice, costliest first, each carrying its model detail rows
	// (also costliest first).
	providerRows := make([]providerStat, 0, len(byProvider))
	for name, ps := range byProvider {
		for _, ms := range byModel[name] {
			ps.Models = append(ps.Models, *ms)
		}
		sort.SliceStable(ps.Models, func(i, j int) bool {
			if ps.Models[i].CostUSD != ps.Models[j].CostUSD {
				return ps.Models[i].CostUSD > ps.Models[j].CostUSD
			}
			return ps.Models[i].InputTokens+ps.Models[i].OutputTokens > ps.Models[j].InputTokens+ps.Models[j].OutputTokens
		})
		providerRows = append(providerRows, *ps)
	}
	sort.SliceStable(providerRows, func(i, j int) bool {
		if providerRows[i].CostUSD != providerRows[j].CostUSD {
			return providerRows[i].CostUSD > providerRows[j].CostUSD
		}
		return providerRows[i].InputTokens+providerRows[i].OutputTokens > providerRows[j].InputTokens+providerRows[j].OutputTokens
	})

	// Trend: aggregate every agent's rows per day over the window.
	since := time.Now().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	hist, err := wsp.DB.UsageHistory(ctx, since)
	if writeDBError(w, err, "") {
		return
	}
	perDay := map[string]*dayPoint{}
	for _, u := range hist {
		p := perDay[u.Day]
		if p == nil {
			p = &dayPoint{Day: u.Day}
			perDay[u.Day] = p
		}
		p.Calls += u.Calls
		p.InputTokens += u.InputTokens
		p.OutputTokens += u.OutputTokens
		// Cost/savings/cache per day come from the same per-model logic the
		// today screen uses (real price, else equivalent-API estimate), so the
		// trend plots caching ROI consistently with the headline figures.
		_, dayCost, daySavings, _, _, dayCacheRead, dayCacheWrite := modelRowsFor(u.ByModel)
		p.CostUSD += dayCost
		p.SavingsUSD += daySavings
		p.CacheReadTokens += dayCacheRead
		p.CacheWriteTokens += dayCacheWrite
		p.CompactSavedBytes += u.CompactSavedBytes
		p.CompactSavedBytesLLM += u.CompactSavedBytesLLM
	}
	trend := make([]dayPoint, 0, len(perDay))
	for _, p := range perDay {
		trend = append(trend, *p)
	}
	sort.SliceStable(trend, func(i, j int) bool { return trend[i].Day < trend[j].Day })

	// Window-cumulative totals ("oturumlar arası toplam" / caching ROI): sum the
	// whole trend window so the screen can show lifetime-over-the-window spend,
	// savings, and a cache hit rate — not just today. cacheHitRate is the share
	// of prompt tokens served from cache: cacheRead / (cacheRead + freshInput +
	// cacheWrite). It is the single ROI signal — higher means the static prefix
	// is being reused instead of re-paid.
	var cumCalls, cumIn, cumOut, cumCacheRead, cumCacheWrite int
	var cumCost, cumSavings float64
	var cumCompactBytes, cumCompactBytesLLM int
	for _, p := range trend {
		cumCalls += p.Calls
		cumIn += p.InputTokens
		cumOut += p.OutputTokens
		cumCacheRead += p.CacheReadTokens
		cumCacheWrite += p.CacheWriteTokens
		cumCost += p.CostUSD
		cumSavings += p.SavingsUSD
		cumCompactBytes += p.CompactSavedBytes
		cumCompactBytesLLM += p.CompactSavedBytesLLM
	}
	var cacheHitRate float64
	if denom := cumCacheRead + cumIn + cumCacheWrite; denom > 0 {
		cacheHitRate = float64(cumCacheRead) / float64(denom)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"day": today,
		"totals": map[string]any{
			"calls":                totals.Calls,
			"inputTokens":          totals.InputTokens,
			"outputTokens":         totals.OutputTokens,
			"cacheReadTokens":      totalCacheRead,
			"cacheWriteTokens":     totalCacheWrite,
			"byKind":               byKind,
			"costUSD":              totalCost,
			"savingsUSD":           totalSavings,         // saved by prompt-cache reads vs full input price
			"priced":               totalPriced,          // false when some spend is unpriced (subscription/custom)
			"estimated":            totalEstimated,       // true when cost includes equivalent-API estimates (e.g. claude-cli)
			"compactSavedBytes":    totalCompactBytes,    // System A: bytes trimmed from tool output (deterministic)
			"compactSavedBytesLLM": totalCompactBytesLLM, // System B: bytes trimmed by LLM summary
		},
		"byProvider": providerRows,
		"agents":     rows,
		"trend":      trend,
		"cumulative": map[string]any{
			"days":                 days,
			"calls":                cumCalls,
			"inputTokens":          cumIn,
			"outputTokens":         cumOut,
			"cacheReadTokens":      cumCacheRead,
			"cacheWriteTokens":     cumCacheWrite,
			"costUSD":              cumCost,
			"savingsUSD":           cumSavings,         // total saved by prompt-cache reads over the window
			"cacheHitRate":         cacheHitRate,       // cacheRead / (cacheRead + input + cacheWrite)
			"compactSavedBytes":    cumCompactBytes,    // System A bytes trimmed over the window
			"compactSavedBytesLLM": cumCompactBytesLLM, // System B bytes trimmed over the window
		},
	})
}
