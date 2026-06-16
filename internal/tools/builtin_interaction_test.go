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
