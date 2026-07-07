package providers

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestApplyTaskBudget covers the task-budget resolver: supported models get the
// directive (with the API minimum enforced), unsupported models and budget 0
// leave the config untouched.
func TestApplyTaskBudget(t *testing.T) {
	cfg, beta := applyTaskBudget("claude-opus-4-8", 50000, nil)
	if cfg == nil || cfg.TaskBudget == nil || cfg.TaskBudget.Total != 50000 || cfg.TaskBudget.Type != "tokens" || !beta {
		t.Errorf("supported model: got cfg=%+v beta=%v", cfg, beta)
	}
	// Below the API minimum → raised to 20000; an existing effort is preserved.
	cfg, beta = applyTaskBudget("claude-sonnet-5", 5000, &outputConfig{Effort: "high"})
	if cfg == nil || cfg.TaskBudget == nil || cfg.TaskBudget.Total != minTaskBudgetTokens || cfg.Effort != "high" || !beta {
		t.Errorf("minimum clamp: got cfg=%+v beta=%v", cfg, beta)
	}
	// Unsupported model: untouched, no beta.
	if cfg, beta = applyTaskBudget("claude-haiku-4-5", 50000, nil); cfg != nil || beta {
		t.Errorf("unsupported model should be a no-op: cfg=%+v beta=%v", cfg, beta)
	}
	// Budget 0 = off.
	if cfg, beta = applyTaskBudget("claude-opus-4-8", 0, nil); cfg != nil || beta {
		t.Errorf("zero budget should be a no-op: cfg=%+v beta=%v", cfg, beta)
	}
}

// TestToAnthropicTools_DeferLoading: any deferred def pulls in the tool-search
// server tool (prepended), deferred flags serialize, and the rolling cache
// breakpoint stays on the LAST entry (a user tool, never the server tool).
func TestToAnthropicTools_DeferLoading(t *testing.T) {
	defs := []ToolDef{
		{Name: "eager_a", Description: "always on"},
		{Name: "lazy_b", Description: "discoverable", DeferLoading: true},
	}
	out := toAnthropicTools(defs, true, serverToolOpts{})
	if len(out) != 3 {
		t.Fatalf("expected search tool + 2 defs, got %d entries", len(out))
	}
	if out[0].Type != nativeToolSearchType || out[0].Name != nativeToolSearchName {
		t.Errorf("search server tool must lead the list: %+v", out[0])
	}
	if out[0].DeferLoading {
		t.Error("the search tool itself must never be deferred")
	}
	if out[0].CacheControl != nil {
		t.Error("breakpoint must not sit on the server tool")
	}
	if out[len(out)-1].CacheControl == nil {
		t.Error("rolling breakpoint missing on the last user tool")
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, `"defer_loading":true`) {
		t.Errorf("deferred flag not serialized: %s", s)
	}
	if strings.Contains(s, `"input_schema":null`) {
		t.Errorf("server tool must omit input_schema, not null it: %s", s)
	}

	// No deferred defs → no server tool appended (behaviour unchanged).
	plain := toAnthropicTools([]ToolDef{{Name: "only"}}, false, serverToolOpts{})
	if len(plain) != 1 || plain[0].Type != "" {
		t.Errorf("plain path grew a server tool: %+v", plain)
	}
}

// TestToAnthropicTools_ProgrammaticCalling: PTC prepends the code-execution
// server tool, code-callable defs get allowed_callers, and strict is dropped on
// them (incompatible).
func TestToAnthropicTools_ProgrammaticCalling(t *testing.T) {
	defs := []ToolDef{
		{Name: "Read", Strict: true, CodeCallable: true},
		{Name: "ask_user", Strict: true}, // interactive → not code-callable
	}
	out := toAnthropicTools(defs, false, serverToolOpts{ptc: true})
	if len(out) != 3 || out[0].Type != codeExecToolType || out[0].Name != codeExecToolName {
		t.Fatalf("code execution server tool must lead: %+v", out)
	}
	for _, at := range out[1:] {
		switch at.Name {
		case "Read":
			if len(at.AllowedCallers) != 1 || at.AllowedCallers[0] != codeExecToolType {
				t.Errorf("Read must be code-callable: %+v", at)
			}
			if at.Strict {
				t.Error("strict must be dropped on code-callable tools")
			}
		case "ask_user":
			if at.AllowedCallers != nil {
				t.Errorf("ask_user must stay direct-only: %+v", at)
			}
			if !at.Strict {
				t.Error("non-callable tool keeps strict")
			}
		}
	}
	// PTC off → CodeCallable ignored entirely.
	off := toAnthropicTools(defs, false, serverToolOpts{})
	for _, at := range off {
		if at.AllowedCallers != nil {
			t.Errorf("allowed_callers must not ship when PTC is off: %+v", at)
		}
	}
}

// TestAnthropicMessage_RawMarshal: a Raw message marshals its verbatim content
// array; a structured message keeps the block form.
func TestAnthropicMessage_RawMarshal(t *testing.T) {
	rawContent := `[{"type":"text","text":"hi"},{"type":"server_tool_use","id":"s1","name":"tool_search_tool_regex"}]`
	b, err := json.Marshal(anthropicMessage{Role: "assistant", Raw: json.RawMessage(rawContent)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"server_tool_use"`) || !strings.Contains(string(b), `"role":"assistant"`) {
		t.Errorf("raw content not echoed verbatim: %s", b)
	}
	b, err = json.Marshal(anthropicMessage{Role: "user", Content: []contentBlock{{Type: "text", Text: "q"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"text":"q"`) {
		t.Errorf("structured content lost: %s", b)
	}
}

// TestToAnthropicMessages_RawPassthrough: a RawContent turn passes through
// untouched — no breakpoint, no dynamic append, no coalescing into neighbours.
func TestToAnthropicMessages_RawPassthrough(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Text: "question"},
		{Role: RoleAssistant, Text: "srv", RawContent: json.RawMessage(`[{"type":"text","text":"srv"}]`)},
	}
	out := toAnthropicMessages(msgs, true, "VOLATILE-DYN", "claude-sonnet-4-6")
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
	last := out[len(out)-1]
	if len(last.Raw) == 0 {
		t.Fatal("raw content dropped")
	}
	b, _ := json.Marshal(last)
	if strings.Contains(string(b), "VOLATILE-DYN") || strings.Contains(string(b), "cache_control") {
		t.Errorf("raw message must stay byte-identical (no dynamic/breakpoint): %s", b)
	}
	// An adjacent same-role plain turn must NOT be coalesced into the raw one.
	msgs = append(msgs, Message{Role: RoleAssistant, Text: "tail"})
	if out = toAnthropicMessages(msgs, false, "", "claude-sonnet-4-6"); len(out) != 3 {
		t.Errorf("raw message was coalesced with a plain neighbour: %d messages", len(out))
	}
}
