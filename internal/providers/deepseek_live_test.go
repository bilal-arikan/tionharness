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
// usage, and (b) a tool turn surfaces a tool_use call — exercised through the
// Registry so the resolve() wiring is covered too.
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

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			// (a) Plain completion.
			resp, err := p.Complete(ctx, Request{
				Model:     "deepseek-v4-flash",
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

			// (b) Tool turn: offer one tool and ask a question that forces its use.
			schema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`)
			tr, err := p.Complete(ctx, Request{
				Model:     "deepseek-v4-flash",
				MaxTokens: 256,
				Tools:     []ToolDef{{Name: "get_weather", Description: "Get the current weather for a city.", InputSchema: schema}},
				Messages: []Message{{
					Role: RoleUser,
					Text: "Use the get_weather tool to check the weather in Istanbul. You MUST call the tool.",
				}},
			})
			if err != nil {
				t.Fatalf("%s: tool Complete: %v", kind, err)
			}
			if len(tr.ToolCalls) == 0 {
				t.Errorf("%s: expected a tool_use call, got none (stop=%s text=%q)", kind, tr.StopReason, tr.Text)
			} else {
				t.Logf("%s tool: call=%s input=%s stop=%s", kind, tr.ToolCalls[0].Name, string(tr.ToolCalls[0].Input), tr.StopReason)
			}
		})
	}
}
