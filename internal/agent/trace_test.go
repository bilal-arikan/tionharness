package agent

import (
	"encoding/json"
	"testing"
)

func TestParseTodos(t *testing.T) {
	in := json.RawMessage(`{"todos":[
		{"content":"plan","status":"completed"},
		{"content":"build","status":"in_progress"}
	]}`)
	got := parseTodos(in)
	if len(got) != 2 {
		t.Fatalf("want 2 items, got %d", len(got))
	}
	if got[0].Content != "plan" || got[0].Status != "completed" {
		t.Fatalf("unexpected first item: %+v", got[0])
	}
	if got[1].Status != "in_progress" {
		t.Fatalf("unexpected second status: %q", got[1].Status)
	}
}

func TestParseTodosBadInput(t *testing.T) {
	if got := parseTodos(json.RawMessage(`not json`)); got != nil {
		t.Fatalf("want nil on bad input, got %v", got)
	}
}

func TestEncodeStepsTodoRoundTrip(t *testing.T) {
	steps := []TurnStep{
		{Kind: StepTodo, Tool: "todo_write", Todos: []TodoItem{{Content: "x", Status: "pending"}}},
		{Kind: StepRecovery, Reason: "max_tool_iterations", Text: "capped"},
	}
	js := encodeSteps(steps)
	var back []TurnStep
	if err := json.Unmarshal([]byte(js), &back); err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	if back[0].Kind != StepTodo || len(back[0].Todos) != 1 {
		t.Fatalf("todo step lost in round-trip: %+v", back[0])
	}
	if back[1].Kind != StepRecovery || back[1].Reason != "max_tool_iterations" {
		t.Fatalf("recovery step lost in round-trip: %+v", back[1])
	}
}

func TestEncodeStepsErrorSteerTombstoneRoundTrip(t *testing.T) {
	steps := []TurnStep{
		{Kind: StepError, Reason: "provider_error", Text: "boom", IsError: true},
		{Kind: StepSteer, Text: "use TypeScript"},
		{Kind: StepToolDelta, ID: "t1", Tool: "shell", Output: "chunk"},
		{Kind: StepTombstone, Ref: "t1"},
	}
	var back []TurnStep
	if err := json.Unmarshal([]byte(encodeSteps(steps)), &back); err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	if back[0].Kind != StepError || back[0].Reason != "provider_error" || !back[0].IsError {
		t.Fatalf("error step lost: %+v", back[0])
	}
	if back[1].Kind != StepSteer || back[1].Text != "use TypeScript" {
		t.Fatalf("steer step lost: %+v", back[1])
	}
	if back[2].Kind != StepToolDelta || back[2].ID != "t1" {
		t.Fatalf("tool_delta step lost: %+v", back[2])
	}
	if back[3].Kind != StepTombstone || back[3].Ref != "t1" {
		t.Fatalf("tombstone step lost: %+v", back[3])
	}
}
