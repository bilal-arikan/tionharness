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
	for _, want := range []string{`AGENT:AG1 "builder"`, "anthropic/claude", "4 oturum (3 aktif)", "1.0M tok bugün", "son etkinlik"} {
		if !strings.Contains(v.Header, want) {
			t.Errorf("header missing %q: %q", want, v.Header)
		}
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
