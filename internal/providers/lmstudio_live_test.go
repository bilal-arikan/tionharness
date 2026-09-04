package providers

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveLMStudio is a REAL end-to-end check of the lmstudio kind against a
// running LM Studio (or Bionic) local server. It spends no money but it does
// need a loaded model, so it is gated behind LMSTUDIO_LIVE_MODEL and skipped in
// normal runs.
//
// It covers the three things that distinguish a local turn from a hosted one:
// the request carries no API key, the whole path runs through the Registry (so
// resolve() wiring is exercised), and the model must actually emit a tool_use
// call — the agent loop is tool-driven, so a local model that cannot call tools
// cannot drive an agent regardless of how well it writes prose.
//
//	LMSTUDIO_LIVE_MODEL=qwen3.5-9b go test ./internal/providers/ -run TestLiveLMStudio -v
//
// Set LMSTUDIO_LIVE_URL to point at a server other than the default port.
func TestLiveLMStudio(t *testing.T) {
	model := os.Getenv("LMSTUDIO_LIVE_MODEL")
	if model == "" {
		t.Skip("set LMSTUDIO_LIVE_MODEL=<loaded model id> to run the live LM Studio test")
	}
	base := os.Getenv("LMSTUDIO_LIVE_URL")

	cfg := map[string]string{}
	if base != "" {
		cfg[FieldKeyBaseURL] = base
	}
	r := NewRegistry()
	r.SetInstances([]Instance{instanceOf("lmstudio", cfg)})

	// No key is set anywhere above: a local instance must be available on its
	// configuration alone.
	if !r.Available("lmstudio") {
		t.Fatal("lmstudio not available without a key")
	}
	p, err := r.Get("lmstudio")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Local inference is far slower than a hosted endpoint; prompt processing
	// alone can take minutes on a cold model.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// (a) Plain completion. MaxTokens is deliberately generous: the open-weight
	// models people run locally are mostly reasoning models (Qwen3, DeepSeek-R1
	// distills) that spend their output budget on a thinking block BEFORE the
	// visible answer. A tight cap does not truncate the answer, it produces an
	// EMPTY one with finish_reason "length" — the reasoning alone exhausts the
	// budget. That failure looks identical to a broken transport, so the budget
	// here is well above what the visible answer needs.
	resp, err := p.Complete(ctx, Request{
		Model:     model,
		MaxTokens: 2048,
		Messages: []Message{{
			Role: RoleUser,
			Text: "Reply with exactly the single word: PONG",
		}},
	})
	if err != nil {
		t.Fatalf("plain Complete: %v", err)
	}
	if !strings.Contains(strings.ToUpper(resp.Text), "PONG") {
		t.Errorf("text = %q, want it to contain PONG", resp.Text)
	}
	t.Logf("plain: text=%q model=%s usage(in=%d out=%d)",
		strings.TrimSpace(resp.Text), resp.Model, resp.Usage.InputTokens, resp.Usage.OutputTokens)

	// (b) Tool turn. This is the load-bearing assertion for local models: a
	// small local model often answers in prose instead of emitting tool_calls,
	// and that failure mode is invisible until an agent turn silently does
	// nothing.
	schema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)
	tr, err := p.Complete(ctx, Request{
		Model:     model,
		MaxTokens: 2048,
		Tools:     []ToolDef{{Name: "get_weather", Description: "Get the current weather for a city.", InputSchema: schema}},
		Messages: []Message{{
			Role: RoleUser,
			Text: "Use the get_weather tool to check the weather in Istanbul. You MUST call the tool.",
		}},
	})
	if err != nil {
		t.Fatalf("tool Complete: %v", err)
	}
	if len(tr.ToolCalls) == 0 {
		t.Errorf("expected a tool_use call, got none (stop=%s text=%q)", tr.StopReason, tr.Text)
	} else {
		t.Logf("tool: call=%s input=%s stop=%s", tr.ToolCalls[0].Name, string(tr.ToolCalls[0].Input), tr.StopReason)
	}
}
