package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestHandoffContinuationSpawnOpts_CarriesSessionSettings: a /handoff seeds the
// clean-window continuation with the parent's working directory and kind, and links
// it to the handed-off session as its parent.
func TestHandoffContinuationSpawnOpts_CarriesSessionSettings(t *testing.T) {
	parent := db.Session{
		ID:         "SES1",
		Kind:       "chat",
		WorkingDir: "C:\\proj",
	}
	got := handoffContinuationSpawnOpts(parent, HandoffOptions{CreatedBy: "AGT1"})

	if got.WorkingDir != "C:\\proj" {
		t.Errorf("WorkingDir = %q, want C:\\proj", got.WorkingDir)
	}
	if got.ParentSessionID != "SES1" {
		t.Errorf("ParentSessionID = %q, want SES1", got.ParentSessionID)
	}
	if got.Kind != "chat" {
		t.Errorf("Kind = %q, want chat", got.Kind)
	}
	if got.CreatedBy != "AGT1" {
		t.Errorf("CreatedBy = %q, want AGT1", got.CreatedBy)
	}
	if got.Origin == nil || got.Origin.Kind != db.OriginHandoff || got.Origin.TriggerSessionID != "SES1" {
		t.Errorf("Origin = %+v, want a handoff edge from SES1", got.Origin)
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
