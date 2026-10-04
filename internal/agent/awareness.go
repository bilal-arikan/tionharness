package agent

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/bilal-arikan/tionharness/internal/awareness"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/notes"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// Workspace awareness glue (_Docs/94): the runtime owns the notes store and the
// awareness service, and offers the api layer and the headless paths three
// calls — BriefBlock (frozen into the static prefix), TurnBlock (the volatile
// suffix, budgeted and metered) and RecordDigest (after every finished turn).

// Notes returns the workspace memory store (nil when it failed to open).
func (r *Runtime) Notes() *notes.Store {
	if r == nil {
		return nil
	}
	return r.notes
}

// Awareness returns the awareness service (nil when the notes store failed).
func (r *Runtime) Awareness() *awareness.Service {
	if r == nil {
		return nil
	}
	return r.aware
}

// SetAwarenessSettings applies the workspace's awareness settings live.
func (r *Runtime) SetAwarenessSettings(s awareness.Settings) {
	n := s.Normalized()
	r.awareSettings.Store(&n)
}

// awarenessSettings returns the live settings (defaults until set).
func (r *Runtime) awarenessSettings() awareness.Settings {
	if p := r.awareSettings.Load(); p != nil {
		return *p
	}
	return awareness.DefaultSettings()
}

// awarenessInput assembles the producers' input for one session + agent.
func (r *Runtime) awarenessInput(ctx context.Context, session db.Session, agent db.Agent, fresh bool) awareness.Input {
	cwd := strings.TrimSpace(session.WorkingDir)
	if cwd == "" {
		cwd = r.WorkspaceDefaultDir()
	}
	running := map[string]bool{}
	for _, id := range r.ActiveSessionIDs() {
		running[id] = true
	}
	in := awareness.Input{
		Session: session, Agent: agent,
		WorkspaceID: r.wsID, WorkspaceName: r.wsName, DataDir: r.dataDir,
		Store: r.db, Notes: r.notes,
		Cwd: cwd, ProgressDir: r.ProgressDir(session.ID), ProgressResume: r.tun != nil && r.tun.ProgressResume(),
		Fresh: fresh, IsCoordinator: session.IsCoordinator(), Running: running,
	}
	in.RankNotes = r.briefNoteRanker(agent)
	in.JudgeUrgent = r.pulseUrgencyJudge(agent)
	in.SuggestNote = r.digestNoteSuggester(agent)
	return in
}

// BriefBlock returns the session's briefing for the static prefix. Composed on
// first use and then served frozen (the prompt epoch compares live against the
// frozen prefix, so this must not drift between adopt points); InvalidateBrief
// recomposes it where the epoch adopts anyway.
func (r *Runtime) BriefBlock(ctx context.Context, session db.Session, agent db.Agent) string {
	if r == nil || r.aware == nil || strings.TrimSpace(session.ID) == "" {
		return ""
	}
	comp := r.aware.Brief(ctx, r.awarenessInput(ctx, session, agent, false))
	return comp.Text
}

// InvalidateBrief forgets a session's frozen brief (compaction fold, handoff,
// explicit refresh).
func (r *Runtime) InvalidateBrief(sessionID string) {
	if r != nil && r.aware != nil && sessionID != "" {
		r.aware.InvalidateBrief(sessionID)
	}
}

// TurnBlock composes the volatile per-turn suffix: the caller's lead sections,
// the built-in turn producers (checklist, artifacts, de-duplicated pulse) and
// the trail sections, fitted to the turn budget and metered. The api chat path
// and the headless path both route through it, so every turn's context cost is
// measured the same way.
func (r *Runtime) TurnBlock(ctx context.Context, session db.Session, agent db.Agent, fresh bool, lead []awareness.Section, trail ...awareness.Section) awareness.Composition {
	if r == nil || r.aware == nil {
		return awareness.Compose(awareness.MomentTurn, append(lead, trail...), 0)
	}
	return r.aware.Turn(ctx, r.awarenessInput(ctx, session, agent, fresh), lead, trail...)
}

// RecordDigest computes and stores the session digest after a finished turn,
// publishing a workspace event when it changed. Best-effort and quiet: a digest
// failure is logged, never surfaced to the turn.
func (r *Runtime) RecordDigest(ctx context.Context, sessionID string) {
	if r == nil || r.aware == nil || r.db == nil || sessionID == "" {
		return
	}
	session, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	agent, _ := r.db.GetAgent(ctx, session.AgentID)
	d, changed, err := r.aware.Digest(ctx, r.awarenessInput(ctx, session, agent, false))
	if err != nil {
		r.logger.Debug("session digest failed", "session", sessionID, "error", err)
		return
	}
	if !changed {
		return
	}
	r.publish(events.Event{
		Type:   events.TypeAwarenessDigest,
		Level:  "info",
		Title:  "Oturum özeti güncellendi",
		Body:   d.Line(),
		Target: map[string]string{"view": "notes", "sessionId": sessionID},
	})
}

// ForgetAwareness drops a deleted session's awareness state and digest row.
func (r *Runtime) ForgetAwareness(sessionID string) {
	if r != nil && r.aware != nil {
		r.aware.Forget(sessionID)
	}
}

// notesBridge adapts the runtime to tools.NotesBridge.
type notesBridge struct{ rt *Runtime }

func (b notesBridge) Store() *notes.Store { return b.rt.notes }

func (b notesBridge) Project(ctx context.Context) string { return b.rt.sessionCwd(ctx) }

// Supersedes is the supersede judgement behind remember: the lexical rule (a
// near-identical title) is the baseline; the note-supersede authority may
// confirm or overrule it when switched on.
func (b notesBridge) Supersedes(ctx context.Context, candidate, existing notes.Note) bool {
	return b.rt.decideNoteSupersedes(ctx, candidate, existing)
}

// ensure the tools package sees the bridge type.
var _ tools.NotesBridge = notesBridge{}

// awareSettingsPtr is the atomic holder type (kept local for the struct field).
type awareSettingsPtr = atomic.Pointer[awareness.Settings]
