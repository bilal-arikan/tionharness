package goals

import (
	"errors"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func f(v float64) *float64 { return &v }

func validGoal() db.Goal {
	return db.Goal{
		Name:       "Kod inceleme ucuzlasın",
		Status:     db.GoalStatusDraft,
		Primary:    db.GoalMetric{Metric: "recipe.avgCostUSD", Direction: "min"},
		Guardrails: []db.GoalGuardrail{{Metric: "recipe.successRate", Min: f(0.9)}},
		Policy:     db.GoalPolicy{Mode: db.GoalModePropose},
	}
}

// TestValidateRules: every rule the writer prompt calls "enforced" is enforced
// here, whoever wrote the goal.
func TestValidateRules(t *testing.T) {
	g := validGoal()
	Normalize(&g)
	if err := Validate(g); err != nil {
		t.Fatalf("valid goal rejected: %v", err)
	}
	if g.Kind != db.GoalKindMetric {
		t.Fatalf("kind defaulted to %q, want metric", g.Kind)
	}
	cases := []struct {
		name string
		mut  func(*db.Goal)
		want string
	}{
		{"empty name", func(g *db.Goal) { g.Name = "" }, "name is required"},
		{"unknown metric", func(g *db.Goal) { g.Primary.Metric = "recipe.vibes" }, "unknown primary metric"},
		{"bad direction", func(g *db.Goal) { g.Primary.Direction = "up" }, "direction must be"},
		{"guardrail unknown", func(g *db.Goal) { g.Guardrails[0].Metric = "nope" }, "unknown metric"},
		{"guardrail no bound", func(g *db.Goal) { g.Guardrails[0].Min = nil }, "needs a min or a max"},
		{"guardrail min>max", func(g *db.Goal) { g.Guardrails[0].Max = f(0.5) }, "min above max"},
		{"guardrail equals primary", func(g *db.Goal) { g.Guardrails[0].Metric = "recipe.avgCostUSD" }, "already the primary"},
		{"guardrail duplicate", func(g *db.Goal) {
			g.Guardrails = append(g.Guardrails, db.GoalGuardrail{Metric: "recipe.successRate", Max: f(1)})
		}, "listed twice"},
		{"auto without surfaces", func(g *db.Goal) { g.Policy.Mode = db.GoalModeAuto }, "auto mode needs"},
		{"auto unsafe surface", func(g *db.Goal) {
			g.Policy.Mode = db.GoalModeAuto
			g.Policy.AutoApplySurfaces = []string{"soul"}
		}, "may not be auto-applied"},
		{"bad status", func(g *db.Goal) { g.Status = "done" }, "unknown status"},
		{"priority range", func(g *db.Goal) { g.Priority = 9 }, "priority must be"},
		{"rubric metric without rubric", func(g *db.Goal) { g.Primary.Metric = "judge.rubricScore"; g.Primary.Direction = "max" }, "needs a rubric"},
		{"too long", func(g *db.Goal) { g.Description = strings.Repeat("x", MaxTextLen+1) }, "longer than"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := validGoal()
			tc.mut(&g)
			Normalize(&g)
			err := Validate(g)
			if err == nil || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
	// A safe auto policy passes.
	g = validGoal()
	g.Policy = db.GoalPolicy{Mode: db.GoalModeAuto, AutoApplySurfaces: []string{"thinkingLevel"}}
	Normalize(&g)
	if err := Validate(g); err != nil {
		t.Fatalf("safe auto policy rejected: %v", err)
	}
}

// TestNormalizeDefaults: direction falls back to the catalog default, kind is
// derived from the rubric, lists are trimmed and de-duplicated.
func TestNormalizeDefaults(t *testing.T) {
	g := db.Goal{Name: " x ", Primary: db.GoalMetric{Metric: "recipe.successRate"}, Rubric: "iyi", Scope: db.GoalScope{Recipes: []string{" a ", "a", ""}}}
	Normalize(&g)
	if g.Primary.Direction != "max" {
		t.Fatalf("direction = %q, want catalog default max", g.Primary.Direction)
	}
	if g.Kind != db.GoalKindMixed {
		t.Fatalf("kind = %q, want mixed (metric + rubric)", g.Kind)
	}
	if g.Status != db.GoalStatusDraft || g.Policy.Mode != db.GoalModePropose {
		t.Fatalf("defaults: status=%q mode=%q", g.Status, g.Policy.Mode)
	}
	if len(g.Scope.Recipes) != 1 || g.Scope.Recipes[0] != "a" {
		t.Fatalf("scope not cleaned: %v", g.Scope.Recipes)
	}
	if g.Guardrails == nil {
		t.Fatal("guardrails must be an empty slice, not nil")
	}
}

// TestCanActivate: open questions block activation.
func TestCanActivate(t *testing.T) {
	g := validGoal()
	g.Questions = []string{"Hangi reçete?"}
	Normalize(&g)
	if err := CanActivate(g); err == nil || !strings.Contains(err.Error(), "open question") {
		t.Fatalf("want open-question error, got %v", err)
	}
	g.Questions = nil
	if err := CanActivate(g); err != nil {
		t.Fatalf("want activatable, got %v", err)
	}
}

// TestFromDraft: the writer can never escalate policy, the raw text is kept,
// and a rewrite of an active goal falls back to draft for re-confirmation.
func TestFromDraft(t *testing.T) {
	d := Draft{
		Name: "Ucuz inceleme", Summary: "s", Description: "d", Kind: "metric", Priority: 2,
		Scope:      Scope{Recipes: []string{"code-review"}},
		Primary:    Primary{Metric: "recipe.avgCostUSD", Direction: "min", Target: f(0.5)},
		Guardrails: []Guard{{Metric: "recipe.successRate", Min: f(0.9)}},
		Policy:     Policy{Mode: "auto", AutoApplySurfaces: []string{"thinkingLevel"}, CooldownHours: 72, MinRuns: 5},
		Questions:  []string{"q"},
	}
	g := FromDraft(d, "  incelemeler pahalı  ", nil)
	if g.Policy.Mode != db.GoalModePropose || len(g.Policy.AutoApplySurfaces) != 0 {
		t.Fatalf("writer must not set auto policy: %+v", g.Policy)
	}
	if g.RawText != "incelemeler pahalı" || g.CreatedBy != db.GoalByWriter || g.Status != db.GoalStatusDraft {
		t.Fatalf("draft shape: raw=%q by=%q status=%q", g.RawText, g.CreatedBy, g.Status)
	}
	if g.Primary.Target == nil || *g.Primary.Target != 0.5 || len(g.Guardrails) != 1 || g.Guardrails[0].Min == nil {
		t.Fatalf("numbers lost: %+v %+v", g.Primary, g.Guardrails)
	}
	if err := Validate(g); err != nil {
		t.Fatalf("draft invalid: %v", err)
	}
	base := db.Goal{ID: "GOL3", Status: db.GoalStatusActive, CreatedBy: db.GoalByUser, CreatedAt: 7, History: []db.GoalRevision{{At: 7}}}
	g = FromDraft(d, "yeni cümle", &base)
	if g.ID != "GOL3" || g.Status != db.GoalStatusDraft || g.CreatedBy != db.GoalByUser || g.CreatedAt != 7 || len(g.History) != 1 {
		t.Fatalf("rewrite must keep identity and drop to draft: %+v", g)
	}
	// An unbounded or unknown guardrail becomes a question, not a refusal.
	d.Guardrails = append(d.Guardrails, Guard{Metric: "recipe.avgSessions"}, Guard{Metric: "vibes", Min: f(1)})
	g = FromDraft(d, "x", nil)
	if len(g.Guardrails) != 1 || len(g.Questions) != 3 {
		t.Fatalf("lenient guardrails: %+v / %v", g.Guardrails, g.Questions)
	}
	if err := Validate(g); err != nil {
		t.Fatalf("draft with dropped guardrails must validate: %v", err)
	}
	d.Guardrails = d.Guardrails[:1]
	base.Status = db.GoalStatusPaused
	if g = FromDraft(d, "x", &base); g.Status != db.GoalStatusPaused {
		t.Fatalf("a paused goal stays paused on rewrite, got %q", g.Status)
	}
}

// TestCatalogIntegrity: keys unique, directions valid, lookups consistent.
func TestCatalogIntegrity(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Catalog() {
		if seen[m.Key] {
			t.Fatalf("duplicate metric key %s", m.Key)
		}
		seen[m.Key] = true
		if m.DefaultDirection != "min" && m.DefaultDirection != "max" {
			t.Fatalf("%s: bad default direction %q", m.Key, m.DefaultDirection)
		}
		if m.Label == "" || m.Unit == "" || m.Source == "" {
			t.Fatalf("%s: incomplete entry", m.Key)
		}
		if _, ok := Lookup(m.Key); !ok {
			t.Fatalf("%s: lookup failed", m.Key)
		}
	}
	if len(Keys()) != len(seen) {
		t.Fatal("Keys() disagrees with Catalog()")
	}
	if !IsAutoApplySurface("thinkingLevel") || IsAutoApplySurface("model") {
		t.Fatal("auto-apply surface allowlist wrong")
	}
}
