package agent

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func debugSummary(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

// emitDebug appends one structured observability event to the current session's
// debug.jsonl (a parallel stream to session.jsonl). It is the single funnel every
// emit point in the runtime calls — turn timings, llm-call token spend, tool
// latency/size, hook decisions, errors, compaction and recovery. It resolves the
// session id and call origin from ctx, so callers only fill the type-specific
// fields. Gated by the DebugJournal setting (default on); a blank session id or a
// nil db is a no-op. Failures are swallowed (best-effort) so observability can
// never break a turn.
func (r *Runtime) emitDebug(ctx context.Context, ev db.DebugEvent) {
	if r == nil || r.db == nil || r.tun == nil || !r.tun.DebugJournalEnabled() {
		return
	}
	sid := SessionIDFrom(ctx)
	if sid == "" {
		return
	}
	if ev.Kind == "" {
		ev.Kind = string(callKindFrom(ctx))
	}
	if ev.TurnID == "" {
		ev.TurnID = TurnIDFrom(ctx)
	}
	if err := r.db.AppendDebugEvent(sid, ev, r.tun.DebugJournalCap()); err != nil {
		r.logger.Debug("debug journal append failed", "session", sid, "error", err)
	}
}
