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
