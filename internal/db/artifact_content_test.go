package db

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArtifactContentExternalised verifies a text artifact's body is written to a
// real file under workspace/artifacts/, kept out of the store JSON, and read back
// into Content on reopen.
func TestArtifactContentExternalised(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")

	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	a, err := d.CreateArtifact(ctx, Artifact{Title: "Notes", Kind: ArtifactMarkdown, Content: "# Hello\nbody"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// 1) The content file exists under workspace/artifacts/_shared/<id>.md (no
	// session → the _shared bucket) with the body.
	if a.ContentFile != "artifacts/_shared/"+a.ID+".md" {
		t.Fatalf("unexpected ContentFile: %q", a.ContentFile)
	}
	contentPath := filepath.Join(root, "workspace", "artifacts", "_shared", a.ID+".md")
	body, err := os.ReadFile(contentPath)
	if err != nil {
		t.Fatalf("content file missing: %v", err)
	}
	if string(body) != "# Hello\nbody" {
		t.Errorf("content file body = %q", string(body))
	}

	// 2) The store JSON does NOT embed the body.
	rawJSON, err := os.ReadFile(filepath.Join(storeDir, "artifacts", a.ID+".json"))
	if err != nil {
		t.Fatalf("store json missing: %v", err)
	}
	var stored map[string]any
	_ = json.Unmarshal(rawJSON, &stored)
	if c, _ := stored["content"].(string); c != "" {
		t.Errorf("store JSON should not embed content, got %q", c)
	}
	if !strings.Contains(string(rawJSON), a.ID+".md") {
		t.Errorf("store JSON should reference the content file")
	}

	// 3) Reopen: Content is read back from the file.
	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := d2.GetArtifact(ctx, a.ID)
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if got.Content != "# Hello\nbody" {
		t.Errorf("content not restored on reopen: %q", got.Content)
	}

	// 4) Delete removes the content file too.
	if err := d2.DeleteArtifact(ctx, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(contentPath); !os.IsNotExist(err) {
		t.Errorf("content file should be removed, stat err = %v", err)
	}
}

// TestArtifactLegacyMigration verifies an artifact JSON with an embedded body
// (pre-externalisation) is migrated to a content file on load.
func TestArtifactLegacyMigration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	if err := os.MkdirAll(filepath.Join(storeDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Hand-write a legacy artifact JSON with embedded content + no contentFile.
	legacy := `{"id":"leg1","title":"Old","kind":"markdown","content":"legacy body","createdAt":1,"updatedAt":1}`
	if err := os.WriteFile(filepath.Join(storeDir, "artifacts", "leg1.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, err := d.GetArtifact(ctx, "leg1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Content != "legacy body" {
		t.Errorf("legacy content lost: %q", got.Content)
	}
	if got.ContentFile == "" {
		t.Errorf("legacy artifact should have been migrated to a content file")
	}
	if _, err := os.Stat(filepath.Join(root, "workspace", "artifacts", "_shared", "leg1.md")); err != nil {
		t.Errorf("migrated content file missing: %v", err)
	}
}
