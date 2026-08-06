package view

import (
	"fmt"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/billing"
)

// BudgetInput is a pre-computed billing rollup plus the day it covers. The
// projection RENDERS this rollup; it never re-prices anything. The loader
// (Projector.loadBudget) merges the day's usage and calls billing.RollupOf once,
// so a budget view can never disagree with the Budget screen — both read the same
// computation.
type BudgetInput struct {
	Rollup billing.Rollup
	// Day is the calendar day (YYYY-MM-DD) the rollup covers, shown verbatim.
	Day string
	// Now is the clock used for the asOf stamp. Zero means time.Now().
	Now time.Time
}

// budgetModelRows is how many per-model rows a card-level budget view lists
// before the rest are reported as Elided. LevelFull lists them all.
const budgetModelRows = 6

// ProjectBudget renders today's spend broken down by model. Every figure is
// carried straight from the rollup: the projection adds no arithmetic of its own.
func ProjectBudget(in BudgetInput, level Level, lens Lens) (View, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	roll := in.Rollup
	estimated := roll.Estimated || !roll.Priced
	tokens := rollupTokens(roll)

	v := View{
		Ref:    Ref{Kind: KindBudget, ID: BudgetRefID},
		Level:  level,
		Lens:   lens,
		AsOf:   now,
		Source: fmt.Sprintf("%s/%d", in.Day, len(roll.Rows)),
	}

	day := in.Day
	if day == "" {
		day = "bugün"
	}
	v.Header = fmt.Sprintf("BUDGET · %s bugün · %d model · %s tok · gün %s · asOf %s",
		usd(roll.CostUSD, estimated), len(roll.Rows), compactCount(tokens), day, hhmmss(now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	if len(roll.Rows) == 0 {
		// No priced spend today is a real, distinct state — say it rather than
		// rendering a blank body that reads like a broken projection.
		l.add("(bugün kayıtlı model harcaması yok)")
		v.Body = l.String()
		v.finalize()
		return v, nil
	}

	limit := len(roll.Rows)
	if level != LevelFull && limit > budgetModelRows {
		limit = budgetModelRows
	}
	for _, row := range roll.Rows[:limit] {
		rowTokens := int64(row.Stat.InputTokens) + int64(row.Stat.OutputTokens) +
			int64(row.Stat.CacheReadTokens) + int64(row.Stat.CacheWriteTokens)
		l.add("%-28s %-9s %s tok · %d çağrı", clip(modelRowLabel(row), 28),
			usd(row.CostUSD, row.Estimated || !row.Priced), compactCount(rowTokens), row.Stat.Calls)
	}
	if dropped := len(roll.Rows) - limit; dropped > 0 {
		v.Elided, v.ElidedUnit = dropped, "model"
	}

	v.Body = l.String()
	v.finalize()
	return v, nil
}

// modelRowLabel renders a rollup row's provider/model identity, collapsing an
// empty model to just the provider (claude-cli's session default has none).
func modelRowLabel(row billing.Row) string {
	switch {
	case row.Model == "" && row.Provider == "":
		return "-"
	case row.Model == "":
		return row.Provider
	case row.Provider == "":
		return row.Model
	default:
		return row.Provider + "/" + row.Model
	}
}

// rollupTokens sums every token class across a rollup's rows.
func rollupTokens(roll billing.Rollup) int64 {
	var total int64
	for _, row := range roll.Rows {
		total += int64(row.Stat.InputTokens) + int64(row.Stat.OutputTokens) +
			int64(row.Stat.CacheReadTokens) + int64(row.Stat.CacheWriteTokens)
	}
	return total
}
