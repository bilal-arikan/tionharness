package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// coordination.go implements the M2 coordinator/worker method (see _Docs/47).
//
// A coordinator session spawns WORKER sessions (spawn_worker) that run detached,
// history-aware turns. When a worker's turn finishes (success, failure, or a
// stop_worker cancellation) it does NOT return into the caller's turn; instead it
// injects a <task-notification> user message into the coordinator session and
// asks the per-session turn queue to run one coordinator turn. Concurrent worker
// completions therefore serialize into single coordinator turns, and any that
// pile up while the coordinator is mid-turn coalesce into the next one (their
// notifications are already in history) — this is the whole point of coordSlot.

// coordSlot serializes coordinator turns for one coordinator session and counts
// its active workers. Guarded by mu except workers (atomic, touched from the
// spawn path without the turn lock).
type coordSlot struct {
	mu      sync.Mutex
	free    *sync.Cond   // lazily created; broadcast whenever running flips false
	running    bool      // a turn (auto OR interactive) is currently executing
	pending    bool      // >=1 notification arrived while running; run once more after
	ackedIdle  bool      // ran the "all workers idle" reconcile turn for this batch
	hadWorkers bool      // at least one worker was ever spawned (gates the idle sweep)
	turns      int       // auto-triggered coordinator turns so far (notify-loop cap)
	capWarn bool         // whether the "cap reached" warning has been posted
	workers atomic.Int64 // active workers under this coordinator
}

// signalFree wakes turns blocked in BeginCoordinatorUserTurn. Callers must hold mu.
func (s *coordSlot) signalFree() {
	if s.free != nil {
		s.free.Broadcast()
	}
}

// markHadWorkers records that this coordinator has spawned at least one worker, so
// the idle-reconcile sweep only ever fires for a coordinator that actually has
// workers to reconcile (never for a plain no-worker session).
func (s *coordSlot) markHadWorkers() {
	s.mu.Lock()
	s.hadWorkers = true
	s.mu.Unlock()
}

// workerCtl lets stop_worker cancel an in-flight worker turn and mark it stopped
// so it reports "killed" rather than "failed".
type workerCtl struct {
	cancel  context.CancelFunc
	stopped atomic.Bool
}

// sessionRole returns the Role of the session stamped on ctx ("coordinator" /
// "worker" / ""), or "" when no session is stamped or it can't be loaded.
func (r *Runtime) sessionRole(ctx context.Context) string {
	sid := SessionIDFrom(ctx)
	if sid == "" {
		return ""
	}
	s, err := r.db.GetSession(ctx, sid)
	if err != nil {
		return ""
	}
	return s.Role
}

// withCoordination wires the coordinator/worker tools for one turn — but ONLY when
// the turn runs on a coordinator session. It captures the coordinator session id
// (from ctx) and the caller so the tools carry just the worker-facing arguments.
// A non-coordinator turn returns ctx unchanged, so buildRegistry never registers
// the coordination tools there (and a worker can't spawn workers).
func (r *Runtime) withCoordination(ctx context.Context, caller db.Agent) context.Context {
	coordID := SessionIDFrom(ctx)
	if coordID == "" || r.sessionRole(ctx) != "coordinator" {
		return ctx
	}
	return tools.WithCoordination(ctx, r.coordinationFuncsFor(coordID, caller.ID))
}

// coordinationFuncsFor builds the coordination runner bound to a coordinator
// session + caller. Shared by the native path (withCoordination) and the CLI
// bridge (BridgeTools), so both dispatch the coordinator tools identically.
func (r *Runtime) coordinationFuncsFor(coordID, callerID string) *tools.CoordinationFuncs {
	return &tools.CoordinationFuncs{
		Spawn: func(c context.Context, agentRef, task, model string) (tools.SpawnResult, error) {
			res, err := r.SpawnWorker(c, coordID, agentRef, task, model, callerID)
			if err != nil {
				return tools.SpawnResult{}, err
			}
			return tools.SpawnResult{SessionID: res.SessionID, AgentName: res.AgentName}, nil
		},
		Send: func(c context.Context, workerSessionID, message string) error {
			return r.SendToWorker(c, coordID, workerSessionID, message)
		},
		Stop: func(c context.Context, workerSessionID string) error {
			return r.StopWorker(workerSessionID)
		},
		List: func(c context.Context) (string, error) {
			ws, err := r.ListWorkers(c, coordID)
			if err != nil {
				return "", err
			}
			return formatWorkerList(ws), nil
		},
	}
}

// coordinationBridgeDefs returns the coordinator tool defs for the CLI bridge.
func coordinationBridgeDefs() []providers.ToolDef {
	return []providers.ToolDef{
		tools.NewSpawnWorkerTool().Def(),
		tools.NewSendToWorkerTool().Def(),
		tools.NewStopWorkerTool().Def(),
		tools.NewListWorkersTool().Def(),
	}
}

// dispatchCoordinationBridge handles a coordinator tool call on the CLI bridge:
// it wires the coordination runner into ctx and invokes the matching tool. The
// bool reports whether name was a coordination tool (so the caller can fall
// through to the normal registry dispatch otherwise).
func dispatchCoordinationBridge(ctx context.Context, f *tools.CoordinationFuncs, name string, args json.RawMessage) (string, bool, error) {
	ctx = tools.WithCoordination(ctx, f)
	type caller interface {
		Call(context.Context, json.RawMessage) (string, error)
	}
	var t caller
	switch name {
	case "spawn_worker":
		t = tools.NewSpawnWorkerTool()
	case "send_to_worker":
		t = tools.NewSendToWorkerTool()
	case "stop_worker":
		t = tools.NewStopWorkerTool()
	case "list_workers":
		t = tools.NewListWorkersTool()
	default:
		return "", false, nil
	}
	out, err := t.Call(ctx, args)
	return out, true, err
}

// formatWorkerList renders a coordinator's workers as a compact status list for
// the list_workers tool.
func formatWorkerList(ws []WorkerInfo) string {
	if len(ws) == 0 {
		return "No workers have been spawned under this coordinator yet."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d worker(s):\n", len(ws))
	for _, w := range ws {
		status := "finished"
		if w.Running {
			status = "running"
		}
		fmt.Fprintf(&b, "- %s [%s] (%s)", w.AgentName, status, w.SessionID)
		if w.Summary != "" {
			fmt.Fprintf(&b, " — %s", w.Summary)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// coordSlotFor returns (creating if needed) the slot for a coordinator session.
func (r *Runtime) coordSlotFor(coordSessionID string) *coordSlot {
	v, _ := r.coordSlots.LoadOrStore(coordSessionID, &coordSlot{})
	return v.(*coordSlot)
}

// isSessionActive reports whether a session is currently running an autonomous
// invoke (used by ListWorkers to distinguish running from finished workers).
func (r *Runtime) isSessionActive(id string) bool {
	_, ok := r.activeSessions.Load(id)
	return ok
}

// SpawnWorker launches a background worker under a coordinator session. The
// target must be an EXISTING agent (name or id) — a worker needs a persistent
// session, so ephemeral run_subagent profiles (explore/coder/reviewer) are not
// valid worker targets; use run_subagent (the sync M1 method) for those. Returns
// the worker session id immediately; the turn runs detached and notifies the
// coordinator on completion. Enforces CoordinatorMaxWorkers per coordinator on
// top of the global spawn concurrency cap.
func (r *Runtime) SpawnWorker(ctx context.Context, coordSessionID, agentRef, task, modelOverride, createdBy string) (SpawnResult, error) {
	coordSessionID = strings.TrimSpace(coordSessionID)
	if coordSessionID == "" {
		return SpawnResult{}, fmt.Errorf("spawn_worker requires a coordinator session")
	}
	// A profile target (explore/coder/reviewer) is materialized into a persisted,
	// reusable worker agent so the worker has a real session to run in. An ordinary
	// target passes through unchanged (existing agent name/id). Resolved BEFORE the
	// worker-count reservation so a bad target never leaks a slot.
	agentRef, err := r.resolveWorkerTarget(ctx, coordSessionID, createdBy, agentRef)
	if err != nil {
		return SpawnResult{}, err
	}
	slot := r.coordSlotFor(coordSessionID)
	max := int64(r.tun.CoordinatorMaxWorkers())
	if slot.workers.Add(1) > max {
		slot.workers.Add(-1)
		return SpawnResult{}, fmt.Errorf("coordinator worker limit reached (%d active); wait for some to finish before spawning more", max)
	}
	res, err := r.SpawnSession(ctx, agentRef, task, SpawnOptions{
		ModelOverride:        modelOverride,
		CreatedBy:            createdBy,
		CoordinatorSessionID: coordSessionID,
		Role:                 "worker",
	})
	if err != nil {
		// SpawnSession never launched runWorker, so release the reservation here.
		slot.workers.Add(-1)
		return SpawnResult{}, err
	}
	slot.markHadWorkers()
	return res, nil
}

// resolveWorkerTarget maps a spawn_worker target to a runnable persistent agent
// id. An existing agent (name or id) passes through. A built-in profile
// (explore/coder/reviewer) is materialized once into a reusable persisted worker
// agent — "worker:<profile>" — cloned from the base agent (the coordinator's own
// agent: provider/model/permission) but reshaped with the profile's soul and tool
// allowlist. Reused on subsequent spawns (find-or-create, serialized).
func (r *Runtime) resolveWorkerTarget(ctx context.Context, coordSessionID, baseAgentID, target string) (string, error) {
	target = strings.TrimSpace(target)
	prof, isProfile := defaultSubagentProfiles[strings.ToLower(target)]
	if !isProfile {
		// Ordinary target: must be an existing agent.
		a, err := r.resolveAgent(ctx, target)
		if err != nil {
			return "", err
		}
		return a.ID, nil
	}

	name := "worker:" + prof.ID
	r.profileWorkerMu.Lock()
	defer r.profileWorkerMu.Unlock()
	// Already materialized? Reuse it.
	if a, err := r.resolveAgent(ctx, name); err == nil {
		return a.ID, nil
	}
	// Clone provider/model/permission from the base agent (coordinator's agent), or
	// the coordinator session's agent when no base id was supplied.
	if strings.TrimSpace(baseAgentID) == "" {
		if sess, err := r.db.GetSession(ctx, coordSessionID); err == nil {
			baseAgentID = sess.AgentID
		}
	}
	base, err := r.db.GetAgent(ctx, baseAgentID)
	if err != nil {
		return "", fmt.Errorf("cannot materialize worker profile %q: base agent unavailable: %w", prof.ID, err)
	}
	allow, _ := json.Marshal(prof.AllowedTools)
	created, err := r.db.CreateAgent(ctx, db.Agent{
		Name:           name,
		Soul:           prof.SystemPrompt,
		Provider:       base.Provider,
		Model:          base.Model,
		PermissionMode: base.PermissionMode,
		MCPEnabled:     true,
		AllowedTools:   string(allow),
		CreatedBy:      baseAgentID,
	})
	if err != nil {
		return "", fmt.Errorf("cannot materialize worker profile %q: %w", prof.ID, err)
	}
	r.logger.Info("coordination: materialized profile worker", "profile", prof.ID, "agent", created.ID)
	return created.ID, nil
}

// SendToWorker appends a follow-up message to an existing worker session and runs
// its turn again (history-aware, so it continues with full context), notifying the
// coordinator on completion — the "continue" mechanism (Claude Code's SendMessage
// to a worker). The worker must belong to this coordinator.
func (r *Runtime) SendToWorker(ctx context.Context, coordSessionID, workerSessionID, message string) error {
	coordSessionID = strings.TrimSpace(coordSessionID)
	workerSessionID = strings.TrimSpace(workerSessionID)
	message = strings.TrimSpace(message)
	if workerSessionID == "" || message == "" {
		return fmt.Errorf("send_to_worker requires a worker session id and a message")
	}
	ws, err := r.db.GetSession(ctx, workerSessionID)
	if err != nil {
		return fmt.Errorf("worker session %s not found: %w", workerSessionID, err)
	}
	if ws.CoordinatorSessionID != coordSessionID {
		return fmt.Errorf("session %s is not a worker of this coordinator", workerSessionID)
	}
	if r.isSessionActive(workerSessionID) {
		return fmt.Errorf("worker %s is still running its previous turn; wait for it to finish or stop_worker first", workerSessionID)
	}
	agent, err := r.db.GetAgent(ctx, ws.AgentID)
	if err != nil {
		return fmt.Errorf("worker agent gone: %w", err)
	}
	if !r.acquireSpawnSlot() {
		return fmt.Errorf("background turn limit reached; try again once some finish")
	}
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: workerSessionID,
		Role:      "user",
		Text:      message,
	}); err != nil {
		r.releaseSpawnSlot()
		return err
	}
	slot := r.coordSlotFor(coordSessionID)
	slot.workers.Add(1)
	slot.markHadWorkers()
	go r.runWorker(agent, workerSessionID, message, coordSessionID)
	return nil
}

// StopWorker cancels an in-flight worker turn. The worker's current turn ends and
// reports "killed" to the coordinator; the worker session survives and can be
// continued later with send_to_worker.
func (r *Runtime) StopWorker(workerSessionID string) error {
	workerSessionID = strings.TrimSpace(workerSessionID)
	v, ok := r.workerCancels.Load(workerSessionID)
	if !ok {
		return fmt.Errorf("worker %s is not running (already finished?)", workerSessionID)
	}
	ctl := v.(*workerCtl)
	ctl.stopped.Store(true)
	ctl.cancel()
	return nil
}

// WorkerInfo is a coordinator-facing snapshot of one worker session.
type WorkerInfo struct {
	SessionID string
	AgentName string
	Title     string
	Running   bool
	Summary   string // first line of the worker's latest reply, when finished
}

// ListWorkers returns the workers spawned under a coordinator session, newest
// first, each tagged running or finished (with a one-line summary of its last
// reply).
func (r *Runtime) ListWorkers(ctx context.Context, coordSessionID string) ([]WorkerInfo, error) {
	sessions, err := r.db.ListSessions(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []WorkerInfo
	for _, s := range sessions {
		if s.CoordinatorSessionID != coordSessionID {
			continue
		}
		info := WorkerInfo{
			SessionID: s.ID,
			AgentName: r.agentName(s.AgentID),
			Title:     s.Title,
			Running:   r.isSessionActive(s.ID),
		}
		if !info.Running {
			if msgs, err := r.db.ListMessages(ctx, s.ID); err == nil {
				for i := len(msgs) - 1; i >= 0; i-- {
					if msgs[i].Role == "assistant" {
						info.Summary = notifyLine(msgs[i].Text, 120)
						break
					}
				}
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// runWorker executes a worker's background turn (initial spawn or a send_to_worker
// continuation): it runs a history-aware turn, records the reply (or the failure)
// as an assistant turn, releases the concurrency slots, and — crucially — injects a
// <task-notification> into the coordinator session and triggers a coordinator turn.
// It mirrors runSpawn but is coordinator-aware and notifies on EVERY outcome
// (completed / failed / killed), unlike a plain spawn.
func (r *Runtime) runWorker(agent db.Agent, workerSessionID, prompt, coordSessionID string) {
	defer r.releaseSpawnSlot()
	if slot := r.coordSlotFor(coordSessionID); slot != nil {
		defer slot.workers.Add(-1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), spawnTimeout)
	defer cancel()
	ctl := &workerCtl{cancel: cancel}
	r.workerCancels.Store(workerSessionID, ctl)
	defer r.workerCancels.Delete(workerSessionID)

	turnCtx := tools.WithAsyncChat(WithSessionID(WithCallKind(ctx, KindSpawn), workerSessionID))
	turnCtx, meta := WithTurnMeta(turnCtx)
	turnStart := time.Now()

	r.trackSession(workerSessionID)
	// Raise the "thinking" indicator for the worker session (see emitTurnStart);
	// the completion "worker" event clears it.
	r.emitTurnStart(workerSessionID, "🤝 Worker turu çalışıyor")
	output, steps, err := r.runSessionTurn(turnCtx, agent, workerSessionID, prompt, true)
	r.untrackSession(workerSessionID)

	status := "completed"
	replyText := output
	if err != nil {
		if ctl.stopped.Load() {
			status = "killed"
			replyText = "⏹️ Worker turu koordinatör tarafından durduruldu."
		} else {
			status = "failed"
			replyText = "⚠️ Worker turu çalıştırılamadı:\n\n" + err.Error()
		}
		r.logger.Warn("worker: turn ended", "session", workerSessionID, "coordinator", coordSessionID, "status", status, "error", err)
	} else if strings.TrimSpace(replyText) == "" {
		replyText = "ℹ️ Worker bu tur için boş yanıt döndürdü."
	}

	replyMsg := db.Message{
		SessionID: workerSessionID,
		AgentID:   agent.ID,
		Role:      "assistant",
		Text:      replyText,
		Steps:     encodeSteps(steps),
	}
	meta.apply(&replyMsg, time.Since(turnStart).Milliseconds())
	if _, addErr := r.db.AddMessage(ctx, replyMsg); addErr != nil {
		r.logger.Warn("worker: failed to record reply", "session", workerSessionID, "error", addErr)
	}
	r.emitWorkerEvent(agent, workerSessionID, coordSessionID, status)

	// The report the coordinator actually reads: the successful output verbatim, or
	// the failure/kill note (so the coordinator can react to failures too).
	note := formatTaskNotification(workerSessionID, agent.Name, status, replyText, countToolSteps(steps), time.Since(turnStart).Milliseconds())
	r.NotifyCoordinator(coordSessionID, note)
}

// NotifyCoordinator persists a <task-notification> as a user message in the
// coordinator session, then asks the per-session turn queue to run a coordinator
// turn. Safe to call from many workers concurrently: the queue serializes turns
// and coalesces pile-ups. No-op when coordSessionID is empty.
func (r *Runtime) NotifyCoordinator(coordSessionID, note string) {
	coordSessionID = strings.TrimSpace(coordSessionID)
	if coordSessionID == "" || strings.TrimSpace(note) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: coordSessionID,
		Role:      "user",
		Origin:    "worker-note",
		Text:      note,
	}); err != nil {
		cancel()
		r.logger.Warn("coordination: failed to record task-notification", "coordinator", coordSessionID, "error", err)
		return
	}
	cancel()
	r.enqueueCoordinatorTurn(coordSessionID)
}

// BeginCoordinatorUserTurn claims the coordinator's turn slot for an interactive
// (user-initiated) turn so it never overlaps an auto-triggered coordinator turn:
// auto turns arriving meanwhile fall into pending (enqueueCoordinatorTurn sees
// running=true), and this call blocks until any in-flight auto turn — bounded by
// spawnTimeout — finishes. The returned release func MUST be called when the
// interactive turn ends (defer it); it frees the slot and, when worker
// notifications piled up mid-turn, schedules exactly one coordinator turn to
// process them. A user turn also resets the auto-turn cap: a human is back in
// the loop, so notifications may resume triggering turns after a cap stop.
func (r *Runtime) BeginCoordinatorUserTurn(coordSessionID string) (release func()) {
	return r.claimCoordinatorSlot(coordSessionID, true)
}

// claimTurnSlotIfCoordinator claims the coordinator turn slot when sessionID
// belongs to a coordinator session, so autonomous turns (scheduler wake, scheduled
// prompt, inbox delivery) serialize with coordinator auto turns exactly like
// interactive chat turns. Returns a release func — a no-op for non-coordinator
// sessions (or when the session can't be loaded, since there is then no
// coordinator slot to protect). Unlike a user turn it does NOT reset the
// auto-turn cap: no human re-entered the loop.
func (r *Runtime) claimTurnSlotIfCoordinator(ctx context.Context, sessionID string) (release func()) {
	s, err := r.db.GetSession(ctx, sessionID)
	if err != nil || s.Role != "coordinator" {
		return func() {}
	}
	return r.claimCoordinatorSlot(sessionID, false)
}

// claimCoordinatorSlot blocks until the coordinator's turn slot is free, claims
// it, and returns the release func (see BeginCoordinatorUserTurn for semantics).
// resetCap additionally zeroes the auto-turn budget (human back in the loop).
func (r *Runtime) claimCoordinatorSlot(coordSessionID string, resetCap bool) func() {
	slot := r.coordSlotFor(coordSessionID)
	slot.mu.Lock()
	if slot.free == nil {
		slot.free = sync.NewCond(&slot.mu)
	}
	for slot.running {
		slot.free.Wait()
	}
	slot.running = true
	if resetCap {
		slot.turns = 0
		slot.capWarn = false
	}
	slot.mu.Unlock()
	return func() {
		slot.mu.Lock()
		slot.running = false
		pending := slot.pending
		slot.pending = false
		slot.signalFree()
		slot.mu.Unlock()
		if pending {
			r.enqueueCoordinatorTurn(coordSessionID)
		}
	}
}

// enqueueCoordinatorTurn schedules one coordinator turn. If a turn is already
// running it just flags pending (the running turn will loop once more and see the
// freshly-persisted notification in history). Otherwise it starts the drain loop.
func (r *Runtime) enqueueCoordinatorTurn(coordSessionID string) {
	slot := r.coordSlotFor(coordSessionID)
	slot.mu.Lock()
	// A fresh notification (a worker just finished or continued) re-arms the
	// idle-reconcile sweep: this batch is no longer "acknowledged idle".
	slot.ackedIdle = false
	if slot.running {
		slot.pending = true
		slot.mu.Unlock()
		return
	}
	slot.running = true
	slot.mu.Unlock()
	go r.drainCoordinator(coordSessionID, slot)
}

// drainCoordinator runs coordinator turns until no more notifications are pending,
// bounded by CoordinatorMaxTurns (the notify-loop guard). Each iteration runs one
// history-aware turn that sees every notification persisted so far.
func (r *Runtime) drainCoordinator(coordSessionID string, slot *coordSlot) {
	for {
		slot.mu.Lock()
		if slot.turns >= r.tun.CoordinatorMaxTurns() {
			warn := !slot.capWarn
			slot.capWarn = true
			slot.running = false
			slot.pending = false
			slot.signalFree()
			slot.mu.Unlock()
			if warn {
				r.warnCoordinatorCap(coordSessionID, slot.turns)
			}
			return
		}
		slot.turns++
		slot.mu.Unlock()

		if r.coordRunFn != nil {
			r.coordRunFn(coordSessionID)
		} else {
			r.runCoordinatorTurn(coordSessionID)
		}

		slot.mu.Lock()
		if slot.pending {
			slot.pending = false
			slot.mu.Unlock()
			continue
		}
		// Idle reconciliation (liveness backstop): if EVERY worker is now finished
		// and we have not yet run a reconcile turn for this all-idle transition,
		// inject an authoritative "all workers finished" note and loop ONCE more.
		// This guarantees the coordinator gets a final, unambiguous turn even when
		// it overlooked one notification in a coalesced batch — breaking the "waits
		// forever on an already-finished worker" stall. One-shot per all-idle
		// transition (ackedIdle, re-armed by the next notification) and bounded by
		// CoordinatorMaxTurns (checked at the loop top), so it can never loop.
		if !slot.ackedIdle && slot.hadWorkers && slot.workers.Load() == 0 {
			slot.ackedIdle = true
			slot.mu.Unlock()
			r.appendCoordinationStatus(coordSessionID)
			continue
		}
		slot.running = false
		slot.signalFree()
		slot.mu.Unlock()
		return
	}
}

// appendCoordinationStatus persists a one-shot <coordination-status> note into the
// coordinator session so the idle-reconcile turn opens on an explicit, authoritative
// signal that every worker has finished. Purely a history append (no enqueue): the
// caller is already inside the drain loop and continues to the next turn.
func (r *Runtime) appendCoordinationStatus(coordSessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const note = "<coordination-status>All workers under this coordinator have finished. " +
		"Act on any results you have not handled yet, spawn the next steps if the plan has more, " +
		"or conclude the project. Do NOT wait for a worker that has already finished.</coordination-status>"
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: coordSessionID,
		Role:      "user",
		Origin:    "worker-note",
		Text:      note,
	}); err != nil {
		r.logger.Warn("coordination: failed to record idle status note", "coordinator", coordSessionID, "error", err)
	}
}

// coordinatorWorkerStatusBlock renders an authoritative, always-fresh snapshot of
// every worker under coordSessionID for the coordinator's dynamic system suffix.
// Unlike the prose <task-notification>s in history — which a coalesced batch can
// let the model overlook — this block is regenerated every turn from live session
// state, so the coordinator can never believe a finished worker is still running.
// Empty when the session has no workers.
func (r *Runtime) coordinatorWorkerStatusBlock(ctx context.Context, coordSessionID string) string {
	ws, err := r.ListWorkers(ctx, coordSessionID)
	if err != nil || len(ws) == 0 {
		return ""
	}
	running, finished := 0, 0
	var b strings.Builder
	b.WriteString("# Worker status (live, authoritative)\n")
	b.WriteString("Regenerated every turn from real session state; trust THIS over the notifications in history.\n")
	for _, w := range ws {
		status := "finished"
		if w.Running {
			status = "RUNNING"
			running++
		} else {
			finished++
		}
		fmt.Fprintf(&b, "- %s [%s] (%s)", w.AgentName, status, w.SessionID)
		if w.Summary != "" {
			fmt.Fprintf(&b, " — %s", w.Summary)
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "Summary: %d running, %d finished.", running, finished)
	if running == 0 {
		b.WriteString(" ALL workers are finished — there is NO running worker to wait for; spawn the remaining steps or conclude.")
	}
	return b.String()
}

// runCoordinatorTurn runs one history-aware turn for the coordinator session so it
// synthesizes the worker notifications now sitting in its history, then records the
// reply and fires the turn-finished hook (for tags/automations on the coordinator).
func (r *Runtime) runCoordinatorTurn(coordSessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), spawnTimeout)
	defer cancel()

	sess, err := r.db.GetSession(ctx, coordSessionID)
	if err != nil {
		r.logger.Warn("coordination: coordinator session gone", "coordinator", coordSessionID, "error", err)
		return
	}
	agent, err := r.db.GetAgent(ctx, sess.AgentID)
	if err != nil {
		r.logger.Warn("coordination: coordinator agent gone", "coordinator", coordSessionID, "error", err)
		return
	}

	turnCtx := tools.WithAsyncChat(WithSessionID(WithCallKind(ctx, KindSpawn), coordSessionID))
	turnCtx, meta := WithTurnMeta(turnCtx)
	turnStart := time.Now()

	r.trackSession(coordSessionID)
	output, steps, err := r.runSessionTurn(turnCtx, agent, coordSessionID, "", true)
	r.untrackSession(coordSessionID)

	text := output
	if err != nil {
		text = "⚠️ Koordinatör turu çalıştırılamadı:\n\n" + err.Error()
		r.logger.Error("coordination: coordinator turn failed", "coordinator", coordSessionID, "error", err)
	} else if strings.TrimSpace(text) == "" {
		text = "ℹ️ Koordinatör bu tur için boş yanıt döndürdü."
	}
	replyMsg := db.Message{
		SessionID: coordSessionID,
		AgentID:   agent.ID,
		Role:      "assistant",
		Text:      text,
		Steps:     encodeSteps(steps),
	}
	meta.apply(&replyMsg, time.Since(turnStart).Milliseconds())
	if _, addErr := r.db.AddMessage(ctx, replyMsg); addErr != nil {
		r.logger.Warn("coordination: failed to record coordinator reply", "coordinator", coordSessionID, "error", addErr)
	}
	r.publish(events.Event{
		Type:   "chat",
		Level:  "info",
		Title:  "🧭 Koordinatör turu tamamlandı — " + agent.Name,
		Target: map[string]string{"view": "executions", "sessionId": coordSessionID},
	})
	// Coordinator turns are ordinary turns for the rest of the system: fire the hook
	// so tags/automations on the coordinator session still work. The coordinator has
	// no CoordinatorSessionID, so this never re-enters the coordination loop.
	if err == nil {
		r.FireTurnFinished(coordSessionID, agent.ID, output)
	}
}

// warnCoordinatorCap posts a one-time notice that a coordinator hit its auto-turn
// cap; further worker notifications are still persisted but stop auto-triggering
// turns, so the user can inspect and continue manually.
func (r *Runtime) warnCoordinatorCap(coordSessionID string, turns int) {
	r.logger.Warn("coordination: coordinator auto-turn cap reached", "coordinator", coordSessionID, "turns", turns)
	r.publish(events.Event{
		Type:   "coordination",
		Level:  "info",
		Title:  "🧭 Koordinatör tur limiti",
		Body:   fmt.Sprintf("Otomatik koordinatör turları limiti (%d) aşıldı; yeni worker bildirimleri kaydediliyor ama otomatik tur tetiklenmiyor. Devam etmek için oturuma manuel mesaj gönderin.", turns),
		Target: map[string]string{"view": "executions", "sessionId": coordSessionID},
	})
}

// emitWorkerEvent publishes a worker status transition so the coordination UI can
// live-update its worker cards.
func (r *Runtime) emitWorkerEvent(agent db.Agent, workerSessionID, coordSessionID, status string) {
	level := "success"
	if status == "failed" {
		level = "error"
	} else if status == "killed" {
		level = "info"
	}
	r.publish(events.Event{
		Type:  "worker",
		Level: level,
		Title: "🤖 Worker " + status + " — " + agent.Name,
		Target: map[string]string{
			"view":          "executions",
			"sessionId":     workerSessionID,
			"coordinatorId": coordSessionID,
		},
	})
}

// formatTaskNotification renders a worker outcome as the <task-notification> XML
// the coordinator reads (mirrors Claude Code's coordinator format). status is
// completed | failed | killed; result is the worker's final text.
func formatTaskNotification(workerSessionID, agentName, status, result string, toolUses int, durationMs int64) string {
	var b strings.Builder
	b.WriteString("<task-notification>\n")
	fmt.Fprintf(&b, "<task-id>%s</task-id>\n", workerSessionID)
	fmt.Fprintf(&b, "<agent>%s</agent>\n", agentName)
	fmt.Fprintf(&b, "<status>%s</status>\n", status)
	fmt.Fprintf(&b, "<summary>Worker %q %s</summary>\n", agentName, status)
	if strings.TrimSpace(result) != "" {
		fmt.Fprintf(&b, "<result>%s</result>\n", strings.TrimSpace(result))
	}
	b.WriteString("<usage>")
	fmt.Fprintf(&b, "<tool_uses>%d</tool_uses><duration_ms>%d</duration_ms>", toolUses, durationMs)
	b.WriteString("</usage>\n")
	b.WriteString("</task-notification>")
	return b.String()
}

// countToolSteps counts tool invocations in a turn trace (for the usage section).
func countToolSteps(steps []TurnStep) int {
	n := 0
	for _, s := range steps {
		if s.Kind == StepTool {
			n++
		}
	}
	return n
}
