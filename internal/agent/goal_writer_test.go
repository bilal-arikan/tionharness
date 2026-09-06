package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/prompts"
)

// TestGoalWriterUserPrompt: the intake prompt carries the verbatim statement,
// every catalog key, the scope candidates, the existing goals and (on a
// rewrite) the base goal; the archived goal and the base itself are not listed
// as "existing".
func TestGoalWriterUserPrompt(t *testing.T) {
	cands := GoalScopeCandidates{
		Recipes:     []goalCandidate{{ID: "code-review", Name: "Kod İnceleme"}},
		Agents:      []goalCandidate{{ID: "AGT2", Name: "Dev"}},
		Automations: []goalCandidate{{ID: "AUT1", Name: "nightly"}},
		Tags:        []string{"bugfix"},
	}
	existing := []db.Goal{
		{ID: "GOL1", Status: db.GoalStatusActive, Name: "Hız", Primary: db.GoalMetric{Metric: "board.cycleTimeSec", Direction: "min"}},
		{ID: "GOL2", Status: db.GoalStatusArchived, Name: "Eski", Primary: db.GoalMetric{Metric: "usage.costUSDPerDay", Direction: "min"}},
		{ID: "GOL3", Status: db.GoalStatusDraft, Name: "Taban", Primary: db.GoalMetric{Metric: "recipe.avgCostUSD", Direction: "min"}},
	}
	base := existing[2]
	p := goalWriterUserPrompt("incelemeler çok pahalı", &base, existing, cands, "Türkçe")
	for _, want := range []string{
		"incelemeler çok pahalı", "Reply language", "Türkçe",
		"`recipe.avgCostUSD`", "`judge.rubricScore`", "`config.soulChars`",
		"code-review (Kod İnceleme)", "AGT2 (Dev)", "AUT1 (nightly)", "Tags: bugfix",
		"GOL1 [active] Hız", "Existing goal this statement refines", `"id": "GOL3"`,
		`Policy mode must be "propose"`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "GOL2 [archived]") {
		t.Error("archived goal must not be listed")
	}
	if strings.Contains(p, "- GOL3 [draft]") {
		t.Error("the base goal must not be listed twice")
	}
	if strings.Contains(p, `"history"`) {
		t.Error("base goal history must be stripped")
	}
	// No candidates / no goals renders explicit placeholders instead of blanks.
	p = goalWriterUserPrompt("x", nil, nil, GoalScopeCandidates{}, "")
	if strings.Count(p, "(none)") != 5 || strings.Contains(p, "Reply language") {
		t.Fatalf("empty prompt shape wrong:\n%s", p)
	}
}

// TestGoalWriterRegistered: the system agent, its prompt and the fallback
// mapping all exist under the same key.
func TestGoalWriterRegistered(t *testing.T) {
	found := false
	for _, d := range SystemAgentDefaults() {
		if d.SystemKey == goalWriterSystemKey {
			found = true
			if d.AllowedTools != "[]" {
				t.Fatalf("goal writer must have no tools, got %q", d.AllowedTools)
			}
			if !strings.Contains(d.SystemPrompt, "Goal Writer") {
				t.Fatal("system prompt must be the goal-writer default")
			}
		}
	}
	if !found {
		t.Fatal("goal-writer system agent not registered")
	}
	if analysisPromptKeys[goalWriterSystemKey] != goalWriterSystemKey {
		t.Fatal("analysisPromptKeys lacks goal-writer")
	}
	if _, ok := systemAgentVisuals[goalWriterSystemKey]; !ok {
		t.Fatal("goal-writer has no visual identity")
	}
	if !strings.Contains(prompts.Default(goalWriterSystemKey), `"mode": "propose"`) {
		t.Fatal("goal-writer prompt must pin the propose policy")
	}
}
