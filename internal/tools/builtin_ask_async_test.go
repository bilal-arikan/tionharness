package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAsyncAskReturnsPendingWithoutWaiting(t *testing.T) {
	if Classify("ask_user_async") != RiskRead {
		t.Fatal("clarification must remain available in read-only mode")
	}
	var got []AskQuestion
	ctx := WithAsyncInput(context.Background(), &AsyncInput{Ask: func(_ context.Context, questions []AskQuestion) (string, error) {
		got = questions
		return "SAK1", nil
	}})
	result, err := NewAskUserAsyncTool().Call(ctx, json.RawMessage(`{"questions":[{"question":"Audience?","options":[{"label":"Engineers"}]},{"question":"Format?"}]}`))
	if err != nil || !strings.Contains(result, `"status":"pending"`) || !strings.Contains(result, "SAK1") {
		t.Fatalf("result=%s error=%v", result, err)
	}
	if len(got) != 2 || got[0].Options[0] != "Engineers" {
		t.Fatalf("questions=%+v", got)
	}
}

func TestAsyncAskRejectsHeadlessAndInvalidInput(t *testing.T) {
	called := false
	ctx := WithAsyncInput(context.Background(), &AsyncInput{Ask: func(context.Context, []AskQuestion) (string, error) {
		called = true
		return "SAK1", nil
	}})
	for _, raw := range []string{`{`, `{}`, `{"question":"  "}`} {
		if _, err := NewAskUserAsyncTool().Call(ctx, json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if _, err := NewAskUserAsyncTool().Call(WithAutonomous(ctx), json.RawMessage(`{"question":"q"}`)); err == nil {
		t.Fatal("autonomous prompt accepted")
	}
	if called {
		t.Fatal("invalid question was published")
	}
}
