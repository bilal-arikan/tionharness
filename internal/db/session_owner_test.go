package db

import "testing"

func TestOwnerAgentIDPrefersTheSessionsOwnAgent(t *testing.T) {
	s := Session{AgentID: "AGT1", TargetAgentID: "AGT2"}
	if got := s.OwnerAgentID(); got != "AGT1" {
		t.Fatalf("OwnerAgentID() = %q, want %q", got, "AGT1")
	}
}

// Rows written before delegated runs carried an owner still resolve, from the
// target they were created against.
func TestOwnerAgentIDFallsBackToTheDelegationTarget(t *testing.T) {
	s := Session{TargetAgentID: "AGT2"}
	if got := s.OwnerAgentID(); got != "AGT2" {
		t.Fatalf("OwnerAgentID() = %q, want %q", got, "AGT2")
	}
}

func TestOwnerProfileLabelNamesAnAgentlessProfileRun(t *testing.T) {
	s := Session{TargetProfile: "coder"}
	if got := s.OwnerProfileLabel(); got != "subagent:coder" {
		t.Fatalf("OwnerProfileLabel() = %q, want %q", got, "subagent:coder")
	}
	if got := s.OwnerAgentID(); got != "" {
		t.Fatalf("OwnerAgentID() = %q, want empty", got)
	}
}

// A session with a real agent is named by that agent, never by a profile label.
func TestOwnerProfileLabelEmptyWhenAnAgentResolves(t *testing.T) {
	s := Session{AgentID: "AGT1", TargetProfile: "coder"}
	if got := s.OwnerProfileLabel(); got != "" {
		t.Fatalf("OwnerProfileLabel() = %q, want empty", got)
	}
}
