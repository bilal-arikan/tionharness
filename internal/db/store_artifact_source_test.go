package db

import (
	"context"
	"errors"
	"testing"
)

// TestUpdateArtifactSource covers the in-place source swap: the row is repointed,
// the caller gets the previous path back for cleanup, and the guards fire.
func TestUpdateArtifactSource(t *testing.T) {
	ctx := context.Background()
	database, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original, err := database.CreateArtifact(ctx, Artifact{
		Title: "shot", Kind: ArtifactImage, SourcePath: "artifacts/SES1/old.png",
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, old, err := database.UpdateArtifactSource(ctx, original.ID, "artifacts/SES1/new.png")
	if err != nil {
		t.Fatalf("UpdateArtifactSource: %v", err)
	}
	if old != "artifacts/SES1/old.png" {
		t.Fatalf("old source = %q, want artifacts/SES1/old.png", old)
	}
	if updated.SourcePath != "artifacts/SES1/new.png" {
		t.Fatalf("new source = %q", updated.SourcePath)
	}
	if updated.UpdatedAt < original.UpdatedAt {
		t.Fatalf("UpdatedAt went backwards: %d < %d", updated.UpdatedAt, original.UpdatedAt)
	}

	// The swap is persisted, not just held in the returned copy.
	reloaded, err := database.GetArtifact(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SourcePath != "artifacts/SES1/new.png" {
		t.Fatalf("stored source = %q", reloaded.SourcePath)
	}

	if _, _, err := database.UpdateArtifactSource(ctx, original.ID, ""); err == nil {
		t.Fatal("empty sourcePath must be rejected")
	}
	if _, _, err := database.UpdateArtifactSource(ctx, "ART-nope", "artifacts/SES1/x.png"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id error = %v, want ErrNotFound", err)
	}
}
