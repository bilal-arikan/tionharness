package agent

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// The mirror takes the LAST native checklist of the turn, ignores the bridged
// todo_write (it persisted itself) and errored calls, and understands both the
// claude TodoWrite input and the codex todo_list input the parser synthesises.
func TestNativeChecklistFromTraceTakesLastNativeList(t *testing.T) {
	trace := []providers.TraceStep{
		{Kind: "tool", Tool: interactionToolPrefix + "todo_write", Input: json.RawMessage(`{"todos":[{"content":"bridged","status":"pending"}]}`)},
		{Kind: "tool", Tool: "TodoWrite", Input: json.RawMessage(`{"todos":[{"content":"first","status":"in_progress","activeForm":"Doing first"}]}`)},
		{Kind: "tool", Tool: "TodoWrite", IsError: true, Input: json.RawMessage(`{"todos":[{"content":"errored","status":"pending"}]}`)},
		{Kind: "tool", Tool: "todo_list", Input: json.RawMessage(`{"todos":[{"content":"read contract","status":"completed"},{"content":"write parser","status":"pending"}]}`)},
		{Kind: "text", Text: "done"},
	}
	got := nativeChecklistFromTrace(trace)
	if len(got) != 2 || got[0].Content != "read contract" || got[0].Status != "completed" || got[1].Content != "write parser" {
		t.Fatalf("mirror = %+v, want the last native (codex) list", got)
	}
}

func TestNativeChecklistFromTraceIgnoresBridgedOnly(t *testing.T) {
	trace := []providers.TraceStep{
		{Kind: "tool", Tool: interactionToolPrefix + "todo_write", Input: json.RawMessage(`{"todos":[{"content":"bridged","status":"pending"}]}`)},
		{Kind: "tool", Tool: "Read", Input: json.RawMessage(`{"file_path":"x"}`)},
	}
	if got := nativeChecklistFromTrace(trace); got != nil {
		t.Fatalf("bridged-only turn must not mirror anything, got %+v", got)
	}
}

// Native checklist tools render as the same first-class todo card the bridged
// todo_write gets, and a folded native subagent launch becomes a subagent card
// whose nested trace is converted recursively.
func TestTraceStepToTurnStepPromotesNativeChecklistAndSubagent(t *testing.T) {
	todo := traceStepToTurnStep(providers.TraceStep{Kind: "tool", Tool: "TodoWrite",
		Input: json.RawMessage(`{"todos":[{"content":"a","status":"pending","activeForm":"Doing a"}]}`)})
	if todo.Kind != StepTodo || len(todo.Todos) != 1 || todo.Todos[0].Content != "a" {
		t.Fatalf("TodoWrite step = %+v, want a todo card", todo)
	}
	plan := traceStepToTurnStep(providers.TraceStep{Kind: "tool", Tool: "todo_list",
		Input: json.RawMessage(`{"todos":[{"content":"b","status":"completed"}]}`)})
	if plan.Kind != StepTodo || len(plan.Todos) != 1 || plan.Todos[0].Status != "completed" {
		t.Fatalf("todo_list step = %+v, want a todo card", plan)
	}

	sub := traceStepToTurnStep(providers.TraceStep{
		ID: "toolu_agent", Kind: "tool", Tool: "Agent", Running: true,
		Input:  json.RawMessage(`{"subagent_type":"Explore","description":"find callers"}`),
		Output: "three callers",
		SubSteps: []providers.TraceStep{
			{Kind: "thinking", Text: "looking"},
			{Kind: "tool", Tool: "Grep", Input: json.RawMessage(`{"pattern":"foo"}`), Output: "a.go"},
			{Kind: "text", Text: "found them"},
		},
	})
	if sub.Kind != StepSubagent || sub.Tool != "Agent" || sub.ID != "toolu_agent" || !sub.Running {
		t.Fatalf("Agent step = %+v, want a running subagent card", sub)
	}
	if len(sub.SubSteps) != 3 || sub.SubSteps[1].Kind != StepTool || sub.SubSteps[1].Tool != "Grep" || sub.SubSteps[2].Kind != StepText {
		t.Fatalf("nested steps = %+v", sub.SubSteps)
	}
	// A launcher whose child forwarded nothing is still a subagent card.
	bare := traceStepToTurnStep(providers.TraceStep{Kind: "tool", Tool: "Agent", Output: "ok"})
	if bare.Kind != StepSubagent {
		t.Fatalf("bare Agent step kind = %q, want subagent", bare.Kind)
	}
	// An ordinary tool is untouched.
	read := traceStepToTurnStep(providers.TraceStep{Kind: "tool", Tool: "Read", Output: "x"})
	if read.Kind != StepTool {
		t.Fatalf("Read step kind = %q, want tool", read.Kind)
	}
}
