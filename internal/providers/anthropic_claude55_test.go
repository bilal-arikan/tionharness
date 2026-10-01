package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClaude55AnthropicWire(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5"} {
		for _, budget := range []int{0, 2048, 8192, 16384, 32768, 65536} {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/budget%d/stream%v", model, budget, streaming), func(t *testing.T) {
					var body map[string]json.RawMessage
					var beta string
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						beta = r.Header.Get("anthropic-beta")
						if streaming {
							w.Header().Set("Content-Type", "text/event-stream")
							fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"usage\":{\"input_tokens\":1}}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
						} else {
							w.Header().Set("Content-Type", "application/json")
							fmt.Fprint(w, `{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
						}
					}))
					defer srv.Close()
					a := NewAnthropic("local-test-key").WithRefusalFallback(true)
					a.baseURL = srv.URL
					req := Request{Model: model, ThinkingBudget: budget, Messages: []Message{{Role: RoleUser, Text: "hello"}}}
					var err error
					if streaming {
						_, err = a.Stream(context.Background(), req, func(StreamDelta) {})
					} else {
						_, err = a.Complete(context.Background(), req)
					}
					if err != nil {
						t.Fatal(err)
					}
					var thinking map[string]any
					if err := json.Unmarshal(body["thinking"], &thinking); err != nil {
						t.Fatal(err)
					}
					between := model == "claude-sonnet-5-5" && budget == 0
					if between {
						if len(thinking) != 1 || thinking["type"] != "between_tools" || strings.Contains(beta, betaThinkingBinding) {
							t.Fatalf("invalid between_tools request: %s beta=%s", body["thinking"], beta)
						}
					} else {
						if thinking["type"] != "adaptive" || thinking["display"] != "summarized" || !strings.Contains(beta, betaThinkingBinding) {
							t.Fatalf("invalid adaptive request: %s beta=%s", body["thinking"], beta)
						}
						binding, _ := thinking["block_binding"].(map[string]any)
						if binding["prefix_mismatch_behavior"] != "drop_block" {
							t.Fatal("missing preserved-thinking protection")
						}
					}
					if budget > 0 {
						var cfg outputConfig
						if json.Unmarshal(body["output_config"], &cfg) != nil || cfg.Effort != EffortForThinkingBudget(budget) {
							t.Fatalf("wrong effort: %s", body["output_config"])
						}
					}
					for _, unsupported := range []string{"tool_choice", "fallbacks", "temperature", "top_p", "top_k"} {
						if _, exists := body[unsupported]; exists {
							t.Errorf("unexpected %s in request", unsupported)
						}
					}
				})
			}
		}
	}
}

func TestClaude55BetweenToolsHistory(t *testing.T) {
	const raw = `[{"type":"thinking","thinking":"private","signature":"signed"},{"type":"redacted_thinking","data":"hidden"},{"type":"tool_use","id":"t1","name":"Read","input":{}},{"type":"unknown_server_block","payload":"retain"}]`
	req := Request{Model: "claude-sonnet-5-5", Messages: []Message{
		{Role: RoleUser, Text: "opening"},
		{Role: RoleAssistant, RawContent: json.RawMessage(raw)},
		{Role: RoleUser, ToolResults: []ToolResult{{CallID: "t1", Content: "body"}}},
	}}
	a := NewAnthropic("local-test-key")
	for _, budget := range []int{0, 16384} {
		req.ThinkingBudget = budget
		_, messages := a.buildSystemAndMessages(req, req.Model)
		wire := string(messages[1].Raw)
		if (budget == 0) == strings.Contains(wire, `"type":"thinking"`) {
			t.Errorf("budget %d thinking replay = %s", budget, wire)
		}
		if !strings.Contains(wire, `"type":"tool_use"`) || !strings.Contains(wire, "unknown_server_block") {
			t.Fatalf("non-thinking content lost: %s", wire)
		}
		if string(req.Messages[1].RawContent) != raw {
			t.Fatal("stored conversation mutated")
		}
	}
}
