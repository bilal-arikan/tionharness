package providers

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveDeepSeek is a REAL end-to-end check of both DeepSeek kinds against the
// live API: it spends actual tokens, so it is gated behind DEEPSEEK_LIVE_KEY and
// skipped in normal runs. It verifies (a) a plain completion returns text and
// usage, (b) a thinking turn (low effort) still answers, and (c) a full
// two-step tool loop — tool_use, then the tool_result follow-up — completes.
// Step (c) is the one that fails with a 400 when reasoning is left on in a tool
// loop without echoing reasoning_content, so it guards the explicit thinking
// switch. Exercised through the Registry so the resolve() wiring is covered too.
//
//	DEEPSEEK_LIVE_KEY=sk-... go test ./internal/providers/ -run TestLiveDeepSeek -v
func TestLiveDeepSeek(t *testing.T) {
	key := os.Getenv("DEEPSEEK_LIVE_KEY")
	if key == "" {
		t.Skip("set DEEPSEEK_LIVE_KEY=sk-... to run the live DeepSeek test")
	}

	r := NewRegistry()
	r.SetInstances([]Instance{
		instanceOf("deepseek", map[string]string{FieldKeyAPIKey: key}),
		instanceOf("deepseek-anthropic", map[string]string{FieldKeyAPIKey: key}),
	})

	// Both kinds reuse the DeepSeek key: "deepseek" (OpenAI-compatible) and
	// "deepseek-anthropic" (Anthropic Messages transport → native tool-use).
	for _, kind := range []string{"deepseek", "deepseek-anthropic"} {
		t.Run(kind, func(t *testing.T) {
			if !r.Available(kind) {
				t.Fatalf("%s: not available with key set", kind)
			}
			p, err := r.Get(kind)
			if err != nil {
				t.Fatalf("%s: Get: %v", kind, err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			// (a) Plain completion, reasoning off.
			resp, err := p.Complete(ctx, Request{
				Model:     deepseekDefaultModel,
				MaxTokens: 64,
				Messages: []Message{{
					Role: RoleUser,
					Text: "Reply with exactly the single word: PONG",
				}},
			})
			if err != nil {
				t.Fatalf("%s: plain Complete: %v", kind, err)
			}
			if !strings.Contains(strings.ToUpper(resp.Text), "PONG") {
				t.Errorf("%s: text = %q, want it to contain PONG", kind, resp.Text)
			}
			if resp.Usage.InputTokens == 0 && resp.Usage.OutputTokens == 0 {
				t.Errorf("%s: usage is empty (in=%d out=%d)", kind, resp.Usage.InputTokens, resp.Usage.OutputTokens)
			}
			t.Logf("%s plain: text=%q model=%s usage(in=%d out=%d cacheR=%d)",
				kind, strings.TrimSpace(resp.Text), resp.Model,
				resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.CacheReadTokens)

			// (b) Thinking turn at low effort: the answer must still arrive.
			th, err := p.Complete(ctx, Request{
				Model:          deepseekDefaultModel,
				MaxTokens:      4096,
				ThinkingBudget: 2048,
				Messages: []Message{{
					Role: RoleUser,
					Text: "What is 17 * 3? Reply with just the number.",
				}},
			})
			if err != nil {
				t.Fatalf("%s: thinking Complete: %v", kind, err)
			}
			if !strings.Contains(th.Text, "51") {
				t.Errorf("%s: thinking text = %q, want it to contain 51", kind, th.Text)
			}
			t.Logf("%s thinking: text=%q trace=%d step(s)", kind, strings.TrimSpace(th.Text), len(th.Trace))

			// (c) Two-step tool loop, reasoning off as the native loop sends it.
			schema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)
			tools := []ToolDef{{Name: "get_weather", Description: "Get the current weather for a city.", InputSchema: schema}}
			msgs := []Message{{
				Role: RoleUser,
				Text: "Use the get_weather tool to check the weather in Istanbul. You MUST call the tool.",
			}}
			tr, err := p.Complete(ctx, Request{Model: deepseekDefaultModel, MaxTokens: 256, Tools: tools, Messages: msgs})
			if err != nil {
				t.Fatalf("%s: tool Complete: %v", kind, err)
			}
			if len(tr.ToolCalls) == 0 {
				t.Fatalf("%s: expected a tool_use call, got none (stop=%s text=%q)", kind, tr.StopReason, tr.Text)
			}
			call := tr.ToolCalls[0]
			t.Logf("%s tool: call=%s input=%s stop=%s", kind, call.Name, string(call.Input), tr.StopReason)

			msgs = append(msgs,
				Message{Role: RoleAssistant, Text: tr.Text, ToolCalls: tr.ToolCalls},
				Message{Role: RoleUser, ToolResults: []ToolResult{{CallID: call.ID, Content: `{"city":"Istanbul","tempC":21,"sky":"clear"}`}}},
			)
			fin, err := p.Complete(ctx, Request{Model: deepseekDefaultModel, MaxTokens: 256, Tools: tools, Messages: msgs})
			if err != nil {
				t.Fatalf("%s: tool follow-up Complete: %v", kind, err)
			}
			if !strings.Contains(fin.Text, "21") {
				t.Errorf("%s: follow-up text = %q, want it to use the tool result (21)", kind, fin.Text)
			}
			t.Logf("%s follow-up: text=%q stop=%s", kind, strings.TrimSpace(fin.Text), fin.StopReason)
		})
	}
}
