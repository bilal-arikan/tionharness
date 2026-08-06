package agent

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestHandoffContinuationSpawnOpts_CarriesCoordinatorSettings: a /handoff from a
// coordinator session must seed the clean-window continuation with the same
// coordinator capability + workflow recipe (so it keeps driving the same multi-agent
// setup), while dropping the tree/worker lineage (the continuation is its own fresh
// top-level coordinator, not a worker reporting to the old tree).
func TestHandoffContinuationSpawnOpts_CarriesCoordinatorSettings(t *testing.T) {
	parent := db.Session{
		ID:                       "SES1",
		Kind:                     "chat",
		WorkingDir:               "C:\\proj",
		CoordinatorMode:          true,
		CoordinatorWorkflow:      "tournament",
		CoordinatorMaxTurns:      12,
		Role:                     "worker",
		CoordinatorSessionID:     "SEScoord",
		RootCoordinatorSessionID: "SESroot",
		CoordinatorDepth:         3,
	}
	got := handoffContinuationSpawnOpts(parent, HandoffOptions{CreatedBy: "AGT1"})

	if !got.CoordinatorMode {
		t.Error("CoordinatorMode must carry over to the continuation")
	}
	if got.CoordinatorWorkflow != "tournament" {
		t.Errorf("CoordinatorWorkflow = %q, want tournament", got.CoordinatorWorkflow)
	}
	if got.CoordinatorMaxTurns != 12 {
		t.Errorf("CoordinatorMaxTurns = %d, want 12", got.CoordinatorMaxTurns)
	}
	if got.WorkingDir != "C:\\proj" {
		t.Errorf("WorkingDir = %q, want C:\\proj", got.WorkingDir)
	}
	if got.ParentSessionID != "SES1" {
		t.Errorf("ParentSessionID = %q, want SES1", got.ParentSessionID)
	}
	if got.Kind != "chat" {
		t.Errorf("Kind = %q, want chat", got.Kind)
	}
	// Tree/worker lineage must NOT leak onto the continuation.
	if got.Role != "" || got.CoordinatorSessionID != "" || got.RootCoordinatorSessionID != "" || got.CoordinatorDepth != 0 {
		t.Errorf("worker/tree lineage leaked: Role=%q CoordSID=%q Root=%q Depth=%d",
			got.Role, got.CoordinatorSessionID, got.RootCoordinatorSessionID, got.CoordinatorDepth)
	}
}

// TestHandoffContinuationSpawnOpts_NonCoordinator: an ordinary chat session hands
// off to a plain continuation — no coordinator flags invented.
func TestHandoffContinuationSpawnOpts_NonCoordinator(t *testing.T) {
	got := handoffContinuationSpawnOpts(db.Session{ID: "SES2", Kind: "chat"}, HandoffOptions{})
	if got.CoordinatorMode || got.CoordinatorWorkflow != "" || got.CoordinatorMaxTurns != 0 {
		t.Errorf("non-coordinator parent must not gain coordinator settings: %+v", got)
	}
}

// TestContinuationKind pins the handoff-continuation classification: a chat
// parent (or legacy "") keeps the continuation in the "Sohbet" sidebar filter,
// while every other parent kind falls back to the writable "spawned" kind so a
// non-writable/tree kind never leaks onto the human-continuable continuation.
func TestContinuationKind(t *testing.T) {
	cases := map[string]string{
		"":                 "chat",
		"chat":             "chat",
		" chat ":           "chat",
		"spawned":          "spawned",
		"task":             "spawned",
		"flow":             "spawned",
		"schedule":         "spawned",
		"worker":           "spawned",
		"flow-coordinator": "spawned",
	}
	for parent, want := range cases {
		if got := continuationKind(parent); got != want {
			t.Errorf("continuationKind(%q) = %q, want %q", parent, got, want)
		}
	}
}
