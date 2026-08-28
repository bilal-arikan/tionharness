package view

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// agentFixture builds an agent with a mix of session conditions: active, idle,
// stuck, coordinator, and one archived (which must be excluded from every count).
func agentFixture(now time.Time) AgentInput {
	fresh := now.Add(-2 * time.Hour).Unix()
	old := now.Add(-9 * 24 * time.Hour).Unix()

	return AgentInput{
		Now:   now,
		Agent: db.Agent{ID: "AG1", Name: "builder", Provider: "anthropic", Model: "claude"},
		Sessions: []db.Session{
			{ID: "SES1", AgentID: "AG1", UpdatedAt: fresh, Title: "aktif iş"},
			{ID: "SES2", AgentID: "AG1", UpdatedAt: fresh, StuckTurns: 2, Title: "takılan"},
			{ID: "SES3", AgentID: "AG1", UpdatedAt: old}, // idle
			{ID: "SES4", AgentID: "AG1", UpdatedAt: fresh, State: "archived"},
			{ID: "SES5", AgentID: "AG1", UpdatedAt: fresh, CoordinatorMode: true},
		},
		Usage: db.Usage{
			AgentID: "AG1", InputTokens: 800_000, OutputTokens: 200_000,
		},
	}
}

func TestAgentHeaderCountsLiveSessions(t *testing.T) {
	now := time.Now()
	v, err := ProjectAgent(agentFixture(now), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	// 4 live sessions (the archived one is excluded), 3 of them active (fresh).
	for _, want := range []string{`AGENT:AG1 "builder"`, "anthropic/claude", "4 oturum (3 aktif)", "son etkinlik"} {
		if !strings.Contains(v.Header, want) {
			t.Errorf("header missing %q: %q", want, v.Header)
		}
	}
	// Spend belongs to the budget view; the agent header must not carry it.
	if strings.Contains(v.Header, " tok") || strings.Contains(v.Header, "$") {
		t.Errorf("header still reports spend: %q", v.Header)
	}
	// No priced spend (empty ByModel) → the header must NOT show a bare $0.00.
	if strings.Contains(v.Header, "$") {
		t.Errorf("unpriced spend must not render a dollar figure: %q", v.Header)
	}
}

func TestAgentSurfacesStuckAndCoordinator(t *testing.T) {
	v, err := ProjectAgent(agentFixture(time.Now()), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "1 oturum takılmış") || !strings.Contains(txt, "SES2") {
		t.Errorf("stuck session not flagged:\n%s", txt)
	}
	if !strings.Contains(txt, "1 koordinatör oturumu") {
		t.Errorf("coordinator session not counted:\n%s", txt)
	}
}

func TestAgentHandlesArePerSessionAndCapped(t *testing.T) {
	now := time.Now()
	in := agentFixture(now)
	// Push well past the handle cap so elision is exercised.
	for i := 0; i < agentSessionHandles+5; i++ {
		in.Sessions = append(in.Sessions, db.Session{
			ID: fmt.Sprintf("F%d", i), AgentID: "AG1", UpdatedAt: now.Unix(),
		})
	}
	v, err := ProjectAgent(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if len(v.Handles) != agentSessionHandles {
		t.Errorf("handles = %d, want cap %d", len(v.Handles), agentSessionHandles)
	}
	for _, h := range v.Handles {
		if h.Ref.Kind != KindSession {
			t.Errorf("handle points at %q, want session", h.Ref.Kind)
		}
	}
	if v.Elided == 0 || v.ElidedUnit != "oturum" {
		t.Errorf("elision not reported: elided=%d unit=%q", v.Elided, v.ElidedUnit)
	}
}

// TestAgentCardCarriesBindingAndFlags pins the identity block: what the agent is
// bound to and how it behaves, not just what its sessions are doing.
func TestAgentCardCarriesBindingAndFlags(t *testing.T) {
	in := agentFixture(time.Now())
	in.IsDefault = true
	in.Agent.ThinkingLevel = "high"
	in.Agent.CoordinatorMode = true
	in.Agent.BlockedTools = `["shell","write_file","edit_file","delete_file","git_push","deploy"]`

	v, err := ProjectAgent(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	for _, want := range []string{
		"düşünme: high",
		"koordinatör", "varsayılan",
		"yasaklı araçlar (6): shell, write_file, edit_file, delete_file, git_push … +1",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("card missing %q:\n%s", want, txt)
		}
	}
	// The binding belongs to the header alone. It was once printed there AND in
	// the config block, which read as two different model bindings.
	if n := strings.Count(txt, "anthropic"); n != 1 {
		t.Errorf("provider rendered %d times, want 1:\n%s", n, txt)
	}
	if n := strings.Count(txt, "claude"); n != 1 {
		t.Errorf("model rendered %d times, want 1:\n%s", n, txt)
	}
}

// TestAgentOmitsUnsetConfig: an unset field must vanish, never render as a
// placeholder that asserts a setting the user never made.
func TestAgentOmitsUnsetConfig(t *testing.T) {
	in := agentFixture(time.Now())
	in.Agent.ThinkingLevel = "off"
	in.Agent.BlockedTools = "[]"

	v, err := ProjectAgent(in, LevelFull)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	for _, unwanted := range []string{"düşünme:", "yasaklı araçlar", "varsayılan", "ruh:", "kimlik:"} {
		if strings.Contains(txt, unwanted) {
			t.Errorf("unset field still rendered (%q):\n%s", unwanted, txt)
		}
	}
}

// TestAgentPersonaOnlyAtFull keeps the expensive persona text out of a card.
func TestAgentPersonaOnlyAtFull(t *testing.T) {
	in := agentFixture(time.Now())
	in.Agent.Soul = "Sen titiz bir derleyici mühendisisin."
	in.Agent.Identity = "Her değişikliği testle doğrula."

	card, err := ProjectAgent(in, LevelCard)
	if err != nil {
		t.Fatalf("project card: %v", err)
	}
	if strings.Contains(card.Text(), "ruh:") || strings.Contains(card.Text(), "kimlik:") {
		t.Errorf("persona leaked into the card:\n%s", card.Text())
	}

	full, err := ProjectAgent(in, LevelFull)
	if err != nil {
		t.Fatalf("project full: %v", err)
	}
	txt := full.Text()
	if !strings.Contains(txt, "ruh: Sen titiz") || !strings.Contains(txt, "kimlik: Her değişikliği") {
		t.Errorf("persona missing at full level:\n%s", txt)
	}
}

// TestAgentUnreadableDenylistIsReported: a denylist that will not parse must not
// render as "nothing blocked" — that is the opposite fact.
func TestAgentUnreadableDenylistIsReported(t *testing.T) {
	in := agentFixture(time.Now())
	in.Agent.BlockedTools = `{"shell":true}`

	v, err := ProjectAgent(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "yasaklı araçlar: (liste okunamadı)") {
		t.Errorf("unparseable denylist silently dropped:\n%s", v.Text())
	}
}

func TestAgentTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectAgent(agentFixture(time.Now()), LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
}

func TestAgentEmptyIsRejected(t *testing.T) {
	if _, err := ProjectAgent(AgentInput{}, LevelCard); err == nil {
		t.Error("an agent input with no agent must be an error, not a blank view")
	}
}

// TestAgentConfigCarriesDisabledAndPermissionMode pins the two facts that decide
// whether an agent can act at all: a disabled agent never runs, and a non-default
// permission mode gates every tool call it makes. The "auto" default renders
// nothing — a line that only repeats the default is noise on every card.
func TestAgentConfigCarriesDisabledAndPermissionMode(t *testing.T) {
	now := time.Now()
	v, err := ProjectAgent(AgentInput{
		Agent: db.Agent{
			ID: "AG9", Name: "reader", Provider: "anthropic", Model: "opus",
			Disabled: true, PermissionMode: "read-only",
		},
		Now: now,
	}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "izin: read-only") {
		t.Errorf("permission mode missing:\n%s", txt)
	}
	if !strings.Contains(txt, "devre dışı") {
		t.Errorf("a disabled agent must say so:\n%s", txt)
	}

	auto, err := ProjectAgent(AgentInput{
		Agent: db.Agent{ID: "AG8", Name: "worker", Provider: "anthropic", PermissionMode: "auto"},
		Now:   now,
	}, LevelCard)
	if err != nil {
		t.Fatalf("project auto: %v", err)
	}
	if strings.Contains(auto.Text(), "izin:") || strings.Contains(auto.Text(), "devre dışı") {
		t.Errorf("default permission mode must not render a line:\n%s", auto.Text())
	}
}
