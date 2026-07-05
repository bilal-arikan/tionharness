package tools

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// fakeSessionSink is an in-memory SessionSink for tests (also satisfies GoalSink).
type fakeSessionSink struct {
	state      GoalState
	title      string
	workingDir string
	archived   bool
	tags       []string
}

func (s *fakeSessionSink) Goal(_ context.Context) (GoalState, error) { return s.state, nil }
func (s *fakeSessionSink) SetGoal(_ context.Context, text string, done bool) error {
	s.state = GoalState{Text: text, Done: done}
	return nil
}
func (s *fakeSessionSink) SetTitle(_ context.Context, title string) error { s.title = title; return nil }
func (s *fakeSessionSink) SetWorkingDir(_ context.Context, dir string) error {
	s.workingDir = dir
	return nil
}
func (s *fakeSessionSink) Archive(_ context.Context) error { s.archived = true; return nil }
func (s *fakeSessionSink) Tags(_ context.Context) ([]string, error) { return s.tags, nil }
func (s *fakeSessionSink) SetTags(_ context.Context, tags []string) error {
	s.tags = tags
	return nil
}

func TestUpdateSessionTitle(t *testing.T) {
	sink := &fakeSessionSink{}
	ctx := WithSession(context.Background(), sink)
	out, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"title":"Notify feature"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.title != "Notify feature" || !strings.Contains(out, "Notify feature") {
		t.Fatalf("title not set: %q / %q", sink.title, out)
	}
}

func TestUpdateSessionEmptyTitleRejected(t *testing.T) {
	ctx := WithSession(context.Background(), &fakeSessionSink{})
	if _, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"title":"  "}`)); err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestUpdateSessionGoalWritesAndFlagsReplacement(t *testing.T) {
	sink := &fakeSessionSink{state: GoalState{Text: "Old goal"}}
	ctx := WithSession(context.Background(), sink)
	out, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"goal":"New goal"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.state.Text != "New goal" || sink.state.Done {
		t.Fatalf("goal not set: %+v", sink.state)
	}
	if !strings.Contains(out, "replaced previous goal") || !strings.Contains(out, "Old goal") {
		t.Fatalf("replacement not flagged: %q", out)
	}
}

func TestUpdateSessionGoalDoneMarksExisting(t *testing.T) {
	sink := &fakeSessionSink{state: GoalState{Text: "Ship v1"}}
	ctx := WithSession(context.Background(), sink)
	if _, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"goal_done":true}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sink.state.Done || sink.state.Text != "Ship v1" {
		t.Fatalf("goal not completed (text must be kept): %+v", sink.state)
	}
}

func TestUpdateSessionSetAndCompleteGoalInOneCall(t *testing.T) {
	sink := &fakeSessionSink{}
	ctx := WithSession(context.Background(), sink)
	if _, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"goal":"Ship v1","goal_done":true}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sink.state.Done || sink.state.Text != "Ship v1" {
		t.Fatalf("goal should be set then marked done: %+v", sink.state)
	}
}

func TestUpdateSessionWorkingDirValidates(t *testing.T) {
	ctx := WithSession(context.Background(), &fakeSessionSink{})
	if _, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"working_dir":"/no/such/dir/tionswarm-xyz"}`)); err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestUpdateSessionWorkingDirAcceptsExisting(t *testing.T) {
	dir := t.TempDir()
	sink := &fakeSessionSink{}
	ctx := WithSession(context.Background(), sink)
	if _, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"working_dir":`+strconv.Quote(dir)+`}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.workingDir != dir {
		t.Fatalf("workingDir not set: %q", sink.workingDir)
	}
}

func TestUpdateSessionWorkingDirEmptyResets(t *testing.T) {
	sink := &fakeSessionSink{workingDir: "/old"}
	ctx := WithSession(context.Background(), sink)
	out, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"working_dir":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.workingDir != "" || !strings.Contains(out, "reset") {
		t.Fatalf("reset failed: %q / %q", sink.workingDir, out)
	}
}

func TestUpdateSessionTagsReplaceAndDelta(t *testing.T) {
	sink := &fakeSessionSink{tags: []string{"a", "b"}}
	ctx := WithSession(context.Background(), sink)
	// Incremental add/remove.
	if _, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"add":["c"],"remove":["a"]}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(sink.tags, ",") != "b,c" {
		t.Fatalf("tag delta wrong: %v", sink.tags)
	}
	// Full replacement.
	if _, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"tags":["x"]}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(sink.tags, ",") != "x" {
		t.Fatalf("tag replace wrong: %v", sink.tags)
	}
}

func TestUpdateSessionTagsReplaceAndDeltaConflict(t *testing.T) {
	ctx := WithSession(context.Background(), &fakeSessionSink{})
	if _, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"tags":["x"],"add":["y"]}`)); err == nil {
		t.Fatal("expected error when both tags and add/remove are given")
	}
}

func TestUpdateSessionArchive(t *testing.T) {
	sink := &fakeSessionSink{}
	ctx := WithSession(context.Background(), sink)
	if _, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"archive":true}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sink.archived {
		t.Fatal("session not archived")
	}
}

func TestUpdateSessionMultiFieldAndSummary(t *testing.T) {
	sink := &fakeSessionSink{}
	ctx := WithSession(context.Background(), sink)
	out, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{"title":"T","add":["loop"]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.title != "T" || strings.Join(sink.tags, ",") != "loop" {
		t.Fatalf("multi-field update failed: %q / %v", sink.title, sink.tags)
	}
	if !strings.Contains(out, "title set to") || !strings.Contains(out, "tags:") {
		t.Fatalf("summary missing fields: %q", out)
	}
}

func TestUpdateSessionGracefulWithoutSink(t *testing.T) {
	out, err := NewUpdateSessionTool().Call(context.Background(), json.RawMessage(`{"title":"x"}`))
	if err != nil {
		t.Fatalf("expected graceful no-op, got: %v", err)
	}
	if strings.HasPrefix(out, "session updated") {
		t.Fatal("should not report an update without a sink")
	}
}

func TestUpdateSessionNoFields(t *testing.T) {
	sink := &fakeSessionSink{}
	ctx := WithSession(context.Background(), sink)
	out, err := NewUpdateSessionTool().Call(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "nothing changed") {
		t.Fatalf("expected no-op message, got: %q", out)
	}
}
