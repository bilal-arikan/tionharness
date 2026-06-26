package agent

import (
	"context"
	"sort"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

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
	if err := r.db.AppendDebugEvent(sid, ev, r.tun.DebugJournalCap()); err != nil {
		r.logger.Debug("debug journal append failed", "session", sid, "error", err)
	}
}

// debugReflectMaxSessions caps how many of an agent's most-recent sessions feed
// one performance-note pass, so the dream cycle stays cheap.
const debugReflectMaxSessions = 5

// debugPerfNotes collects de-duplicated anomaly findings across an agent's most
// recently updated sessions, returning them as a short bullet list to fold into
// the reflection prompt — the self-improvement loop that turns raw debug data
// into durable lessons. Returns "" when the journal is off, there are no
// sessions, or nothing notable was found. Best-effort: any error yields "".
func (r *Runtime) debugPerfNotes(ctx context.Context, agentID string) string {
	if r == nil || r.db == nil || r.tun == nil || !r.tun.DebugJournalEnabled() {
		return ""
	}
	sessions, err := r.db.ListSessions(ctx, agentID)
	if err != nil || len(sessions) == 0 {
		return ""
	}
	if len(sessions) > debugReflectMaxSessions {
		sessions = sessions[:debugReflectMaxSessions] // ListSessions is newest-first
	}
	seen := map[string]bool{}
	var notes []string
	for _, s := range sessions {
		sum, err := r.db.GetDebugSummary(ctx, s.ID)
		if err != nil {
			continue
		}
		for _, a := range sum.Anomalies {
			if seen[a.Message] {
				continue
			}
			seen[a.Message] = true
			notes = append(notes, "- "+a.Message)
		}
	}
	if len(notes) == 0 {
		return ""
	}
	sort.Strings(notes)
	return strings.Join(notes, "\n")
}
