package db

import (
	"context"
	"strings"
	"testing"
)

// TestAppendPlanArtifact verifies the per-session rolling plan artifact: the first
// approved plan creates ONE artifact (origin "plan"), each later plan appends a new
// numbered section to the SAME artifact (no new artifact per plan), and the
// appended body is persisted (readable after reopen).
func TestAppendPlanArtifact(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir() + "/store"
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	a1, err := d.AppendPlanArtifact(ctx, "SES1", "AGT1", "Plan A body")
	if err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if a1.Origin != originPlan || a1.Kind != ArtifactMarkdown {
		t.Fatalf("unexpected origin/kind: %q/%q", a1.Origin, a1.Kind)
	}

	a2, err := d.AppendPlanArtifact(ctx, "SES1", "AGT1", "Plan B body")
	if err != nil {
		t.Fatalf("append 2: %v", err)
	}
	// Same artifact reused (one per session), not a second one.
	if a2.ID != a1.ID {
		t.Fatalf("expected same artifact id, got %q vs %q", a2.ID, a1.ID)
	}
	arts, _ := d.ListArtifacts(ctx, "SES1")
	if len(arts) != 1 {
		t.Fatalf("want 1 plan artifact, got %d", len(arts))
	}
	if !strings.Contains(a2.Content, "## Plan 1 ") || !strings.Contains(a2.Content, "## Plan 2 ") {
		t.Fatalf("want both plan sections, got:\n%s", a2.Content)
	}
	if !strings.Contains(a2.Content, "Plan A body") || !strings.Contains(a2.Content, "Plan B body") {
		t.Fatalf("want both plan bodies, got:\n%s", a2.Content)
	}

	// A different session gets its OWN plan artifact (scoping bounds accumulation).
	if _, err := d.AppendPlanArtifact(ctx, "SES2", "AGT1", "Other"); err != nil {
		t.Fatalf("append other session: %v", err)
	}

	// Reopen → the appended body survives (externalised content file rewritten).
	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := d2.GetArtifact(ctx, a1.ID)
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if !strings.Contains(got.Content, "Plan A body") || !strings.Contains(got.Content, "Plan B body") {
		t.Fatalf("content not persisted across reopen, got:\n%s", got.Content)
	}
}
