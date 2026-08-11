package agent

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// cachePrefixSig must change iff the CACHEABLE prefix (static system + tool
// name/schema) changes; the volatile dynamic and the messages must NOT affect it
// (since P1/P2 they ride outside the cached prefix).
func TestCachePrefixSig(t *testing.T) {
	base := providers.Request{
		System: "PERSONA",
		Tools:  []providers.ToolDef{{Name: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	}
	sig := cachePrefixSig(base)

	// Dynamic + messages differ → same signature (they are outside the cached prefix).
	same := base
	same.SystemDynamic = "NOW 2026 volatile"
	same.Summary = "a rolling summary"
	same.Messages = []providers.Message{{Role: providers.RoleUser, Text: "hi"}}
	if got := cachePrefixSig(same); got != sig {
		t.Errorf("dynamic/messages must not change the cache signature: %d != %d", got, sig)
	}

	// System change → different signature.
	sysChg := base
	sysChg.System = "PERSONA v2"
	if cachePrefixSig(sysChg) == sig {
		t.Error("a system prompt change must change the cache signature")
	}

	// Tool schema change → different signature.
	toolChg := base
	toolChg.Tools = []providers.ToolDef{{Name: "read", InputSchema: json.RawMessage(`{"type":"object","required":["path"]}`)}}
	if cachePrefixSig(toolChg) == sig {
		t.Error("a tool schema change must change the cache signature")
	}
}

// attributeCacheBreak prioritises model change, then prefix change, then falls back
// to a TTL/server-eviction explanation when nothing observable changed.
func TestAttributeCacheBreak(t *testing.T) {
	prev := cacheProbe{sig: 111, model: "claude-opus-4-8", warmed: true}

	if tag, _ := attributeCacheBreak(prev, 111, "claude-sonnet-5"); tag != "model-changed" {
		t.Errorf("model change tag = %q, want model-changed", tag)
	}
	if tag, _ := attributeCacheBreak(prev, 222, "claude-opus-4-8"); tag != "prompt-or-tools-changed" {
		t.Errorf("prefix change tag = %q, want prompt-or-tools-changed", tag)
	}
	if tag, _ := attributeCacheBreak(prev, 111, "claude-opus-4-8"); tag != "ttl-or-server-eviction" {
		t.Errorf("no-change tag = %q, want ttl-or-server-eviction", tag)
	}
}

// isConversationKind tracks only main session turns; auxiliary one-shot calls must
// be excluded so their throwaway prompts never register as cache breaks.
func TestIsConversationKind(t *testing.T) {
	for _, k := range []CallKind{KindChat, KindTask, KindSchedule, KindFlow, KindSpawn} {
		if !isConversationKind(k) {
			t.Errorf("%q should be a conversation kind", k)
		}
	}
	for _, k := range []CallKind{KindTitle, KindSummary, KindReflect, KindCompact, KindSubagent, KindDelegate} {
		if isConversationKind(k) {
			t.Errorf("%q must NOT be a conversation kind (auxiliary one-shot)", k)
		}
	}
}

// Only the "something changed" causes reach the chat as a card. A TTL cooldown is
// the normal price of a pause; carding it every time would train the user to
// ignore the card.
func TestInlineCacheBreak(t *testing.T) {
	for _, tag := range []string{"model-changed", "prompt-or-tools-changed"} {
		if !inlineCacheBreak(tag) {
			t.Errorf("%q should be carded inline", tag)
		}
	}
	if inlineCacheBreak("ttl-or-server-eviction") {
		t.Error("a TTL/eviction break must NOT be carded inline (normal, would be noise)")
	}
}

// ConsumeCacheBreak is a one-shot per session: it returns the armed break once,
// clears it, and keeps sessions isolated from each other.
func TestConsumeCacheBreak(t *testing.T) {
	r := &Runtime{}
	if r.ConsumeCacheBreak("SES1") != nil {
		t.Fatal("nothing armed → nil")
	}
	if r.ConsumeCacheBreak("") != nil {
		t.Fatal("a blank session id must be a no-op")
	}

	r.pendingCacheBreaks.Store("SES1", CacheBreak{Reason: "model-changed", Detail: "d", ColdTokens: 9000})
	r.pendingCacheBreaks.Store("SES2", CacheBreak{Reason: "prompt-or-tools-changed"})

	cb := r.ConsumeCacheBreak("SES1")
	if cb == nil || cb.Reason != "model-changed" || cb.ColdTokens != 9000 {
		t.Fatalf("first consume = %+v, want the armed SES1 break", cb)
	}
	if again := r.ConsumeCacheBreak("SES1"); again != nil {
		t.Errorf("second consume = %+v, want nil (one-shot)", again)
	}
	if other := r.ConsumeCacheBreak("SES2"); other == nil || other.Reason != "prompt-or-tools-changed" {
		t.Errorf("SES2 = %+v, want its own untouched break", other)
	}
}

// CacheBreakStep maps a consumed break onto the persisted trace step, and yields
// a ZERO step (which callers drop) for nil / reasonless input.
func TestCacheBreakStep(t *testing.T) {
	st := CacheBreakStep(&CacheBreak{Reason: "model-changed", Detail: "Model değişti", ColdTokens: 12000})
	if st.Kind != StepCacheBreak || st.Reason != "model-changed" || st.Text != "Model değişti" || st.ColdTokens != 12000 {
		t.Fatalf("step = %+v, want a populated cache_break step", st)
	}
	if got := CacheBreakStep(nil); got.Kind != "" {
		t.Errorf("nil break = %+v, want the zero step", got)
	}
	if got := CacheBreakStep(&CacheBreak{}); got.Kind != "" {
		t.Errorf("reasonless break = %+v, want the zero step", got)
	}
}
