package agent

import (
	"context"
	"strings"
	"testing"
)

// cliRejection reproduces the exact shape claude-cli puts on its stream-json trace
// when the model calls a tool its own registry does not carry (WS20/SES79).
func cliRejection(name string) TurnStep {
	return TurnStep{
		Kind:    StepTool,
		Tool:    name,
		IsError: true,
		Output:  "<tool_use_error>Error: No such tool available: " + name + "</tool_use_error>",
	}
}

// fakeActivator records every activation attempt and answers from a fixed catalog.
type fakeActivator struct {
	catalog map[string]bool
	calls   []string
}

func (f *fakeActivator) activate(_ context.Context, token, bare string) bool {
	f.calls = append(f.calls, token+"/"+bare)
	return f.catalog[bare]
}

func newTestDeadToolRepair(f *fakeActivator, journal *[]string) *deadToolRepair {
	return newDeadToolRepair("tok1", f.activate, func(_ context.Context, name string) {
		*journal = append(*journal, name)
	})
}

func TestDeadToolRepairActivatesCatalogTool(t *testing.T) {
	f := &fakeActivator{catalog: map[string]bool{"list_tasks": true}}
	var journal []string
	d := newTestDeadToolRepair(f, &journal)

	st := cliRejection("mcp__tionharness_extended__list_tasks")
	d.repair(context.Background(), &st)

	if len(f.calls) != 1 || f.calls[0] != "tok1/list_tasks" {
		t.Fatalf("expected one activation of tok1/list_tasks, got %v", f.calls)
	}
	if !strings.Contains(st.Output, "[dead tool repair]") {
		t.Fatalf("nudge missing from step output: %q", st.Output)
	}
	if !strings.Contains(st.Output, "mcp__tionharness_extended__list_tasks") {
		t.Fatalf("nudge must name the exact callable form: %q", st.Output)
	}
	if !strings.Contains(st.Output, "No such tool available") {
		t.Fatalf("original error must be preserved, not replaced: %q", st.Output)
	}
	if len(journal) != 1 || journal[0] != "mcp__tionharness_extended__list_tasks" {
		t.Fatalf("expected one journal entry for the repaired tool, got %v", journal)
	}
}

func TestDeadToolRepairLeavesUnknownToolUntouched(t *testing.T) {
	f := &fakeActivator{catalog: map[string]bool{"list_tasks": true}}
	var journal []string
	d := newTestDeadToolRepair(f, &journal)

	st := cliRejection("mcp__tionharness_extended__totally_made_up")
	before := st.Output
	d.repair(context.Background(), &st)

	if st.Output != before {
		t.Fatalf("a name outside the catalog must pass through unchanged, got %q", st.Output)
	}
	if len(journal) != 0 {
		t.Fatalf("no repair happened, so nothing should be journalled: %v", journal)
	}
}

func TestDeadToolRepairRepairsEachNameOnlyOnce(t *testing.T) {
	f := &fakeActivator{catalog: map[string]bool{"list_tasks": true}}
	var journal []string
	d := newTestDeadToolRepair(f, &journal)

	first := cliRejection("mcp__tionharness_extended__list_tasks")
	d.repair(context.Background(), &first)

	second := cliRejection("mcp__tionharness_extended__list_tasks")
	before := second.Output
	d.repair(context.Background(), &second)

	if second.Output != before {
		t.Fatalf("second rejection of the same tool must pass through unchanged, got %q", second.Output)
	}
	if len(f.calls) != 1 {
		t.Fatalf("expected exactly one activation across both rejections, got %v", f.calls)
	}
	if len(journal) != 1 {
		t.Fatalf("expected exactly one journal entry, got %v", journal)
	}
}

func TestDeadToolRepairIgnoresForeignAndHealthySteps(t *testing.T) {
	f := &fakeActivator{catalog: map[string]bool{"list_tasks": true}}
	var journal []string
	d := newTestDeadToolRepair(f, &journal)

	cases := []TurnStep{
		// External MCP server: not ours to activate.
		cliRejection("mcp__codebase-memory-mcp__search_graph"),
		// CLI-native bare name: the model's own mis-address, it self-corrects.
		cliRejection("PowerShell"),
		// A successful step that merely mentions the phrase.
		{Kind: StepTool, Tool: "Bash", Output: "grep found: No such tool available"},
		// A non-tool step.
		{Kind: StepText, Text: "No such tool available: mcp__tionharness_extended__list_tasks"},
	}
	for i := range cases {
		st := cases[i]
		before := st.Output
		d.repair(context.Background(), &st)
		if st.Output != before {
			t.Fatalf("case %d must be untouched, got %q", i, st.Output)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("no activation should have been attempted, got %v", f.calls)
	}
}

func TestNilDeadToolRepairIsNoOp(t *testing.T) {
	var d *deadToolRepair
	st := cliRejection("mcp__tionharness_extended__list_tasks")
	before := st.Output
	d.repair(context.Background(), &st)
	if st.Output != before {
		t.Fatalf("nil repairer must not modify the step, got %q", st.Output)
	}
}

func TestParseDeadToolName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<tool_use_error>Error: No such tool available: mcp__tionharness_extended__list_tasks</tool_use_error>",
			"mcp__tionharness_extended__list_tasks"},
		{"No such tool available: PowerShell. PowerShell exists but is not enabled in this context.",
			"PowerShell"},
		{"No such tool available: list_agents. Did you mean mcp__tionharness_extended__list_agents?",
			"list_agents"},
		{"some unrelated tool failure", ""},
		{"No such tool available:", ""},
	}
	for _, c := range cases {
		if got := parseDeadToolName(c.in); got != c.want {
			t.Errorf("parseDeadToolName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBareInteractionToolName(t *testing.T) {
	if bare, ok := bareInteractionToolName("mcp__tionharness_extended__list_tasks"); !ok || bare != "list_tasks" {
		t.Fatalf("extended prefix: got (%q,%v)", bare, ok)
	}
	if bare, ok := bareInteractionToolName("mcp__tionharness_interaction__use_skill"); !ok || bare != "use_skill" {
		t.Fatalf("core prefix: got (%q,%v)", bare, ok)
	}
	if _, ok := bareInteractionToolName("mcp__other__thing"); ok {
		t.Fatal("a foreign MCP namespace must not be claimed")
	}
	if _, ok := bareInteractionToolName("Bash"); ok {
		t.Fatal("a bare name must not be claimed")
	}
}
