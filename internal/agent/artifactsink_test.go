package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestNewArtifactSinkPersists verifies the autonomous artifact sink persists an
// artifact to the workspace DB, stamped with the origin session/agent — so
// create_artifact works on scheduler/spawn/flow turns, not just chat.
func TestNewArtifactSinkPersists(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	sink := rt.NewArtifactSink("SES1", "AGT1")
	ref, err := sink.CreateArtifact(ctx, tools.CreateArtifactSpec{Title: "Doc", Kind: "markdown", Content: "hello"})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if ref.ID == "" {
		t.Fatal("expected an artifact id")
	}
	got, err := rt.db.GetArtifact(ctx, ref.ID)
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if got.SessionID != "SES1" || got.AgentID != "AGT1" || got.Origin != "tool" {
		t.Fatalf("artifact = %+v, want session SES1 / agent AGT1 / origin tool", got)
	}
	if got.Content != "hello" {
		t.Fatalf("artifact content = %q", got.Content)
	}
}
