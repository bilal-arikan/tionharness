package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// A delegated run against an EXISTING agent must own that agent, not just target
// it: the sidebar and the executions feed resolve a name and avatar from AgentID,
// so a child row without one renders anonymous.
func TestSubagentSessionMetaOwnsTargetAgent(t *testing.T) {
	agent := db.Agent{ID: "AGT7", Name: "Kâşif"}
	meta := subagentSessionMeta("SES1", agent, false, tools.RunAgentSpec{Target: "Kâşif", Task: "graf taramasını çalıştır"})

	if meta.AgentID != agent.ID {
		t.Fatalf("AgentID = %q, want %q", meta.AgentID, agent.ID)
	}
	// The target still marks the row a delegation.
	if meta.TargetAgentID != agent.ID {
		t.Fatalf("TargetAgentID = %q, want %q", meta.TargetAgentID, agent.ID)
	}
	if meta.TargetProfile != "" {
		t.Fatalf("TargetProfile = %q, want empty for a persistent agent", meta.TargetProfile)
	}
}

// An ephemeral profile subagent has no persisted agent row, so it must stay
// identified by its profile alone — setting an AgentID there would point at an
// agent that cannot be resolved.
func TestSubagentSessionMetaKeepsProfileTargetAgentless(t *testing.T) {
	meta := subagentSessionMeta("SES1", db.Agent{Name: "subagent:coder"}, true, tools.RunAgentSpec{Target: "coder", Task: "testi düzelt"})

	if meta.AgentID != "" {
		t.Fatalf("AgentID = %q, want empty for an ephemeral profile", meta.AgentID)
	}
	if meta.TargetProfile != "coder" {
		t.Fatalf("TargetProfile = %q, want %q", meta.TargetProfile, "coder")
	}
}

// Every delegated run is named from its target and task, so the session list
// never falls back to its "new chat" placeholder for one.
func TestSubagentSessionMetaTitlesFromTargetAndTask(t *testing.T) {
	meta := subagentSessionMeta("SES1", db.Agent{ID: "AGT7", Name: "Kâşif"}, false, tools.RunAgentSpec{Target: "Kâşif", Task: "graf taramasını çalıştır"})

	if meta.Title == "" {
		t.Fatal("Title is empty; a delegated run must be named")
	}
	if !strings.Contains(meta.Title, "Kâşif") {
		t.Errorf("Title = %q, want it to name the target agent", meta.Title)
	}
	if !strings.Contains(meta.Title, "graf taramasını") {
		t.Errorf("Title = %q, want a snippet of the task", meta.Title)
	}
}

// A task spanning several lines must still yield a single-line title.
func TestSubagentTitleIsSingleLineAndBounded(t *testing.T) {
	task := "ilk satır\nikinci satır" + strings.Repeat(" uzun", 60)
	title := subagentTitle(db.Agent{Name: "Kâşif"}, tools.RunAgentSpec{Task: task})

	if strings.Contains(title, "\n") {
		t.Errorf("title = %q, want a single line", title)
	}
	if strings.Contains(title, "ikinci satır") {
		t.Errorf("title = %q, want only the first line of the task", title)
	}
	if n := len([]rune(title)); n > 120 {
		t.Errorf("title runes = %d, want a bounded title", n)
	}
}

// With no agent name to use (an ephemeral clone whose name was not set), the
// title falls back to the requested target rather than losing the identity.
func TestSubagentTitleFallsBackToSpecTarget(t *testing.T) {
	title := subagentTitle(db.Agent{}, tools.RunAgentSpec{Target: "coder", Task: "testi düzelt"})

	if !strings.Contains(title, "coder") {
		t.Errorf("title = %q, want it to name the requested target", title)
	}
}
