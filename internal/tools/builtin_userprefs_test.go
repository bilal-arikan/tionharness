package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestUpdateUserPreferences covers field patching, notes replace vs append,
// and the guard rails (both notes modes at once, empty input).
func TestUpdateUserPreferences(t *testing.T) {
	b := &fakeSettingsBridge{path: "/data/settings.json", state: map[string]any{"userNotes": "Likes short answers."}}
	tool := NewUpdateUserPreferencesTool(b)
	ctx := context.Background()

	// Plain fields map onto the settings keys.
	if _, err := tool.Call(ctx, json.RawMessage(`{"name":"Bilal","country":"Turkey","timezone":"Europe/Istanbul"}`)); err != nil {
		t.Fatalf("call: %v", err)
	}
	if b.state["userName"] != "Bilal" || b.state["userCountry"] != "Turkey" || b.state["userTimezone"] != "Europe/Istanbul" {
		t.Fatalf("profile fields not patched: %#v", b.state)
	}
	if b.state["userNotes"] != "Likes short answers." {
		t.Fatalf("notes must be untouched when not passed: %#v", b.state["userNotes"])
	}

	// notes_append extends the existing notes on a new line.
	if _, err := tool.Call(ctx, json.RawMessage(`{"notes_append":"Prefers PowerShell."}`)); err != nil {
		t.Fatalf("append: %v", err)
	}
	if got := b.state["userNotes"]; got != "Likes short answers.\nPrefers PowerShell." {
		t.Fatalf("append wrong: %q", got)
	}

	// notes replaces wholesale.
	if _, err := tool.Call(ctx, json.RawMessage(`{"notes":"Fresh start."}`)); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if got := b.state["userNotes"]; got != "Fresh start." {
		t.Fatalf("replace wrong: %q", got)
	}

	// Guards: both notes modes, and an empty patch.
	if _, err := tool.Call(ctx, json.RawMessage(`{"notes":"a","notes_append":"b"}`)); err == nil {
		t.Fatalf("notes + notes_append together must error")
	}
	if _, err := tool.Call(ctx, json.RawMessage(`{}`)); err == nil {
		t.Fatalf("empty input must error")
	}

	// Nil bridge fails loudly, not silently.
	if _, err := NewUpdateUserPreferencesTool(nil).Call(ctx, json.RawMessage(`{"name":"x"}`)); err == nil || !strings.Contains(err.Error(), "bridge") {
		t.Fatalf("nil bridge must error, got %v", err)
	}
}
