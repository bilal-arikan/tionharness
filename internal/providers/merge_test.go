package providers

import "testing"

// TestMergeCatalogOverridesBuiltinByID pins the fix for the duplicate "openrouter"
// catalog entry: a custom provider sharing a built-in ID must replace it in place
// (no second entry with the same ID), while a new custom ID appends at the end.
func TestMergeCatalogOverridesBuiltinByID(t *testing.T) {
	builtin := []CatalogEntry{
		{ID: "anthropic", Label: "Anthropic"},
		{ID: "openrouter", Label: "OpenRouter (built-in)", Models: []ModelInfo{{ID: "a"}, {ID: "b"}}},
	}
	custom := []CatalogEntry{
		{ID: "openrouter", Label: "OpenRouter (custom)", Models: []ModelInfo{{ID: "z"}}},
		{ID: "my-llm", Label: "My LLM"},
	}

	got := MergeCatalog(builtin, custom)

	// No duplicate IDs.
	seen := map[string]int{}
	for _, e := range got {
		seen[e.ID]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Fatalf("duplicate catalog id %q appears %d times", id, n)
		}
	}

	// The custom openrouter must have replaced the built-in one in place.
	var or *CatalogEntry
	for i := range got {
		if got[i].ID == "openrouter" {
			or = &got[i]
		}
	}
	if or == nil {
		t.Fatal("openrouter entry missing after merge")
	}
	if or.Label != "OpenRouter (custom)" || len(or.Models) != 1 || or.Models[0].ID != "z" {
		t.Fatalf("custom provider did not override built-in: %+v", or)
	}

	// Expected order: anthropic, openrouter (overridden in place), my-llm (appended).
	wantOrder := []string{"anthropic", "openrouter", "my-llm"}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(wantOrder), got)
	}
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Fatalf("order[%d] = %q, want %q", i, got[i].ID, id)
		}
	}

	// Inputs must be untouched (fresh result slice).
	if builtin[1].Label != "OpenRouter (built-in)" {
		t.Fatalf("MergeCatalog mutated its builtin input: %+v", builtin[1])
	}
}
