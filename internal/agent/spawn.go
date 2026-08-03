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
	// Kind overrides the created session's kind for a NON-worker spawn (ignored
	// when CoordinatorSessionID is set, which always forces "worker"). Empty keeps
	// the default "spawned". A context-reset handoff sets this to "chat" when it
	// continues a chat, so the continuation lands under the sidebar's "Sohbet"
	// filter next to its parent instead of being hidden under the "Spawn" tab —
	// handoff children are explicitly human-continuable (see isWritableSessionKind).
	Kind string
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

	// Coordinator tree placement, stamped once at creation (see
	// db.Session.RootCoordinatorSessionID / CoordinatorDepth): the tree root this
	// worker belongs to and its distance from that root. Computed by SpawnWorker
	// from the parent session, never supplied by a tool caller.
	RootCoordinatorSessionID string
	CoordinatorDepth         int

	// CoordinatorMode makes the SPAWNED session a coordinator in its own right, so
	// it can nest another level of workers under itself. This is what turns a flat
	// coordinator/worker pair into an arbitrarily deep tree.
	CoordinatorMode bool
	// CoordinatorWorkflow optionally pins a coordinator recipe (M5) on the spawned
	// sub-coordinator, with CoordinatorMaxTurns as its resolved notify-loop cap.
	// Deliberately NOT inherited from the parent: a recursive recipe (tournament,
	// fanout) would otherwise repeat itself all the way down the tree.
	CoordinatorWorkflow string
	CoordinatorMaxTurns int
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
	// reached. The slot is released when the background turn finishes. Depth-aware
	// so a deep coordinator branch cannot drain the pool that shallower work — and
	// any unrelated chat/schedule spawn — depends on.
	if !r.acquireSpawnSlotAtDepth(opts.CoordinatorDepth) {
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
	if k := strings.TrimSpace(opts.Kind); k != "" {
		kind = k
	}
	coordID := strings.TrimSpace(opts.CoordinatorSessionID)
	if coordID != "" {
		kind = "worker"
	}

	// Each spawn is its own independent session — a fresh sourceID (not GetOrCreate)
	// so two spawns never collapse into one thread.
	session, err := r.db.CreateSession(ctx, db.Session{
		AgentID:                  agent.ID,
		Kind:                     kind,
		SourceID:                 "spawn:" + uuid.NewString(),
		Title:                    title,
		ParentSessionID:          strings.TrimSpace(opts.ParentSessionID),
		WorkingDir:               cwd,
		Tags:                     opts.Tags,
		Role:                     strings.TrimSpace(opts.Role),
		CoordinatorSessionID:     coordID,
		RootCoordinatorSessionID: strings.TrimSpace(opts.RootCoordinatorSessionID),
		CoordinatorDepth:         opts.CoordinatorDepth,
		// A sub-coordinator: this worker may spawn workers of its own (the nesting
		// switch). Role stays "worker" — it still reports up to coordID.
		CoordinatorMode:     opts.CoordinatorMode,
		CoordinatorWorkflow: strings.TrimSpace(opts.CoordinatorWorkflow),
		CoordinatorMaxTurns: opts.CoordinatorMaxTurns,
	})
	if err != nil {
		r.releaseSpawnSlot()
		return SpawnResult{}, err
	}

	// Record the spawn prompt as the opening user turn so the thread reads as a
	// real conversation in the activity feed — and bridge it to the hub so a window
	// watching the spawned session renders the prompt live, in order before the reply
	// (_Docs/58), not only on reload.
	if _, err := r.recordInjectedUserNote(ctx, session.ID, "", prompt); err != nil {
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
	hardCap, idleCap := r.tun.SpawnTimeout(), r.tun.SpawnIdleTimeout()
	ctx, cancel := withActivityTimeout(context.Background(), hardCap, idleCap)
	defer cancel()

	// Serialize this detached spawn turn on the session's turn slot so it never
	// overlaps a user/wake/peer turn opened on the same session (all of which claim
	// the same slot). A fresh spawn is usually alone, but the session can be chatted
	// into or woken while the spawn runs.
	releaseSlot := r.claimSessionTurnSlot(sessionID)
	defer releaseSlot()

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

	// Why the turn ended, independent of err: a spawn can be cut short and still
	// return (text, nil) — claude-cli salvages the text captured before its
	// subprocess was killed, and the native loop returns nil after appending its own
	// terminal marker (iteration cap, guardrail halt, context/output exhaustion).
	// Without this a truncated spawn read as a finished one in the transcript, and
	// any tag automation chained off it continued on half-done work.
	outcome := classifyTurnOutcome(ctx, steps, hardCap, idleCap)

	if err != nil {
		// A watchdog cancellation arrives here as a bare "context canceled". Record
		// the note naming the ceiling that fired instead of that raw Go error.
		if outcome.Truncated() {
			r.logger.Warn("spawn: turn truncated",
				"session", sessionID, "agent", agent.ID, "status", outcome.Status,
				"hardCap", hardCap, "idleCap", idleCap)
			steps = appendOutcomeStep(steps, outcome)
			if addErr := r.recordAssistantMessage(ctx, sessionID, agent.ID, outcome.Note, steps, meta, time.Since(turnStart).Milliseconds()); addErr != nil {
				r.logger.Warn("spawn: failed to record truncated reply", "session", sessionID, "error", addErr)
			}
		} else {
			r.logger.Error("spawn: agent invoke failed",
				"session", sessionID, "agent", agent.ID,
				"provider", agent.Provider, "model", agent.Model, "error", err)
			r.recordTurnError(ctx, sessionID, agent.ID, err, steps, meta, time.Since(turnStart).Milliseconds(), "⚠️ Spawn turu çalıştırılamadı:")
		}
		r.emitSpawnEvent(agent, sessionID, prompt, false)
		r.AutoTagTurn(ctx, sessionID, steps, "spawn_error")
		return
	}

	if outcome.Truncated() {
		// Keep the salvaged text (the only record of how far the work got) but lead
		// with the note, so neither a human nor a chained agent reads the fragment as
		// a result. turnCtx, not ctx: only it carries the session id emitDebug keys on.
		r.logger.Warn("spawn: turn truncated",
			"session", sessionID, "agent", agent.ID, "status", outcome.Status,
			"hardCap", hardCap, "idleCap", idleCap)
		r.emitDebug(turnCtx, db.DebugEvent{Type: db.DebugError, AgentID: agent.ID, Detail: "spawn turn " + outcome.Status, Err: true})
		steps = appendOutcomeStep(steps, outcome)
		output = applyTurnOutcome(output, outcome)
	}

	output, addErr := r.recordAssistantReply(ctx, sessionID, agent.ID, output, steps, meta, time.Since(turnStart).Milliseconds(), "ℹ️ Ajan bu spawn için boş yanıt döndürdü.")
	if addErr != nil {
		r.logger.Warn("spawn: failed to record reply", "session", sessionID, "error", addErr)
	}
	r.logger.Info("spawn: finished", "session", sessionID, "agent", agent.ID, "status", outcome.Status)
	// Self-completion: if the spawned turn stalled with unfinished work (activated
	// tools it never used, or open todos), keep it going — there is no human to send
	// the follow-up. No-op when the turn finished cleanly. Bounded + budget-gated.
	r.maybeAutoContinue(ctx, agent, sessionID, KindSpawn, steps)
	r.emitSpawnEvent(agent, sessionID, prompt, !outcome.Truncated())
	// Auto-tag any tool errors from this spawned turn.
	r.AutoTagTurn(ctx, sessionID, steps, "")
	// Repair completion: a fixer spawned to clear an errored PARENT session's tag
	// cannot reach it via the current-session-scoped update_session tool. Now that
	// the fixer's turn finished cleanly, clear the requested tag(s) from the parent.
	// (On failure the runSpawn error branch above returned early, so the tag survives
	// and the bounded repair loop can retry.) A TRUNCATED fixer counts as a failure
	// for the same reason: it never proved the parent's problem was fixed, so the tag
	// must survive and let the bounded repair loop try again.
	if !outcome.Truncated() && len(opts.ClearParentTagsOnSuccess) > 0 && strings.TrimSpace(opts.ParentSessionID) != "" {
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

// deepSpawnDepth is the coordinator-tree depth from which a spawn counts as
// "deep" and must leave headroom for shallower work. Depth 0 is a root
// coordinator and depth 1 its direct workers — the level a user or a flow is
// actually waiting on — so the squeeze starts at 2.
const deepSpawnDepth = 2

// acquireSpawnSlot reserves one of the bounded concurrency slots, returning false
// when the cap is already reached. Paired with releaseSpawnSlot. Equivalent to a
// top-level spawn (see acquireSpawnSlotAtDepth).
func (r *Runtime) acquireSpawnSlot() bool { return r.acquireSpawnSlotAtDepth(0) }

// acquireSpawnSlotAtDepth reserves a background-turn slot for a spawn at a given
// coordinator-tree depth, keeping part of the pool for shallow work.
//
// SpawnMaxConcurrent is a single GLOBAL pool, and a deep coordinator tree can
// legitimately want more background turns than it holds. Without a reservation
// one busy branch fills every slot and its siblings — and any unrelated
// chat/schedule spawn — simply fail with "spawn limit reached" until it drains.
// That is not a deadlock (a coordinator's auto turns take no slot, so the tree
// still makes progress), but it starves exactly the levels a human is watching.
//
// So a deep spawn may only take a slot while a quarter of the pool is still
// free. Shallow spawns keep the full pool: the top of the tree never waits on
// its own descendants.
func (r *Runtime) acquireSpawnSlotAtDepth(depth int) bool {
	max := int64(r.tun.SpawnMaxConcurrent())
	limit := max
	if depth >= deepSpawnDepth {
		reserve := max / 4
		if reserve < 1 {
			reserve = 1
		}
		if limit = max - reserve; limit < 1 {
			// A pool of 1 cannot be shared; let the deep spawn have it rather than
			// making deep work impossible on a tiny cap.
			limit = 1
		}
	}
	if r.spawnActive.Add(1) > limit {
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
