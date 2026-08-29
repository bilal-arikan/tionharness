package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNoteModelResolutionMirrorsToGlobal(t *testing.T) {
	appDir := t.TempDir()
	global, err := OpenGlobalModelResolutions(appDir)
	if err != nil {
		t.Fatal(err)
	}

	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d.SetGlobalModelResolutions(global)

	if err := d.NoteModelResolution(t.Context(), "claude-cli", "", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}
	if got := global.Resolved("claude-cli", ""); got != "claude-opus-5" {
		t.Fatalf("global resolved = %q, want claude-opus-5", got)
	}
	if _, err := os.Stat(filepath.Join(appDir, modelResolutionsFile)); err != nil {
		t.Fatalf("app-global document not written: %v", err)
	}

	// An echo teaches nothing in the global store either.
	if err := d.NoteModelResolution(t.Context(), "anthropic", "claude-opus-5", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}
	if got := global.Resolved("anthropic", "claude-opus-5"); got != "" {
		t.Fatalf("echo stored globally as %q, want empty", got)
	}
}

// The whole point of the layer: a workspace that has never completed a turn
// still answers via the global fallback, while its own map stays empty.
func TestGlobalResolvedModelForFallback(t *testing.T) {
	appDir := t.TempDir()
	global, err := OpenGlobalModelResolutions(appDir)
	if err != nil {
		t.Fatal(err)
	}

	learner, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	learner.SetGlobalModelResolutions(global)
	if err := learner.NoteModelResolution(t.Context(), "claude-cli", "", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}

	fresh, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fresh.SetGlobalModelResolutions(global)

	if got := fresh.ResolvedModelFor("claude-cli", ""); got != "" {
		t.Fatalf("workspace-local = %q, want empty (semantics must not change)", got)
	}
	if got := fresh.GlobalResolvedModelFor("claude-cli", ""); got != "claude-opus-5" {
		t.Fatalf("global fallback = %q, want claude-opus-5", got)
	}
}

func TestGlobalModelResolutionsSurvivesReopen(t *testing.T) {
	appDir := t.TempDir()
	global, err := OpenGlobalModelResolutions(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := global.Note("claude-cli", "opus", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenGlobalModelResolutions(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Resolved("claude-cli", "opus"); got != "claude-opus-5" {
		t.Fatalf("after reopen = %q, want claude-opus-5", got)
	}
}

// A DB with no global store attached must keep working, and must not claim a
// resolution it does not have.
func TestGlobalResolutionUnwired(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := d.NoteModelResolution(t.Context(), "claude-cli", "opus", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}
	if got := d.GlobalResolvedModelFor("claude-cli", "opus"); got != "" {
		t.Fatalf("unwired global = %q, want empty", got)
	}
}

// A document that exists but is unparseable is an error, not a silently empty
// store: ignoring real state on disk must be a decision the caller makes.
func TestOpenGlobalModelResolutionsCorrupt(t *testing.T) {
	appDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(appDir, modelResolutionsFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenGlobalModelResolutions(appDir); err == nil {
		t.Fatal("corrupt document opened without error")
	}
}
