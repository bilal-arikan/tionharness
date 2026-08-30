package tools

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
)

// recordingNavigateSink captures the last spec passed to Navigate for assertions.
type recordingNavigateSink struct {
	called bool
	spec   NavigateSpec
}

func (s *recordingNavigateSink) Navigate(_ context.Context, spec NavigateSpec) error {
	s.called = true
	s.spec = spec
	return nil
}

func TestFocusViewDispatchesToSink(t *testing.T) {
	sink := &recordingNavigateSink{}
	ctx := WithNavigate(context.Background(), sink)
	out, err := NewFocusViewTool().Call(ctx, json.RawMessage(`{"view":"artifacts"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "navigated the UI to artifacts" {
		t.Fatalf("got %q", out)
	}
	if !sink.called || sink.spec.View != "artifacts" {
		t.Fatalf("spec mismatch: %+v", sink.spec)
	}
}

func TestFocusViewPassesEntityHints(t *testing.T) {
	sink := &recordingNavigateSink{}
	ctx := WithNavigate(context.Background(), sink)
	if _, err := NewFocusViewTool().Call(ctx, json.RawMessage(`{"view":"chat","sessionId":"SES9"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.spec.SessionID != "SES9" {
		t.Fatalf("sessionId not passed: %+v", sink.spec)
	}
}

func TestFocusViewRejectsUnknownView(t *testing.T) {
	sink := &recordingNavigateSink{}
	ctx := WithNavigate(context.Background(), sink)
	if _, err := NewFocusViewTool().Call(ctx, json.RawMessage(`{"view":"nope"}`)); err == nil {
		t.Fatal("expected error for unknown view")
	}
	if sink.called {
		t.Fatal("sink should not be called for an invalid view")
	}
}

func TestFocusViewSchemaMatchesNavigationReshuffle(t *testing.T) {
	var schema struct {
		Properties struct {
			View struct {
				Enum []string `json:"enum"`
			} `json:"view"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(NewFocusViewTool().Def().InputSchema, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if !slices.Contains(schema.Properties.View.Enum, "prompts") {
		t.Fatal("focus_view schema must expose top-level prompts view")
	}
	if slices.Contains(schema.Properties.View.Enum, "logs") {
		t.Fatal("focus_view schema must not expose retired top-level logs view")
	}
}

func TestFocusViewRequiresView(t *testing.T) {
	ctx := WithNavigate(context.Background(), &recordingNavigateSink{})
	if _, err := NewFocusViewTool().Call(ctx, json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for missing view")
	}
}

func TestFocusViewGracefulWithoutSink(t *testing.T) {
	out, err := NewFocusViewTool().Call(context.Background(), json.RawMessage(`{"view":"board"}`))
	if err != nil {
		t.Fatalf("expected graceful no-op, got error: %v", err)
	}
	if out == "navigated the UI to board" {
		t.Fatal("should not report navigated when no sink is available")
	}
}
