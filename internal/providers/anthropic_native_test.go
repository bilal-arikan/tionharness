package providers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
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

// TestToAnthropicTools_WebTools: the web toggle adds both server tools with
// per-turn use caps; the dynamic variants ride 4.6+ models, the basic variants
// ride older models AND PTC turns (which already carry a code-execution
// environment); the rolling breakpoint never lands on a server tool.
func TestToAnthropicTools_WebTools(t *testing.T) {
	defs := []ToolDef{{Name: "Read"}}
	out := toAnthropicTools(defs, true, serverToolOpts{webTools: true, model: "claude-opus-4-8"})
	if len(out) != 3 {
		t.Fatalf("expected web_search + web_fetch + Read, got %d entries", len(out))
	}
	if out[0].Type != webSearchDynType || out[0].MaxUses != webSearchMaxUses {
		t.Errorf("dynamic web search expected on 4.6+: %+v", out[0])
	}
	if out[1].Type != webFetchDynType || out[1].MaxUses != webFetchMaxUses {
		t.Errorf("dynamic web fetch expected on 4.6+: %+v", out[1])
	}
	if out[0].CacheControl != nil || out[1].CacheControl != nil {
		t.Error("breakpoint must not sit on server tools")
	}
	if out[2].Name != "Read" || out[2].CacheControl == nil {
		t.Errorf("breakpoint must sit on the last user tool: %+v", out[2])
	}
	// Older model → basic variants.
	out = toAnthropicTools(defs, false, serverToolOpts{webTools: true, model: "claude-haiku-4-5"})
	if out[0].Type != webSearchBasicType || out[1].Type != webFetchBasicType {
		t.Errorf("basic variants expected on older models: %+v %+v", out[0], out[1])
	}
	// PTC + web on a 4.6+ model → basic variants (no second execution env).
	out = toAnthropicTools(defs, false, serverToolOpts{ptc: true, webTools: true, model: "claude-opus-4-8"})
	types := map[string]bool{}
	for _, at := range out {
		types[at.Type] = true
	}
	if !types[codeExecToolType] || !types[webSearchBasicType] || types[webSearchDynType] {
		t.Errorf("PTC must force basic web variants: %+v", out)
	}
	// Toggle off → no web tools.
	out = toAnthropicTools(defs, false, serverToolOpts{model: "claude-opus-4-8"})
	if len(out) != 1 {
		t.Errorf("web tools must not ship when off: %+v", out)
	}
}

// TestContextMgmt_ServerCompaction: the compaction beta contributes the
// compact_20260112 edit + its beta header; independent of context editing.
func TestContextMgmt_ServerCompaction(t *testing.T) {
	a := &Anthropic{serverCompaction: true}
	cm := a.contextMgmt()
	if cm == nil || len(cm.Edits) != 1 || cm.Edits[0].Type != "compact_20260112" {
		t.Fatalf("compact edit missing: %+v", cm)
	}
	if !strings.Contains(a.betaHeader(), betaServerCompaction) {
		t.Errorf("compact beta header missing: %q", a.betaHeader())
	}
	// Both flags on → both edits, both betas.
	a = &Anthropic{serverCompaction: true, contextEditing: true}
	cm = a.contextMgmt()
	if cm == nil || len(cm.Edits) != 2 {
		t.Fatalf("expected clear + compact edits: %+v", cm)
	}
	h := a.betaHeader()
	if !strings.Contains(h, betaServerCompaction) || !strings.Contains(h, betaContextManagement) {
		t.Errorf("both betas expected: %q", h)
	}
	// Both off → nil (field omitted).
	if cm = (&Anthropic{}).contextMgmt(); cm != nil {
		t.Errorf("no edits expected: %+v", cm)
	}
}

// TestSanitizeFallbackEcho: content with a mid-output fallback boundary drops
// pre-boundary thinking/tool_use blocks on echo; content without one passes
// through byte-identical.
func TestSanitizeFallbackEcho(t *testing.T) {
	plain := json.RawMessage(`[{"type":"text","text":"hi"},{"type":"tool_use","id":"t1","name":"Read"}]`)
	if got := sanitizeFallbackEcho(plain); string(got) != string(plain) {
		t.Errorf("content without a fallback block must pass through untouched")
	}
	mixed := json.RawMessage(`[
		{"type":"thinking","thinking":"..."},
		{"type":"text","text":"partial"},
		{"type":"tool_use","id":"t1","name":"Read"},
		{"type":"fallback","from":{"model":"claude-fable-5"},"to":{"model":"claude-opus-4-8"}},
		{"type":"text","text":"continued"},
		{"type":"tool_use","id":"t2","name":"Grep"}
	]`)
	out := string(sanitizeFallbackEcho(mixed))
	if strings.Contains(out, `"thinking"`) || strings.Contains(out, `"t1"`) {
		t.Errorf("pre-boundary thinking/tool_use must be dropped: %s", out)
	}
	for _, keep := range []string{"partial", "continued", `"fallback"`, `"t2"`} {
		if !strings.Contains(out, keep) {
			t.Errorf("echo lost %q: %s", keep, out)
		}
	}
	// "fallback" only inside a string value → untouched.
	str := json.RawMessage(`[{"type":"text","text":"the word fallback appears here"}]`)
	if got := sanitizeFallbackEcho(str); string(got) != string(str) {
		t.Errorf("string-only mention must not trigger sanitization")
	}
}

// TestRequestCtx pins the model-class wall-clock budgets.
func TestRequestCtx(t *testing.T) {
	a := &Anthropic{}
	ctx, cancel := a.requestCtx(context.Background(), "claude-fable-5")
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok || time.Until(dl) < 9*time.Minute {
		t.Errorf("adaptive class should get the long budget, got %v", time.Until(dl))
	}
	ctx2, cancel2 := a.requestCtx(context.Background(), "claude-haiku-4-5")
	defer cancel2()
	dl2, _ := ctx2.Deadline()
	if time.Until(dl2) > 3*time.Minute {
		t.Errorf("legacy class should keep the short budget, got %v", time.Until(dl2))
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
