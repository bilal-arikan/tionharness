package agent

import (
	"context"
	"hash/fnv"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// cacheProbe is a session's last-known prompt-cache state, held in
// Runtime.cacheProbes keyed by session id and used to detect (and attribute) a
// prompt-cache break turn-to-turn.
type cacheProbe struct {
	sig    uint64 // hash of the cacheable prefix (static system + tool name/schema)
	model  string
	warmed bool // at least one warm cache READ has been observed for this session
}

// cacheBreakMinTokens is the cold-prefix floor below which a re-write is not worth
// flagging (small prompts are cheap and their miss is noise). It is measured against
// cacheWrite + input because providers report a cold prefix differently: native
// Anthropic puts the re-written prefix in cache_creation (cacheWrite) with a small
// input, while OpenRouter reports NO write counter and bills the whole cold prefix
// as plain input — so summing the two catches a break on either transport.
const cacheBreakMinTokens = 2000

// cachePrefixSig hashes the part of a request that MUST stay byte-stable for the
// prompt cache to hit: the static system prefix and the tool definitions (name +
// schema). The volatile dynamic and the messages are excluded on purpose — since
// P1/P2 they ride OUTSIDE the cached prefix (or extend it), so they never define a
// break. A change in this signature is the "systemPromptChanged / toolSchemasChanged"
// cause in Claude Code's promptCacheBreakDetection.
func cachePrefixSig(req providers.Request) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(req.System))
	for _, t := range req.Tools {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(t.Name))
		_, _ = h.Write(t.InputSchema)
	}
	return h.Sum64()
}

// isConversationKind reports whether a call origin is a MAIN session turn (chat or
// an autonomous session turn) whose prompt prefix is the stable per-session prefix
// worth tracking for cache breaks. Auxiliary one-shot calls (title/summary/reflect/
// compact/subagent/delegate) use their own throwaway prompts, so tracking them would
// only produce spurious "prompt changed" breaks.
func isConversationKind(k CallKind) bool {
	switch k {
	case KindChat, KindTask, KindSchedule, KindFlow, KindSpawn:
		return true
	default:
		return false
	}
}

// noteCacheOutcome inspects one provider call's usage against the session's last
// cache state and, when the warm prefix was lost (a large cold re-write with zero
// reads AFTER the cache had been warm), emits a `cache_break` debug event with an
// attributed reason. This is TionSwarm's analogue of Claude Code's
// promptCacheBreakDetection: the token data already exists (Usage.Cache*) — this
// only adds attribution so P1/P2's cache HITs (or their absence) are observable.
//
// Best-effort and side-effect-free beyond the debug journal: a blank session id, a
// non-conversation call kind, or the debug journal being off makes it a no-op. It
// never gates a turn.
func (r *Runtime) noteCacheOutcome(ctx context.Context, agent db.Agent, req providers.Request, model string, u providers.Usage) {
	sid := SessionIDFrom(ctx)
	if sid == "" || !isConversationKind(callKindFrom(ctx)) {
		return
	}
	sig := cachePrefixSig(req)

	var prev cacheProbe
	if v, ok := r.cacheProbes.Load(sid); ok {
		prev = v.(cacheProbe)
	}

	// A warm read this call → the prefix is healthy; record the good baseline and stop.
	if u.CacheReadTokens > 0 {
		r.cacheProbes.Store(sid, cacheProbe{sig: sig, model: model, warmed: true})
		return
	}

	// A large cold prefix (no read, and the prefix was re-paid — as cacheWrite on
	// Anthropic or as plain input on OpenRouter) AFTER the session had been warm =
	// the cache broke. Attribute the most likely cause from what changed.
	coldPrefix := u.CacheWriteTokens + u.InputTokens
	if prev.warmed && coldPrefix >= cacheBreakMinTokens {
		tag, detail := attributeCacheBreak(prev, sig, model)
		ev := db.DebugEvent{
			Type:       db.DebugCacheBreak,
			AgentID:    agent.ID,
			Model:      model,
			CacheWrite: u.CacheWriteTokens,
			In:         u.InputTokens,
			Name:       tag, // stable machine tag
			Detail:     detail,
		}
		// Only a TTL/eviction break is avoidable "cooling waste": model/prompt/tool
		// changes legitimately invalidate the prefix, so re-writing it is not a
		// missed saving. Attribute the avoidable overpay from the re-written prefix
		// (native Anthropic's cache_creation); OpenRouter folds it into input and
		// reports ~0 write, so this is naturally 0 there (see CoolingWaste).
		if tag == "ttl-or-server-eviction" {
			if usd, est, ok := providers.CoolingWaste(agent.Provider, model, u.CacheWriteTokens); ok && usd > 0 {
				ev.WasteUSD = usd
				ev.WasteEstimated = est
				// Roll the avoidable overpay into today's usage so the Budget screen can
				// show a workspace/window total (best-effort; never gates the turn).
				if err := r.db.AddCoolingWaste(ctx, agent.ID, usd, est); err != nil && r.logger != nil {
					r.logger.Warn("record cooling waste", "agent", agent.ID, "error", err)
				}
			}
		}
		r.emitDebug(ctx, ev)
	}
	// Keep the warmth flag (a genuine break is rare; a cold write establishes a new
	// baseline once the next call reads it) but refresh the signature/model. The probe
	// MUST survive across turns (the comparison is turn-to-turn), so it is intentionally
	// not cleared per turn; entries are one small struct per session touched this
	// process lifetime.
	r.cacheProbes.Store(sid, cacheProbe{sig: sig, model: model, warmed: prev.warmed})
}

// attributeCacheBreak names the most likely cause of a lost warm prefix by
// comparing the current signature/model against the last-good probe.
func attributeCacheBreak(prev cacheProbe, sig uint64, model string) (tag, human string) {
	switch {
	case prev.model != "" && prev.model != model:
		return "model-changed", "Model değişti (" + prev.model + " → " + model + ") → cache önekinin tamamı yeniden yazıldı."
	case prev.sig != sig:
		return "prompt-or-tools-changed", "Sistem promptu veya araç şemaları değişti → cache öneki geçersizleşti (yeniden yazıldı)."
	default:
		return "ttl-or-server-eviction", "Önek değişmedi ama cache okunmadı → 1s TTL doldu veya sunucu-tarafı eviction (KV-page)."
	}
}
