package tools

import "testing"

func TestGroupKeyValidation(t *testing.T) {
	if !IsGroupKey("group:files") || ValidGroupKey("group:files") != true {
		t.Fatal("group:files must be a valid group key")
	}
	if ValidGroupKey("group:nope") {
		t.Fatal("unknown category must be rejected")
	}
	if IsGroupKey("group:") {
		t.Fatal("a bare prefix names no category")
	}
	if IsGroupKey("Read") || ValidGroupKey("mcp__linear__*") {
		t.Fatal("plain names and prefix patterns are not group keys")
	}
}

func TestMatchesGroup(t *testing.T) {
	if !MatchesGroup("Bash", "group:files") {
		t.Fatal("Bash belongs to the files category")
	}
	if MatchesGroup("Bash", "group:search") {
		t.Fatal("Bash must not match an unrelated category")
	}
	// An unmapped built-in falls into "other" and is targetable there.
	if !MatchesGroup("totally_unmapped_tool", "group:other") {
		t.Fatal("unmapped built-ins belong to group:other")
	}
	// MCP tools group by server, never by functional category.
	if MatchesGroup("mcp__linear__issue", "group:other") || MatchesGroup("mcp__linear__issue", "group:files") {
		t.Fatal("namespaced MCP tools must never match a category group")
	}
	if MatchesGroup("Bash", "group:bogus") {
		t.Fatal("an invalid group key matches nothing")
	}
}

func TestCategoriesStableAndComplete(t *testing.T) {
	got := Categories()
	if len(got) == 0 || got[0] != CategoryFiles || got[len(got)-1] != CategoryOther {
		t.Fatalf("unexpected category order: %v", got)
	}
	// Every category a built-in is mapped to must be listed.
	seen := map[string]bool{}
	for _, c := range got {
		seen[c] = true
	}
	for name, cat := range builtinCategory {
		if !seen[cat] {
			t.Fatalf("category %q of tool %q missing from Categories()", cat, name)
		}
	}
	// The returned slice is a copy: mutating it must not corrupt the source.
	got[0] = "mutated"
	if Categories()[0] != CategoryFiles {
		t.Fatal("Categories() must return a defensive copy")
	}
}
