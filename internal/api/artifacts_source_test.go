package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

func TestUpdateArtifactFromSessionFileWithoutInlineBody(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "document.md")
	if err := os.WriteFile(path, []byte("updated document"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	session, err := database.CreateSession(ctx, db.Session{AgentID: "agent", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	sink := newArtifactSink(database, session.ID, "agent", nil)
	ref, err := sink.CreateArtifact(ctx, tools.CreateArtifactSpec{Title: "Document", Kind: "file", SourcePath: "document.md"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("second revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]string{"id": ref.ID, "sourcePath": "document.md"})
	if _, err := tools.NewUpdateArtifactTool().Call(tools.WithArtifacts(ctx, sink), input); err != nil {
		t.Fatal(err)
	}
	a, err := database.GetArtifact(ctx, ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(database.Root()), "workspace", filepath.FromSlash(a.SourcePath)))
	if err != nil || string(body) != "second revision" {
		t.Fatalf("stale artifact: %q %v", body, err)
	}
	if _, err := sink.UpdateArtifactSource(ctx, ref.ID, "missing.md"); err == nil {
		t.Fatal("missing source accepted")
	}
}
