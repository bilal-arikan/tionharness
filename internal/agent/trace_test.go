package agent

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/providers"
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

func TestTraceStepCompactionMetadataMapping(t *testing.T) {
	step := traceStepToTurnStep(providers.TraceStep{
		Kind:          "compaction",
		Text:          "native compact",
		Source:        "cli-native",
		Provider:      "codex-cli",
		SessionAction: "native-compact",
		FoldedMsgs:    7,
		BeforeTokens:  1000,
		AfterTokens:   250,
		Trigger:       "auto",
	})
	if step.Kind != StepCompaction || step.Source != "cli-native" || step.Provider != "codex-cli" || step.SessionAction != "native-compact" {
		t.Fatalf("compaction provenance lost: %+v", step)
	}
	if step.FoldedMsgs != 7 || step.BeforeTokens != 1000 || step.AfterTokens != 250 || step.Trigger != "auto" {
		t.Fatalf("compaction figures lost: %+v", step)
	}
}

func TestTraceStepCollabMetadataMapping(t *testing.T) {
	step := traceStepToTurnStep(providers.TraceStep{
		Kind: "tool", Tool: "collab_tool_call", Operation: "spawn_agent",
		Target: []string{"thread-a", "thread-b"}, Status: "completed",
		DurationMs: 42, Summary: "spawn_agent · receivers: thread-a, thread-b · completed",
	})
	if step.Operation != "spawn_agent" || len(step.Target) != 2 || step.Status != "completed" || step.DurationMs != 42 {
		t.Fatalf("collab metadata lost: %+v", step)
	}
	if step.Summary == "" {
		t.Fatal("safe collab summary lost")
	}
}

func TestCollabDebugEventUsesSafeMetadata(t *testing.T) {
	event := collabDebugEvent("agent-1", providers.TraceStep{
		Operation: "send_message", Summary: "send_message · receivers: thread-a · completed",
		DurationMs: 75, IsError: false,
	})
	if event.Name != "send_message" || event.Detail != "send_message · receivers: thread-a · completed" || event.DurMs != 75 {
		t.Fatalf("debug event = %+v", event)
	}
	fallback := collabDebugEvent("agent-1", providers.TraceStep{})
	if fallback.Name != "collab_tool_call" || fallback.Detail != "collab_tool_call" {
		t.Fatalf("fallback event = %+v", fallback)
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
