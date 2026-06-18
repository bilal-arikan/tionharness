package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
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
	Priced          bool                `json:"priced"` // false when any of this agent's spend is unpriced (e.g. claude-cli)
	DailyCallLimit  int                 `json:"dailyCallLimit"`
	DailyTokenLimit int                 `json:"dailyTokenLimit"`
}

// modelStat is one provider+model's slice of the spend — the detail row under a
// provider. CacheRead/Write are the prompt-cache token tiers, SavingsUSD the
// amount cache reads saved versus paying full input price.
type modelStat struct {
	Model            string  `json:"model"`
	Calls            int     `json:"calls"`
	InputTokens      int     `json:"inputTokens"`
	OutputTokens     int     `json:"outputTokens"`
	CacheReadTokens  int     `json:"cacheReadTokens"`
	CacheWriteTokens int     `json:"cacheWriteTokens"`
	CostUSD          float64 `json:"costUSD"`
	SavingsUSD       float64 `json:"savingsUSD"`
	Priced           bool    `json:"priced"`
}

// providerStat is one provider's slice of the workspace's spend, with the USD
// cost where the models are priced and a per-model detail list. Priced is false
// when any of the provider's spend carries no list price (claude-cli
// subscription, or a custom/unknown model).
type providerStat struct {
	Provider         string      `json:"provider"`
	Calls            int         `json:"calls"`
	InputTokens      int         `json:"inputTokens"`
	OutputTokens     int         `json:"outputTokens"`
	CacheReadTokens  int         `json:"cacheReadTokens"`
	CacheWriteTokens int         `json:"cacheWriteTokens"`
	CostUSD          float64     `json:"costUSD"`
	SavingsUSD       float64     `json:"savingsUSD"`
	Priced           bool        `json:"priced"`
	Models           []modelStat `json:"models"`
}

// costOf sums the USD cost (including cache tiers) of a usage rollup's per-model
// breakdown and reports whether every model in it was priced. Tokens with no
// list price contribute 0 to cost and flip priced to false.
func costOf(byModel map[string]db.KindStat) (cost float64, priced bool) {
	priced = true
	for key, st := range byModel {
		provider, model, _ := strings.Cut(key, "|")
		if p, ok := providers.PriceFor(provider, model); ok {
			cost += p.CostDetailed(st.InputTokens, st.OutputTokens, st.CacheReadTokens, st.CacheWriteTokens)
		} else if st.InputTokens+st.OutputTokens+st.CacheReadTokens+st.CacheWriteTokens > 0 {
			priced = false
		}
	}
	return cost, priced
}

// dayPoint is one day's workspace-wide totals for the trend chart.
type dayPoint struct {
	Day          string `json:"day"`
	Calls        int    `json:"calls"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
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
	totalPriced := true
	rows := make([]agentBudgetRow, 0, len(agents))

	for _, a := range agents {
		u, uerr := wsp.DB.GetUsageToday(ctx, a.ID)
		if uerr != nil {
			continue
		}
		cost, priced := costOf(u.ByModel)
		row := agentBudgetRow{
			AgentID:         a.ID,
			Name:            a.Name,
			Avatar:          a.Avatar,
			Color:           a.Color,
			Provider:        a.Provider,
			Calls:           u.Calls,
			InputTokens:     u.InputTokens,
			OutputTokens:    u.OutputTokens,
			CostUSD:         cost,
			Priced:          priced,
			DailyCallLimit:  a.DailyCallLimit,
			DailyTokenLimit: a.DailyTokenLimit,
		}
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
			price, ok := providers.PriceFor(provider, model)
			var mCost, mSave float64
			if ok {
				mCost = price.CostDetailed(st.InputTokens, st.OutputTokens, st.CacheReadTokens, st.CacheWriteTokens)
				mSave = price.CacheSavings(st.CacheReadTokens)
			}

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
			if !ok && st.InputTokens+st.OutputTokens+st.CacheReadTokens+st.CacheWriteTokens > 0 {
				ps.Priced = false
			}

			mm := byModel[provider]
			if mm == nil {
				mm = map[string]*modelStat{}
				byModel[provider] = mm
			}
			ms := mm[model]
			if ms == nil {
				ms = &modelStat{Model: model, Priced: ok}
				mm[model] = ms
			}
			ms.Calls += st.Calls
			ms.InputTokens += st.InputTokens
			ms.OutputTokens += st.OutputTokens
			ms.CacheReadTokens += st.CacheReadTokens
			ms.CacheWriteTokens += st.CacheWriteTokens
			ms.CostUSD += mCost
			ms.SavingsUSD += mSave
			if !ok && st.InputTokens+st.OutputTokens+st.CacheReadTokens+st.CacheWriteTokens > 0 {
				ms.Priced = false
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
	}
	trend := make([]dayPoint, 0, len(perDay))
	for _, p := range perDay {
		trend = append(trend, *p)
	}
	sort.SliceStable(trend, func(i, j int) bool { return trend[i].Day < trend[j].Day })

	writeJSON(w, http.StatusOK, map[string]any{
		"day": today,
		"totals": map[string]any{
			"calls":            totals.Calls,
			"inputTokens":      totals.InputTokens,
			"outputTokens":     totals.OutputTokens,
			"cacheReadTokens":  totalCacheRead,
			"cacheWriteTokens": totalCacheWrite,
			"byKind":           byKind,
			"costUSD":          totalCost,
			"savingsUSD":       totalSavings, // saved by prompt-cache reads vs full input price
			"priced":           totalPriced,  // false when some spend is unpriced (subscription/custom)
		},
		"byProvider": providerRows,
		"agents":     rows,
		"trend":      trend,
	})
}
