package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestToolRecapBlock renders tool steps compactly and skips non-tool steps.
func TestToolRecapBlock(t *testing.T) {
	steps := `[
		{"kind":"text","text":"thinking out loud"},
		{"kind":"tool","tool":"Bash","input":{"command":"git status"},"output":"On branch main\nclean"},
		{"kind":"tool","tool":"Read","input":{"file_path":"main.go"},"output":"package main","isError":false}
	]`
	block := toolRecapBlock(steps)
	if !strings.Contains(block, "<recent_tool_activity>") {
		t.Fatalf("missing wrapper: %q", block)
	}
	if !strings.Contains(block, "Bash(git status)") {
		t.Errorf("bash arg hint missing: %q", block)
	}
	if !strings.Contains(block, "Read(main.go)") {
		t.Errorf("read arg hint missing: %q", block)
	}
	if strings.Contains(block, "thinking out loud") {
		t.Errorf("non-tool step should not appear: %q", block)
	}
	// Newlines in output are flattened to keep each line compact.
	if strings.Contains(block, "On branch main\nclean") {
		t.Errorf("output newlines should be flattened: %q", block)
	}
}

// TestToolRecapBlockEmpty returns "" for a turn with no tool steps.
func TestToolRecapBlockEmpty(t *testing.T) {
	if got := toolRecapBlock(`[{"kind":"text","text":"just text"}]`); got != "" {
		t.Errorf("expected empty recap, got %q", got)
	}
	if got := toolRecapBlock("not json"); got != "" {
		t.Errorf("invalid steps should yield empty recap, got %q", got)
	}
}

// TestAppendRecentToolSummaries folds the recap into recent assistant turns only,
// leaves user turns and step-less turns untouched, and never mutates the input.
func TestAppendRecentToolSummaries(t *testing.T) {
	withTool := `[{"kind":"tool","tool":"Bash","input":{"command":"ls"},"output":"a b c"}]`
	history := []db.Message{
		{Role: "user", Text: "do it"},
		{Role: "assistant", Text: "done", Steps: withTool},
	}
	out := appendRecentToolSummaries(history)

	if !strings.Contains(out[1].Text, "<recent_tool_activity>") || !strings.Contains(out[1].Text, "Bash(ls)") {
		t.Errorf("recent assistant turn should get a recap: %q", out[1].Text)
	}
	if out[0].Text != "do it" {
		t.Errorf("user turn must be untouched: %q", out[0].Text)
	}
	// Input not mutated (recap applied on a copy).
	if strings.Contains(history[1].Text, "recent_tool_activity") {
		t.Errorf("input history was mutated: %q", history[1].Text)
	}
}
