package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureOAI serves one canned Chat Completions reply and records each request
// body as a raw field map, so a test can assert which reasoning fields were sent
// (and which were omitted).
func captureOAI(t *testing.T, reply string) (*httptest.Server, *map[string]json.RawMessage) {
	t.Helper()
	got := map[string]json.RawMessage{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = map[string]json.RawMessage{}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

const oaiPlainReply = `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`

// TestDeepSeekThinkingSwitch pins the deepseek kind's wire: tool-loop turns (zero
// budget) switch reasoning OFF explicitly — DeepSeek reasons by default and
// would otherwise demand the earlier reasoning_content back on the next tool
// call — and a thinking turn sends the switch plus a low/high/max effort.
func TestDeepSeekThinkingSwitch(t *testing.T) {
	srv, got := captureOAI(t, oaiPlainReply)
	m := NewOpenAICompat("deepseek", "k", srv.URL, deepseekDefaultModel).WithCaps(true, "").WithThinkingToggle()
	schema := json.RawMessage(`{"type":"object"}`)

	if _, err := m.Complete(context.Background(), Request{
		Tools:    []ToolDef{{Name: "t", Description: "d", InputSchema: schema}},
		Messages: []Message{{Role: RoleUser, Text: "hi"}},
	}); err != nil {
		t.Fatal(err)
	}
	if string((*got)["thinking"]) != `{"type":"disabled"}` {
		t.Errorf("tool turn thinking = %s, want disabled", (*got)["thinking"])
	}
	if _, ok := (*got)["reasoning_effort"]; ok {
		t.Errorf("tool turn must not send reasoning_effort: %s", (*got)["reasoning_effort"])
	}
	if string((*got)["model"]) != `"deepseek-flash"` {
		t.Errorf("default model = %s, want deepseek-flash", (*got)["model"])
	}

	// medium (8192) folds onto high — DeepSeek has no medium level.
	if _, err := m.Complete(context.Background(), Request{
		Model:          "deepseek-v4-pro",
		ThinkingBudget: 8192,
		Messages:       []Message{{Role: RoleUser, Text: "hi"}},
	}); err != nil {
		t.Fatal(err)
	}
	if string((*got)["thinking"]) != `{"type":"enabled"}` || string((*got)["reasoning_effort"]) != `"high"` {
		t.Errorf("thinking turn = thinking %s effort %s, want enabled/high", (*got)["thinking"], (*got)["reasoning_effort"])
	}
}

// TestForcedThinkingEffortOnOpenAICompat: a GLM-5.3 model behind a
// reasoning-capable OpenAI-compatible endpoint (e.g. the zhipu-glm market pack)
// gets "low" at a zero budget instead of the server's slow "max" default, and
// never a "disabled" switch.
func TestForcedThinkingEffortOnOpenAICompat(t *testing.T) {
	srv, got := captureOAI(t, oaiPlainReply)
	m := NewOpenAICompat("zhipu-glm", "k", srv.URL, "glm-5.3").WithCaps(true, "")
	if _, err := m.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	if string((*got)["reasoning_effort"]) != `"low"` {
		t.Errorf("reasoning_effort = %s, want low", (*got)["reasoning_effort"])
	}
	if _, ok := (*got)["thinking"]; ok {
		t.Errorf("no toggle opted in, yet thinking was sent: %s", (*got)["thinking"])
	}

	// With the toggle a forced model stays "enabled" even at a zero budget.
	toggled := NewOpenAICompat("zhipu-glm", "k", srv.URL, "glm-5.3-flash").WithCaps(true, "").WithThinkingToggle()
	if _, err := toggled.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	if string((*got)["thinking"]) != `{"type":"enabled"}` {
		t.Errorf("forced model thinking = %s, want enabled", (*got)["thinking"])
	}
}

// TestOpenAICompatSendsNoReasoningFieldsByDefault: endpoints that did not opt in
// keep receiving neither field, whatever the model.
func TestOpenAICompatSendsNoReasoningFieldsByDefault(t *testing.T) {
	srv, got := captureOAI(t, oaiPlainReply)
	m := NewOpenAICompat("custom", "k", srv.URL, "glm-5.3")
	if _, err := m.Complete(context.Background(), Request{ThinkingBudget: 16384, Messages: []Message{{Role: RoleUser, Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"thinking", "reasoning_effort"} {
		if v, ok := (*got)[field]; ok {
			t.Errorf("%s sent without opt-in: %s", field, v)
		}
	}
}

// TestOpenAICompatReasoningContent: the separate reasoning channel lands in the
// thinking trace, never in the visible answer — for Complete and Stream alike.
func TestOpenAICompatReasoningContent(t *testing.T) {
	srv, _ := captureOAI(t, `{"choices":[{"message":{"content":"Answer","reasoning_content":"Let me reason."},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	resp, err := NewOpenAICompat("deepseek", "k", srv.URL, "deepseek-flash").Complete(context.Background(),
		Request{Messages: []Message{{Role: RoleUser, Text: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Answer" {
		t.Errorf("Text = %q, want Answer", resp.Text)
	}
	if len(resp.Trace) != 1 || resp.Trace[0].Kind != "thinking" || resp.Trace[0].Text != "Let me reason." {
		t.Errorf("Trace = %+v, want one thinking step", resp.Trace)
	}

	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"reasoning_content":"Let me "}}]}`,
		``,
		`data: {"choices":[{"delta":{"reasoning_content":"reason."}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"Answer"}}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	sse := sseServer(t, body)
	defer sse.Close()

	var thinking, text []string
	sresp, err := NewOpenAICompat("deepseek", "k", sse.URL, "deepseek-flash").Stream(context.Background(),
		Request{Messages: []Message{{Role: RoleUser, Text: "hi"}}},
		func(d StreamDelta) {
			if d.Kind == DeltaThinking {
				thinking = append(thinking, d.Text)
			} else {
				text = append(text, d.Text)
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(thinking, "") != "Let me reason." || strings.Join(text, "") != "Answer" {
		t.Errorf("deltas: thinking %v text %v", thinking, text)
	}
	if sresp.Text != "Answer" || len(sresp.Trace) != 1 || sresp.Trace[0].Text != "Let me reason." {
		t.Errorf("stream response = text %q trace %+v", sresp.Text, sresp.Trace)
	}
}
