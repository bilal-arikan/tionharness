package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestHandoffContinuationOrigin: a context-reset continuation is a handoff from
// the old session and stays in the old session's tree.
func TestHandoffContinuationOrigin(t *testing.T) {
	parent := db.Session{ID: "SES10", Kind: "chat", Title: "work",
		Origin: &db.SessionOrigin{Kind: db.OriginSpawn, TriggerSessionID: "SES1", RootSessionID: "SES1"}}
	opts := handoffContinuationSpawnOpts(parent, HandoffOptions{})
	if opts.Origin == nil || opts.Origin.Kind != db.OriginHandoff {
		t.Fatalf("continuation origin = %+v, want handoff", opts.Origin)
	}
	if opts.Origin.TriggerSessionID != "SES10" {
		t.Fatalf("continuation trigger = %q, want the handed-off session SES10", opts.Origin.TriggerSessionID)
	}
	if opts.Origin.RootSessionID != "SES1" {
		t.Fatalf("continuation root = %q, want the parent's root SES1", opts.Origin.RootSessionID)
	}
	if opts.ParentSessionID != "SES10" {
		t.Fatalf("ParentSessionID must still mirror the handoff lineage, got %q", opts.ParentSessionID)
	}
}
