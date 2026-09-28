package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

func TestAsyncAskContinuesAndReceivesReplyAtNextBoundary(t *testing.T) {
	rt := loopRuntime(t)
	posted, answered, delivered := false, false, false
	input := &tools.AsyncInput{
		Ask: func(context.Context, []tools.AskQuestion) (string, error) { posted = true; return "SAK1", nil },
		Drain: func() []string {
			if !answered || delivered {
				return nil
			}
			delivered = true
			return []string{"[User reply to ask_user_async request_id=SAK1] Engineers"}
		},
	}
	fp := &fakeProvider{script: []scriptedResp{
		{stop: providers.StopToolUse, toolCalls: []providers.ToolCall{{ID: "q1", Name: "ask_user_async", Input: json.RawMessage(`{"question":"Audience?"}`)}}},
		{stop: providers.StopToolUse, toolCalls: []providers.ToolCall{{ID: "work1", Name: "todo_write", Input: json.RawMessage(`{"todos":[]}`)}}},
		{stop: providers.StopEndTurn, text: "Tailored for engineers"},
	}, onRequest: func(i int, req providers.Request) {
		if i == 1 {
			if !posted || delivered {
				t.Fatal("independent work must run before an answer")
			}
			answered = true
		}
		if i == 2 {
			found := false
			for _, msg := range req.Messages {
				if strings.Contains(msg.Text, "request_id=SAK1") {
					found = true
				}
			}
			if !found {
				t.Fatal("answer not injected before next model request")
			}
		}
	}}
	ctx := tools.WithAsyncInput(WithDurableAsk(context.Background()), input)
	_, _, err := rt.CompleteWithToolsStream(ctx, db.Agent{ID: "a1", Model: "m", MCPEnabled: true}, fp,
		providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "Prepare report"}}}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fp.calls != 3 || !delivered {
		t.Fatalf("calls=%d delivered=%v", fp.calls, delivered)
	}
}
