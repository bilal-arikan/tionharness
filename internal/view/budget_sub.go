package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/billing"
)

// projectBudgetSub renders one provider's share of the day's spend
// (Sub "provider:<name>"): the leaves under the Bütçe node of the Explorer map.
// Rows are carried straight from the rollup, filtered by provider — no
// re-pricing. A provider without rows today is an error, not a blank card.
func projectBudgetSub(in BudgetInput, level Level) (View, error) {
	if !strings.HasPrefix(in.Sub, BudgetSubProviderPrefix) {
		return View{}, fmt.Errorf("view: budget: unknown selector %q", in.Sub)
	}
	name := strings.TrimPrefix(in.Sub, BudgetSubProviderPrefix)
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	var rows []billing.Row
	var cost float64
	estimated := false
	for _, row := range in.Rollup.Rows {
		rowProvider := row.Provider
		if rowProvider == "" {
			rowProvider = "-"
		}
		if rowProvider != name {
			continue
		}
		rows = append(rows, row)
		cost += row.CostUSD
		if row.Estimated || !row.Priced {
			estimated = true
		}
	}
	if len(rows) == 0 {
		return View{}, fmt.Errorf("view: budget: no spend for provider %q", name)
	}
	tokens := rollupTokens(billing.Rollup{Rows: rows})

	day := in.Day
	if day == "" {
		day = "bugün"
	}
	v := View{
		Ref:    Ref{Kind: KindBudget, ID: BudgetRefID, Sub: in.Sub},
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("%s/%s/%d", in.Day, name, len(rows)),
	}
	v.Header = fmt.Sprintf("BUDGET · %s · %s · %d model · %s tok · gün %s · asOf %s",
		name, usd(cost, estimated), len(rows), compactCount(tokens), day, hhmmss(now))
	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	limit := len(rows)
	if level != LevelFull && limit > budgetModelRows {
		limit = budgetModelRows
	}
	var l lines
	for _, row := range rows[:limit] {
		rowTokens := int64(row.Stat.InputTokens) + int64(row.Stat.OutputTokens) +
			int64(row.Stat.CacheReadTokens) + int64(row.Stat.CacheWriteTokens)
		l.add("%-28s %-9s %s tok · %d çağrı", clip(orDash(row.Model), 28),
			usd(row.CostUSD, row.Estimated || !row.Priced), compactCount(rowTokens), row.Stat.Calls)
	}
	if dropped := len(rows) - limit; dropped > 0 {
		v.Elided, v.ElidedUnit = dropped, "model"
	}
	v.Body = l.String()
	v.finalize()
	return v, nil
}
