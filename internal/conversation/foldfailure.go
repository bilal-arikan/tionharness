package conversation

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// foldFailureCooldown is how long the AUTOMATIC rolling fold stands down for a
// session after its summarizer call failed.
//
// A failed fold changes nothing about the footprint that triggered it, so the
// gate is still over budget on the very next turn. Without a stand-down a
// session whose fold provider is briefly unhealthy (a 429, a stream timeout)
// would pay for — and lose — a summarizer call on every single turn until the
// provider recovered. The cooldown turns that retry storm into one attempt per
// window.
//
// It gates ONLY the automatic fold. Manual /compact (ForceCompact) never
// consults it: an explicit user request must always get a real try, the same
// rule harici ajanin force=True path applies (_Docs/analiz-hermes-baglam-yonetimi.md §6).
//
// A package var rather than a const so tests can shrink it.
var foldFailureCooldown = 45 * time.Second

// noteFoldFailure starts (or restarts) the automatic fold's stand-down for a
// session and returns when it expires.
func (m *Manager) noteFoldFailure(sessionID string) time.Time {
	until := time.Now().Add(foldFailureCooldown)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.foldFailedUntil == nil {
		m.foldFailedUntil = map[string]time.Time{}
	}
	m.foldFailedUntil[sessionID] = until
	return until
}

// foldCoolingDown reports whether the automatic fold is still standing down for
// this session, and until when.
func (m *Manager) foldCoolingDown(sessionID string) (time.Time, bool) {
	m.mu.RLock()
	until, ok := m.foldFailedUntil[sessionID]
	m.mu.RUnlock()
	if !ok || !time.Now().Before(until) {
		return time.Time{}, false
	}
	return until, true
}

// clearFoldCooldown drops a session's stand-down. Called after a fold succeeds:
// the entry would expire on its own, but dropping it keeps the map from growing
// one dead entry per session that ever had a transient fold failure.
func (m *Manager) clearFoldCooldown(sessionID string) {
	m.mu.Lock()
	delete(m.foldFailedUntil, sessionID)
	m.mu.Unlock()
}

// recordFoldFailureDebug journals a fold whose summarizer call failed. The turn
// continues UNCOMPACTED after this, so the event is the durable record of why
// the context did not shrink — without it the session would just look like a
// gate that never fired.
//
// ErrorKind is the closed "compaction_failed" enum the journal keeps verbatim;
// the provider's own message is deliberately NOT journalled (it is free text
// from a remote service) — it reaches the user through the on-screen warning
// step instead.
func (m *Manager) recordFoldFailureDebug(database *db.DB, sessionID, agentID string, until time.Time) {
	if database == nil || sessionID == "" {
		return
	}
	if err := database.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type:      db.DebugCompaction,
		AgentID:   agentID,
		Name:      "fold_failed",
		ErrorKind: "compaction_failed",
		Detail: fmt.Sprintf("summary call failed; turn continues uncompacted · automatic fold paused for %s",
			time.Until(until).Round(time.Second)),
	}); err != nil {
		m.log(slog.LevelError, "fold failure journal append failed", "session", sessionID, "error", err)
	}
}

// recordFoldCooldownDebug journals an over-budget turn on which the automatic
// fold was skipped because the stand-down from an earlier failure is still
// running. Prepare leaves compacted=false on this path, so the pressure warning
// still fires and the over-budget state stays visible on its own channel; this
// event is what says WHY nothing folded.
func (m *Manager) recordFoldCooldownDebug(database *db.DB, sessionID, agentID string, until time.Time) {
	if database == nil || sessionID == "" {
		return
	}
	if err := database.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type:    db.DebugCompaction,
		AgentID: agentID,
		Name:    "fold_cooldown",
		Detail: fmt.Sprintf("automatic fold skipped · %s left of the post-failure stand-down",
			time.Until(until).Round(time.Second)),
	}); err != nil {
		m.log(slog.LevelError, "fold cooldown journal append failed", "session", sessionID, "error", err)
	}
}
