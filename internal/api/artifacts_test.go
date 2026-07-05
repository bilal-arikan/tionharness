package api

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestArtifactsContextBlock verifies the system-prompt block lists a session's
// artifacts (id/title/kind) so the agent can revise them with update_artifact,
// and stays empty when there is nothing to surface.
func TestArtifactsContextBlock(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	// Empty session → no block.
	if got := artifactsContextBlock(ctx, database, "sess-1"); got != "" {
		t.Fatalf("expected empty block for no artifacts, got %q", got)
	}
	// Empty session id → no block.
	if got := artifactsContextBlock(ctx, database, ""); got != "" {
		t.Fatalf("expected empty block for empty session id, got %q", got)
	}

	a, err := database.CreateArtifact(ctx, db.Artifact{
		SessionID: "sess-1", AgentID: "agent-1",
		Title: "Plan Doc", Kind: db.ArtifactMarkdown, Content: "# Plan",
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	// An artifact in a different session must not leak into sess-1's block.
	if _, err := database.CreateArtifact(ctx, db.Artifact{
		SessionID: "sess-2", Title: "Other", Kind: db.ArtifactCode, Language: "go", Content: "x",
	}); err != nil {
		t.Fatalf("create other artifact: %v", err)
	}

	block := artifactsContextBlock(ctx, database, "sess-1")
	if !strings.Contains(block, a.ID) {
		t.Errorf("block missing artifact id %q:\n%s", a.ID, block)
	}
	if !strings.Contains(block, "Plan Doc") {
		t.Errorf("block missing title:\n%s", block)
	}
	if !strings.Contains(block, "update_artifact") {
		t.Errorf("block should instruct update_artifact:\n%s", block)
	}
	if strings.Contains(block, "Other") {
		t.Errorf("block leaked another session's artifact:\n%s", block)
	}
}
