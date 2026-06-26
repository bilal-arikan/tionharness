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

func TestSetSessionTitleWrites(t *testing.T) {
	sink := &fakeSessionSink{}
	ctx := WithSession(context.Background(), sink)
	out, err := NewSetSessionTitleTool().Call(ctx, json.RawMessage(`{"title":"Notify feature"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.title != "Notify feature" || !strings.Contains(out, "Notify feature") {
		t.Fatalf("title not set: %q / %q", sink.title, out)
	}
}

func TestSetSessionTitleRequiresText(t *testing.T) {
	ctx := WithSession(context.Background(), &fakeSessionSink{})
	if _, err := NewSetSessionTitleTool().Call(ctx, json.RawMessage(`{"title":"  "}`)); err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestSetWorkingDirValidatesPath(t *testing.T) {
	ctx := WithSession(context.Background(), &fakeSessionSink{})
	if _, err := NewSetWorkingDirTool().Call(ctx, json.RawMessage(`{"path":"/no/such/dir/swarmgo-xyz"}`)); err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestSetWorkingDirAcceptsExistingDir(t *testing.T) {
	dir := t.TempDir()
	sink := &fakeSessionSink{}
	ctx := WithSession(context.Background(), sink)
	if _, err := NewSetWorkingDirTool().Call(ctx, json.RawMessage(`{"path":`+strconv.Quote(dir)+`}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.workingDir != dir {
		t.Fatalf("workingDir not set: %q", sink.workingDir)
	}
}

func TestSetWorkingDirEmptyResets(t *testing.T) {
	sink := &fakeSessionSink{workingDir: "/old"}
	ctx := WithSession(context.Background(), sink)
	out, err := NewSetWorkingDirTool().Call(ctx, json.RawMessage(`{"path":""}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.workingDir != "" || !strings.Contains(out, "reset") {
		t.Fatalf("reset failed: %q / %q", sink.workingDir, out)
	}
}

func TestArchiveSession(t *testing.T) {
	sink := &fakeSessionSink{}
	ctx := WithSession(context.Background(), sink)
	if _, err := NewArchiveSessionTool().Call(ctx, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sink.archived {
		t.Fatal("session not archived")
	}
}

func TestSessionEditGracefulWithoutSink(t *testing.T) {
	out, err := NewSetSessionTitleTool().Call(context.Background(), json.RawMessage(`{"title":"x"}`))
	if err != nil {
		t.Fatalf("expected graceful no-op, got: %v", err)
	}
	if strings.HasPrefix(out, "session renamed") {
		t.Fatal("should not report renamed without a sink")
	}
}
