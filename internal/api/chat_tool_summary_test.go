package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestToolRecapLines renders tool steps compactly and skips non-tool steps.
func TestToolRecapLines(t *testing.T) {
	steps := `[
		{"kind":"text","text":"thinking out loud"},
		{"kind":"tool","tool":"Bash","input":{"command":"git status"},"output":"On branch main\nclean"},
		{"kind":"tool","tool":"Read","input":{"file_path":"main.go"},"output":"package main","isError":false}
	]`
	block := strings.Join(toolRecapLines(steps), "\n")
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

// TestToolRecapLinesUseCallName shows the exact (namespaced) callable name in the
// recap when a step carries CallName — so a claude-cli agent re-reading its history
// calls mcp__tionswarm_extended__list_tasks, not the bare list_tasks the CLI rejects.
func TestToolRecapLinesUseCallName(t *testing.T) {
	steps := `[
		{"kind":"tool","tool":"list_tasks","callName":"mcp__tionswarm_extended__list_tasks","input":{},"output":"[]"},
		{"kind":"tool","tool":"Grep","input":{"pattern":"foo"},"output":"hit"}
	]`
	block := strings.Join(toolRecapLines(steps), "\n")
	if !strings.Contains(block, "mcp__tionswarm_extended__list_tasks") {
		t.Errorf("recap must show the namespaced callable name: %q", block)
	}
	// A bridged tool's bare name must NOT be what the model sees to re-call.
	if strings.Contains(block, "- list_tasks →") {
		t.Errorf("recap should not show the bare bridged name: %q", block)
	}
	// Native tools (no CallName) stay bare.
	if !strings.Contains(block, "Grep(foo)") {
		t.Errorf("native tool should stay bare: %q", block)
	}
}

// TestToolRecapLinesEmpty returns nil for a turn with no tool steps.
func TestToolRecapLinesEmpty(t *testing.T) {
	if got := toolRecapLines(`[{"kind":"text","text":"just text"}]`); got != nil {
		t.Errorf("expected no recap lines, got %q", got)
	}
	if got := toolRecapLines("not json"); got != nil {
		t.Errorf("invalid steps should yield no recap lines, got %q", got)
	}
}

// TestRecentToolActivityBlock renders ONE combined dynamic block from the newest
// assistant turns' tool traces, labels turn ages, ignores user turns, and — the
// point of the design — never touches the history messages themselves.
func TestRecentToolActivityBlock(t *testing.T) {
	withTool := `[{"kind":"tool","tool":"Bash","input":{"command":"ls"},"output":"a b c"}]`
	older := `[{"kind":"tool","tool":"Read","input":{"file_path":"go.mod"},"output":"module x"}]`
	history := []db.Message{
		{Role: "user", Text: "do it"},
		{Role: "assistant", Text: "reading", Steps: older},
		{Role: "user", Text: "and?"},
		{Role: "assistant", Text: "done", Steps: withTool},
	}
	block := recentToolActivityBlock(history)

	if !strings.Contains(block, "<recent_tool_activity>") {
		t.Fatalf("missing wrapper: %q", block)
	}
	if !strings.Contains(block, "Bash(ls)") || !strings.Contains(block, "Read(go.mod)") {
		t.Errorf("both recent turns should be recapped: %q", block)
	}
	if !strings.Contains(block, "[latest assistant turn]") || !strings.Contains(block, "[1 assistant turn(s) ago]") {
		t.Errorf("turn-age labels missing: %q", block)
	}
	// History must stay untouched (byte-stable for the rolling cache breakpoint).
	if strings.Contains(history[3].Text, "recent_tool_activity") {
		t.Errorf("history was mutated: %q", history[3].Text)
	}
	// A history with no tool steps yields no block at all.
	if got := recentToolActivityBlock([]db.Message{{Role: "assistant", Text: "hi"}}); got != "" {
		t.Errorf("expected empty block, got %q", got)
	}
}
