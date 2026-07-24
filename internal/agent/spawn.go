package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
)

// spawnTimeout bounds the turn-finished / failed-turn HOOK firing (a completion
// side-effect, not a work turn) that shares this package-level default. Every
// background WORK turn — spawn, worker, coordinator, inbox delivery — instead uses
// the settings-driven r.tun.SpawnTimeout() hard ceiling PLUS the r.tun.SpawnIdleTimeout()
// inactivity watchdog (see withActivityTimeout), so a productive long turn is not
// killed as "hung" and both bounds are tunable from the Settings screen.
const spawnTimeout = 10 * time.Minute

// SpawnOptions tunes a spawn. ModelOverride swaps just the model (the target
// agent's provider is always preserved); Title overrides the auto-generated one;
// CreatedBy records provenance (the spawning agent's id, or "" for user/API).
type SpawnOptions struct {
	ModelOverride string
	Title         string
	CreatedBy     string
	// ParentSessionID links a spawned session back to the one it continues, set by
	// a context-reset handoff so the UI can walk the reset chain. "" for an
	// ordinary spawn with no lineage.
	ParentSessionID string
	// WorkingDir optionally pins the spawned session's cwd. When empty, the spawn
	// inherits the workspace's configured default working directory (Path), same
	// as a UI-created session — so spawns/handoffs start scoped to that folder.
	WorkingDir string
	// Tags are applied to the spawned session at creation. Set by tag-triggered
	// automations so the new session carries the trigger tag (and thus re-fires the
	// automation on its own completion — the loop). Applying them at creation, not
	// after, avoids a race with the background turn finishing before the tag lands.
	Tags []string

	// ClearParentTagsOnSuccess names tags to remove from ParentSessionID once THIS
	// spawn's turn completes successfully (err == nil). Set by an error-repair
	// automation so the fixer clears the trigger tag from the ERRORED (parent)
	// session — which it otherwise cannot, since the in-turn update_session tool only
	// edits the fixer's OWN session. Skipped on failure so the tag survives and the
	// bounded repair loop can retry. Requires ParentSessionID.
	ClearParentTagsOnSuccess []string

	// Coordinator/worker link (see coordination.go, _Docs/47). When
	// CoordinatorSessionID is set the spawn is a WORKER: the session is created with
	// Kind="worker" + Role="worker" + this back-link, and its background turn runs
	// via runWorker (history-aware, notifies the coordinator on completion) instead
	// of runSpawn. Empty leaves an ordinary detached spawn unchanged.
	CoordinatorSessionID string
	Role                 string
}

// SpawnResult is what a spawn returns to its caller immediately — the new
// session's id (so a UI can deep-link it) and the resolved agent's name.
type SpawnResult struct {
	SessionID string
	AgentName string
}

// SpawnSession opens a NEW, independent session ("spawned" kind), records the
// prompt as the opening user turn, and runs the agent's turn in the background —
// fire-and-forget. It returns as soon as the session exists, without waiting for
// the turn to finish, so the caller (a UI button or the spawn_session tool) gets
// an immediate handle. This is the manual/agent-driven complement to the
// scheduler: same deliverPrompt shape, but detached and per-call independent.
//
// Guards: the global SpawnMaxConcurrent cap bounds how many spawned turns run at
// once (a spawn-storm brake); the turn is autonomous so each agent's daily budget
// still applies. resolveAgent is workspace-scoped, so a spawn can never reach
// across workspaces.
func (r *Runtime) SpawnSession(ctx context.Context, agentRef, prompt string, opts SpawnOptions) (SpawnResult, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return SpawnResult{}, fmt.Errorf("prompt is required to spawn a session")
	}
	agent, err := r.resolveAgent(ctx, agentRef)
	if err != nil {
		return SpawnResult{}, err
	}
	// Per-agent provider is preserved; only the model may be overridden.
	if m := strings.TrimSpace(opts.ModelOverride); m != "" {
		agent.Model = m
	}

	// Concurrency guard: refuse once the cap of simultaneously-running spawns is
	// reached. The slot is released when the background turn finishes.
	if !r.acquireSpawnSlot() {
		return SpawnResult{}, fmt.Errorf("spawn limit reached (%d concurrent spawned sessions); try again once some finish", r.tun.SpawnMaxConcurrent())
	}

	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "✨ " + spawnTitle(prompt)
	}

	// Seed the cwd like a UI-created session: an explicit option wins (e.g. a
	// handoff continuing in the parent's dir); otherwise inherit the workspace's
	// configured default working directory (Path). Empty leaves it unset so the
	// runtime falls back to the physical workspace dir, matching effectiveWorkDir.
	cwd := strings.TrimSpace(opts.WorkingDir)
	if cwd == "" {
		if p := r.defaultWorkDir.Load(); p != nil {
			cwd = strings.TrimSpace(*p)
		}
	}

	// Worker spawns (coordinator/worker M2) are tagged as such so the UI and the
	// coordination loop can tell them apart from ordinary detached spawns.
	kind := "spawned"
	coordID := strings.TrimSpace(opts.CoordinatorSessionID)
	if coordID != "" {
		kind = "worker"
	}

	// Each spawn is its own independent session — a fresh sourceID (not GetOrCreate)
	// so two spawns never collapse into one thread.
	session, err := r.db.CreateSession(ctx, db.Session{
		AgentID:              agent.ID,
		Kind:                 kind,
		SourceID:             "spawn:" + uuid.NewString(),
		Title:                title,
		ParentSessionID:      strings.TrimSpace(opts.ParentSessionID),
		WorkingDir:           cwd,
		Tags:                 opts.Tags,
		Role:                 strings.TrimSpace(opts.Role),
		CoordinatorSessionID: coordID,
	})
	if err != nil {
		r.releaseSpawnSlot()
		return SpawnResult{}, err
	}

	// Record the spawn prompt as the opening user turn so the thread reads as a
	// real conversation in the activity feed.
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      "user",
		Text:      prompt,
	}); err != nil {
		r.releaseSpawnSlot()
		return SpawnResult{}, err
	}

	r.logger.Info("spawn: launched",
		"session", session.ID, "agent", agent.ID, "model", agent.Model, "by", opts.CreatedBy)

	// Cross-window live refresh: the open session sidebar + ExecutionsPanel
	// activity feed must show the new row immediately. Carrying op="spawn" lets
	// the listener tell a brand-new session apart from a handoff continuation
	// (which would also touch the new-session id but with op="create"). The
	// later emitSpawnEvent("spawned" event) drives the executions-feed live
	// status (running/completed); this one drives the session list.
	r.publish(events.Event{
		Type:   "session",
		Level:  "info",
		Target: map[string]string{"sessionId": session.ID, "op": "spawn", "kind": kind},
	})

	// Fire-and-forget: run the turn detached from the caller's context so a closed
	// HTTP request or finished tool call can never cancel the spawn mid-flight. A
	// worker takes the coordinator-aware path (history-aware turn + notify-back);
	// an ordinary spawn takes the plain path.
	if coordID != "" {
		go r.runWorker(agent, session.ID, prompt, coordID)
	} else {
		go r.runSpawn(agent, session.ID, prompt, opts)
	}

	return SpawnResult{SessionID: session.ID, AgentName: agent.Name}, nil
}

// runSpawn executes the background turn for a spawned session: it tracks the
// session live (so the executions feed shows a "running" indicator), runs the
// agent autonomously (daily budget enforced), records the reply (or the failure)
// as an assistant turn, then releases the concurrency slot and notifies.
func (r *Runtime) runSpawn(agent db.Agent, sessionID, prompt string, opts SpawnOptions) {
	defer r.releaseSpawnSlot()

	// Hard wall-clock ceiling PLUS an idle watchdog (see withActivityTimeout): a
	// spawn that streams no step for SpawnIdleTimeout is reclaimed fast, while a
	// long-but-productive one runs up to SpawnTimeout.
	ctx, cancel := withActivityTimeout(context.Background(), r.tun.SpawnTimeout(), r.tun.SpawnIdleTimeout())
	defer cancel()

	r.trackSession(sessionID)
	// Raise the chat "thinking" indicator immediately, mirroring the wake path: a
	// spawned turn runs detached in the runtime (never registered in the api
	// server's chatRuns), so without this the session shows no running state and
	// an idle-looking composer until it happens to emit its next step. The
	// completion "spawned" event clears it.
	r.emitTurnStart(sessionID, "✨ Spawn turu çalışıyor")
	turnCtx, overflow := withOverflowFlag(WithSessionID(WithCallKind(ctx, KindSpawn), sessionID))
	turnCtx, meta := WithTurnMeta(turnCtx)
	turnStart := time.Now()
	output, steps, err := r.invokeTraced(turnCtx, agent, prompt, true)
	r.untrackSession(sessionID)

	if err != nil {
		r.logger.Error("spawn: agent invoke failed",
			"session", sessionID, "agent", agent.ID,
			"provider", agent.Provider, "model", agent.Model, "error", err)
		errMsg := db.Message{
			SessionID: sessionID,
			AgentID:   agent.ID,
			Role:      "assistant",
			Text:      "⚠️ Spawn turu çalıştırılamadı:\n\n" + err.Error(),
			Steps:     encodeSteps(steps),
		}
		meta.apply(&errMsg, time.Since(turnStart).Milliseconds())
		if _, addErr := r.db.AddMessage(ctx, errMsg); addErr != nil {
			r.logger.Warn("spawn: failed to record error reply", "session", sessionID, "error", addErr)
		}
		r.emitSpawnEvent(agent, sessionID, prompt, false)
		r.AutoTagTurn(ctx, sessionID, steps, "spawn_error")
		return
	}

	if strings.TrimSpace(output) == "" {
		output = "ℹ️ Ajan bu spawn için boş yanıt döndürdü."
	}
	replyMsg := db.Message{
		SessionID: sessionID,
		AgentID:   agent.ID,
		Role:      "assistant",
		Text:      output,
		Steps:     encodeSteps(steps),
	}
	meta.apply(&replyMsg, time.Since(turnStart).Milliseconds())
	if _, err := r.db.AddMessage(ctx, replyMsg); err != nil {
		r.logger.Warn("spawn: failed to record reply", "session", sessionID, "error", err)
	}
	r.logger.Info("spawn: finished", "session", sessionID, "agent", agent.ID)
	// Self-completion: if the spawned turn stalled with unfinished work (activated
	// tools it never used, or open todos), keep it going — there is no human to send
	// the follow-up. No-op when the turn finished cleanly. Bounded + budget-gated.
	r.maybeAutoContinue(ctx, agent, sessionID, KindSpawn, steps)
	r.emitSpawnEvent(agent, sessionID, prompt, true)
	// Auto-tag any tool errors / goal state from this spawned turn.
	r.AutoTagTurn(ctx, sessionID, steps, "")
	// Repair completion: a fixer spawned to clear an errored PARENT session's tag
	// cannot reach it via the current-session-scoped update_session tool. Now that
	// the fixer's turn finished cleanly, clear the requested tag(s) from the parent.
	// (On failure the runSpawn error branch above returned early, so the tag survives
	// and the bounded repair loop can retry.)
	if len(opts.ClearParentTagsOnSuccess) > 0 && strings.TrimSpace(opts.ParentSessionID) != "" {
		r.RemoveSessionTags(ctx, opts.ParentSessionID, opts.ClearParentTagsOnSuccess)
	}
	// Tag-triggered automations: a spawned session completing is the natural loop
	// step — if it carries an automation's trigger tag, this fires the next spawn.
	r.FireTurnFinished(sessionID, agent.ID, output)

	// Context-reset handoff: if this autonomous turn ran up against the context
	// limit (reactive compaction fired), optionally write a handoff and continue
	// the work in a fresh session. No-op unless HandoffAuto is enabled.
	r.maybeAutoHandoff(ctx, sessionID, agent, overflow.Load())
}

// emitTurnStart raises the chat "thinking" indicator for an autonomous turn the
// user did not initiate (spawn / worker) by publishing a "chat" event tagged
// phase=start for the session — the same signal the scheduler's wake path uses.
// The matching completion event ("spawned" / "worker") clears the indicator.
// "chat" events never raise an OS toast (the frontend returns before notifying),
// so this is a quiet pending/reload signal, not a notification.
func (r *Runtime) emitTurnStart(sessionID, title string) {
	if sessionID == "" {
		return
	}
	r.publish(events.Event{
		Type:   "chat",
		Level:  "info",
		Title:  title,
		Target: map[string]string{"view": "chat", "sessionId": sessionID, "phase": "start"},
	})
}

// emitSpawnEvent publishes a "spawned"-typed notification for a finished spawn,
// deep-linking to its transcript in the executions feed.
func (r *Runtime) emitSpawnEvent(agent db.Agent, sessionID, prompt string, ok bool) {
	level := "success"
	title := "✨ Spawn tamamlandı — " + agent.Name
	if !ok {
		level = "error"
		title = "✨ Spawn başarısız — " + agent.Name
	}
	r.publish(events.Event{
		Type:   events.TypeSpawned,
		Level:  level,
		Title:  title,
		Body:   notifyLine(prompt, 120),
		Target: map[string]string{"view": "executions", "sessionId": sessionID},
	})
}

// acquireSpawnSlot reserves one of the bounded concurrency slots, returning false
// when the cap is already reached. Paired with releaseSpawnSlot.
func (r *Runtime) acquireSpawnSlot() bool {
	max := int64(r.tun.SpawnMaxConcurrent())
	if r.spawnActive.Add(1) > max {
		r.spawnActive.Add(-1)
		return false
	}
	return true
}

// releaseSpawnSlot frees a concurrency slot taken by acquireSpawnSlot.
func (r *Runtime) releaseSpawnSlot() { r.spawnActive.Add(-1) }

// spawnTitle derives a short, single-line title from a spawn prompt (rune-capped,
// so Turkish characters never get split). The auto-titler can refine it later.
func spawnTitle(prompt string) string {
	t := notifyLine(prompt, 60)
	if t == "" {
		return "Spawn"
	}
	return t
}
