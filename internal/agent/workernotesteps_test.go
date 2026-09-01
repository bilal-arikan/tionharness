package agent

import (
	"encoding/json"
	"testing"
)

// TestDigestWorkerStepsDropsOutputs is the storage fix: a worker's shell/search
// trace must not be copied onto the coordinator's notification.
func TestDigestWorkerStepsDropsOutputs(t *testing.T) {
	big := make([]byte, 64*1024)
	for i := range big {
		big[i] = 'x'
	}
	steps := []TurnStep{
		{Kind: StepThinking, Text: "long reasoning"},
		{Kind: StepTool, Tool: "Bash", Output: string(big)},
		{Kind: StepTool, Tool: "mcp__codebase-memory-mcp__search_code", Output: string(big)},
		{Kind: StepText, Text: "final answer"},
	}
	got := digestWorkerSteps(steps)
	if len(got) != 0 {
		t.Fatalf("a read-only worker's trace must digest to nothing, got %d steps", len(got))
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(encoded) > 64 {
		t.Fatalf("digest encodes to %d bytes, want a near-empty array", len(encoded))
	}
}

// TestDigestWorkerStepsKeepsFileChanges: the card renders file changes, so those
// must survive intact — including a namespaced apply_patch from a CLI provider.
func TestDigestWorkerStepsKeepsFileChanges(t *testing.T) {
	steps := []TurnStep{
		{Kind: StepTool, Tool: "Bash", Output: "noise"},
		{Kind: StepDiff, Tool: "Write", Path: "a.go", Patch: "@@ -1 +1 @@", Added: 3, Removed: 1, Created: true},
		{Kind: StepTool, Tool: "mcp__tionharness_extended__apply_patch", Input: json.RawMessage(`{"patch":"x"}`)},
		{Kind: StepTodo, Todos: []TodoItem{{Content: "step one", Status: "completed"}}},
	}
	got := digestWorkerSteps(steps)
	if len(got) != 3 {
		t.Fatalf("digest kept %d steps, want 3 (diff + apply_patch + todo)", len(got))
	}
	if got[0].Kind != StepDiff || got[0].Path != "a.go" || got[0].Added != 3 || !got[0].Created {
		t.Fatalf("diff step lost its payload: %+v", got[0])
	}
	if got[1].Tool != "mcp__tionharness_extended__apply_patch" || len(got[1].Input) == 0 {
		t.Fatalf("edit tool step must keep its input for diff synthesis: %+v", got[1])
	}
	if got[2].Kind != StepTodo || len(got[2].Todos) != 1 {
		t.Fatalf("todo step lost its items: %+v", got[2])
	}
}

// TestDigestWorkerStepsHoistsNestedChanges: a subagent's edits hit the same disk
// and must not be lost because the parent step itself is noise.
func TestDigestWorkerStepsHoistsNestedChanges(t *testing.T) {
	steps := []TurnStep{{
		Kind: StepSubagent,
		Text: "delegated",
		SubSteps: []TurnStep{
			{Kind: StepTool, Tool: "Grep", Output: "noise"},
			{Kind: StepDiff, Tool: "Edit", Path: "b.ts", Added: 1},
		},
	}}
	got := digestWorkerSteps(steps)
	if len(got) != 1 || got[0].Kind != StepDiff || got[0].Path != "b.ts" {
		t.Fatalf("nested change must be hoisted, got %+v", got)
	}
}
