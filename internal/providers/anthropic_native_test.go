package providers

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestApplyOutputSchema: supported models get output_config.format; others are
// untouched (callers must parse-with-fallback).
func TestApplyOutputSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}},"required":["title"],"additionalProperties":false}`)
	cfg := applyOutputSchema("claude-opus-4-8", schema, nil)
	if cfg == nil || cfg.Format == nil || cfg.Format.Type != "json_schema" {
		t.Errorf("supported model: got %+v", cfg)
	}
	// Existing effort preserved.
	cfg = applyOutputSchema("claude-sonnet-5", schema, &outputConfig{Effort: "low"})
	if cfg == nil || cfg.Format == nil || cfg.Effort != "low" {
		t.Errorf("effort lost: %+v", cfg)
	}
	// Opus 4.7 / Sonnet 4.6 are NOT in the structured-outputs matrix.
	if cfg = applyOutputSchema("claude-opus-4-7", schema, nil); cfg != nil {
		t.Errorf("opus-4-7 must not get a format: %+v", cfg)
	}
	if cfg = applyOutputSchema("claude-opus-4-8", nil, nil); cfg != nil {
		t.Errorf("empty schema must be a no-op: %+v", cfg)
	}
}

// TestFoldSystemMessages_Native: Opus 4.8 passes system entries through and the
// converter emits them with role "system".
func TestFoldSystemMessages_Native(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Text: "q"},
		{Role: RoleSystem, Text: "terse mode on"},
	}
	out := toAnthropicMessages(msgs, false, "", "claude-opus-4-8")
	if len(out) != 2 || out[1].Role != "system" {
		t.Fatalf("system message must pass through natively: %+v", out)
	}
	b, _ := json.Marshal(out[1])
	if !strings.Contains(string(b), "terse mode on") {
		t.Errorf("system text lost: %s", b)
	}
}

// TestFoldSystemMessages_Fallback: on non-supporting models the system entry is
// folded into the PRECEDING user message as a <system-reminder> block (keeps
// role alternation intact); with no user predecessor it downgrades to user.
func TestFoldSystemMessages_Fallback(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Text: "q"},
		{Role: RoleSystem, Text: "terse mode on"},
	}
	out := toAnthropicMessages(msgs, false, "", "claude-sonnet-4-6")
	if len(out) != 1 || out[0].Role != RoleUser {
		t.Fatalf("system entry must fold into the preceding user turn: %+v", out)
	}
	// json.Marshal escapes angle brackets (<), so check the wrapper name.
	b, _ := json.Marshal(out[0])
	if !strings.Contains(string(b), "system-reminder") || !strings.Contains(string(b), "terse mode on") {
		t.Errorf("reminder wrapper missing: %s", b)
	}
	// Fold into a tool_results user message too (steer between tool iterations).
	msgs = []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "t1", Name: "Read"}}},
		{Role: RoleUser, ToolResults: []ToolResult{{CallID: "t1", Content: "ok"}}},
		{Role: RoleSystem, Text: "stop after this"},
	}
	out = toAnthropicMessages(msgs, false, "", "claude-haiku-4-5")
	if len(out) != 2 {
		t.Fatalf("expected fold into the tool_results turn: %+v", out)
	}
	b, _ = json.Marshal(out[1])
	if !strings.Contains(string(b), "tool_result") || !strings.Contains(string(b), "stop after this") {
		t.Errorf("fold into tool_results turn failed: %s", b)
	}
	// No user predecessor → standalone user turn.
	out = toAnthropicMessages([]Message{{Role: RoleSystem, Text: "solo"}}, false, "", "claude-haiku-4-5")
	if len(out) != 1 || out[0].Role != RoleUser {
		t.Errorf("solo system entry must downgrade to user: %+v", out)
	}
}

// TestToAnthropicMessages_PureToolResults: the message answering a programmatic
// batch must not receive the dynamic-suffix text block.
func TestToAnthropicMessages_PureToolResults(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "t1", Name: "Read", Caller: "code_execution_20260120"}}},
		{Role: RoleUser, ToolResults: []ToolResult{{CallID: "t1", Content: "data"}}, OnlyToolResults: true},
	}
	out := toAnthropicMessages(msgs, true, "VOLATILE-DYN", "claude-opus-4-8")
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "VOLATILE-DYN") {
		t.Errorf("dynamic text leaked into a pure tool_result message: %s", b)
	}
	// Ordinary tool_results keep the dynamic append.
	msgs[1].OnlyToolResults = false
	out = toAnthropicMessages(msgs, true, "VOLATILE-DYN", "claude-opus-4-8")
	b, _ = json.Marshal(out)
	if !strings.Contains(string(b), "VOLATILE-DYN") {
		t.Errorf("dynamic text missing on an ordinary tool_result message: %s", b)
	}
}

// TestToolCallProgrammatic covers the caller classification helper.
func TestToolCallProgrammatic(t *testing.T) {
	if (ToolCall{Caller: "direct"}).Programmatic() || (ToolCall{}).Programmatic() {
		t.Error("direct/empty callers are not programmatic")
	}
	if !(ToolCall{Caller: "code_execution_20260120"}).Programmatic() {
		t.Error("code execution caller must be programmatic")
	}
}

// TestEffortForThinkingBudget_Extended pins the new xhigh/max tiers.
func TestEffortForThinkingBudget_Extended(t *testing.T) {
	if got := EffortForThinkingBudget(32768); got != "xhigh" {
		t.Errorf("32768 = %q, want xhigh", got)
	}
	if got := EffortForThinkingBudget(65536); got != "max" {
		t.Errorf("65536 = %q, want max", got)
	}
	// Legacy models clamp oversized budgets down to the enabled-shape ceiling.
	p, _, _ := thinkingFor("claude-haiku-4-5", 65536, 4096)
	if p == nil || p.BudgetTokens != 16384 {
		t.Errorf("legacy budget not clamped: %+v", p)
	}
}
