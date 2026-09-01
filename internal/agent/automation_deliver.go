package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// ErrEmptyAutomationPrompt guards deliverAutomationTurn against an empty rendered
// prompt. The token/board fire paths already reject an empty prompt before
// dispatch, so this is a defensive backstop rather than the primary check.
var ErrEmptyAutomationPrompt = errors.New("automation prompt is empty")

// SessionKindAutomation is the Kind of an automation's persistent maintenance
// session — the token-triggered analog of a cron schedule's stable "schedule"
// thread. One such session per automation (keyed by its id via
// GetOrCreateSourceSession), so consecutive fires continue the SAME conversation
// instead of each spawning a fresh session. Its tokens are deliberately excluded
// from session-scoped token-crossing detection (see OnUsageRecorded) so a
// session-scoped rule cannot re-trigger itself on the spend of its own upkeep turn.
const SessionKindAutomation = "automation"

// SessionKindAutomationRun is the Kind of a ONE-SHOT session an automation spawns
// when its session mode is NOT "continue" (fireToken/fireBoard/fireTag/fireCounter
// dispatching through LaunchRun's spawn path instead of deliverAutomationTurn).
// Distinct from SessionKindAutomation (the persistent maintenance thread) so the
// self-amplification guard in OnUsageRecorded — which excludes ONLY the
// maintenance session's own upkeep tokens from session-scoped crossings — does not
// also swallow a fresh spawn's legitimate token crossing. Both kinds share the
// sidebar's "Otomasyon" grouping (see frontend's kindChipKey) so a one-shot
// automation fire is not miscategorized under "Spawn".
const SessionKindAutomationRun = "automation-run"

// deliverAutomationTurn delivers an automation's rendered prompt into its
// persistent per-automation session as a fresh, history-aware turn — the
// continuity that makes a token automation behave like a cron schedule: each fire
// picks up where the last one left off rather than starting from a blank session.
//
// It mirrors the scheduler's deliverPrompt lifecycle (per-session turn slot,
// history-aware invoke, watchdog + single-shot idle-resume, reply/error record,
// bounded auto-continue) but keys the session by (kind "automation", sourceId =
// automation id) and runs the history-aware runSessionTurn so the agent sees the
// whole prior thread. Only the session/agent driver uses this; flow-backed token
// automations keep running through LaunchRun (a flow already accumulates its own
// per-flow transcript). Returns the reused session's id (for delivery bookkeeping)
// and any invoke error.
func (r *Runtime) deliverAutomationTurn(ctx context.Context, a db.Automation, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", ErrEmptyAutomationPrompt
	}
	agent, err := r.db.GetAgent(ctx, a.TargetAgentID)
	if err != nil {
		return "", err
	}
	// One stable, continuing thread per automation. Reused across every fire so the
	// agent carries its own earlier turns forward (compaction bounds the growth just
	// as it does for a cron schedule's long-lived session).
	session, err := r.db.GetOrCreateSourceSession(ctx, SessionKindAutomation, a.ID, a.TargetAgentID, "⚡ "+automationLabel(a))
	if err != nil {
		return "", err
	}

	// Own cancelable context for the delivery turn so a human "Durdur"
	// (CancelSession) can stop it — it never enters the api server's chatRuns.
	// Registered BEFORE the turn-slot claim below: the claim can queue behind any
	// other turn on this session, and a stop landing in that window must find
	// something to cancel instead of being outlived by a turn that starts afterwards.
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	r.trackSession(session.ID, cancelRun)
	defer r.untrackSession(session.ID)

	// Serialize this fire's turn with any concurrent turn on the same session (a
	// prior fire still running, a user who opened the maintenance thread) via the
	// single per-session turn slot.
	release, slotErr := r.claimSessionTurnSlotCtx(runCtx, session.ID, turnqueue.KindAutomation, "otomasyon tetiği")
	defer release()
	if slotErr != nil {
		// Stopped while waiting for the slot: the turn never ran and the prompt was not
		// recorded. Return before the auto-continue / auto-handoff chain below — a fire
		// the user stopped must not resurrect itself as a follow-up turn.
		r.logger.Info("automation deliver: cancelled before its turn started",
			"automation", a.ID, "agent", a.TargetAgentID, "session", session.ID)
		return session.ID, fmt.Errorf("otomasyon turu sırasını beklerken durduruldu: %w", slotErr)
	}

	// Record the automation prompt as a user turn first so the thread reads as a
	// real conversation. Origin "automation" lets the UI render it as a triggered
	// note rather than a user bubble; role stays "user" so the model's context is
	// unchanged.
	autoMsg, err := r.db.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      "user",
		Origin:    "automation",
		Text:      prompt,
	})
	if err != nil {
		return session.ID, err
	}
	// Bridge it to the hub so a window watching this session renders the note live
	// and in order before the reply (_Docs/58), not only on reload.
	r.emitInjectedUserNote(session.ID, autoMsg)

	// Bound the turn with the spawn watchdog: it holds the per-session turn slot, so
	// a hung turn must not block the session's queue forever (the slot's Cond wait
	// ignores ctx).
	hardCap, idleCap := r.tun.SpawnTimeout(), r.tun.SpawnIdleTimeout()
	var (
		overflow *atomic.Bool
		meta     *turnMeta
	)
	turnStart := time.Now()
	// Single-shot idle-resume: an idle-cut turn gets ONE more attempt under a fresh
	// window before reconcileTurnOutcome marks it unfinished. runSessionTurn is the
	// history-aware runner — the automation prompt was just persisted as the last
	// user message, so the agent continues with the FULL prior thread.
	turnBase, cancelTurn, output, steps, err := r.runTurnWithIdleResume(runCtx, hardCap, idleCap, r.tun.IdleResumeMax(),
		func(attemptCtx context.Context, _ context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			var turnCtx context.Context
			turnCtx, overflow = withOverflowFlag(WithSessionID(WithCallKind(attemptCtx, KindSchedule), session.ID))
			turnCtx, meta = WithTurnMeta(turnCtx)
			p := prompt
			if attempt > 1 {
				p = resumeContinuationPrompt(prompt, prevOutput)
			}
			return r.runSessionTurn(turnCtx, agent, session.ID, p, true) // autonomous
		})
	defer cancelTurn()

	// A watchdog cut / self-truncated loop hands back salvaged text; lead it with the
	// outcome note (nil error) so it records as an explaining reply, not a clean one.
	output, steps, err, truncated := r.reconcileTurnOutcome(turnBase, output, steps, err, hardCap, idleCap)
	if err != nil {
		r.logger.Error("automation deliver: agent invoke failed",
			"automation", a.ID, "agent", a.TargetAgentID, "session", session.ID,
			"provider", agent.Provider, "model", agent.Model, "error", err)
		r.recordTurnError(ctx, session.ID, a.TargetAgentID, err, steps, meta, time.Since(turnStart).Milliseconds(), "⚠️ Otomasyon promptu çalıştırılamadı:")
		r.AutoTagTurn(ctx, session.ID, steps, "automation_error")
		return session.ID, err
	}
	// Persist the reply (empty → explicit note; agent id + activity trace stamped so
	// it renders like a normal chat turn).
	output, err = r.recordAssistantReply(ctx, session.ID, a.TargetAgentID, output, steps, meta, time.Since(turnStart).Milliseconds(), "ℹ️ Ajan bu otomasyon promptu için boş yanıt döndürdü.")
	// Self-completion: an autonomous fire has no human to send the follow-up, so if
	// the turn stalled with unfinished work keep it going until done. Bounded + budget-gated.
	r.maybeAutoContinue(ctx, agent, session.ID, KindSchedule, steps)
	// Context-reset handoff: if this turn hit the context limit, optionally continue
	// the work in a fresh session. No-op unless HandoffAuto is enabled.
	r.maybeAutoHandoff(ctx, session.ID, agent, overflow.Load())
	// Auto-tag tool errors from this turn.
	r.AutoTagTurn(ctx, session.ID, steps, "")
	// The maintenance session carries no trigger tag, so FireTurnFinished cannot chain
	// a tag automation onto it; it is still emitted (skipped when truncated) so any
	// turn-finished consumer stays consistent with the other delivery paths.
	if !truncated {
		r.FireTurnFinished(session.ID, a.TargetAgentID, output)
	}
	return session.ID, err
}
