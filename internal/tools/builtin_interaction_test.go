package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTodoWriteValid(t *testing.T) {
	tool := NewTodoWriteTool()
	in := json.RawMessage(`{"todos":[
		{"content":"design","status":"completed"},
		{"content":"build","status":"in_progress"},
		{"content":"test","status":"pending"}
	]}`)
	out, err := tool.Call(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "3 total") || !strings.Contains(out, "1 completed") {
		t.Fatalf("unexpected summary: %q", out)
	}
}

func TestTodoWriteRejectsBadStatus(t *testing.T) {
	tool := NewTodoWriteTool()
	in := json.RawMessage(`{"todos":[{"content":"x","status":"wip"}]}`)
	if _, err := tool.Call(context.Background(), in); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestTodoWriteRejectsEmpty(t *testing.T) {
	tool := NewTodoWriteTool()
	if _, err := tool.Call(context.Background(), json.RawMessage(`{"todos":[]}`)); err == nil {
		t.Fatal("expected error for empty list")
	}
}

func TestAskUserWithoutAsker(t *testing.T) {
	tool := NewAskUserTool()
	in := json.RawMessage(`{"question":"proceed?"}`)
	if _, err := tool.Call(context.Background(), in); err == nil {
		t.Fatal("expected error when no asker is wired into context")
	}
}

func TestAskUserWithAsker(t *testing.T) {
	tool := NewAskUserTool()
	var gotQ string
	var gotOpts []string
	ctx := WithAsker(context.Background(), func(_ context.Context, q string, opts []string) (string, error) {
		gotQ, gotOpts = q, opts
		return "yes", nil
	})
	out, err := tool.Call(ctx, json.RawMessage(`{"question":"proceed?","options":["yes","no"]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "yes" {
		t.Fatalf("want answer %q, got %q", "yes", out)
	}
	if gotQ != "proceed?" || len(gotOpts) != 2 {
		t.Fatalf("asker received wrong args: q=%q opts=%v", gotQ, gotOpts)
	}
}

func TestAskUserRequiresQuestion(t *testing.T) {
	tool := NewAskUserTool()
	ctx := WithAsker(context.Background(), func(context.Context, string, []string) (string, error) {
		return "x", nil
	})
	if _, err := tool.Call(ctx, json.RawMessage(`{"question":"  "}`)); err == nil {
		t.Fatal("expected error for blank question")
	}
}

// TestAskUserOptionsAsString verifies that the tool accepts options encoded as a
// plain JSON string (model emit error) instead of the documented array form.
func TestAskUserOptionsAsString(t *testing.T) {
	tool := NewAskUserTool()
	var gotOpts []string
	ctx := WithAsker(context.Background(), func(_ context.Context, _ string, opts []string) (string, error) {
		gotOpts = opts
		return "yes", nil
	})
	_, err := tool.Call(ctx, json.RawMessage(`{"question":"proceed?","options":"yes"}`))
	if err != nil {
		t.Fatalf("unexpected error with string options: %v", err)
	}
	if len(gotOpts) != 1 || gotOpts[0] != "yes" {
		t.Fatalf("expected [yes], got %v", gotOpts)
	}
}

// TestAskUserAutonomousMalformedPayload confirms that a malformed payload in an
// autonomous session (no asker) returns the graceful no-asker message rather than
// a JSON parse error.
func TestAskUserAutonomousMalformedPayload(t *testing.T) {
	tool := NewAskUserTool()
	// Bad payload: options is an array but if struct were string this would panic.
	_, err := tool.Call(context.Background(), json.RawMessage(`{"question":"q","options":["a","b"]}`))
	if err == nil {
		t.Fatal("expected error when no asker")
	}
	if !strings.Contains(err.Error(), "interactive chat") {
		t.Fatalf("expected graceful no-asker message, got: %v", err)
	}
}

func TestRequestConfirmationAutonomous(t *testing.T) {
	tool := NewRequestConfirmationTool()
	_, err := tool.Call(context.Background(), json.RawMessage(`{"question":"delete?"}`))
	if err == nil {
		t.Fatal("expected error in autonomous context")
	}
	if !strings.Contains(err.Error(), "interactive chat") {
		t.Fatalf("expected graceful no-asker message, got: %v", err)
	}
}
