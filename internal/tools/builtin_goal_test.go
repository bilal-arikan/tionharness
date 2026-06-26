package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeGoalSink is an in-memory GoalSink for tests.
type fakeGoalSink struct {
	state GoalState
}

func (s *fakeGoalSink) Goal(_ context.Context) (GoalState, error) { return s.state, nil }
func (s *fakeGoalSink) SetGoal(_ context.Context, text string, done bool) error {
	s.state = GoalState{Text: text, Done: done}
	return nil
}

func TestSetSessionGoalWrites(t *testing.T) {
	sink := &fakeGoalSink{}
	ctx := WithGoal(context.Background(), sink)
	out, err := NewSetSessionGoalTool().Call(ctx, json.RawMessage(`{"goal":"Ship v1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.state.Text != "Ship v1" || sink.state.Done {
		t.Fatalf("goal not set: %+v", sink.state)
	}
	if !strings.Contains(out, "Ship v1") {
		t.Fatalf("output missing goal: %q", out)
	}
}

func TestSetSessionGoalFlagsReplacement(t *testing.T) {
	sink := &fakeGoalSink{state: GoalState{Text: "Old goal"}}
	ctx := WithGoal(context.Background(), sink)
	out, err := NewSetSessionGoalTool().Call(ctx, json.RawMessage(`{"goal":"New goal"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "replaced the previous goal") || !strings.Contains(out, "Old goal") {
		t.Fatalf("replacement not flagged: %q", out)
	}
}

func TestSetSessionGoalRequiresText(t *testing.T) {
	ctx := WithGoal(context.Background(), &fakeGoalSink{})
	if _, err := NewSetSessionGoalTool().Call(ctx, json.RawMessage(`{"goal":"   "}`)); err == nil {
		t.Fatal("expected error for empty goal")
	}
}

func TestSetSessionGoalGracefulWithoutSink(t *testing.T) {
	out, err := NewSetSessionGoalTool().Call(context.Background(), json.RawMessage(`{"goal":"x"}`))
	if err != nil {
		t.Fatalf("expected graceful no-op, got: %v", err)
	}
	if strings.HasPrefix(out, "session goal set") {
		t.Fatal("should not report set without a sink")
	}
}

func TestCompleteGoalMarksDone(t *testing.T) {
	sink := &fakeGoalSink{state: GoalState{Text: "Ship v1"}}
	ctx := WithGoal(context.Background(), sink)
	if _, err := NewCompleteGoalTool().Call(ctx, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sink.state.Done || sink.state.Text != "Ship v1" {
		t.Fatalf("goal not completed (text must be kept): %+v", sink.state)
	}
}

func TestCompleteGoalNoGoal(t *testing.T) {
	ctx := WithGoal(context.Background(), &fakeGoalSink{})
	out, err := NewCompleteGoalTool().Call(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "nothing to complete") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCompleteGoalAlreadyDone(t *testing.T) {
	sink := &fakeGoalSink{state: GoalState{Text: "Ship v1", Done: true}}
	ctx := WithGoal(context.Background(), sink)
	out, err := NewCompleteGoalTool().Call(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "already marked complete") {
		t.Fatalf("unexpected output: %q", out)
	}
}
