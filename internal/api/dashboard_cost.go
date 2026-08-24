package api

// Cost and period-delta helpers for the workspace overview. Kept beside the
// dashboard handler but in their own file: the money math routes through
// internal/billing (the single pricing source), while the counters in
// dashboard.go stay pure token/entity counting.

import (
	"context"
	"time"

	"github.com/bilal-arikan/tionharness/internal/billing"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// dayCostPoint is one bucket of a daily USD series — the float sibling of
// daySeriesPoint, which only carries integer counts.
type dayCostPoint struct {
	Day   string  `json:"day"` // YYYY-MM-DD
	Value float64 `json:"value"`
}

// namedCost is one agent's priced spend for the "costliest agents" ranking, the
// money counterpart to topAgents (which ranks by session volume).
type namedCost struct {
	Name string  `json:"name"`
	Cost float64 `json:"cost"`
}

// costBlock is the headline money summary. Every figure is priced by
// billing.RollupOf so it can never disagree with the Budget screen.
type costBlock struct {
	Today          float64 `json:"today"`
	Month          float64 `json:"month"`          // calendar month-to-date
	Estimated      bool    `json:"estimated"`      // any spend priced via equivalent-API estimate
	BurnRate       float64 `json:"burnRate"`       // average USD/day over the window
	ProjectedMonth float64 `json:"projectedMonth"` // burnRate × days in the current month
	CoolingWaste   float64 `json:"coolingWaste"`   // avoidable cache-cooling overpay over the window
}

// deltaStat compares a metric's current window against the previous one of equal
// length. Pct is nil when there is no baseline (prev == 0): a percentage change
// off zero is undefined, and rendering "+∞%" or "+100%" would both mislead.
type deltaStat struct {
	Curr float64  `json:"curr"`
	Prev float64  `json:"prev"`
	Pct  *float64 `json:"pct"`
}

// makeDelta builds a deltaStat, leaving Pct nil when the previous period was zero.
func makeDelta(curr, prev float64) deltaStat {
	d := deltaStat{Curr: curr, Prev: prev}
	if prev > 0 {
		p := (curr - prev) / prev
		d.Pct = &p
	}
	return d
}

// rollupCost prices one usage row's per-model breakdown. estimated latches when
// any slice used an equivalent-API estimate or had no real list price.
func rollupCost(u db.Usage) (float64, bool) {
	roll := billing.RollupOf(u.ByModel)
	return roll.CostUSD, roll.Estimated || !roll.Priced
}

// keySet turns a day-key list into a membership set.
func keySet(keys []string) map[string]struct{} {
	m := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		m[k] = struct{}{}
	}
	return m
}

// dashboardCost computes the money summary and the per-day USD series in one pass
// over the usage history, so the two never diverge. It reads the whole month plus
// the trend window (whichever is longer) once from the in-memory usage map.
func dashboardCost(ctx context.Context, database *db.DB, days int, now time.Time) (costBlock, []dayCostPoint) {
	windowKeys := dayKeys(days, now)
	firstOfMonth := now.Format("2006-01") + "-01"
	// Read from whichever start is earlier so a single pass covers both the trend
	// window and month-to-date.
	since := windowKeys[0]
	if firstOfMonth < since {
		since = firstOfMonth
	}

	rows, err := database.UsageHistory(ctx, since)
	if err != nil {
		// Cost degrades to zero rather than failing the dashboard; the counters and
		// the projection above it are still correct.
		return costBlock{}, zeroCostSeries(windowKeys)
	}

	inWindow := keySet(windowKeys)
	today := db.Today()

	series := make([]dayCostPoint, len(windowKeys))
	idx := make(map[string]int, len(windowKeys))
	for i, k := range windowKeys {
		series[i] = dayCostPoint{Day: k}
		idx[k] = i
	}

	var block costBlock
	var windowCost float64
	for _, u := range rows {
		cost, estimated := rollupCost(u)
		if estimated && cost > 0 {
			block.Estimated = true
		}
		if u.Day >= firstOfMonth {
			block.Month += cost
		}
		if u.Day == today {
			block.Today += cost
		}
		if _, ok := inWindow[u.Day]; ok {
			windowCost += cost
			block.CoolingWaste += u.CoolingWasteUSD
			if i, ok := idx[u.Day]; ok {
				series[i].Value += cost
			}
		}
	}

	block.BurnRate = windowCost / float64(days)
	block.ProjectedMonth = block.BurnRate * float64(daysInMonth(now))
	return block, series
}

// zeroCostSeries returns an all-zero series covering the keys, so a failed usage
// read still draws a full-width axis instead of an empty chart.
func zeroCostSeries(keys []string) []dayCostPoint {
	out := make([]dayCostPoint, len(keys))
	for i, k := range keys {
		out[i] = dayCostPoint{Day: k}
	}
	return out
}

// daysInMonth returns the number of days in the calendar month of t.
func daysInMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
}

// topAgentsByCost ranks agents by priced spend over the window, resolving display
// names. It sums each agent's daily rollups so the ranking matches the money the
// Budget screen attributes, not the session count topAgents uses.
func topAgentsByCost(ctx context.Context, database *db.DB, agents []db.Agent, days int, now time.Time) []namedCost {
	rows, err := database.UsageHistory(ctx, dayKeys(days, now)[0])
	if err != nil {
		// Non-nil empty so the field serializes as [] not null (the UI reads .length).
		return []namedCost{}
	}
	inWindow := keySet(dayKeys(days, now))
	name := make(map[string]string, len(agents))
	for _, a := range agents {
		name[a.ID] = a.Name
	}
	costs := map[string]float64{}
	for _, u := range rows {
		if _, ok := inWindow[u.Day]; !ok {
			continue
		}
		cost, _ := rollupCost(u)
		if cost <= 0 {
			continue
		}
		costs[u.AgentID] += cost
	}
	out := make([]namedCost, 0, len(costs))
	for id, c := range costs {
		label := name[id]
		if label == "" {
			// A deleted agent still owns spend; showing its id beats dropping the
			// cost and under-reporting the total.
			label = id
		}
		out = append(out, namedCost{Name: label, Cost: c})
	}
	// Costliest first; ties broken by name for a stable order across refreshes.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].Cost > out[j-1].Cost ||
			(out[j].Cost == out[j-1].Cost && out[j].Name < out[j-1].Name)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// windowSplitCounts buckets unix-second stamps into the current window and the
// equal-length window before it, for a period-over-period delta.
func windowSplitCounts(stamps []int64, days int, now time.Time) (curr, prev float64) {
	currKeys := keySet(dayKeys(days, now))
	prevKeys := keySet(dayKeys(days, now.AddDate(0, 0, -days)))
	for _, ts := range stamps {
		if ts <= 0 {
			continue
		}
		day := time.Unix(ts, 0).Format("2006-01-02")
		if _, ok := currKeys[day]; ok {
			curr++
		} else if _, ok := prevKeys[day]; ok {
			prev++
		}
	}
	return curr, prev
}

// usageWindowSplit sums a usage figure over the current window and the one before
// it. pick extracts the number to accumulate from a row (tokens, cost, …).
func usageWindowSplit(ctx context.Context, database *db.DB, days int, now time.Time,
	pick func(db.Usage) float64) (curr, prev float64) {

	prevStart := dayKeys(days, now.AddDate(0, 0, -days))[0]
	rows, err := database.UsageHistory(ctx, prevStart)
	if err != nil {
		return 0, 0
	}
	currKeys := keySet(dayKeys(days, now))
	prevKeys := keySet(dayKeys(days, now.AddDate(0, 0, -days)))
	for _, u := range rows {
		if _, ok := currKeys[u.Day]; ok {
			curr += pick(u)
		} else if _, ok := prevKeys[u.Day]; ok {
			prev += pick(u)
		}
	}
	return curr, prev
}

// dashboardDeltas builds the period-over-period comparison for the headline
// series. Sessions and runs come from creation stamps; tokens and cost from the
// usage history.
func dashboardDeltas(ctx context.Context, database *db.DB, sessions []db.Session,
	runs []db.FlowRun, days int, now time.Time) map[string]deltaStat {

	sessStamps := make([]int64, 0, len(sessions))
	for _, s := range sessions {
		sessStamps = append(sessStamps, s.CreatedAt)
	}
	runStamps := make([]int64, 0, len(runs))
	for _, r := range runs {
		runStamps = append(runStamps, r.CreatedAt)
	}

	sc, sp := windowSplitCounts(sessStamps, days, now)
	rc, rp := windowSplitCounts(runStamps, days, now)
	tc, tp := usageWindowSplit(ctx, database, days, now, func(u db.Usage) float64 {
		return float64(u.InputTokens) + float64(u.OutputTokens) +
			float64(u.CacheReadTokens) + float64(u.CacheWriteTokens)
	})
	cc, cp := usageWindowSplit(ctx, database, days, now, func(u db.Usage) float64 {
		cost, _ := rollupCost(u)
		return cost
	})

	return map[string]deltaStat{
		"sessions": makeDelta(sc, sp),
		"runs":     makeDelta(rc, rp),
		"tokens":   makeDelta(tc, tp),
		"cost":     makeDelta(cc, cp),
	}
}
