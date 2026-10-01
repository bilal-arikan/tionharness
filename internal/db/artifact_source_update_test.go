package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactSourceImportsNeverOverwriteAnotherArtifact(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	path := filepath.Join(t.TempDir(), "document.md")
	if err := os.WriteFile(path, []byte("first artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := d.ImportMediaSource("SES1", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("second artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := d.ImportMediaSource("SES1", path)
	if err != nil || first == second {
		t.Fatalf("imports shared a path: first=%q second=%q err=%v", first, second, err)
	}
	body, err := os.ReadFile(filepath.Join(d.workspaceDir(), filepath.FromSlash(first)))
	if err != nil || string(body) != "first artifact" {
		t.Fatalf("first artifact was overwritten: %q %v", body, err)
	}
}

func TestTextArtifactSourceUsesSessionDirectoryAndChecksOwnership(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	dir := t.TempDir()
	session, err := d.CreateSession(ctx, Session{AgentID: "test", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	a, err := d.CreateArtifact(ctx, Artifact{SessionID: session.ID, Kind: ArtifactMarkdown, Title: "Document", Content: "original"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "document.md"), []byte("source update"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.UpdateArtifactFromSource(ctx, a.ID, "another-session", "document.md"); err == nil {
		t.Fatal("cross-session source update accepted")
	}
	if _, err := d.UpdateArtifactFromSource(ctx, a.ID, session.ID, dir); err == nil {
		t.Fatal("directory accepted as text")
	}
	a, err = d.UpdateArtifactFromSource(ctx, a.ID, session.ID, "document.md")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(d.workspaceDir(), filepath.FromSlash(a.ContentFile)))
	if err != nil || string(body) != "source update" {
		t.Fatalf("text artifact stayed stale: %q %v", body, err)
	}
	a, err = d.UpdateArtifactContent(ctx, a.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(filepath.Join(d.workspaceDir(), filepath.FromSlash(a.ContentFile)))
	if err != nil || len(body) != 0 {
		t.Fatalf("empty content retained the old file: %q %v", body, err)
	}
}
