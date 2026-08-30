package api

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestResolvedCatalogModelPrecedenceForEmptyAlias(t *testing.T) {
	global, err := db.OpenGlobalModelResolutions(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.SetGlobalModelResolutions(global)

	if err := global.Note("claude-cli", "", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}
	if got := resolvedCatalogModel(database, "claude-cli", ""); got != "claude-opus-5" {
		t.Fatalf("global fallback = %q, want claude-opus-5", got)
	}

	if err := database.NoteModelResolution(t.Context(), "claude-cli", "", "claude-sonnet-5"); err != nil {
		t.Fatal(err)
	}
	if err := global.Note("claude-cli", "", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}
	if got := resolvedCatalogModel(database, "claude-cli", ""); got != "claude-sonnet-5" {
		t.Fatalf("workspace resolution = %q, want claude-sonnet-5", got)
	}
}
