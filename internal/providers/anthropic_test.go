package providers

import (
	"encoding/json"
	"strings"
	"testing"
)

// asBlocks marshals a systemField result back to []systemBlock for assertions.
func asBlocks(t *testing.T, v any) []systemBlock {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var blocks []systemBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatalf("unmarshal blocks: %v", err)
	}
	return blocks
}

func TestSystemField_NoCacheConcatenates(t *testing.T) {
	a := &Anthropic{} // extendedCache off
	got := a.systemField("PERSONA", "MEMORY")
	s, ok := got.(string)
	if !ok {
		t.Fatalf("want plain string without caching, got %T", got)
	}
	if s != "PERSONA\n\nMEMORY" {
		t.Errorf("system = %q, want %q", s, "PERSONA\n\nMEMORY")
	}
}

func TestSystemField_EmptyReturnsNil(t *testing.T) {
	a := &Anthropic{extendedCache: true}
	if got := a.systemField("  ", ""); got != nil {
		t.Errorf("empty system = %v, want nil", got)
	}
}

// With caching on, the system field is STATIC-ONLY: the volatile dynamic is moved
// out to the message tail (see toAnthropicMessages) so it never invalidates the
// cached tools+system+history prefix. So systemField("PERSONA","MEMORY") yields a
// single cached PERSONA block — MEMORY is NOT present here.
func TestSystemField_CacheBreakpointOnStaticOnly(t *testing.T) {
	a := &Anthropic{extendedCache: true}
	blocks := asBlocks(t, a.systemField("PERSONA", "MEMORY"))
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1 (static only)", len(blocks))
	}
	if blocks[0].Text != "PERSONA" || blocks[0].CacheControl == nil {
		t.Errorf("static block must carry cache_control: %+v", blocks[0])
	}
	if blocks[0].CacheControl.TTL != "1h" {
		t.Errorf("static cache TTL = %q, want 1h", blocks[0].CacheControl.TTL)
	}
}

func TestSystemField_StaticOnlyCached(t *testing.T) {
	a := &Anthropic{extendedCache: true}
	blocks := asBlocks(t, a.systemField("PERSONA", ""))
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].CacheControl == nil {
		t.Error("lone static block should be cached")
	}
}

// When there is no static prefix, the system field is nil (with caching on): the
// dynamic no longer lives in the system at all — it rides the message tail. A
// static-less prompt therefore has nothing cacheable in the system block.
func TestSystemField_DynamicOnlyReturnsNil(t *testing.T) {
	a := &Anthropic{extendedCache: true}
	if got := a.systemField("", "MEMORY"); got != nil {
		t.Errorf("static-less system (caching on) = %v, want nil (dynamic rides messages)", got)
	}
}

// With caching on, exactly ONE cache breakpoint rides the tools block, on the
// LAST tool (Anthropic caches by prefix, so one breakpoint covers all preceding
// tools) and at the same 1h TTL as the system block.
func TestToAnthropicTools_CacheBreakpointOnLastTool(t *testing.T) {
	defs := []ToolDef{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got := toAnthropicTools(defs, true, serverToolOpts{})
	if len(got) != 3 {
		t.Fatalf("got %d tools, want 3", len(got))
	}
	for i := 0; i < len(got)-1; i++ {
		if got[i].CacheControl != nil {
			t.Errorf("tool %q must not carry cache_control (only the last one does)", got[i].Name)
		}
	}
	last := got[len(got)-1]
	if last.CacheControl == nil || last.CacheControl.TTL != "1h" {
		t.Errorf("last tool must carry a 1h cache breakpoint, got %+v", last.CacheControl)
	}
	// Empty schemas are still filled so the tool is valid.
	if string(got[0].InputSchema) != `{"type":"object"}` {
		t.Errorf("empty schema must default to object, got %s", got[0].InputSchema)
	}
}

// With caching off, no tool carries a breakpoint — the caching on/off policy is
// unchanged, only its granularity improves when on.
func TestToAnthropicTools_NoCacheWhenDisabled(t *testing.T) {
	got := toAnthropicTools([]ToolDef{{Name: "a"}, {Name: "b"}}, false, serverToolOpts{})
	for _, tl := range got {
		if tl.CacheControl != nil {
			t.Errorf("tool %q must not be cached when extendedCache is off", tl.Name)
		}
	}
}

// With caching on, the rolling breakpoint lands on the last block of the last
// message and a HEDGE breakpoint on the second-to-last (≈ the previous call's
// breakpoint position, so cache lookup finds the old prefix even when the new
// tail exceeds Anthropic's ~20-block lookback). No earlier message carries one.
func TestToAnthropicMessages_RollingHistoryBreakpoint(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Text: "hi"},
		{Role: RoleAssistant, Text: "hello"},
		{Role: RoleUser, Text: "again"},
	}
	got := toAnthropicMessages(msgs, true, "", "claude-sonnet-4-6")
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3", len(got))
	}
	for i := 0; i < len(got)-2; i++ {
		for _, b := range got[i].Content {
			if b.CacheControl != nil {
				t.Errorf("message %d must not carry cache_control (only the last two do)", i)
			}
		}
	}
	hedge := got[len(got)-2].Content
	if bp := hedge[len(hedge)-1].CacheControl; bp == nil || bp.TTL != "1h" {
		t.Errorf("second-to-last block must carry the 1h hedge breakpoint, got %+v", bp)
	}
	last := got[len(got)-1].Content
	bp := last[len(last)-1].CacheControl
	if bp == nil || bp.TTL != "1h" {
		t.Errorf("last block must carry a 1h rolling breakpoint, got %+v", bp)
	}
}

// A single-message history carries only the rolling breakpoint — there is no
// prior message to hedge on.
func TestToAnthropicMessages_NoHedgeOnSingleMessage(t *testing.T) {
	got := toAnthropicMessages([]Message{{Role: RoleUser, Text: "hi"}}, true, "", "claude-sonnet-4-6")
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].Content[0].CacheControl == nil {
		t.Error("single message must still carry the rolling breakpoint")
	}
}

// The hedge walk skips Raw (verbatim-echo) messages — they cannot carry
// cache_control — and lands on the newest non-Raw predecessor instead.
func TestToAnthropicMessages_HedgeSkipsRaw(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Text: "hi"},
		{Role: RoleAssistant, Text: "raw turn", RawContent: json.RawMessage(`[{"type":"text","text":"raw turn"}]`)},
		{Role: RoleUser, Text: "again"},
	}
	got := toAnthropicMessages(msgs, true, "", "claude-sonnet-4-6")
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3", len(got))
	}
	if len(got[1].Raw) == 0 || len(got[1].Content) != 0 {
		t.Fatalf("middle message must stay a verbatim Raw echo: %+v", got[1])
	}
	if got[0].Content[0].CacheControl == nil {
		t.Error("hedge must fall back to the newest non-Raw predecessor (message 0)")
	}
	last := got[2].Content
	if last[len(last)-1].CacheControl == nil {
		t.Error("last message must carry the rolling breakpoint")
	}
}

// Back-to-back same-role plain turns merge BLOCK-WISE: the later turn becomes an
// extra text block on the earlier message, so the earlier block's bytes stay
// identical (a cached prefix ending there keeps hitting) and roles still
// alternate. An all-empty placeholder block is filled in place instead.
func TestToAnthropicMessages_BlockWiseCoalesce(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Text: "hi"},
		{Role: RoleAssistant, Text: "[Ada]: first"},
		{Role: RoleAssistant, Text: "[Kai]: second"},
		{Role: RoleUser, Text: "next"},
	}
	got := toAnthropicMessages(msgs, true, "", "claude-sonnet-4-6")
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3 (two assistant turns merged)", len(got))
	}
	merged := got[1]
	if merged.Role != RoleAssistant || len(merged.Content) != 2 {
		t.Fatalf("merged assistant message must carry two text blocks, got %+v", merged)
	}
	if merged.Content[0].Text != "[Ada]: first" {
		t.Errorf("earlier block's bytes must be untouched, got %q", merged.Content[0].Text)
	}
	if merged.Content[1].Text != "[Kai]: second" {
		t.Errorf("later turn must ride as its own block, got %q", merged.Content[1].Text)
	}

	// Placeholder fill: an empty first turn's placeholder block takes the text.
	msgs = []Message{
		{Role: RoleAssistant, Text: ""},
		{Role: RoleAssistant, Text: "x"},
	}
	got = toAnthropicMessages(msgs, true, "", "claude-sonnet-4-6")
	if len(got) != 1 || len(got[0].Content) != 1 || got[0].Content[0].Text != "x" {
		t.Errorf("empty placeholder block must be filled in place, got %+v", got)
	}
}

// With caching on and a non-empty dynamic suffix, the dynamic rides as a trailing
// text block on the last message — AFTER the rolling breakpoint, so it stays
// OUTSIDE the cached prefix. The breakpoint must sit on the persisted block
// (second-to-last here), NOT on the appended dynamic block.
func TestToAnthropicMessages_DynamicTrailsAfterBreakpoint(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Text: "hi"},
		{Role: RoleAssistant, Text: "hello"},
		{Role: RoleUser, Text: "again"},
	}
	got := toAnthropicMessages(msgs, true, "NOW: 2026 + recalled memory", "claude-sonnet-4-6")
	last := got[len(got)-1].Content
	if len(last) != 2 {
		t.Fatalf("last message should have persisted block + dynamic block, got %d", len(last))
	}
	if last[0].Text != "again" || last[0].CacheControl == nil {
		t.Errorf("breakpoint must sit on the PERSISTED block: %+v", last[0])
	}
	if last[1].Text != "NOW: 2026 + recalled memory" || last[1].CacheControl != nil {
		t.Errorf("dynamic block must trail AFTER the breakpoint and be uncached: %+v", last[1])
	}
}

// With caching OFF, the dynamic is NOT moved into the messages (it stays in the
// concatenated system field) — the non-cached path is unchanged.
func TestToAnthropicMessages_DynamicIgnoredWhenCacheOff(t *testing.T) {
	got := toAnthropicMessages([]Message{{Role: RoleUser, Text: "hi"}}, false, "VOLATILE", "claude-sonnet-4-6")
	last := got[len(got)-1].Content
	if len(last) != 1 || last[0].Text != "hi" {
		t.Errorf("dynamic must not be appended to messages when caching is off, got %+v", last)
	}
}

// The rolling breakpoint lands on the last block even when the final message is
// a tool_result (assistant tool_use → user tool_result is a common turn tail).
func TestToAnthropicMessages_BreakpointOnLastBlockAcrossKinds(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "t1", Name: "x", Input: json.RawMessage(`{}`)}}},
		{Role: RoleUser, ToolResults: []ToolResult{{CallID: "t1", Content: "ok"}}},
	}
	got := toAnthropicMessages(msgs, true, "", "claude-sonnet-4-6")
	last := got[len(got)-1].Content
	if last[len(last)-1].Type != "tool_result" {
		t.Fatalf("expected last block to be tool_result, got %q", last[len(last)-1].Type)
	}
	if last[len(last)-1].CacheControl == nil {
		t.Error("rolling breakpoint must attach to a trailing tool_result block")
	}
}

// With caching off, no message carries a breakpoint.
func TestToAnthropicMessages_NoCacheWhenDisabled(t *testing.T) {
	got := toAnthropicMessages([]Message{{Role: RoleUser, Text: "hi"}}, false, "", "claude-sonnet-4-6")
	for _, b := range got[0].Content {
		if b.CacheControl != nil {
			t.Error("no breakpoint expected when extendedCache is off")
		}
	}
}

// P2: prependSummaryMessage inserts the summary as a synthetic head user message
// without mutating the caller's slice, and is a no-op for an empty summary.
func TestPrependSummaryMessage(t *testing.T) {
	orig := []Message{{Role: RoleAssistant, Text: "hello"}}
	got := prependSummaryMessage(orig, "  SUMMARY  ")
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2 (head summary + original)", len(got))
	}
	if got[0].Role != RoleUser || got[0].Text != "SUMMARY" {
		t.Errorf("head message = %+v, want trimmed user summary", got[0])
	}
	if got[1].Text != "hello" {
		t.Errorf("original message must follow the summary head, got %+v", got[1])
	}
	if len(orig) != 1 {
		t.Errorf("caller slice must NOT be mutated, got len %d", len(orig))
	}
	if same := prependSummaryMessage(orig, "   "); len(same) != 1 {
		t.Errorf("empty summary must be a no-op, got %d messages", len(same))
	}
}

// P2 (caching ON): the summary rides a head user message INSIDE the cached prefix
// (the rolling breakpoint sits on the LAST message, so the summary head is cached),
// and the volatile dynamic still trails after the breakpoint on the last message.
func TestBuildSystemAndMessages_SummaryHeadCachedWhenOn(t *testing.T) {
	a := &Anthropic{extendedCache: true}
	req := Request{
		System:        "PERSONA",
		SystemDynamic: "NOW-VOLATILE",
		Summary:       "PRIOR SUMMARY",
		Messages:      []Message{{Role: RoleAssistant, Text: "hello"}, {Role: RoleUser, Text: "again"}},
	}
	sysField, msgs := a.buildSystemAndMessages(req, "claude-sonnet-4-6")
	// System is static-only (summary is NOT here — it moved to the message head).
	blocks := asBlocks(t, sysField)
	if len(blocks) != 1 || blocks[0].Text != "PERSONA" {
		t.Fatalf("system must be static-only PERSONA, got %+v", blocks)
	}
	// First message is the summary head, uncached (a middle-of-prefix block).
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3 (summary head + 2)", len(msgs))
	}
	if msgs[0].Role != RoleUser || msgs[0].Content[0].Text != "PRIOR SUMMARY" {
		t.Errorf("first message must be the summary head, got %+v", msgs[0])
	}
	if msgs[0].Content[0].CacheControl != nil {
		t.Error("summary head must NOT carry the breakpoint (it is a cached-prefix READ, not the marker)")
	}
	// The rolling breakpoint sits on the persisted block of the LAST message.
	last := msgs[len(msgs)-1].Content
	if last[0].Text != "again" || last[0].CacheControl == nil {
		t.Errorf("rolling breakpoint must sit on the last persisted block, got %+v", last[0])
	}
	if last[len(last)-1].Text != "NOW-VOLATILE" || last[len(last)-1].CacheControl != nil {
		t.Errorf("volatile dynamic must trail after the breakpoint, uncached, got %+v", last[len(last)-1])
	}
}

// P5 hardening: across a full cached request (tools + static system + rolling
// history) EVERY cache_control breakpoint must share the single cacheTTL, and
// there must be EXACTLY TWO message-level breakpoints — the rolling one on the
// last PERSISTED block and the hedge on the message before it — never on the
// volatile dynamic trailer or the summary head. A mixed TTL or a stray extra
// marker silently breaks Anthropic caching (4-breakpoint API budget).
func TestCacheBreakpointStability(t *testing.T) {
	a := &Anthropic{extendedCache: true}
	req := Request{
		System:        "PERSONA",
		SystemDynamic: "NOW-VOLATILE",
		Summary:       "PRIOR SUMMARY",
		Tools:         []ToolDef{{Name: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		Messages:      []Message{{Role: RoleAssistant, Text: "hello"}, {Role: RoleUser, Text: "again"}},
	}
	sysField, msgs := a.buildSystemAndMessages(req, "claude-sonnet-4-6")
	tools := toAnthropicTools(req.Tools, a.extendedCache, serverToolOpts{})

	// Every TTL present must equal cacheTTL.
	for _, b := range asBlocks(t, sysField) {
		if b.CacheControl != nil && b.CacheControl.TTL != cacheTTL {
			t.Errorf("system breakpoint TTL = %q, want %q", b.CacheControl.TTL, cacheTTL)
		}
	}
	if tc := tools[len(tools)-1].CacheControl; tc == nil || tc.TTL != cacheTTL {
		t.Errorf("tools breakpoint TTL = %+v, want %q", tc, cacheTTL)
	}

	// Exactly two message-level breakpoints: hedge + rolling, on persisted blocks.
	var marked []string
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.CacheControl != nil {
				marked = append(marked, b.Text)
				if b.CacheControl.TTL != cacheTTL {
					t.Errorf("message breakpoint TTL = %q, want %q", b.CacheControl.TTL, cacheTTL)
				}
			}
		}
	}
	if len(marked) != 2 {
		t.Fatalf("want exactly two message breakpoints (hedge + rolling), got %d: %v", len(marked), marked)
	}
	if marked[0] != "hello" {
		t.Errorf("hedge breakpoint must sit on the second-to-last persisted block (\"hello\"), got %q", marked[0])
	}
	if marked[1] != "again" {
		t.Errorf("rolling breakpoint must sit on the last persisted block (\"again\"), got %q", marked[1])
	}
}

// P3: with the context-editing beta OFF, no context_management is attached and the
// beta header omits context-management; with it ON, the clear_tool_uses strategy is
// present with sane defaults and the beta header advertises it.
func TestContextEditing_OffByDefault(t *testing.T) {
	a := &Anthropic{extendedCache: true} // contextEditing off
	if cm := a.contextMgmt(); cm != nil {
		t.Errorf("context_management must be nil when the beta is off, got %+v", cm)
	}
	if strings.Contains(a.betaHeader(), betaContextManagement) {
		t.Errorf("beta header must not advertise context-management when off: %q", a.betaHeader())
	}
}

func TestContextEditing_On(t *testing.T) {
	a := (&Anthropic{}).WithBetas(true, true, false)
	cm := a.contextMgmt()
	if cm == nil || len(cm.Edits) != 1 {
		t.Fatalf("expected one context edit, got %+v", cm)
	}
	e := cm.Edits[0]
	if e.Type != "clear_tool_uses_20250919" {
		t.Errorf("edit type = %q, want clear_tool_uses_20250919", e.Type)
	}
	if e.Trigger == nil || e.Trigger.Type != "input_tokens" || e.Trigger.Value != contextClearTriggerTokens {
		t.Errorf("trigger = %+v, want input_tokens/%d", e.Trigger, contextClearTriggerTokens)
	}
	if e.Keep == nil || e.Keep.Type != "tool_uses" || e.Keep.Value != contextClearKeepToolUses {
		t.Errorf("keep = %+v, want tool_uses/%d", e.Keep, contextClearKeepToolUses)
	}
	if e.ClearAtLeast == nil || e.ClearAtLeast.Value != contextClearAtLeastTokens {
		t.Errorf("clearAtLeast = %+v, want input_tokens/%d", e.ClearAtLeast, contextClearAtLeastTokens)
	}
	if !strings.Contains(a.betaHeader(), betaContextManagement) {
		t.Errorf("beta header must advertise context-management when on: %q", a.betaHeader())
	}
	// The request body serializes context_management with the beta on.
	raw, _ := json.Marshal(anthropicReq{Model: "m", ContextManagement: a.contextMgmt()})
	if !strings.Contains(string(raw), `"context_management"`) || !strings.Contains(string(raw), `"clear_tool_uses_20250919"`) {
		t.Errorf("serialized body missing context_management: %s", raw)
	}
}

// P2 (caching OFF): there is no cached prefix to protect, so the summary folds back
// into the concatenated system prompt (its pre-P2 home) and is NOT a head message.
func TestBuildSystemAndMessages_SummaryFoldsIntoSystemWhenOff(t *testing.T) {
	a := &Anthropic{} // extendedCache off
	req := Request{
		System:        "PERSONA",
		SystemDynamic: "DYN",
		Summary:       "PRIOR SUMMARY",
		Messages:      []Message{{Role: RoleUser, Text: "hi"}},
	}
	sysField, msgs := a.buildSystemAndMessages(req, "claude-sonnet-4-6")
	s, ok := sysField.(string)
	if !ok {
		t.Fatalf("caching off must yield a plain string system, got %T", sysField)
	}
	if s != "PERSONA\n\nDYN\n\nPRIOR SUMMARY" {
		t.Errorf("summary must fold into system when caching is off, got %q", s)
	}
	if len(msgs) != 1 || msgs[0].Content[0].Text != "hi" {
		t.Errorf("no head summary message when caching is off, got %+v", msgs)
	}
}
