package e2e

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/skills"
)

// TestSkills_AdvertiseAndLoad covers the full skill lifecycle through a turn: a
// workspace skill is created, advertised to the agent in its system prompt
// (slug + summary only), and its full body is loaded on demand when the model
// calls use_skill — the lazy-loading contract.
func TestSkills_AdvertiseAndLoad(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Loading the skill.", tc("c1", "use_skill", map[string]any{
			"slug": "weather-report",
		})),
		sayText("Following the weather-report skill now."),
	)
	h := newHarness(t, prov)

	// A shared skill is visible to every agent (no per-agent assignment needed).
	const bodyMarker = "STEP_ONE_FETCH_FORECAST"
	if _, err := h.rt.Skills().Create("weather-report", skills.SkillInput{
		Name:        "Weather Report",
		Description: "Produce a structured local weather report.",
		WhenToUse:   "the user asks about weather",
		Shared:      true,
		Body:        "# Weather Report\n\n" + bodyMarker + "\n\nThen format the result.",
	}); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	ag := h.newAgent("Meteo")

	// The catalog block advertises the slug + summary but NOT the body (lazy).
	catalog := h.rt.SkillsCatalogBlockForAgent(ag)
	if !strings.Contains(catalog, "weather-report") {
		t.Fatalf("skill not advertised in catalog block:\n%s", catalog)
	}
	if strings.Contains(catalog, bodyMarker) {
		t.Errorf("catalog leaked the skill body (should be lazy):\n%s", catalog)
	}

	sess := h.newSession(ag)
	res := h.send(ag, sess, "What's the weather like?")

	// use_skill returned the full body — the lazy load happened in-loop.
	step := findToolStep(res.steps, "use_skill")
	if step == nil {
		t.Fatalf("no use_skill step in trace: %+v", res.steps)
	}
	if step.IsError {
		t.Fatalf("use_skill errored: %s", step.Output)
	}
	if !strings.Contains(step.Output, bodyMarker) {
		t.Errorf("use_skill output missing the skill body marker:\n%s", step.Output)
	}
}

// TestSkills_RestrictedSkillUnreachable verifies the per-agent allowlist: a
// non-shared skill the agent was never assigned cannot be loaded via use_skill.
func TestSkills_RestrictedSkillUnreachable(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Trying a private skill.", tc("c1", "use_skill", map[string]any{
			"slug": "secret-playbook",
		})),
		sayText("Could not load it."),
	)
	h := newHarness(t, prov)

	// Restricted (not shared) and never assigned to the agent below.
	if _, err := h.rt.Skills().Create("secret-playbook", skills.SkillInput{
		Name:        "Secret Playbook",
		Description: "Internal only.",
		Shared:      false,
		Body:        "# Secret\n\nTOP_SECRET_MARKER",
	}); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	// A second SHARED skill exists so the use_skill tool is still registered
	// (it only ships when the agent's allow-set is non-empty) — isolating the
	// allowlist check from the tool-registration check.
	if _, err := h.rt.Skills().Create("public-helper", skills.SkillInput{
		Name: "Public Helper", Description: "Shared.", Shared: true, Body: "# Public",
	}); err != nil {
		t.Fatalf("create shared skill: %v", err)
	}

	ag := h.newAgent("Restricted") // no Skills assigned → only shared ones allowed
	sess := h.newSession(ag)

	// The restricted skill must not be advertised to this agent.
	if cat := h.rt.SkillsCatalogBlockForAgent(ag); strings.Contains(cat, "secret-playbook") {
		t.Errorf("restricted skill leaked into catalog:\n%s", cat)
	}

	res := h.send(ag, sess, "Use the secret playbook.")
	step := findToolStep(res.steps, "use_skill")
	if step == nil {
		t.Fatalf("no use_skill step in trace: %+v", res.steps)
	}
	if !step.IsError {
		t.Errorf("expected use_skill to reject a restricted skill, got output: %s", step.Output)
	}
	if strings.Contains(step.Output, "TOP_SECRET_MARKER") {
		t.Errorf("restricted skill body leaked through use_skill: %s", step.Output)
	}
}
