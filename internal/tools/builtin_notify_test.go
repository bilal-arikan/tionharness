package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// recordingNotifySink captures the last spec passed to Notify for assertions.
type recordingNotifySink struct {
	called bool
	spec   NotifySpec
}

func (s *recordingNotifySink) Notify(_ context.Context, spec NotifySpec) error {
	s.called = true
	s.spec = spec
	return nil
}

func TestNotifyDispatchesToSink(t *testing.T) {
	sink := &recordingNotifySink{}
	ctx := WithNotify(context.Background(), sink)
	out, err := NewNotifyTool().Call(ctx, json.RawMessage(`{"title":"Done","body":"build finished","level":"success"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "notification sent" {
		t.Fatalf("got %q, want confirmation", out)
	}
	if !sink.called {
		t.Fatal("sink was not called")
	}
	if sink.spec.Title != "Done" || sink.spec.Body != "build finished" || sink.spec.Level != "success" {
		t.Fatalf("spec mismatch: %+v", sink.spec)
	}
}

func TestNotifyDefaultsLevel(t *testing.T) {
	sink := &recordingNotifySink{}
	ctx := WithNotify(context.Background(), sink)
	if _, err := NewNotifyTool().Call(ctx, json.RawMessage(`{"title":"Hi"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.spec.Level != "info" {
		t.Fatalf("level = %q, want info default", sink.spec.Level)
	}
}

func TestNotifyNormalisesUnknownLevel(t *testing.T) {
	sink := &recordingNotifySink{}
	ctx := WithNotify(context.Background(), sink)
	if _, err := NewNotifyTool().Call(ctx, json.RawMessage(`{"title":"Hi","level":"critical"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sink.spec.Level != "info" {
		t.Fatalf("unknown level should fall back to info, got %q", sink.spec.Level)
	}
}

func TestNotifyRequiresTitle(t *testing.T) {
	sink := &recordingNotifySink{}
	ctx := WithNotify(context.Background(), sink)
	if _, err := NewNotifyTool().Call(ctx, json.RawMessage(`{"body":"no title"}`)); err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestNotifyGracefulWithoutSink(t *testing.T) {
	// No sink wired (autonomous run): must not error so the turn proceeds.
	out, err := NewNotifyTool().Call(context.Background(), json.RawMessage(`{"title":"Hi"}`))
	if err != nil {
		t.Fatalf("expected graceful no-op, got error: %v", err)
	}
	if out == "notification sent" {
		t.Fatal("should not report sent when no sink is available")
	}
}
