package goals

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Every shipped seed must be a goal the store would accept from any other
// writer: a valid shape, a propose-only policy, and a metric the fitness
// computation can actually measure today. A seed that breaks one of these is
// skipped silently at runtime (EnsureDefaultGoals drops invalid ones), so this
// test is the only thing that catches a malformed built-in.
func TestDefaultGoalsAreValidAndMeasurable(t *testing.T) {
	seen := map[string]bool{}
	for _, dg := range defaultGoals {
		if dg.Seed == "" {
			t.Fatal("a default goal has an empty seed key")
		}
		if seen[dg.Seed] {
			t.Fatalf("duplicate seed key %q", dg.Seed)
		}
		seen[dg.Seed] = true

		g := dg.Make()
		Normalize(&g)
		if err := Validate(g); err != nil {
			t.Fatalf("seed %s: %v", dg.Seed, err)
		}
		if g.Policy.Mode != db.GoalModePropose {
			t.Fatalf("seed %s: policy mode is %q, want propose", dg.Seed, g.Policy.Mode)
		}
		// Scope must stay empty: a seed cannot know which recipes or agents a
		// workspace will grow, and a stale id would silently measure nothing.
		if len(g.Scope.Recipes)+len(g.Scope.Agents)+len(g.Scope.Automations)+len(g.Scope.Tags) > 0 {
			t.Fatalf("seed %s: ships with a non-empty scope", dg.Seed)
		}
		// The whole point of the seed set: a metric with Available:false reports
		// "not measured yet" forever and only clutters the screen.
		assertMeasurable(t, dg.Seed, g.Primary.Metric)
		for _, gr := range g.Guardrails {
			assertMeasurable(t, dg.Seed, gr.Metric)
		}
	}
}

func assertMeasurable(t *testing.T, seed, key string) {
	t.Helper()
	m, ok := Lookup(key)
	if !ok {
		t.Fatalf("seed %s: metric %q is not in the catalog", seed, key)
	}
	if !m.Available {
		t.Fatalf("seed %s: metric %q is not measurable yet", seed, key)
	}
}

// Seeding is idempotent, and a user deletion is permanent: the ledger, not the
// presence of the row, decides whether a seed comes back.
func TestEnsureDefaultGoalsIdempotentAndDeletionAware(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := EnsureDefaultGoals(ctx, database, dir); err != nil {
		t.Fatal(err)
	}
	first, _ := database.ListGoals(ctx)
	if len(first) != len(defaultGoals) {
		t.Fatalf("seeded %d goals, want %d", len(first), len(defaultGoals))
	}
	for _, g := range first {
		if g.Status != db.GoalStatusDraft {
			t.Fatalf("goal %s seeded as %q, want draft", g.ID, g.Status)
		}
		if g.CreatedBy != db.GoalBySeed {
			t.Fatalf("goal %s provenance is %q, want %q", g.ID, g.CreatedBy, db.GoalBySeed)
		}
	}

	// A second pass must add nothing.
	if err := EnsureDefaultGoals(ctx, database, dir); err != nil {
		t.Fatal(err)
	}
	if again, _ := database.ListGoals(ctx); len(again) != len(first) {
		t.Fatalf("re-seeding changed the count: %d → %d", len(first), len(again))
	}

	// Delete one and re-run: it must NOT come back.
	if err := database.DeleteGoal(ctx, first[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDefaultGoals(ctx, database, dir); err != nil {
		t.Fatal(err)
	}
	after, _ := database.ListGoals(ctx)
	if len(after) != len(first)-1 {
		t.Fatalf("a deleted seed was resurrected: %d goals, want %d", len(after), len(first)-1)
	}
}
