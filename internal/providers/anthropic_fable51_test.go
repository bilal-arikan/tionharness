package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSupportsThinkingBinding pins the preserved-thinking class: Fable/Mythos 5.1
// only — Fable 5, Opus 5 and everything older do not get the binding controls.
func TestSupportsThinkingBinding(t *testing.T) {
	for _, m := range []string{"claude-fable-5-1", "Claude-Fable-5.1", "claude-mythos-5-1"} {
		if !SupportsThinkingBinding(m) {
			t.Errorf("%q must support thinking binding", m)
		}
	}
	for _, m := range []string{"claude-fable-5", "claude-opus-5", "claude-opus-4-8", "claude-sonnet-5", "fable", ""} {
		if SupportsThinkingBinding(m) {
			t.Errorf("%q must NOT get the binding controls", m)
		}
	}
}

// TestSupportsSystemInMessages_Fable51 extends the native mid-conversation system
// channel to the classes the API documents: Opus 4.8, Opus 5 and Fable/Mythos 5.x —
// not Sonnet 5.
func TestSupportsSystemInMessages_Fable51(t *testing.T) {
	for _, m := range []string{"claude-fable-5-1", "claude-fable-5", "claude-opus-5", "claude-opus-4-8", "claude-mythos-5-1"} {
		if !SupportsSystemInMessages(m) {
			t.Errorf("%q must accept role:system messages", m)
		}
	}
	for _, m := range []string{"claude-sonnet-5", "claude-sonnet-4-6", "claude-haiku-4-5-20251001", "claude-opus-4-7"} {
		if SupportsSystemInMessages(m) {
			t.Errorf("%q must fold system messages instead", m)
		}
	}
}

// TestApplyThinkingBinding: the binding is attached only on the 5.1 class; an
// omitted thinking field (always-on model, thinking "off") gets an adaptive
// carrier; other models' params are untouched and no beta is requested.
func TestApplyThinkingBinding(t *testing.T) {
	p, beta := applyThinkingBinding("claude-fable-5-1", nil)
	if !beta || p == nil || p.Type != "adaptive" || p.BlockBinding == nil || p.BlockBinding.PrefixMismatchBehavior != "drop_block" {
		t.Fatalf("fable-5-1 with omitted thinking: %+v beta=%v", p, beta)
	}
	p, beta = applyThinkingBinding("claude-fable-5-1", &thinkingParam{Type: "adaptive", Display: "summarized"})
	if !beta || p.Display != "summarized" || p.BlockBinding == nil {
		t.Fatalf("existing adaptive param must keep its fields and gain the binding: %+v", p)
	}
	b, _ := json.Marshal(p)
	if !strings.Contains(string(b), `"block_binding":{"prefix_mismatch_behavior":"drop_block"}`) {
		t.Errorf("wire shape: %s", b)
	}
	p, beta = applyThinkingBinding("claude-fable-5", nil)
	if beta || p != nil {
		t.Fatalf("fable-5 must be untouched: %+v beta=%v", p, beta)
	}
	b, _ = json.Marshal(thinkingParam{Type: "adaptive"})
	if strings.Contains(string(b), "block_binding") {
		t.Errorf("nil binding must be omitted from the wire: %s", b)
	}
}

// TestComplete_Fable51BindingAndTransformations drives a full Complete against a
// fake API: the request carries the beta header + block_binding, and the
// response's input_transformations reach Response.InputTransformations.
func TestComplete_Fable51BindingAndTransformations(t *testing.T) {
	var gotBeta string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBeta = r.Header.Get("anthropic-beta")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"model":"claude-fable-5-1",
			"usage":{"input_tokens":10,"output_tokens":2},
			"input_transformations":[
				{"type":"thinking_dropped","path":"messages.1.content.0","reason":"prefix_binding_mismatch"}
			]
		}`))
	}))
	defer srv.Close()

	a := NewAnthropic("test-key")
	a.baseURL = srv.URL
	resp, err := a.Complete(context.Background(), Request{
		Model:    "claude-fable-5-1",
		Messages: []Message{{Role: RoleUser, Text: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBeta, betaThinkingBinding) {
		t.Errorf("beta header = %q, want %s", gotBeta, betaThinkingBinding)
	}
	th, _ := gotBody["thinking"].(map[string]any)
	bb, _ := th["block_binding"].(map[string]any)
	if th["type"] != "adaptive" || bb["prefix_mismatch_behavior"] != "drop_block" {
		t.Errorf("thinking param on the wire = %v", gotBody["thinking"])
	}
	if len(resp.InputTransformations) != 1 || resp.InputTransformations[0].Reason != "prefix_binding_mismatch" || resp.InputTransformations[0].Path != "messages.1.content.0" {
		t.Errorf("input_transformations not surfaced: %+v", resp.InputTransformations)
	}

	// Fable 5: no binding, no beta, no transformations field expected.
	gotBeta, gotBody = "", nil
	if _, err := a.Complete(context.Background(), Request{Model: "claude-fable-5", Messages: []Message{{Role: RoleUser, Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotBeta, betaThinkingBinding) || gotBody["thinking"] != nil {
		t.Errorf("fable-5 must not carry the binding: beta=%q thinking=%v", gotBeta, gotBody["thinking"])
	}
}

// TestToAnthropicMessages_DynamicAnchorsOnOpeningUserMessage: inside a tool loop
// the volatile dynamic block rides the turn's OPENING user message, not the
// trailing tool_result batch — so the request stays append-only across the
// loop's iterations (preserved thinking) and the in-turn prefix stays byte-stable.
func TestToAnthropicMessages_DynamicAnchorsOnOpeningUserMessage(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Text: "earlier"},
		{Role: RoleAssistant, Text: "sure"},
		{Role: RoleUser, Text: "do the task"},
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "t1", Name: "Read", Input: json.RawMessage(`{}`)}}},
		{Role: RoleUser, ToolResults: []ToolResult{{CallID: "t1", Content: "file body"}}},
	}
	got := toAnthropicMessages(msgs, true, "NOW + lessons", "claude-fable-5-1")
	opening := got[2].Content
	if len(opening) != 2 || opening[0].Text != "do the task" || opening[1].Text != "NOW + lessons" {
		t.Fatalf("dynamic must trail the opening user message of the turn, got %+v", opening)
	}
	tail := got[len(got)-1].Content
	for _, b := range tail {
		if b.Type == "text" && b.Text == "NOW + lessons" {
			t.Fatalf("dynamic must NOT ride the tool_result batch: %+v", tail)
		}
	}
	if tail[len(tail)-1].Type != "tool_result" || tail[len(tail)-1].CacheControl == nil {
		t.Errorf("rolling breakpoint must still sit on the trailing tool_result: %+v", tail)
	}
	// Second iteration of the same turn: the opening message is byte-identical
	// (append-only), only the new round is added.
	more := append(append([]Message(nil), msgs...),
		Message{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "t2", Name: "Grep", Input: json.RawMessage(`{}`)}}},
		Message{Role: RoleUser, ToolResults: []ToolResult{{CallID: "t2", Content: "hits"}}})
	got2 := toAnthropicMessages(more, true, "NOW + lessons", "claude-fable-5-1")
	b1, _ := json.Marshal(got[2].Content[:2])
	b2, _ := json.Marshal(got2[2].Content[:2])
	if string(b1) != string(b2) {
		t.Errorf("opening user message must be byte-stable across iterations:\n%s\n%s", b1, b2)
	}
}

// TestPriceFor_Fable51CacheRead pins the launch pricing: same $10/$50 as Fable 5,
// cache reads at 0.025× ($0.25/MTok).
func TestPriceFor_Fable51CacheRead(t *testing.T) {
	p, ok := PriceFor("anthropic", "claude-fable-5-1")
	if !ok {
		t.Fatal("fable-5-1 price missing")
	}
	if got := p.Cost(1_000_000, 1_000_000); !approx(got, 60) {
		t.Errorf("plain cost = %v, want 60", got)
	}
	if got := p.CostDetailed(0, 0, 1_000_000, 0); !approx(got, 0.25) {
		t.Errorf("cache-read cost = %v, want 0.25", got)
	}
	if _, ok := PriceFor("openrouter", "anthropic/claude-fable-5.1"); !ok {
		t.Error("openrouter fable-5.1 price missing")
	}
}
