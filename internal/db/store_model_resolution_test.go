package db

import (
	"testing"
)

func TestNoteModelResolution(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()

	if err := d.NoteModelResolution(ctx, "claude-cli", "opus", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}
	if got := d.ResolvedModelFor("claude-cli", "opus"); got != "claude-opus-5" {
		t.Fatalf("resolved = %q, want claude-opus-5", got)
	}

	// The empty requested id (claude-cli's "use the session default") is a real
	// key, not a missing one — it is the case where the model is MOST hidden.
	if err := d.NoteModelResolution(ctx, "claude-cli", "", "claude-sonnet-5"); err != nil {
		t.Fatal(err)
	}
	if got := d.ResolvedModelFor("claude-cli", ""); got != "claude-sonnet-5" {
		t.Fatalf("default resolved = %q, want claude-sonnet-5", got)
	}

	// A provider echoing the concrete id it was given teaches nothing and must not
	// be stored, or every native provider would fill the map with identities.
	if err := d.NoteModelResolution(ctx, "anthropic", "claude-opus-5", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}
	if got := d.ResolvedModelFor("anthropic", "claude-opus-5"); got != "" {
		t.Fatalf("echo stored as %q, want empty", got)
	}

	if got := d.ResolvedModelFor("claude-cli", "haiku"); got != "" {
		t.Fatalf("unobserved alias = %q, want empty", got)
	}
}

func TestModelResolutionSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.NoteModelResolution(t.Context(), "claude-cli", "opus", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.ResolvedModelFor("claude-cli", "opus"); got != "claude-opus-5" {
		t.Fatalf("after reopen = %q, want claude-opus-5", got)
	}
}

// A newer answer replaces the older one: when Anthropic moves an alias to a new
// model, the label must follow rather than pin to whatever was seen first.
func TestNoteModelResolutionOverwrites(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err := d.NoteModelResolution(ctx, "claude-cli", "opus", "claude-opus-4-8"); err != nil {
		t.Fatal(err)
	}
	if err := d.NoteModelResolution(ctx, "claude-cli", "opus", "claude-opus-5"); err != nil {
		t.Fatal(err)
	}
	if got := d.ResolvedModelFor("claude-cli", "opus"); got != "claude-opus-5" {
		t.Fatalf("resolved = %q, want claude-opus-5", got)
	}
}
