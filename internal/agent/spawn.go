package agent

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// spawnTimeout bounds the turn-finished / failed-turn HOOK firing (a completion
// side-effect, not a work turn) that shares this package-level default. Every
// background WORK turn — spawn, worker, coordinator, inbox delivery — instead uses
// the run-scoped semantic inactivity watchdog (see withActivityTimeout). Deprecated
// SpawnTimeout storage no longer bounds productive execution.
const spawnTimeout = 10 * time.Minute

// SpawnOptions tunes a spawn. ModelOverride swaps just the model (the target
// agent's provider is always preserved); Title overrides the auto-generated one;
// CreatedBy records provenance (the spawning agent's id, or "" for user/API).
type SpawnOptions struct {
	ModelOverride  string
	Title          string
	CreatedBy      string
	IdempotencyKey string
	// RuntimeBaseAgentID supplies provider/model/permission settings for a worker
	// system agent while leaving its identity, soul, and allowlist intact.
	RuntimeBaseAgentID string
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

	// Origin is the explicit lineage record stamped on the spawned session
	// (db.Session.Origin). Launchers that know more than the legacy fields carry
	// — the automation and the session that tripped it, the schedule, the
	// handed-off session — set it; left nil, the store derives the origin from the
	// spawn's own shape (worker → coordinator, subagent, handoff, plain spawn), so
	// a caller never has to set it just to be correct.
	Origin *db.SessionOrigin

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

	// NoQueue preserves callers that require an immediate SessionID.
	NoQueue      bool
	ChildSession *db.Session
	// onDrop releases caller-owned reservations if shutdown discards this spawn.
	// It returns whether releasing them took the owning coordinator's fleet to zero
	// (the same zero-crossing runWorkerWithCtl observes), so the drop notification
	// can carry the all-idle signal instead of losing the transition.
	onDrop func(error) bool
}

// SpawnResult is what a spawn returns to its caller immediately — the new
// session's id (so a UI can deep-link it) and the resolved agent's name.
type SpawnResult struct {
	SessionID string
	AgentName string
	Queued    bool
	// QueuePosition is one-based across both priority queues at enqueue time.
	QueuePosition int
	// TreeBudgetUsed / TreeBudgetTotal report the coordinator TREE's LIVE-worker
	// occupancy right after this spawn (Used counts the just-spawned worker). Total
	// is the ceiling; 0 means no ceiling is configured (or this was not a worker
	// spawn). They let a coordinator see remaining quota on every spawn instead of
	// only when it slams into the wall.
	TreeBudgetUsed  int
	TreeBudgetTotal int
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
	if baseID := strings.TrimSpace(opts.RuntimeBaseAgentID); baseID != "" && agent.System && strings.HasPrefix(agent.SystemKey, "subagent-") {
		base, err := r.db.GetAgent(ctx, baseID)
		if err != nil {
			return SpawnResult{}, fmt.Errorf("worker runtime base agent unavailable: %w", err)
		}
		agent.Provider = base.Provider
		agent.ProviderInstanceID = base.ProviderInstanceID
		agent.Model = base.Model
		agent.PermissionMode = base.PermissionMode
	}
	// A profile worker's allowlist is a SAFETY CONTRACT that lives in code, not a
	// user preference stored on the agent row. Re-assert it for this run whatever
	// the record says, so an edited (or stale) system-agent row can never widen
	// what an explore/reviewer/validator worker may touch.
	if err := r.applyProfileAllowlist(&agent); err != nil {
		return SpawnResult{}, err
	}
	// Per-agent provider is preserved; only the model may be overridden.
	if m := strings.TrimSpace(opts.ModelOverride); m != "" {
		agent.Model = m
	}
	if isCLIProviderKind(agent.Provider) {
		provider, err := r.providers.Get(agent.ProviderRef())
		if err != nil {
			return SpawnResult{}, fmt.Errorf("spawn refused: CLI provider unavailable: %w", err)
		}
		cli, ok := providers.AsCLI(provider)
		if !ok {
			return SpawnResult{}, fmt.Errorf("spawn refused: provider %q is configured as CLI but does not implement CLI preflight", agent.Provider)
		}
		switch p := provider.(type) {
		case *providers.ClaudeCLI:
			if p.ConfigDir() == "" {
				p.SetConfigDir(r.claudeHomeDir())
			}
		case *providers.CodexCLI:
			if p.ConfigDir() == "" {
				p.SetConfigDir(r.codexHomeDir())
			}
		}
		if err := cli.Preflight(ctx); err != nil {
			return SpawnResult{}, fmt.Errorf("spawn refused: %w", err)
		}
	}

	// Concurrency guard: refuse once the cap of simultaneously-running spawns is
	// reached. The slot is released when the background turn finishes. Depth-aware
	// so a deep coordinator branch cannot drain the pool that shallower work — and
	// any unrelated chat/schedule spawn — depends on.
	if !r.acquireSpawnSlotAtDepth(opts.CoordinatorDepth) {
		if opts.NoQueue {
			return SpawnResult{}, fmt.Errorf("spawn limit reached (%d concurrent spawned sessions); try again once some finish", r.tun.SpawnMaxConcurrent())
		}
		position, enqueueErr := r.enqueueSpawn(spawnQueueItem{agent: agent, prompt: prompt, opts: opts, enqueuedAt: time.Now()})
		if enqueueErr != nil {
			return SpawnResult{}, enqueueErr
		}
		return SpawnResult{AgentName: agent.Name, Queued: true, QueuePosition: position}, nil
	}
	return r.launchSpawn(ctx, agent, prompt, opts)
}

// launchSpawn creates and starts a spawn after its concurrency slot is reserved.
func (r *Runtime) launchSpawn(ctx context.Context, agent db.Agent, prompt string, opts SpawnOptions) (SpawnResult, error) {

	title := strings.TrimSpace(opts.Title)
	if title == "" {
		// Worker sessions (coordinator fan-out) carry their own marker so the
		// session list tells them apart from plain spawn_session runs at a glance.
		marker := "✨ "
		if opts.Role == db.SessionRoleWorker {
			marker = "🤖 "
		}
		title = marker + spawnTitle(prompt)
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
	parentID := strings.TrimSpace(opts.ParentSessionID)
	if coordID != "" {
		// CoordinatorSessionID drives report-back; ParentSessionID exposes the same
		// owner to generic session/execution consumers.
		parentID = coordID
	}

	// Each spawn is its own independent session — a fresh sourceID (not GetOrCreate)
	// so two spawns never collapse into one thread.
	sourceID := "spawn:" + uuid.NewString()
	if opts.IdempotencyKey != "" {
		sourceID = "spawn-dispatch:" + opts.IdempotencyKey
	}
	newSession := db.Session{
		AgentID:                  agent.ID,
		Kind:                     kind,
		SourceID:                 sourceID,
		DispatchKey:              opts.IdempotencyKey,
		Title:                    title,
		ParentSessionID:          parentID,
		Origin:                   opts.Origin,
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
	}
	if opts.ChildSession != nil {
		meta := *opts.ChildSession
		meta.AgentID, meta.Kind, meta.SourceID, meta.Title, meta.WorkingDir = newSession.AgentID, newSession.Kind, newSession.SourceID, newSession.Title, newSession.WorkingDir
		newSession = meta
	}
	var session db.Session
	var err error
	if opts.ChildSession != nil {
		session, err = r.db.CreateChildSession(ctx, newSession)
	} else {
		session, err = r.db.CreateSession(ctx, newSession)
	}
	if err != nil {
		r.releaseSpawnSlot()
		return SpawnResult{}, err
	}
	// A spawn is a new prompt-cache lineage. Clear any accidental in-memory or
	// sidecar entry for the newly allocated id through the normal refresh path so
	// a parent session's stale snapshot can never be inherited by a child.
	r.RefreshPromptEpoch(ctx, session.ID)

	// Record the spawn prompt as the opening user turn so the thread reads as a
	// real conversation in the activity feed — and bridge it to the hub so a window
	// watching the spawned session renders the prompt live, in order before the reply
	// (_Docs/58), not only on reload.
	//
	// When an AGENT ordered this spawn (opts.CreatedBy), the prompt is that agent
	// speaking, so it is attributed to it and renders as an incoming peer message
	// rather than as the human's own turn (TSK507). A user/API spawn leaves
	// CreatedBy empty and keeps the plain bubble.
	addOpeningMessage := func() error {
		_, err := r.recordAgentAuthoredNote(ctx, session.ID, opts.CreatedBy, agent.ID, prompt)
		return err
	}
	if opts.ChildSession != nil {
		err = r.initializeChildSession(ctx, session.ID, addOpeningMessage)
	} else {
		err = addOpeningMessage()
	}
	if err != nil {
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
		// Same reason as the plain-spawn branch below: SpawnWorker returns the worker
		// session id to the coordinator's tool loop, which may call stop_worker in the
		// very next iteration. Registering the ctl and the session cancel BEFORE the
		// goroutine starts is what makes that stop land.
		runCtx, cancelRun, ctl := r.newWorkerRun(session.ID)
		go r.runWorkerRegistered(runCtx, cancelRun, agent, session.ID, prompt, coordID, ctl)
	} else {
		// The cancel func is registered HERE, not inside runSpawn: SpawnSession hands
		// the session id back to its caller the moment this returns, and a caller may
		// stop the run before the goroutine is even scheduled — let alone before it
		// clears the blocking turn-slot claim. Registering inside the turn left that
		// whole window uncancellable while the row already read "running", so a stop
		// answered "already finished" for a run that kept going.
		runCtx, cancelRun := context.WithCancel(context.Background())
		run := r.trackSession(session.ID, cancelRun)
		go r.runSpawn(runCtx, cancelRun, run, agent, session.ID, prompt, opts)
	}

	return SpawnResult{SessionID: session.ID, AgentName: agent.Name}, nil
}

// runSpawn executes the background turn for a spawned session: it tracks the
// session live (so the executions feed shows a "running" indicator), runs the
// agent autonomously (daily budget enforced), records the reply (or the failure)
// as an assistant turn, then releases the concurrency slot and notifies.
//
// runCtx/cancelRun are created and REGISTERED by launchSpawn before this goroutine
// starts (see trackSession there), so the run is cancellable from the instant
// SpawnSession returns — including while it is still queued behind another turn.
// run is THAT registration: releasing it by handle leaves any turn queued behind
// this spawn on the same session still registered (and therefore still stoppable).
func (r *Runtime) runSpawn(runCtx context.Context, cancelRun context.CancelFunc, run *sessionRun, agent db.Agent, sessionID, prompt string, opts SpawnOptions) {
	defer r.releaseSpawnSlot()
	defer cancelRun()
	// Backstop only: the two sites below release at the exact moment the old code
	// untracked, so the timing of isSessionActive is unchanged. release is idempotent,
	// and a registration that outlives this goroutine would now leak forever.
	defer run.release()

	// Spawn lifetime is bounded only by semantic inactivity. Productive work has
	// no elapsed wall-clock ceiling; SpawnTimeout remains storage/API compatibility.
	hardCap, idleCap := time.Duration(0), r.tun.SpawnIdleTimeout()

	// Serialize this detached spawn turn on the session's turn slot so it never
	// overlaps a user/wake/peer turn opened on the same session (all of which claim
	// the same slot). A fresh spawn is usually alone, but the session can be chatted
	// into or woken while the spawn runs. The claim watches runCtx: a stop issued
	// while this turn waits in the queue must not be outlived by it.
	releaseSlot, slotErr := r.claimSessionTurnSlotCtx(runCtx, sessionID, turnqueue.KindSpawn, "spawn turu")
	defer releaseSlot()
	if slotErr != nil {
		// Cancelled while waiting for the session's turn slot: the turn never ran, so
		// record the terminal state instead of starting work nobody is waiting for.
		// This covers a plain CancelSession on a spawn that never got to run.
		r.logger.Info("spawn: cancelled before its turn started", "session", sessionID)
		if opts.ChildSession != nil {
			// context.Background() deliberately: runCtx is already cancelled and the
			// terminal state must still be persisted.
			if err := r.db.SetSessionRunState(context.Background(), sessionID, "killed", time.Now().Unix()); err != nil {
				r.logger.Error("spawn: failed to persist killed state", "session", sessionID, "error", err)
			}
		}
		run.release()
		r.emitSpawnEvent(agent, sessionID, prompt, false)
		return
	}

	// Raise the chat "thinking" indicator immediately, mirroring the wake path: a
	// spawned turn runs detached in the runtime (never registered in the api
	// server's chatRuns), so without this the session shows no running state and
	// an idle-looking composer until it happens to emit its next step. The
	// completion "spawned" event clears it.
	r.emitTurnStart(sessionID, "✨ Spawn turu çalışıyor")

	// Single-shot idle-resume: a spawn cut by the idle watchdog (a long non-streaming
	// tool call that outran even the heartbeat) gets ONE more attempt under a fresh
	// window before its partial work is reported unfinished (FND-708844f8). A hard-cap
	// cut, a clean finish, or a loop-terminal outcome is never resumed. turnCtx/overflow/
	// meta escape the closure so the post-turn handling below reads the FINAL attempt.
	var (
		turnCtx  context.Context
		overflow *atomic.Bool
		meta     *turnMeta
	)
	turnStart := time.Now()
	ctx, cancel, output, steps, err := r.runTurnWithIdleResume(runCtx, hardCap, idleCap, r.tun.IdleResumeMax(),
		func(attemptCtx context.Context, _ context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			turnCtx, overflow = withOverflowFlag(WithSessionID(WithCallKind(attemptCtx, KindSpawn), sessionID))
			turnCtx, meta = WithTurnMeta(turnCtx)
			p := prompt
			if attempt > 1 {
				p = resumeContinuationPrompt(prompt, prevOutput)
			}
			return r.invokeTraced(turnCtx, agent, p, true)
		})
	defer cancel()
	run.release()

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
			if opts.ChildSession == nil {
				if addErr := r.recordAssistantMessage(ctx, sessionID, agent.ID, outcome.Note, steps, meta, time.Since(turnStart).Milliseconds()); addErr != nil {
					r.logger.Warn("spawn: failed to record truncated reply", "session", sessionID, "error", addErr)
				}
			}
		} else {
			r.logger.Error("spawn: agent invoke failed",
				"session", sessionID, "agent", agent.ID,
				"provider", agent.Provider, "model", agent.Model, "error", err)
			if opts.ChildSession == nil {
				r.recordTurnError(ctx, sessionID, agent.ID, err, steps, meta, time.Since(turnStart).Milliseconds(), "⚠️ Spawn turu çalıştırılamadı:")
			}
		}
		r.emitSpawnEvent(agent, sessionID, prompt, false)
		if opts.ChildSession != nil {
			state := "failed"
			if outcome.Status == "killed" || outcome.Status == "timeout" || outcome.Status == "incomplete" {
				state = outcome.Status
			}
			summary := "subagent failed (" + string(classifyProviderError(err)) + ")"
			if persistErr := r.recordChildAssistantMessage(ctx, sessionID, agent.ID, summary, steps, meta, time.Since(turnStart).Milliseconds(), state); persistErr != nil {
				r.logger.Error("spawn: failed to persist child failure", "session", sessionID, "error", persistErr)
			}
		}
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
	if strings.TrimSpace(output) == "" {
		output = "ℹ️ Ajan bu spawn için boş yanıt döndürdü."
	}
	var addErr error
	if opts.ChildSession != nil {
		state := "completed"
		if outcome.Truncated() {
			state = outcome.Status
		}
		addErr = r.recordChildAssistantMessage(ctx, sessionID, agent.ID, output, steps, meta, time.Since(turnStart).Milliseconds(), state)
	} else {
		output, addErr = r.recordAssistantReply(ctx, sessionID, agent.ID, output, steps, meta, time.Since(turnStart).Milliseconds(), "ℹ️ Ajan bu spawn için boş yanıt döndürdü.")
	}
	if addErr != nil {
		r.logger.Warn("spawn: failed to record reply", "session", sessionID, "error", addErr)
		if opts.ChildSession != nil {
			r.emitDebug(turnCtx, db.DebugEvent{Type: db.DebugError, AgentID: agent.ID, Detail: debugSummary(addErr.Error(), 500), Err: true})
			r.emitSpawnEvent(agent, sessionID, prompt, false)
			r.AutoTagTurn(ctx, sessionID, steps, "spawn_error")
			return
		}
	}
	r.logger.Info("spawn: finished", "session", sessionID, "agent", agent.ID, "status", outcome.Status)
	// Self-completion: if the spawned turn stalled with unfinished work (activated
	// tools it never used, or open todos), keep it going — there is no human to send
	// the follow-up. No-op when the turn finished cleanly, and skipped outright when
	// it was cut short. Bounded + budget-gated.
	r.maybeAutoContinue(ctx, agent, sessionID, KindSpawn, steps, outcome.Truncated())
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
func (r *Runtime) releaseSpawnSlot() {
	r.spawnActive.Add(-1)
	r.signalSpawnQueue()
}

// spawnTitle derives a short, single-line title from a spawn prompt (rune-capped,
// so Turkish characters never get split). The auto-titler can refine it later.
func spawnTitle(prompt string) string {
	t := notifyLine(prompt, 60)
	if t == "" {
		return "Spawn"
	}
	return t
}
