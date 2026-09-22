// Coordinator turn scheduling: the batching window that folds worker notes into one wake, the drain loop that runs coordinator turns while notes keep arriving, the settle backstop, and the coordinator turn itself.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
	"github.com/bilal-arikan/tionharness/internal/view"
)

// enqueueCoordinatorTurn schedules an immediate coordinator turn for generic
// wake sources such as flow start, recovery, resume and stall nudges.
func (r *Runtime) enqueueCoordinatorTurn(coordSessionID string) {
	r.enqueueCoordinatorTurnKeepingIdleAck(coordSessionID, false)
}

// enqueueCoordinatorTurnKeepingIdleAck is enqueueCoordinatorTurn with control over
// the all-idle claim. keepIdleAck=true is used by exactly one caller: the
// notification that ALREADY carries the folded <coordination-status> note. Without
// it the default re-arm below would clear the ackedIdle that notification just
// claimed, allowing a later notification from the same wave to duplicate it.
func (r *Runtime) enqueueCoordinatorTurnKeepingIdleAck(coordSessionID string, keepIdleAck bool) {
	r.enqueueCoordinatorWake(coordSessionID, keepIdleAck, false, time.Time{})
}

// enqueueCoordinatorWorkerTurn opens a fixed batching window on the first worker
// note. Later notes join that batch without moving its deadline.
func (r *Runtime) enqueueCoordinatorWorkerTurn(coordSessionID string, keepIdleAck bool) {
	r.enqueueCoordinatorWorkerTurnAt(coordSessionID, keepIdleAck, time.Now())
}

func (r *Runtime) enqueueCoordinatorWorkerTurnAt(coordSessionID string, keepIdleAck bool, persistedAt time.Time) {
	r.enqueueCoordinatorWake(coordSessionID, keepIdleAck, true, persistedAt)
}

func (r *Runtime) enqueueCoordinatorWake(coordSessionID string, keepIdleAck, worker bool, persistedAt time.Time) {
	// Archive is a HARD stop on automatic turns, and it has to be enforced here --
	// the wake entry point -- not only in RecoverOrphanedTurns. WHY: on 2026-08-27
	// archiving the three runaway coordinators in WS5 was not enough. Their orphaned
	// workers are still reclaimed at boot, and each reclaim calls NotifyCoordinator,
	// which lands right here and starts a drain on a session the user had explicitly
	// archived. The whole subtree had to be archived by hand to break the loop.
	// The note itself is already durably recorded by the caller, so nothing is lost:
	// un-archiving and resuming replays it.
	if sess, err := r.db.GetSession(context.Background(), coordSessionID); err == nil && sess.State == "archived" {
		r.logger.Info("coordination: skipping turn, coordinator session is archived", "session", coordSessionID)
		return
	}
	slot := r.coordSlotFor(coordSessionID)
	slot.mu.Lock()
	// A hard stall halt must gate the wake entry point, not only a drain already in
	// progress. Worker notes remain durably recorded, but cannot silently start a
	// fresh automatic drain while the UI says auto-turns are stopped. Resume clears
	// the flag and explicitly enqueues the next turn, preserving all queued notes.
	if slot.stallHalted {
		slot.mu.Unlock()
		return
	}
	// Re-arm the all-idle claim for the NEXT transition, UNLESS the
	// current one has already been reported by a folded notification (idleFolded).
	//
	// Clearing unconditionally is what let the fold be undone: the last worker folds
	// the note and claims ackedIdle, then its already-finished siblings' notifications
	// cleared the claim and duplicated the status. idleFolded is narrow: it suppresses re-arming for exactly the
	// transition a fold already covered, and a spawn/continuation clears it.
	if !keepIdleAck && !slot.idleFolded {
		slot.ackedIdle = false
	}
	if worker {
		if !slot.workerPending {
			slot.workerPending = true
			slot.workerDeadline = persistedAt.Add(r.tun.CoordinatorWorkerBatchWindow())
		}
	} else {
		slot.pending = true
	}
	select {
	case slot.wake <- struct{}{}:
	default:
	}
	if slot.driving {
		slot.mu.Unlock()
		return
	}
	slot.driving = true
	slot.mu.Unlock()
	// Registered BEFORE the goroutine starts: NotifyCoordinator returns into the worker
	// path immediately, so a "Durdur" in the very next instant must already find this
	// drain's cancel — not a nil map entry (the same defect closed for spawn, worker,
	// wake, scheduled, automation and peer inbox turns).
	runCtx, cancelRun := context.WithCancel(context.Background())
	run := r.trackSession(coordSessionID, cancelRun)
	// startCoordinatorDrain refuses once CloseMCP has begun draining. The arming state
	// and the registration we just minted must then be undone, or the slot stays
	// "driving" with no goroutine to drive it and the session looks active forever.
	if !r.startCoordinatorDrain(coordSessionID, slot, runCtx, cancelRun, run) {
		run.release()
		cancelRun()
		r.clearCoordinatorDrain(slot)
	}
}

func (r *Runtime) clearCoordinatorDrain(slot *coordSlot) {
	slot.mu.Lock()
	r.clearCoordinatorDrainLocked(slot)
	slot.mu.Unlock()
}

func (r *Runtime) clearCoordinatorDrainLocked(slot *coordSlot) {
	slot.pending = false
	slot.workerPending = false
	slot.workerDeadline = time.Time{}
	slot.stopRequested = false
	slot.driving = false
}

// waitCoordinatorBatch waits outside admission. Generic wakes interrupt the
// timer; worker wakes only cause a deadline recheck and never extend it.
func (r *Runtime) waitCoordinatorBatch(ctx context.Context, slot *coordSlot) bool {
	for {
		slot.mu.Lock()
		if slot.stallHalted {
			r.clearCoordinatorDrainLocked(slot)
			slot.mu.Unlock()
			return false
		}
		if slot.pending {
			slot.mu.Unlock()
			return true
		}
		if !slot.workerPending {
			r.clearCoordinatorDrainLocked(slot)
			slot.mu.Unlock()
			return false
		}
		wait := time.Until(slot.workerDeadline)
		slot.mu.Unlock()
		if wait <= 0 {
			return true
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			slot.mu.Lock()
			r.clearCoordinatorDrainLocked(slot)
			slot.mu.Unlock()
			return false
		case <-slot.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			return true
		}
	}
}

// drainCoordinator runs coordinator turns until no more wakes are pending,
// bounded by CoordinatorMaxTurns. Worker batching never holds admission.
//
// The loop re-enters the session's admission queue EVERY iteration rather than
// holding the slot across the drain. That is the fairness property: a message the
// user queued mid-drain is already in the FIFO, so it runs after the current turn —
// not after the whole drain. Nothing is lost by yielding; wake intent is tracked
// separately for immediate work and fixed-deadline worker batches.
//
// ctx is the RUNTIME-scoped coordinator context (CloseMCP cancels it). It bounds the
// batch wait, the admission wait and the session read — the parts that must end when
// the workspace shuts down, not when one turn is stopped.
//
// runCtx/cancelRun/run are the FIRST iteration's turn context and its registration,
// minted by the caller before the `go` so a stop issued in the instant after enqueue
// still finds something to cancel. Each later iteration mints its own; the drain owns
// the cleanup of whichever triple it currently holds. The handle is deliberately NOT
// hoisted over the whole drain: an early iteration releasing a hoisted handle would
// evict a later iteration's registration.
func (r *Runtime) drainCoordinator(ctx context.Context, coordSessionID string, slot *coordSlot, runCtx context.Context, cancelRun context.CancelFunc, run *sessionRun) {
	// Ends the iteration's turn context and its cancel registration. Called on EVERY
	// exit path and at the end of every iteration; nil-safe so the loop can re-arm.
	// The release runs BEFORE the slot is released: in the gap after a release another
	// queued turn can take the slot and register its own cancel, and releasing by
	// handle is what keeps this cleanup from touching THAT registration.
	endTurnCtx := func() {
		if cancelRun == nil {
			return
		}
		run.release()
		cancelRun()
		runCtx, cancelRun, run = nil, nil, nil
	}
	for {
		// Batch worker replies OUTSIDE admission: the first worker note opens a fixed
		// window and every note landing inside it joins the same turn without moving the
		// deadline. A generic wake (flow start, resume, stall nudge) skips the wait.
		// A false return means nothing is owed any more and the slot is already cleared,
		// so the iteration's registration has to be released here too — leaving it
		// tracked would keep the session listed as active forever.
		if !r.waitCoordinatorBatch(ctx, slot) {
			endTurnCtx()
			return
		}

		// Adopt the caller's ctx on the first iteration, mint a fresh one on every later
		// one. Per-iteration, never hoisted over the whole drain: a stop must not also
		// kill the next iteration's claim, because a real worker notification supersedes
		// an earlier human Stop (see the pending branch below).
		if cancelRun == nil {
			var c context.Context
			c, cancelRun = context.WithCancel(context.Background())
			runCtx = c
			run = r.trackSession(coordSessionID, cancelRun)
		}

		// Queue for the slot like everyone else. Whatever is ahead of us — a user
		// message, a /compact, a peer delivery — runs first.
		release, slotErr := r.claimSessionTurnSlotCtx(runCtx, coordSessionID, turnqueue.KindCoordinator, "worker bildirimi")
		if slotErr != nil {
			// Stopped while queued behind another turn on this session. The turn never
			// ran, so it must not spend the auto-turn budget (turns++ is deliberately
			// skipped) and must not consume the notification that armed it (pending is
			// left as-is: a worker note is still owed a turn once the loop is re-armed).
			// No markCoordinatorBlocked — a human stopped this, the coordinator is not
			// wedged — and no settle backstop while a note is still pending.
			release()
			r.logger.Info("coordination: drain turn cancelled before it started", "coordinator", coordSessionID)
			slot.mu.Lock()
			slot.stopRequested = true // parity with runCoordinatorTurn's stop path
			// driving MUST be cleared: otherwise every later notification takes the
			// "already driving" branch and no drain ever starts again — a permanent freeze.
			slot.driving = false
			// A worker note still inside its batch window is also "owed" a turn: the
			// backstop must not fire while one is waiting on the deadline.
			owed := !slot.pending && !slot.workerPending
			slot.mu.Unlock()
			endTurnCtx()
			if owed {
				// Nothing pending can wake this node again: if it is a mid-level node that
				// still owes report_to_coordinator, the branch above it would wait forever.
				r.scheduleSettleBackstop(coordSessionID)
			}
			return
		}

		// Worker persistence/arming and this final turn-start gate share admission.
		// Acquiring it only after FIFO admission preserves queue fairness and ensures
		// every durable note that precedes this gate is either consumed by this turn or
		// remains armed for a later one. It is bound to the RUNTIME ctx, not the turn's:
		// a human Stop must not abandon the token while a note is still armed.
		releaseAdmission, admErr := acquireCoordinatorAdmission(ctx, slot)
		if admErr != nil {
			release()
			endTurnCtx()
			r.clearCoordinatorDrain(slot)
			return
		}

		// TURN START linearizes here. Archive/halt commits before this DB read reject;
		// commits after it observe an already-active turn. Holding mu across the read
		// keeps rejection cleanup atomic with resume and generic wake arming.
		slot.mu.Lock()
		sess, sessErr := r.db.GetSession(ctx, coordSessionID)
		// A selected recipe (M5) may lower/raise the notify-loop cap for just this
		// coordinator session; fall back to the workspace default when unset (0).
		maxTurns := r.tun.CoordinatorMaxTurns()
		if sessErr == nil && sess.CoordinatorMaxTurns > 0 {
			maxTurns = sess.CoordinatorMaxTurns
		}
		if sessErr != nil {
			// A failed read is NOT a stop: the note that armed this iteration is durable
			// and would be silently dropped. Fall back to the workspace cap and let the
			// turn run — runCoordinatorTurn reads the session again and bails if it is
			// genuinely gone. Logged because a read failing here is never routine.
			r.logger.Warn("coordination: drain could not read the coordinator session, using the workspace turn cap",
				"coordinator", coordSessionID, "error", sessErr)
		}
		closing := ctx.Err() != nil
		archived := sessErr == nil && sess.State == "archived"
		noPending := !slot.pending && !slot.workerPending
		capped := slot.turns >= maxTurns
		if closing || archived || slot.stallHalted || noPending || capped {
			warn := capped && !slot.capWarn
			if capped {
				slot.capWarn = true
			}
			turns := slot.turns
			r.clearCoordinatorDrainLocked(slot)
			slot.mu.Unlock()
			releaseAdmission()
			endTurnCtx()
			release()
			if warn {
				r.warnCoordinatorCap(coordSessionID, turns)
			}
			return
		}
		// Count the auto-turn HERE, not before the wait: while we were queued a user
		// turn may have reset the cap (a human is back in the loop), and a turn that
		// never ran must not spend the budget.
		slot.turns++
		turnNo := slot.turns
		// Consume the notification(s) that armed this iteration: the turn about to run
		// is history-aware, so it sees every note persisted so far, including any that
		// landed while we waited for the slot.
		slot.pending = false
		slot.workerPending = false
		slot.workerDeadline = time.Time{}
		slot.mu.Unlock()
		releaseAdmission()

		r.observeDrain(DrainEvent{CoordinatorID: coordSessionID, Phase: "turn_start", Turn: turnNo})
		if r.coordRunFn != nil {
			r.coordRunFn(coordSessionID)
		} else {
			r.runCoordinatorTurn(runCtx, coordSessionID, run)
		}
		// Neither call is deferred: this is a loop body that can iterate up to
		// CoordinatorMaxTurns times, and a deferred release would hold the slot across
		// the whole drain, destroying the fairness property documented above.
		endTurnCtx()
		release()
		r.observeDrain(DrainEvent{CoordinatorID: coordSessionID, Phase: "turn_end", Turn: turnNo})
		if ctx.Err() != nil {
			r.clearCoordinatorDrain(slot)
			return
		}

		slot.mu.Lock()
		// Hard-halt escalation (FND-99caeb31): the turn-end stall guard just spent the
		// last nudge on a coordinator STILL narrating phantom spawns and escalated to a
		// halt. Stop auto-turning it — no re-arm, and skip the idle-reconcile turn below
		// (which would otherwise hand the wedged model one more shot). A real worker
		// notification (enqueueCoordinatorTurn) still starts a fresh drain, and a turn
		// that finally calls a coordination tool clears the flag. The sweeper stays live
		// as the long-horizon backstop.
		if slot.stallHalted {
			slot.stopRequested = false
			slot.pending = false
			slot.workerPending = false
			slot.workerDeadline = time.Time{}
			slot.driving = false
			slot.mu.Unlock()
			return
		}
		if slot.pending || slot.workerPending {
			// A real worker notification supersedes an earlier human Stop: a still-running
			// or just-finished worker is allowed to continue the coordinator (only the
			// no-pending idle-reconcile is suppressed by a Stop — see below).
			slot.stopRequested = false
			slot.mu.Unlock()
			continue
		}
		// Human Stop honoured (see coordSlot.stopRequested): the user cancelled the live
		// coordinator turn AND no worker notification is pending. Suppress ONLY the
		// idle-reconcile turn below — running it would inject a fresh <coordination-status>
		// note and hand the coordinator one more turn, making a manual Stop look like it
		// kept going. Consumed one-shot; a still-running worker re-arms the loop via
		// enqueueCoordinatorTurn when it finishes (workers keep running through a Stop).
		slot.stopRequested = false
		// The all-idle signal is delivered only by the last worker's own result.
		// A standalone reconcile here would create a second notification turn with
		// no new worker result, which is both costly and prone to false spawn nudges.
		// Paths that finish workers must therefore preserve the zero-crossing value
		// from releaseOnce and pass it to notifyCoordinator.
		slot.driving = false
		slot.mu.Unlock()
		// No worker and no queued notification can wake this coordinator now.
		r.markCoordinatorBlocked(context.Background(), coordSessionID)
		// This coordinator may itself be a worker that owes its own coordinator a
		// result (a mid-level node). It has now had its reconcile turn with every
		// worker finished; if it still has not called report_to_coordinator, the
		// branch above it would wait forever. The backstop reports for it — see
		// settleReportBackstop for why it never claims "completed".
		r.scheduleSettleBackstop(coordSessionID)
		return
	}
}

// scheduleSettleBackstop arms the upward-report backstop for a mid-level node
// whose drain loop just went idle. Delayed rather than immediate: a notification
// racing the drain exit re-enters the loop and the node still gets its chance to
// report itself, which is always preferable to the runtime guessing on its behalf.
// A no-op for a node that owes nothing (the overwhelmingly common case: a root
// coordinator).
func (r *Runtime) scheduleSettleBackstop(coordSessionID string) {
	probe, cancelProbe := context.WithTimeout(context.Background(), 10*time.Second)
	owes := r.owesReportNow(probe, coordSessionID)
	cancelProbe()
	if !owes {
		return
	}
	go func() {
		time.Sleep(r.tun.CoordinatorSettleGrace())
		slot := r.coordSlotFor(coordSessionID)
		slot.mu.Lock()
		busy := slot.driving || slot.pending || slot.workerPending
		slot.mu.Unlock()
		// Also check the admission queue: a turn from ANY path (a user message, a
		// peer delivery) may have taken this session meanwhile — the node is alive
		// and should report for itself.
		if busy || r.sessionTurnBusy(coordSessionID) {
			return // it woke up again; let it report for itself
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		r.settleReportBackstop(ctx, coordSessionID)
	}()
}

// coordinationStatusNote is the authoritative "every worker has finished" signal.
// It is always piggybacked onto the last worker's <task-notification>, so the turn
// that delivers the final result also delivers the all-idle signal.
const coordinationStatusNote = "<coordination-status>All workers under this coordinator have finished. " +
	"Act on any results you have not handled yet, spawn the next steps if the plan has more, " +
	"or conclude the project. Do NOT wait for a worker that has already finished.</coordination-status>"

// attachCoordinationStatus appends the all-idle signal to a worker notification so
// the coordinator reads the final result and "everyone is done" in the SAME turn.
//
// WHY: the drain loop's idle-reconcile used to inject the note as a separate history
// entry and loop once more, which cost a full extra LLM turn — the whole conversation
// re-sent — purely to tell the coordinator something the notification it had just
// read already implied. Folding the two into one message removes that turn without
// weakening the signal: it is the same authoritative sentence, on the message that
// proves it (see drainCoordinator, which treats a piggybacked note as the reconcile).
func attachCoordinationStatus(note string) string {
	return note + "\n\n" + coordinationStatusNote
}

// coordinatorWorkerStatusBlock renders an authoritative, always-fresh snapshot of
// every worker under coordSessionID for the coordinator's dynamic system suffix.
// Unlike the prose <task-notification>s in history — which a coalesced batch can
// let the model overlook — this block is regenerated every turn from live session
// state, so the coordinator can never believe a finished worker is still running.
// Empty when the session has no workers.
//
// The rendering itself lives in internal/view (ProjectWorkers): this is a PUSH
// projection into every coordinator turn, so it inherits that package's discipline
// — a cap on how many entries a wide fleet may inject, with the dropped ones still
// counted in the summary line the coordinator reasons over.
func (r *Runtime) coordinatorWorkerStatusBlock(ctx context.Context, coordSessionID string) string {
	ws, err := r.ListWorkers(ctx, coordSessionID)
	if err != nil || len(ws) == 0 {
		return ""
	}
	v, err := view.ProjectWorkers(view.WorkersInput{Workers: toViewWorkers(ws)}, view.LevelCard)
	if err != nil {
		// The block is an optional prompt enrichment; losing it must not fail the
		// turn. It IS worth a log line — a coordinator silently running without its
		// authoritative fleet state is the exact condition this block prevents.
		r.logger.Warn("coordination: worker status block render failed",
			"coordinator", coordSessionID, "error", err)
		return ""
	}
	return v.Text()
}

// toViewWorkers maps the runtime's live fleet onto the renderer's input. The
// view package cannot import this one (agent → tools → view), so the mapping
// lives here.
func toViewWorkers(ws []WorkerInfo) []view.Worker {
	out := make([]view.Worker, 0, len(ws))
	for _, w := range ws {
		out = append(out, view.Worker{
			SessionID:  w.SessionID,
			AgentName:  w.AgentName,
			Running:    w.Running,
			Delegating: w.Delegating,
			Summary:    w.Summary,
			StartedAt:  w.StartedAt,
		})
	}
	return out
}

// runCoordinatorTurn runs one history-aware turn for the coordinator session so it
// synthesizes the worker notifications now sitting in its history, then records the
// reply and fires the turn-finished hook (for tags/automations on the coordinator).
// drainCtx is the drain iteration's cancellable context: created and registered with
// trackSession by drainCoordinator BEFORE it queued for the turn slot, so a stop
// issued while this turn was still waiting is not outlived by it. run is THAT
// iteration's registration — never a handle hoisted over the whole drain.
func (r *Runtime) runCoordinatorTurn(drainCtx context.Context, coordSessionID string, run *sessionRun) {
	// Hard wall-clock ceiling (settings-driven, same as spawns) PLUS an idle
	// watchdog: a worker/coordinator turn that streams no step for SpawnIdleTimeout
	// is reclaimed fast, while a long-but-productive one runs up to SpawnTimeout.
	hardCap, idleCap := time.Duration(0), r.tun.SpawnIdleTimeout()

	// Metadata reads are quick and must not be bound to the turn watchdog (which the
	// resume loop owns per attempt) — use the background context for them.
	sess, err := r.db.GetSession(context.Background(), coordSessionID)
	if err != nil {
		r.logger.Warn("coordination: coordinator session gone", "coordinator", coordSessionID, "error", err)
		return
	}
	agent, err := r.db.GetAgent(context.Background(), sess.AgentID)
	if err != nil {
		r.logger.Warn("coordination: coordinator agent gone", "coordinator", coordSessionID, "error", err)
		return
	}

	// Single-shot idle-resume (FND-708844f8): an idle-cut coordinator DRAIN turn gets
	// ONE more attempt under a fresh window before reconcileTurnOutcome marks it
	// unfinished. Safe against the drain/stall machinery: the resume is transparent to
	// the notify loop (it just sees one longer turn); slot.lastTurnUnix is stamped
	// AFTER, and guardCoordinatorStall still runs only on a clean (err==nil, untruncated)
	// turn — a resumed-then-idle turn stays truncated and skips it, exactly as Faz E.
	// No double recovery: a coordinator reports UP via runWorker, not this drain turn.
	var (
		turnCtx context.Context
		meta    *turnMeta
	)
	turnStart := time.Now()
	ctx, cancel, output, steps, err := r.runTurnWithIdleResume(drainCtx, hardCap, idleCap, r.tun.IdleResumeMax(),
		func(attemptCtx context.Context, _ context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			turnCtx = tools.WithAsyncChat(WithSessionID(WithCallKind(attemptCtx, KindSpawn), coordSessionID))
			turnCtx, meta = WithTurnMeta(turnCtx)
			p := "" // the coordinator drains its inbox via history, not a prompt
			if attempt > 1 {
				p = resumeContinuationPrompt("", prevOutput)
			}
			return r.runSessionTurn(turnCtx, agent, coordSessionID, p, true)
		})
	defer cancel()
	// Released HERE, not only by the drain loop afterwards: it is what keeps the
	// record/publish tail below uncancellable. The drain's own endTurnCtx repeats it,
	// which is safe — removal is by pointer identity, so the second call finds nothing
	// and touches no other turn's registration.
	run.release()

	// A watchdog cut (hard/idle) or a self-truncated loop hands back salvaged text;
	// lead it with the outcome note (nil error) so the recorded reply reads as a
	// fragment, not a clean result, and the success-only follow-ups below are skipped.
	output, steps, err, truncated := r.reconcileTurnOutcome(ctx, output, steps, err, hardCap, idleCap)
	// Runtime teardown is not a human Stop. Do not append a synthetic stop message
	// or arm stopRequested while CloseMCP drains coordinator goroutines. The signal is
	// the runtime-scoped coordinator context, not this turn's: drainCtx is cancelled by
	// a human Stop too, and the two must not be confused.
	if errors.Is(err, context.Canceled) && r.coordCtx.Err() != nil {
		r.logger.Info("coordination: coordinator turn cancelled by runtime shutdown", "coordinator", coordSessionID)
		return
	}
	text := output
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// A viewer pressed "Durdur"/"Kes": the autonomous run's cancel aborted the
			// turn (see autonomousInteraction). Report a clean stop, not a failure.
			text = "⏹️ Koordinatör turu durduruldu."
			r.logger.Info("coordination: coordinator turn stopped", "coordinator", coordSessionID)
			// Signal the drain loop to exit without the idle-reconcile turn: honour the
			// human Stop instead of injecting a <coordination-status> note and running
			// once more (see coordSlot.stopRequested).
			stopSlot := r.coordSlotFor(coordSessionID)
			stopSlot.mu.Lock()
			stopSlot.stopRequested = true
			stopSlot.mu.Unlock()
		} else {
			text = "⚠️ Koordinatör turu çalıştırılamadı:\n\n" + err.Error()
			r.logger.Error("coordination: coordinator turn failed", "coordinator", coordSessionID, "error", err)
		}
	} else if strings.TrimSpace(text) == "" {
		text = "ℹ️ Koordinatör bu tur için boş yanıt döndürdü."
	}
	if DetectUnbackedSpawnClaim(text, turnToolNames(steps), sess.IsCoordinator()) {
		const warning = "⚠️ No worker was actually spawned this turn (no spawn tool call was made)."
		text = strings.TrimRight(text, "\n") + "\n\n" + warning
		r.emitDebug(ctx, db.DebugEvent{
			Type:    db.DebugGuardrail,
			AgentID: agent.ID,
			Name:    "unbacked_spawn_claim",
			Detail:  "coordinator claimed worker delegation without a spawn tool call",
		})
	}
	// text was pre-composed above (success output / failure / stop / empty note).
	if addErr := r.recordAssistantMessage(ctx, coordSessionID, agent.ID, text, steps, meta, time.Since(turnStart).Milliseconds()); addErr != nil {
		r.logger.Warn("coordination: failed to record coordinator reply", "coordinator", coordSessionID, "error", addErr)
	}
	r.publish(events.Event{
		Type:   events.TypeChat,
		Level:  "info",
		Title:  "🧭 Koordinatör turu tamamlandı — " + agent.Name,
		Target: map[string]string{"view": "executions", "sessionId": coordSessionID},
	})
	// Coordinator turns are ordinary turns for the rest of the system: fire the hook
	// so tags/automations on the coordinator session still work. The coordinator has
	// no CoordinatorSessionID, so this never re-enters the coordination loop.
	// Stamp turn-completion time for the stall sweeper's staleness check, regardless
	// of success (a failed/stopped turn still counts as activity).
	if slot := r.coordSlotFor(coordSessionID); slot != nil {
		slot.mu.Lock()
		slot.lastTurnUnix = time.Now().Unix()
		slot.mu.Unlock()
	}
	// A truncated turn only produced a fragment (already led with a "not done" note):
	// withhold completion automations AND skip the spawn-narration stall check, which
	// must not judge a turn the watchdog cut short.
	if err == nil && !truncated {
		r.FireTurnFinished(coordSessionID, agent.ID, output)
		// Catch the "narrated a spawn but never called the tool" degradation before the
		// drain loop's post-turn pending check, so a corrective re-prompt runs THIS batch.
		r.guardCoordinatorStall(coordSessionID, agent.ID, agent, output, steps)
	}
}

// turnCalledCoordinationTool reports whether any (possibly nested) step in a turn
// invoked a coordination tool. Matches the bare Tool name and the namespaced CallName
// alike (mcp__tionharness_interaction__spawn_worker on the claude-cli path).
func turnCalledCoordinationTool(steps []TurnStep) bool {
	for _, s := range steps {
		if s.Kind == StepTool {
			n := strings.ToLower(s.Tool + " " + s.CallName)
			if strings.Contains(n, "spawn_worker") || strings.Contains(n, "list_workers") ||
				strings.Contains(n, "send_to_worker") || strings.Contains(n, "stop_worker") {
				return true
			}
		}
		if len(s.SubSteps) > 0 && turnCalledCoordinationTool(s.SubSteps) {
			return true
		}
	}
	return false
}

// warnCoordinatorCap posts a one-time notice that a coordinator hit its auto-turn
// cap; further worker notifications are still persisted but stop auto-triggering
// turns, so the user can inspect and continue manually.
func (r *Runtime) warnCoordinatorCap(coordSessionID string, turns int) {
	r.logger.Warn("coordination: coordinator auto-turn cap reached", "coordinator", coordSessionID, "turns", turns)
	r.publish(events.Event{
		Type:   events.TypeCoordination,
		Level:  "info",
		Title:  "🧭 Koordinatör tur limiti",
		Body:   fmt.Sprintf("Otomatik koordinatör turları limiti (%d) aşıldı; yeni worker bildirimleri kaydediliyor ama otomatik tur tetiklenmiyor. Devam etmek için oturuma manuel mesaj gönderin.", turns),
		Target: map[string]string{"view": "executions", "sessionId": coordSessionID},
	})
}
