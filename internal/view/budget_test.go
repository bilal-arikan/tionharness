package view

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/billing"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// budgetFixture hand-builds a priced rollup so the projection is tested without
// depending on the live price tables — the projection RENDERS a rollup, it never
// prices one, so the rollup is the correct seam to inject at.
func budgetFixture() BudgetInput {
	return BudgetInput{
		Day: "2026-08-06",
		Rollup: billing.Rollup{
			CostUSD: 3.14,
			Priced:  true,
			Rows: []billing.Row{
				{Provider: "anthropic", Model: "opus", CostUSD: 2.50, Priced: true,
					Stat: db.KindStat{Calls: 10, InputTokens: 500_000, OutputTokens: 100_000}},
				{Provider: "openai", Model: "gpt", CostUSD: 0.64, Priced: true,
					Stat: db.KindStat{Calls: 4, InputTokens: 120_000, OutputTokens: 40_000}},
			},
		},
	}
}

func TestBudgetHeaderCarriesRollupTotals(t *testing.T) {
	v, err := ProjectBudget(budgetFixture(), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	for _, want := range []string{"BUDGET", "$3.14", "2 model", "gün 2026-08-06"} {
		if !strings.Contains(v.Header, want) {
			t.Errorf("header missing %q: %q", want, v.Header)
		}
	}
	// The covered period is named once. It used to appear as both a "bugün" beside
	// the cost and a "gün <date>" at the end, which read as two periods.
	if strings.Contains(v.Header, "bugün") {
		t.Errorf("day must be named once, not twice: %q", v.Header)
	}
}

func TestBudgetBodyListsModelRows(t *testing.T) {
	v, err := ProjectBudget(budgetFixture(), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "anthropic/opus") || !strings.Contains(txt, "$2.50") {
		t.Errorf("costliest model row missing:\n%s", txt)
	}
	if !strings.Contains(txt, "openai/gpt") {
		t.Errorf("second model row missing:\n%s", txt)
	}
}

func TestBudgetCardCapsModelsAndCountsElided(t *testing.T) {
	in := budgetFixture()
	for i := 0; i < budgetModelRows+3; i++ {
		in.Rollup.Rows = append(in.Rollup.Rows, billing.Row{
			Provider: "p", Model: string(rune('a' + i)), CostUSD: 0.01, Priced: true,
			Stat: db.KindStat{Calls: 1, InputTokens: 100},
		})
	}
	v, err := ProjectBudget(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Elided != len(in.Rollup.Rows)-budgetModelRows || v.ElidedUnit != "model" {
		t.Errorf("elision wrong: elided=%d unit=%q (rows=%d)", v.Elided, v.ElidedUnit, len(in.Rollup.Rows))
	}
	// LevelFull lists every model — no elision.
	full, err := ProjectBudget(in, LevelFull)
	if err != nil {
		t.Fatalf("project full: %v", err)
	}
	if full.Elided != 0 {
		t.Errorf("full level must not elide models, got %d", full.Elided)
	}
}

func TestBudgetEmptyIsExplicit(t *testing.T) {
	v, err := ProjectBudget(BudgetInput{Day: "2026-08-06"}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "kayıtlı model harcaması yok") {
		t.Errorf("empty budget not stated:\n%s", v.Text())
	}
}

func TestBudgetTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectBudget(budgetFixture(), LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
}
