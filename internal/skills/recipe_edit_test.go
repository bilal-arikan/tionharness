package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const editableRecipe = `---
name: "Plan → Dev → Test"
description: three phases
kind: coordinator-workflow
pattern: custom
version: 3
auto_prune: true
max_turns: 24
phases:
  - id: plan
    profile: planner
    gate: { kind: artifact, value: plan }
  - id: code
    profile: coder
    watchers: [summarize-board]
  - id: review
    profile: validator
    gate: { kind: verdict, value: "VERDICT: PASS" }
    max_rounds: 2
  - id: ship
    optional: true
watchers: [update-docs, lint]
optimizer: recipe-optimizer
paths:
  - src/**
access: shared
---
# body

Keep me exactly.
`

func editStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "ws")
	writeSkill(t, wsDir, "plan-dev-test", editableRecipe)
	st := New(filepath.Join(dir, "global"), wsDir)
	st.Reload()
	return st, filepath.Join(wsDir, "plan-dev-test", "SKILL.md")
}

// TestApplyRecipeProposalPrunes: pruning a watcher / phase and making a phase
// optional rewrite only the structured block, bump the version, keep the
// prose and the unrelated frontmatter, and round-trip through the parser.
func TestApplyRecipeProposalPrunes(t *testing.T) {
	st, path := editStore(t)
	sk, _ := st.Get("plan-dev-test")
	if sk.Recipe == nil || !sk.Recipe.AutoPrune {
		t.Fatalf("fixture must parse with auto_prune, got %+v", sk.Recipe)
	}
	v, err := ApplyRecipeProposal(st, "plan-dev-test", RecipeActionPruneWatcher, "update-docs", "")
	if err != nil || v != "4" {
		t.Fatalf("prune watcher: %v / version %s", err, v)
	}
	sk, _ = st.Get("plan-dev-test")
	if sk.Version != "4" || len(sk.Recipe.Watchers) != 1 || sk.Recipe.Watchers[0] != "lint" {
		t.Fatalf("after prune watcher: version %s watchers %v", sk.Version, sk.Recipe.Watchers)
	}
	if _, err := ApplyRecipeProposal(st, "plan-dev-test", RecipeActionMakeOptional, "review", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRecipeProposal(st, "plan-dev-test", RecipeActionPrunePhase, "ship", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRecipeProposal(st, "plan-dev-test", RecipeActionChangeProfile, "plan", "explore"); err != nil {
		t.Fatal(err)
	}
	sk, _ = st.Get("plan-dev-test")
	if sk.Version != "7" || len(sk.Recipe.Phases) != 3 || !sk.Recipe.Phases[2].Optional || sk.Recipe.Phases[0].Profile != "explore" {
		t.Fatalf("after edits: %+v", sk.Recipe)
	}
	if sk.Recipe.Phases[1].Watchers[0] != "summarize-board" || sk.Recipe.Phases[2].Gate == nil || sk.Recipe.Phases[2].Gate.Value != "VERDICT: PASS" || sk.Recipe.Phases[2].MaxRounds != 2 {
		t.Fatalf("untouched phase fields must survive: %+v", sk.Recipe.Phases)
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	for _, want := range []string{"Keep me exactly.", "max_turns: 24", "auto_prune: true", "access: shared", "paths:\n  - src/**", "optimizer: recipe-optimizer", "version: 7"} {
		if !strings.Contains(text, want) {
			t.Fatalf("rewritten file missing %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "phases:") != 1 || strings.Count(text, "version:") != 1 || strings.Contains(text, "- id: ship") {
		t.Fatalf("block must be rewritten once:\n%s", text)
	}
	// Refusals: unknown phase, last phase, unsupported action, non-recipe skill.
	if _, err := ApplyRecipeProposal(st, "plan-dev-test", RecipeActionPrunePhase, "nope", ""); err == nil {
		t.Fatal("unknown phase must be refused")
	}
	if _, err := ApplyRecipeProposal(st, "plan-dev-test", "split_phase", "plan", ""); err == nil {
		t.Fatal("split_phase must not be applied automatically")
	}
	if _, err := ApplyRecipeProposal(st, "missing", RecipeActionMakeOptional, "plan", ""); err == nil {
		t.Fatal("unknown skill must be refused")
	}
}

func TestBumpVersionAndQuoting(t *testing.T) {
	if bumpVersion("3") != "4" || bumpVersion("") != "2" || bumpVersion("v1") != "v1.1" {
		t.Fatal("bumpVersion")
	}
	if quoteIfNeeded("plan") != "plan" || quoteIfNeeded("VERDICT: PASS") != `"VERDICT: PASS"` {
		t.Fatal("quoteIfNeeded")
	}
	block := renderPhaseBlock([]PhaseSpec{{ID: "a", Gate: &GateSpec{Kind: "human"}, Watchers: []string{"x"}, Optional: true}})
	if !strings.Contains(block, "gate: { kind: human }") || !strings.Contains(block, "watchers: [x]") || !strings.Contains(block, "optional: true") {
		t.Fatalf("render = %s", block)
	}
}
