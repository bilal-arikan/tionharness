package skills

import (
	"path/filepath"
	"strings"
	"testing"
)

const recipeWithPhases = `---
name: "Plan → Dev → Test"
kind: coordinator-workflow
pattern: custom
version: 3
worker_targets: [planner, coder, validator]
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
watchers: [update-docs]
optimizer: recipe-optimizer
paths:
  - src/**
access: shared
---
# body
`

// TestFrontmatterNestedBlockDoesNotLeak: the phases block is captured raw and
// its nested keys never become top-level scalars; flat block lists still work.
func TestFrontmatterNestedBlockDoesNotLeak(t *testing.T) {
	fm, body := parseFrontmatter(recipeWithPhases)
	if _, leaked := fm.scalars["profile"]; leaked {
		t.Fatal("nested phase key leaked into top-level scalars")
	}
	if _, leaked := fm.scalars["gate"]; leaked {
		t.Fatal("nested gate key leaked into top-level scalars")
	}
	if fm.scalar("max_turns") != "24" || fm.scalar("access") != "shared" || fm.scalar("version") != "3" {
		t.Fatalf("scalars after the block lost: %+v", fm.scalars)
	}
	if got := fm.list("paths"); len(got) != 1 || got[0] != "src/**" {
		t.Fatalf("flat block list = %v, want [src/**]", got)
	}
	if got := fm.list("watchers"); len(got) != 1 || got[0] != "update-docs" {
		t.Fatalf("inline top-level list = %v", got)
	}
	if !strings.Contains(fm.blocks["phases"], "- id: plan") || !strings.Contains(fm.blocks["phases"], "max_rounds: 2") {
		t.Fatalf("phases block not captured: %q", fm.blocks["phases"])
	}
	if strings.TrimSpace(body) != "# body" {
		t.Fatalf("body = %q", body)
	}
}

// TestParseRecipeSpec: the structured plan round-trips with gates, watchers,
// rounds and the optional flag; prose-only recipes yield nil; bad blocks error.
func TestParseRecipeSpec(t *testing.T) {
	fm, _ := parseFrontmatter(recipeWithPhases)
	spec, err := parseRecipeSpec(fm, fm.scalar("version"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if spec == nil || spec.Version != "3" || spec.Optimizer != "recipe-optimizer" || len(spec.Watchers) != 1 || len(spec.Phases) != 4 {
		t.Fatalf("spec = %+v", spec)
	}
	p := spec.Phases
	if p[0].ID != "plan" || p[0].Profile != "planner" || p[0].Gate == nil || p[0].Gate.Kind != "artifact" || p[0].Gate.Value != "plan" {
		t.Fatalf("plan phase = %+v", p[0])
	}
	if p[1].ID != "code" || len(p[1].Watchers) != 1 || p[1].Watchers[0] != "summarize-board" {
		t.Fatalf("code phase = %+v", p[1])
	}
	if p[2].Gate == nil || p[2].Gate.Kind != "verdict" || p[2].Gate.Value != "VERDICT: PASS" || p[2].MaxRounds != 2 {
		t.Fatalf("review phase = %+v", p[2])
	}
	if !p[3].Optional || p[3].Profile != "" {
		t.Fatalf("ship phase = %+v", p[3])
	}

	prose, _ := parseFrontmatter("---\nname: x\nkind: coordinator-workflow\n---\nbody")
	if spec, err := parseRecipeSpec(prose, ""); err != nil || spec != nil {
		t.Fatalf("prose-only recipe: spec=%+v err=%v, want nil/nil", spec, err)
	}

	bad := []struct{ name, block string }{
		{"unknown gate kind", "phases:\n  - id: a\n    gate: teleport\n"},
		{"duplicate id", "phases:\n  - id: a\n  - id: a\n"},
		{"bad id", "phases:\n  - id: Plan Phase\n"},
		{"empty block", "phases:\n  - \n"},
		{"rounds not int", "phases:\n  - id: a\n    max_rounds: two\n"},
		{"watchers not array", "phases:\n  - id: a\n    watchers: x\n"},
	}
	for _, tc := range bad {
		fm, _ := parseFrontmatter("---\nname: x\nkind: coordinator-workflow\n" + tc.block + "---\nbody")
		if _, err := parseRecipeSpec(fm, ""); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

// TestRecipeRefRoundTrip covers the "slug@version" spelling and its readers.
func TestRecipeRefRoundTrip(t *testing.T) {
	if RecipeRef("wf", "3") != "wf@3" || RecipeRef("wf", "") != "wf" || RecipeRef("", "3") != "" {
		t.Fatal("RecipeRef")
	}
	for ref, want := range map[string][2]string{"wf@3": {"wf", "3"}, "wf": {"wf", ""}, " wf@1.2 ": {"wf", "1.2"}, "@x": {"@x", ""}} {
		s, v := ParseRecipeRef(ref)
		if s != want[0] || v != want[1] {
			t.Fatalf("ParseRecipeRef(%q) = %q,%q want %v", ref, s, v, want)
		}
	}
}

// TestStoreLoadsRecipeAndFlagsInvalidOnes: a valid block lands on
// Skill.Recipe, an invalid one on Skill.RecipeError without dropping the skill,
// and resolution accepts a versioned ref.
func TestStoreLoadsRecipeAndFlagsInvalidOnes(t *testing.T) {
	global := filepath.Join(t.TempDir(), "global")
	writeSkill(t, global, "wf-ok", recipeWithPhases)
	writeSkill(t, global, "wf-bad", "---\nname: bad\nkind: coordinator-workflow\nversion: 2\nphases:\n  - id: a\n    gate: nope\n---\nbody")
	writeSkill(t, global, "wf-prose", "---\nname: prose\nkind: coordinator-workflow\n---\nbody")
	st := New(global, filepath.Join(t.TempDir(), "ws"))

	ok, found := st.Get("wf-ok")
	if !found || ok.Recipe == nil || len(ok.Recipe.Phases) != 4 || ok.RecipeError != "" {
		t.Fatalf("wf-ok = %+v", ok)
	}
	bad, found := st.Get("wf-bad")
	if !found || bad.Recipe != nil || !strings.Contains(bad.RecipeError, "gate kind") {
		t.Fatalf("wf-bad = %+v (recipeError %q)", bad, bad.RecipeError)
	}
	prose, _ := st.Get("wf-prose")
	if prose.Recipe != nil || prose.RecipeError != "" {
		t.Fatalf("wf-prose = %+v", prose)
	}
	if RecipeRefFor(st, "wf-ok") != "wf-ok@3" || RecipeRefFor(st, "wf-prose") != "wf-prose" || RecipeRefFor(st, "missing") != "missing" {
		t.Fatalf("RecipeRefFor: %q / %q / %q", RecipeRefFor(st, "wf-ok"), RecipeRefFor(st, "wf-prose"), RecipeRefFor(st, "missing"))
	}
	if mt, err := ResolveCoordinatorWorkflow(st, "wf-ok@3"); err != nil || mt != 24 {
		t.Fatalf("resolve versioned ref: mt=%d err=%v", mt, err)
	}
	if sk, err := ResolveRecipe(st, "wf-ok@9"); err != nil || sk == nil || sk.Recipe.Version != "3" {
		t.Fatalf("resolve stale version must still resolve the current file: %+v err=%v", sk, err)
	}
	if _, err := ResolveRecipe(st, "wf-missing@1"); err == nil {
		t.Fatal("missing recipe must error")
	}
}

// TestShippedRecipesParse: every shipped coordinator recipe loads with a valid
// (or absent) structured block — a broken default must fail here, not in a user's
// workspace.
func TestShippedRecipesParse(t *testing.T) {
	global := filepath.Join(t.TempDir(), "global")
	if err := EnsureDefaults(global); err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}
	st := New(global, filepath.Join(t.TempDir(), "ws"))
	seen := 0
	for _, sk := range st.List() {
		if !sk.IsCoordinatorWorkflow() {
			continue
		}
		seen++
		if sk.RecipeError != "" {
			t.Errorf("shipped recipe %s has an invalid block: %s", sk.Slug, sk.RecipeError)
		}
	}
	pdt, ok := st.Get("coordinator-wf-plan-dev-test")
	if !ok || pdt.Recipe == nil || len(pdt.Recipe.Phases) != 4 || pdt.Recipe.Version != "1" {
		t.Fatalf("plan-dev-test recipe = %+v", pdt.Recipe)
	}
	if seen == 0 {
		t.Fatal("no shipped coordinator recipes found")
	}
}
