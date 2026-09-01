package agent

import (
	"context"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// The auto-continue nudge lives in the central registry (internal/prompts, key
// "auto-continue"): it is delivered to an autonomous session that ended a turn
// with work still unfinished, persisted as a user turn (Origin "auto-continue")
// so the history-aware continuation reads it as a real prompt and the thread
// shows the progression.

// autoContinueTools are the lazy-loading meta-tools whose activation only takes
// effect on the NEXT turn (the CLI's fixed per-process tool set, or the native
// registry recomputed per turn). A turn that ENDS on one of these has staged tools
// it never got to use — the stall this feature repairs.
var autoContinueTools = map[string]bool{
	"activate_tools":   true,
	"deactivate_tools": true,
	"ToolSearch":       true,
	"tool_search":      true,
}

// hasToolStep reports whether a trace made any tool-level progress this turn
// (a tool call, checklist write, file diff or subagent). A continuation that makes
// none can't advance a stall, so the loop stops rather than burn the budget.
func hasToolStep(steps []TurnStep) bool {
	for _, s := range steps {
		switch s.Kind {
		case StepTool, StepTodo, StepDiff, StepSubagent:
			return true
		}
	}
	return false
}

// needsAutoContinue reports whether an autonomous turn's trace signals unfinished
// work worth an automatic continuation:
//   - it left todo items open (pending / in_progress) in its latest checklist, or
//   - its last meaningful action was a lazy-tool activation, whose tools only
//     become usable on the next turn — the exact "activated then stopped" stall.
func needsAutoContinue(steps []TurnStep) bool {
	if len(steps) == 0 {
		return false
	}
	// Open todos in the LATEST checklist snapshot → work explicitly unfinished.
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Kind == StepTodo {
			for _, t := range steps[i].Todos {
				if t.Status == "pending" || t.Status == "in_progress" {
					return true
				}
			}
			break // only the most recent checklist matters
		}
	}
	// Ended on a lazy-tool activation (skip trailing narration/thinking to find the
	// last real action).
	for i := len(steps) - 1; i >= 0; i-- {
		switch steps[i].Kind {
		case StepText, StepThinking:
			continue
		case StepTool:
			if autoContinueTools[steps[i].Tool] {
				return true
			}
		}
		break
	}
	return false
}

// shouldAutoContinue suppresses the generic unfinished-work nudge while a
// coordinator still has live workers. Their completion notifications are the
// continuation trigger; nudging now would make the coordinator race ahead and
// show a misleading unfinished-turn message while it waits for delegated work.
func (r *Runtime) shouldAutoContinue(sessionID string, steps []TurnStep) bool {
	if r.coordSlotFor(sessionID).workers.Load() > 0 {
		return false
	}
	return needsAutoContinue(steps)
}

// maybeAutoContinue keeps an autonomous run going until its work is actually done.
// After the caller has persisted the turn's reply, this inspects the trace: if it
// signals unfinished work (needsAutoContinue) it issues a continuation turn on the
// same session — history-aware, so the agent resumes with full context and the now
// live tools it just activated — persists that reply, and repeats, up to the
// configured max. A continuation that makes no tool progress, hits an error
// (including a daily-budget stop), or reports the work done ends the loop.
//
// This is the unattended-completion guarantee: a scheduled/spawned/woken agent
// that "planned + activated tools + stopped" no longer strands its own task,
// because there is no human to send the follow-up the lazy tools were waiting for.
func (r *Runtime) maybeAutoContinue(ctx context.Context, agent db.Agent, sessionID string, kind CallKind, lastSteps []TurnStep) {
	if !r.tun.AutoContinue() || sessionID == "" {
		return
	}
	max := r.tun.AutoContinueMax()
	steps := lastSteps
	for i := 0; i < max; i++ {
		if !r.shouldAutoContinue(sessionID, steps) {
			return
		}
		// The preceding turn may have consumed the whole spawn/schedule deadline; a
		// continuation on an already-expired context fails instantly and would surface
		// a bare "context deadline exceeded". There IS unfinished work but no budget
		// left — skip the continuation and record a clear, human-readable note instead.
		if cerr := ctx.Err(); cerr != nil {
			r.logger.Info("auto-continue: parent deadline already exceeded — skipping continuation",
				"session", sessionID, "agent", agent.ID, "iteration", i+1, "error", cerr)
			// ctx is expired, so persist with a detached context that ignores its deadline.
			bg := context.WithoutCancel(ctx)
			note := db.Message{
				SessionID: sessionID,
				AgentID:   agent.ID,
				Role:      "assistant",
				Text: "⏱️ Süre doldu — önceki tur ayrılan spawn süresini doldurduğu için " +
					"otomatik devam turu çalıştırılamadı. İş yarım kalmış olabilir; oturumu elle " +
					"sürdürebilir ya da spawn süresini Ayarlar'dan artırabilirsiniz.",
			}
			if _, addErr := r.db.AddMessage(bg, note); addErr != nil {
				r.logger.Warn("auto-continue: failed to record timeout note", "session", sessionID, "error", addErr)
			}
			r.AutoTagTurn(bg, sessionID, steps, "auto_continue_error")
			return
		}
		r.logger.Info("auto-continue: unfinished autonomous turn — issuing continuation",
			"session", sessionID, "agent", agent.ID, "iteration", i+1, "max", max)

		// Persist the nudge as a user turn so the history-aware continuation loads it
		// (the wake-turn runner reads the prompt from history, not a separate arg) and
		// the thread reads as a real prompt→reply progression.
		nudge := r.readPrompt("auto-continue")
		if _, err := r.recordInjectedUserNote(ctx, sessionID, "auto-continue", nudge); err != nil {
			r.logger.Warn("auto-continue: failed to record nudge", "session", sessionID, "error", err)
			return
		}

		// Own cancelable context per continuation turn so a human "Durdur"
		// (CancelSession) can stop the autonomous loop mid-turn.
		runCtx, cancelRun := context.WithCancel(ctx)
		// One registration PER ITERATION, released at the end of that iteration. It
		// nests inside the outer scheduled/spawn/automation registration whose caller
		// is still holding its own handle — two live entries for one logical run, which
		// the per-turn list handles correctly (the outer one used to be silently
		// overwritten here, and with it the only way to stop the outer turn).
		turnCtx, overflow := withOverflowFlag(WithSessionID(WithCallKind(runCtx, kind), sessionID))
		turnCtx, meta := WithTurnMeta(turnCtx)
		turnStart := time.Now()
		output, cSteps, err := r.runTrackedContinuation(sessionID, cancelRun, func() (string, []TurnStep, error) {
			return r.runSessionTurn(turnCtx, agent, sessionID, nudge, true)
		})

		if err != nil {
			// A provider error or a daily-budget stop ends the loop; surface it inline
			// so the thread explains why the autonomous run halted. Shared continuation
			// error recorder — same message shape as spawn/inbox/wake.
			r.recordTurnError(ctx, sessionID, agent.ID, err, cSteps, meta, time.Since(turnStart).Milliseconds(), "⚠️ Otomatik devam turu çalıştırılamadı:")
			r.AutoTagTurn(ctx, sessionID, cSteps, "auto_continue_error")
			return
		}

		// Shared continuation reply recorder (empty-substitution + step trace + meta),
		// identical to the spawn/inbox/wake path.
		if _, err := r.recordAssistantReply(ctx, sessionID, agent.ID, output, cSteps, meta, time.Since(turnStart).Milliseconds(), "ℹ️ Ajan otomatik devam turunda boş yanıt döndürdü."); err != nil {
			r.logger.Warn("auto-continue: failed to record reply", "session", sessionID, "error", err)
		}
		r.maybeAutoHandoff(ctx, sessionID, agent, overflow.Load())
		r.AutoTagTurn(ctx, sessionID, cSteps, "")

		// No-progress guard: a continuation that ran no tools can't advance a
		// tool-activation/todo stall — stop instead of looping to the cap.
		if !hasToolStep(cSteps) {
			return
		}
		steps = cSteps
	}
	r.logger.Info("auto-continue: reached max continuations", "session", sessionID, "agent", agent.ID, "max", max)
}

// runTrackedContinuation registers ONE auto-continue iteration's turn, runs it, and
// releases that registration on the way out — including when the turn panics. The
// release cannot be a plain deferred call in maybeAutoContinue: the registration is
// per iteration, and a function-scoped defer would hold every iteration's handle until
// the whole loop ends. With the per-turn registration list a missed release never heals
// (nothing overwrites it any more): the session would read "running" forever and the
// agent stay busy forever.
func (r *Runtime) runTrackedContinuation(sessionID string, cancelRun context.CancelFunc, turn func() (string, []TurnStep, error)) (string, []TurnStep, error) {
	run := r.trackSession(sessionID, cancelRun)
	defer cancelRun()
	defer run.release()
	return turn()
}
