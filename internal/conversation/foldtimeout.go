package conversation

import (
	"context"
	"log/slog"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// FoldIdleOutputFloor is the stdout-silence budget a single-shot fold call gets.
//
// A fold (/compact, /handoff, the reactive mid-loop compaction) is one request
// carrying the whole pending transcript, and the CLI providers emit nothing
// between "turn.started" and the model's first token. On a near-full context
// window that gap alone can run for minutes, which the per-turn watchdog
// (codexStdoutIdleSec, default 90s) reads as a wedged process and kills — the
// failure mode that made /handoff return 500 instead of resetting the context.
// Ten minutes is comfortably below the turn idle watchdog (default 20 minutes),
// so a genuinely wedged fold is still bounded.
const FoldIdleOutputFloor = 10 * time.Minute

// foldCtx marks ctx as a single-shot fold call so the provider watchdog uses
// FoldIdleOutputFloor instead of the per-turn window. It never lowers an already
// larger window.
func foldCtx(ctx context.Context) context.Context {
	return providers.WithMinIdleOutputTimeout(ctx, FoldIdleOutputFloor)
}

// summarizeTimed is Manager.summarize plus the stall observation below. Every
// budgeted/manual rolling fold goes through it so a fold that only survived
// because of FoldIdleOutputFloor is recorded instead of looking like an ordinary
// slow turn.
func (m *Manager) summarizeTimed(ctx context.Context, database *db.DB, provider providers.Provider, agent db.Agent, sessionID, existing string, msgs []db.Message) (string, error) {
	started := time.Now()
	summary, err := m.summarize(ctx, database, provider, agent, existing, msgs)
	m.recordFoldIdleFloorDebug(database, sessionID, agent.ID, time.Since(started))
	return summary, err
}

// recordFoldIdleFloorDebug journals a fold that outlived the ordinary
// stdout-silence watchdog — the window foldCtx raised to FoldIdleOutputFloor.
// Below that window the floor never mattered, and a disabled (<=0) watchdog means
// there was nothing to override, so both cases stay silent.
//
// Wall time is an UPPER bound on the silent gap: a fold can exceed the window
// while still emitting. It is the only figure available above the provider layer
// (the watchdog itself lives in internal/providers, which has no store), and it
// cannot produce a false negative — a fold that really was killed-but-for-the-floor
// always trips it.
func (m *Manager) recordFoldIdleFloorDebug(database *db.DB, sessionID, agentID string, elapsed time.Duration) {
	if database == nil || sessionID == "" {
		return
	}
	global := providers.StdoutIdleWindow()
	if global <= 0 || global >= FoldIdleOutputFloor || elapsed <= global {
		return
	}
	if err := database.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type:       db.DebugPressure,
		AgentID:    agentID,
		Name:       "fold_idle_floor",
		DurationMs: elapsed.Milliseconds(),
	}); err != nil {
		m.log(slog.LevelError, "fold idle floor journal append failed", "session", sessionID, "error", err)
	}
}
