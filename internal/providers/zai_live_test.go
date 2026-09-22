package providers

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveZAI is a REAL end-to-end check of the "zai" kind against Z.ai's
// Anthropic-compatible endpoint: it spends actual tokens, so it is gated behind
// ZAI_LIVE_KEY and skipped in normal runs. It targets the GLM-5.3 family, whose
// reasoning cannot be disabled: every request here carries output_config.effort
// (low at a zero budget) and never thinking {type:"disabled"}, which Z.ai
// rejects on these models. ZAI_LIVE_MODEL overrides the model (default
// glm-5.3-flash, the cheapest tier).
//
//	ZAI_LIVE_KEY=... go test ./internal/providers/ -run TestLiveZAI -v
func TestLiveZAI(t *testing.T) {
	key := os.Getenv("ZAI_LIVE_KEY")
	if key == "" {
		t.Skip("set ZAI_LIVE_KEY=... to run the live Z.ai test")
	}
	model := os.Getenv("ZAI_LIVE_MODEL")
	if model == "" {
		model = "glm-5.3-flash"
	}

	r := NewRegistry()
	r.SetInstances([]Instance{instanceOf("zai", map[string]string{FieldKeyAPIKey: key})})
	p, err := r.Get("zai")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// (a) Plain turn at a zero budget → effort low, no thinking field.
	resp, err := p.Complete(ctx, Request{
		Model:     model,
		MaxTokens: 2048,
		Messages:  []Message{{Role: RoleUser, Text: "Reply with exactly the single word: PONG"}},
	})
	if err != nil {
		t.Fatalf("plain Complete: %v", err)
	}
	if !strings.Contains(strings.ToUpper(resp.Text), "PONG") {
		t.Errorf("text = %q, want it to contain PONG", resp.Text)
	}
	t.Logf("plain: text=%q model=%s usage(in=%d out=%d cacheR=%d) trace=%d",
		strings.TrimSpace(resp.Text), resp.Model, resp.Usage.InputTokens, resp.Usage.OutputTokens,
		resp.Usage.CacheReadTokens, len(resp.Trace))

	// (b) Two-step tool loop, as the native loop sends it (zero budget).
	schema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)
	tools := []ToolDef{{Name: "get_weather", Description: "Get the current weather for a city.", InputSchema: schema}}
	msgs := []Message{{Role: RoleUser, Text: "Use the get_weather tool to check the weather in Istanbul. You MUST call the tool."}}
	tr, err := p.Complete(ctx, Request{Model: model, MaxTokens: 2048, Tools: tools, Messages: msgs})
	if err != nil {
		t.Fatalf("tool Complete: %v", err)
	}
	if len(tr.ToolCalls) == 0 {
		t.Fatalf("expected a tool_use call, got none (stop=%s text=%q)", tr.StopReason, tr.Text)
	}
	call := tr.ToolCalls[0]
	msgs = append(msgs,
		Message{Role: RoleAssistant, Text: tr.Text, ToolCalls: tr.ToolCalls},
		Message{Role: RoleUser, ToolResults: []ToolResult{{CallID: call.ID, Content: `{"city":"Istanbul","tempC":21,"sky":"clear"}`}}},
	)
	fin, err := p.Complete(ctx, Request{Model: model, MaxTokens: 2048, Tools: tools, Messages: msgs})
	if err != nil {
		t.Fatalf("tool follow-up Complete: %v", err)
	}
	if !strings.Contains(fin.Text, "21") {
		t.Errorf("follow-up text = %q, want it to use the tool result (21)", fin.Text)
	}
	t.Logf("follow-up: text=%q stop=%s", strings.TrimSpace(fin.Text), fin.StopReason)
}
