package goals

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func proposalGoal() db.Goal {
	return db.Goal{
		ID: "GOL1", Status: db.GoalStatusActive,
		Scope:      db.GoalScope{Agents: []string{"AGT1"}},
		Primary:    db.GoalMetric{Metric: "usage.costUSDPerSession", Direction: "min"},
		Guardrails: []db.GoalGuardrail{{Metric: "session.errorTurnsRatio", Max: f(0.2)}},
	}
}

func goodRaw() RawProposal {
	return RawProposal{
		Surface: "agent", EntityID: "AGT1", Field: "thinkingLevel", Action: "set", Value: "low",
		Title: "Düşünme seviyesini düşür", Rationale: "kısa görevlerde fark yok",
		Evidence: "cost $1.40/session over 8 sessions under cb72 vs $0.90 under c2c8", ExpectedMetric: "usage.costUSDPerSession", ExpectedDelta: -0.4,
	}
}

// TestCheckProposalRules: every refusal reason the design names is produced
// by code, and a clean proposal passes with the right signature.
func TestCheckProposalRules(t *testing.T) {
	cx := ProposalContext{
		Goal:       proposalGoal(),
		InScope:    func(surface, id string) bool { return surface != SurfaceAgent || id == "AGT1" },
		Exists:     func(surface, id string) bool { return id != "GHOST" },
		SkillCount: func(string) int { return MaxSkillsPerAgent },
	}
	p, why := CheckProposal(goodRaw(), cx)
	if why != "" {
		t.Fatalf("clean proposal refused: %s", why)
	}
	if Signature(p) != "evolution:gol1:agent:agt1:thinkinglevel:set" || p.Kind != "change" {
		t.Fatalf("signature/kind = %s / %s", Signature(p), p.Kind)
	}
	cases := []struct {
		name string
		mut  func(*RawProposal)
		want string
	}{
		{"invisible field", func(r *RawProposal) { r.Field = "permissionMode" }, "not editable"},
		{"invisible surface", func(r *RawProposal) { r.Surface = "hook"; r.Field = "command" }, "not editable"},
		{"bad action", func(r *RawProposal) { r.Action = "delete" }, "not allowed"},
		{"no evidence", func(r *RawProposal) { r.Evidence = "it feels slow" }, "no measured evidence"},
		{"negative judgement", func(r *RawProposal) { r.Rationale = "this agent is unreliable" }, "negative judgement"},
		{"value missing", func(r *RawProposal) { r.Value = "" }, "value missing"},
		{"value too long", func(r *RawProposal) { r.Value = strings.Repeat("x", 17) }, "longer than"},
		{"missing entity", func(r *RawProposal) { r.EntityID = "GHOST" }, "does not exist"},
		{"out of scope", func(r *RawProposal) { r.EntityID = "AGT2" }, "outside the goal's scope"},
		{"foreign metric", func(r *RawProposal) { r.ExpectedMetric = "board.cycleTimeSec" }, "not one of the goal's metrics"},
		{"wrong direction", func(r *RawProposal) { r.ExpectedDelta = 0.3 }, "does not improve"},
		{"zero delta", func(r *RawProposal) { r.ExpectedDelta = 0 }, "does not improve"},
		{"guardrail side effect", func(r *RawProposal) { r.SideEffects = []string{"session.errorTurnsRatio"} }, "own guardrail"},
		{"primary side effect", func(r *RawProposal) { r.SideEffects = []string{"usage.costUSDPerSession"} }, "own primary"},
		{"budget without removal", func(r *RawProposal) {
			r.Field, r.Action, r.Value = "skills", "add", "new-skill"
		}, "growth budget"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := goodRaw()
			tc.mut(&r)
			if _, why := CheckProposal(r, cx); why == "" || !strings.Contains(why, tc.want) {
				t.Fatalf("want refusal containing %q, got %q", tc.want, why)
			}
		})
	}
	// A guardrail metric may be the expected one, moving its own way (max → down).
	r := goodRaw()
	r.ExpectedMetric, r.ExpectedDelta = "session.errorTurnsRatio", -0.1
	if _, why := CheckProposal(r, cx); why != "" {
		t.Fatalf("guardrail improvement refused: %s", why)
	}
	// Addition naming a removal passes the budget; prune needs no value.
	r = goodRaw()
	r.Field, r.Action, r.Value, r.Removes = "skills", "add", "new-skill", "old-skill"
	if _, why := CheckProposal(r, cx); why != "" {
		t.Fatalf("addition with removal refused: %s", why)
	}
	r = goodRaw()
	r.Surface, r.EntityID, r.Field, r.Action, r.Value = "recipe", "code-review", "watcher", "prune", ""
	cx2 := cx
	cx2.InScope = nil
	if _, why := CheckProposal(r, cx2); why != "" {
		t.Fatalf("prune without value refused: %s", why)
	}
	// ws-settings needs no entity.
	r = goodRaw()
	r.Surface, r.EntityID, r.Field, r.Value = "ws-settings", "", "terseMode", "true"
	if _, why := CheckProposal(r, cx); why != "" {
		t.Fatalf("settings proposal refused: %s", why)
	}
}

// TestCheckProposalConflict: a side effect on another active, overlapping
// goal's metric marks the proposal as a conflict instead of refusing it.
func TestCheckProposalConflict(t *testing.T) {
	other := db.Goal{ID: "GOL2", Status: db.GoalStatusActive, Primary: db.GoalMetric{Metric: "session.humanAsksPerSession", Direction: "min"}}
	cx := ProposalContext{Goal: proposalGoal(), OtherGoals: []db.Goal{other}}
	r := goodRaw()
	r.SideEffects = []string{"session.humanAsksPerSession"}
	p, why := CheckProposal(r, cx)
	if why != "" || p.Kind != "conflict" || !strings.Contains(p.Evidence, "GOL2") {
		t.Fatalf("conflict: why=%q kind=%s evidence=%s", why, p.Kind, p.Evidence)
	}
	// Disjoint scopes do not conflict; paused goals do not either.
	other.Scope = db.GoalScope{Agents: []string{"AGT9"}}
	cx.OtherGoals = []db.Goal{other}
	if p, _ := CheckProposal(r, cx); p.Kind != "change" {
		t.Fatal("disjoint scopes must not conflict")
	}
	other.Scope = db.GoalScope{}
	other.Status = db.GoalStatusPaused
	cx.OtherGoals = []db.Goal{other}
	if p, _ := CheckProposal(r, cx); p.Kind != "change" {
		t.Fatal("paused goals must not conflict")
	}
}

func TestProposalDefaultsAndRules(t *testing.T) {
	if EffectiveMinRuns(db.Goal{}) != DefaultMinRuns || EffectiveCooldownHours(db.Goal{}) != DefaultCooldownHours {
		t.Fatal("defaults")
	}
	g := db.Goal{Policy: db.GoalPolicy{MinRuns: 2, CooldownHours: 1}}
	if EffectiveMinRuns(g) != 2 || EffectiveCooldownHours(g) != 1 {
		t.Fatal("policy values must win")
	}
	seen := map[string]bool{}
	for _, r := range ProposalRules() {
		k := r.Surface + "." + r.Field
		if seen[k] {
			t.Fatalf("duplicate rule %s", k)
		}
		seen[k] = true
		if len(r.Actions) == 0 {
			t.Fatalf("%s has no actions", k)
		}
	}
	for _, forbidden := range []string{"agent.permissionMode", "agent.inboundPolicy", "agent.provider", "hook.command", "ws-settings.ignoredRecommendations"} {
		if seen[forbidden] {
			t.Fatalf("%s must never be editable", forbidden)
		}
	}
	text := RulesForPrompt()
	if !strings.Contains(text, "- agent:") || !strings.Contains(text, "thinkingLevel") || strings.Contains(text, "permissionMode") {
		t.Fatalf("rules prompt wrong:\n%s", text)
	}
}
