package agent

import (
	"context"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// autoContinuePrompt is the nudge delivered to an autonomous session that ended a
// turn with work still unfinished. It is persisted as a user turn (Origin
// "auto-continue") so the history-aware continuation reads it as a real prompt and
// the thread shows the progression.
const autoContinuePrompt = "Önceki turda görevi tamamlamadan durdun (açık todo maddeleri var veya son eylemin bir araç aktivasyonuydu — aktive edilen araçlar ancak bir sonraki turda kullanılabilir, o tur da bu). Aktive ettiğin araçlar artık HAZIR. Göreve kaldığın yerden DEVAM ET ve tamamla; soru sorma, otonom çalış. İş gerçekten bittiyse kısa bir tamamlanma özeti ver ve dur."

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
		if !needsAutoContinue(steps) {
			return
		}
		r.logger.Info("auto-continue: unfinished autonomous turn — issuing continuation",
			"session", sessionID, "agent", agent.ID, "iteration", i+1, "max", max)

		// Persist the nudge as a user turn so the history-aware continuation loads it
		// (the wake-turn runner reads the prompt from history, not a separate arg) and
		// the thread reads as a real prompt→reply progression.
		if _, err := r.db.AddMessage(ctx, db.Message{
			SessionID: sessionID,
			Role:      "user",
			Origin:    "auto-continue",
			Text:      autoContinuePrompt,
		}); err != nil {
			r.logger.Warn("auto-continue: failed to record nudge", "session", sessionID, "error", err)
			return
		}

		r.trackSession(sessionID)
		turnCtx, overflow := withOverflowFlag(WithSessionID(WithCallKind(ctx, kind), sessionID))
		turnCtx, meta := WithTurnMeta(turnCtx)
		turnStart := time.Now()
		output, cSteps, err := r.runSessionTurn(turnCtx, agent, sessionID, autoContinuePrompt, true)
		r.untrackSession(sessionID)

		if err != nil {
			// A provider error or a daily-budget stop ends the loop; surface it inline
			// so the thread explains why the autonomous run halted.
			errMsg := db.Message{
				SessionID: sessionID,
				AgentID:   agent.ID,
				Role:      "assistant",
				Text:      "⚠️ Otomatik devam turu çalıştırılamadı:\n\n" + err.Error(),
				Steps:     encodeSteps(cSteps),
			}
			meta.apply(&errMsg, time.Since(turnStart).Milliseconds())
			if _, addErr := r.db.AddMessage(ctx, errMsg); addErr != nil {
				r.logger.Warn("auto-continue: failed to record error reply", "session", sessionID, "error", addErr)
			}
			r.AutoTagTurn(ctx, sessionID, cSteps, "auto_continue_error")
			return
		}

		if strings.TrimSpace(output) == "" {
			output = "ℹ️ Ajan otomatik devam turunda boş yanıt döndürdü."
		}
		replyMsg := db.Message{
			SessionID: sessionID,
			AgentID:   agent.ID,
			Role:      "assistant",
			Text:      output,
			Steps:     encodeSteps(cSteps),
		}
		meta.apply(&replyMsg, time.Since(turnStart).Milliseconds())
		if _, err := r.db.AddMessage(ctx, replyMsg); err != nil {
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
