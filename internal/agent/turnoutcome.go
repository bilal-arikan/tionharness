package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// turnoutcome.go classifies how a BACKGROUND work turn actually ended.
//
// The problem it solves: a turn can be cut short and still return (output, nil).
// The claude-cli provider salvages the assistant text it captured before the
// subprocess was killed, and the native tool loop appends a StepRecovery and
// returns nil when it hits the iteration cap or a guardrail halt. Both look
// exactly like success to the caller, so a worker that ran out of wall clock
// mid-sentence was reported to its coordinator as <status>completed</status> —
// and the coordinator moved on believing the work was done (SES17).
//
// Every truncation the runtime can detect leaves one of two fingerprints:
//
//	ctx cancellation cause  → ErrTurnHardTimeout / ErrTurnIdleTimeout
//	terminal recovery step  → termMaxIters / termGuardrailHalt / term*Exhausted
//
// classifyTurnOutcome reads both and returns a status plus an operator-facing
// note that says, in plain words, that the work is NOT finished.

// Statuses a background work turn reports. completed/failed/killed predate this
// file; timeout and incomplete were added so a cut-short turn stops masquerading
// as a clean one.
const (
	turnStatusCompleted  = "completed"
	turnStatusTimeout    = "timeout"
	turnStatusIncomplete = "incomplete"
	turnStatusFailed     = "failed"
	turnStatusKilled     = "killed"
)

// turnOutcome is the verdict on a finished turn. Note is empty exactly when
// Status is completed; otherwise it is prepended to the reported result so the
// reader (a coordinator model, or a human in the transcript) cannot miss that
// the text below it is a fragment.
type turnOutcome struct {
	Status string
	Note   string
}

// Truncated reports whether the turn ended before the agent was done.
func (o turnOutcome) Truncated() bool { return o.Status != turnStatusCompleted }

// classifyTurnOutcome inspects a turn that returned WITHOUT an error and decides
// whether it really completed. ctx is the (already-finished) turn context, steps
// its trace, and hard/idle the deadlines it ran under — quoted in the note so the
// reader knows which knob to raise.
func classifyTurnOutcome(ctx context.Context, steps []TurnStep, hard, idle time.Duration) turnOutcome {
	if o, ok := classifyTurnContext(ctx, hard, idle); ok {
		return o
	}
	if o, ok := classifyTurnSteps(steps); ok {
		return o
	}
	return turnOutcome{Status: turnStatusCompleted}
}

// classifyTurnContext maps a watchdog cancellation cause to an outcome. Reports
// false when the context was not cancelled by a deadline (clean finish, or a
// human stop — which the caller already reports as "killed").
func classifyTurnContext(ctx context.Context, hard, idle time.Duration) (turnOutcome, bool) {
	cause := context.Cause(ctx)
	switch {
	case errors.Is(cause, ErrTurnHardTimeout):
		return turnOutcome{
			Status: turnStatusTimeout,
			Note: fmt.Sprintf(
				"⏱️ SÜRE DOLDU — tur %s'lik mutlak süre tavanına takıldı ve YARIDA kesildi. "+
					"Aşağıdaki metin kesilme anına aittir; iş BİTMİŞ DEĞİL, doğrulanmamış olabilir. "+
					"Kalan işi yeniden görevlendir. Tavan: Ayarlar ▸ Araçlar ▸ \"Spawn süresi — üst sınır (dk)\".",
				formatMinutes(hard)),
		}, true
	case errors.Is(cause, ErrTurnIdleTimeout):
		return turnOutcome{
			Status: turnStatusTimeout,
			Note: fmt.Sprintf(
				"⏱️ ASILI KALDI — tur %s boyunca hiçbir adım (araç/düşünce/token) üretmediği için "+
					"etkinlik izleyicisi tarafından kesildi. İş BİTMİŞ DEĞİL. "+
					"Pencere: Ayarlar ▸ Araçlar ▸ \"Spawn boşta süresi (dk)\".",
				formatMinutes(idle)),
		}, true
	}
	return turnOutcome{}, false
}

// classifyTurnSteps finds the loop's own terminal marker in the trace. Terminal
// markers use termReason values, which never collide with the contReason values
// a mid-turn recovery step carries, so scanning backwards is unambiguous.
func classifyTurnSteps(steps []TurnStep) (turnOutcome, bool) {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Kind != StepRecovery {
			continue
		}
		switch termReason(steps[i].Reason) {
		case termMaxIters:
			return turnOutcome{
				Status: turnStatusIncomplete,
				Note: fmt.Sprintf(
					"🛑 ARAÇ LİMİTİ — tur tek turda izin verilen %d araç iterasyonunu tüketti ve orada sonlandırıldı. "+
						"İş BİTMİŞ DEĞİL: model bir sonraki adımı çağıramadan kesildi. "+
						"Kalan işi daha küçük parçalara böl ya da limiti yükselt (TIONHARNESS_MAX_TOOL_ITERS).",
					maxToolIters),
			}, true
		case termGuardrailHalt:
			return turnOutcome{
				Status: turnStatusIncomplete,
				Note: "🛑 DÖNGÜ KORUMASI — tur, aynı aracı kısır döngüde çağırdığı için guardrail tarafından durduruldu. " +
					"İş BİTMİŞ DEĞİL; tekrar görevlendirmeden önce ajanın neden takıldığına bak.",
			}, true
		case termContextExhausted:
			return turnOutcome{
				Status: turnStatusIncomplete,
				Note: "🛑 BAĞLAM DOLDU — bağlam penceresi tükendi (sıkıştırma hakkı da bitti), yanıt bu noktada kesildi. " +
					"İş BİTMİŞ DEĞİL; kalanı taze bir oturumda sürdür.",
			}, true
		case termMaxTokenExhausted:
			return turnOutcome{
				Status: turnStatusIncomplete,
				Note: "🛑 ÇIKTI LİMİTİ — yanıt çıktı-token tavanına dayandı ve devam hakkı tükendi, metin kesik. " +
					"İş BİTMİŞ DEĞİL.",
			}, true
		}
	}
	return turnOutcome{}, false
}

// appendOutcomeStep marks a truncation in the turn trace so the transcript (and
// any later analysis over steps) explains itself. Only a WATCHDOG truncation is
// appended: the term* markers come from the tool loop, which already recorded its
// own StepRecovery before returning — appending again would duplicate it.
func appendOutcomeStep(steps []TurnStep, o turnOutcome) []TurnStep {
	if o.Status != turnStatusTimeout {
		return steps
	}
	return append(steps, TurnStep{Kind: StepRecovery, Reason: string(termTimeout), Text: o.Note})
}

// applyTurnOutcome prepends the outcome note to a reported result, so a reader
// that only skims the first line still sees the turn was cut short. Returns the
// text unchanged for a clean turn.
func applyTurnOutcome(result string, o turnOutcome) string {
	if o.Note == "" {
		return result
	}
	if strings.TrimSpace(result) == "" {
		return o.Note
	}
	return o.Note + "\n\n---\n\n" + result
}

// reconcileTurnOutcome folds a truncation verdict into a background turn's
// (output, steps, err) BEFORE it is recorded, so a cut-short turn can never be
// reported as a clean one. It packages the handling that runSpawn/runWorker do
// inline for the simpler notify paths (wake / schedule / inbox) that only record a
// reply:
//
//   - Watchdog fired (hard/idle) — detected via the ctx cancellation cause, so it
//     holds whether the loop returned the salvaged text with a nil error OR bailed
//     with a bare "context canceled": lead the salvaged text with the outcome note,
//     append the trace marker, and return a NIL error so the caller records an
//     explaining assistant reply instead of a cryptic recordTurnError string.
//   - Loop truncated itself (iteration cap / guardrail halt / context/output
//     exhaustion, err == nil): same salvage — note + marker — err stays nil.
//   - Clean turn, or a real provider error / human stop (not a deadline): returned
//     unchanged so the caller keeps its normal success / error branch.
//
// The returned truncated flag lets a caller suppress success-only follow-ups
// (FireTurnFinished, "completed" events) so a fragment is never signalled as a
// finished result.
func (r *Runtime) reconcileTurnOutcome(ctx context.Context, output string, steps []TurnStep, err error, hard, idle time.Duration) (string, []TurnStep, error, bool) {
	outcome := classifyTurnOutcome(ctx, steps, hard, idle)
	if !outcome.Truncated() {
		// Clean finish, or an error that is NOT a deadline (real provider fault, or a
		// human "Durdur"): leave (output, steps, err) for the caller's own branch.
		return output, steps, err, false
	}
	steps = appendOutcomeStep(steps, outcome)
	output = applyTurnOutcome(output, outcome)
	return output, steps, nil, true
}

// The idle-resume budget is the out-of-loop analogue of the in-loop recovery budgets
// in recovery.go. The idle watchdog fires OUTSIDE decideRecovery — the loop never
// observes ErrTurnIdleTimeout, its context is simply cancelled under it — so a turn
// that went quiet (a long non-streaming tool call that outran even the heartbeat, an
// alt-agent wait) entered NO recovery budget and its partial work stayed permanently
// half-done (FND-708844f8). runTurnWithIdleResume grants a configurable number of
// automatic re-runs (Tunables.IdleResumeMax, default DefaultIdleResumeMax = 1) under
// a fresh idle window before the turn is reported unfinished.
//
// Scope is deliberately narrow so it never fights the layers around it:
//   - IDLE only. A HARD wall-clock cut is a real ceiling; resuming would just blow
//     it again, so ErrTurnHardTimeout is never resumed (see turnHitIdleTimeout).
//   - Bounded + single-shot by default. Once the budget is spent, the final idle cut
//     is terminal and flows straight through to the "timeout"/unfinished reporting the
//     caller already does (Faz E) — no unbounded loop, and no double recovery with the
//     coordinator's higher-level re-task, which stays the SECOND line of defence after
//     this in-place first attempt.

// turnHitIdleTimeout reports whether ctx was cancelled specifically by the idle
// watchdog (ErrTurnIdleTimeout), as opposed to the hard ceiling (ErrTurnHardTimeout),
// a clean finish, or a human "Durdur" (context.Canceled). Only an idle cut is
// eligible for the single-shot resume. Reads the cancellation CAUSE, which
// withActivityTimeout records under WithCancelCause and keeps as the FIRST cause —
// so a later stop() (context.Canceled) never masks an idle cut the timer already set.
func turnHitIdleTimeout(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), ErrTurnIdleTimeout)
}

// runTurnWithIdleResume runs a background turn via invoke and grants it the
// idle-resume budget maxResume (Tunables.IdleResumeMax; 0 disables the resume). Each
// attempt runs under its OWN fresh watchdog window (hard, idle); invoke receives that
// per-attempt ctx, the 1-based attempt number, and the PRIOR attempt's salvaged
// output ("" on the first) so a resume can continue from where the interrupted work
// stopped rather than restart it — see resumeContinuationPrompt. The turn is re-run
// iff the just-finished attempt closed specifically on ErrTurnIdleTimeout AND the
// budget (maxResume) is not yet spent.
//
// It returns the FINAL attempt's (ctx, cancel, output, steps, err): the caller keeps
// its usual `defer cancel()` and its classifyTurnOutcome/reconcileTurnOutcome read
// the returned ctx, whose cancellation cause is that final attempt's — so a resumed
// turn that also idles still reports "timeout"/unfinished exactly as an un-resumed one
// would. Intermediate attempts' timers are released before the next window opens.
// invoke also receives the attempt's cancel func so a caller that lets an OUTSIDE
// goroutine abort the turn (the worker path, whose coordinator can stop_worker) can
// re-point its controller at the attempt currently in flight — otherwise a resume
// would leave the stop wired to the previous, already-cancelled attempt.
func (r *Runtime) runTurnWithIdleResume(
	parent context.Context,
	hard, idle time.Duration,
	maxResume int,
	invoke func(attemptCtx context.Context, cancel context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error),
) (context.Context, func(), string, []TurnStep, error) {
	var (
		output string
		steps  []TurnStep
		err    error
	)
	for attempt := 1; ; attempt++ {
		ctx, cancel := withActivityTimeout(parent, hard, idle)
		output, steps, err = invoke(ctx, cancel, attempt, output)
		// Resume ONLY on a genuine idle-watchdog cut, and only within budget (attempt
		// counts from 1, so `attempt > maxResume` first trips after maxResume resumes;
		// maxResume == 0 disables it outright). A hard cut, a clean finish, a
		// loop-internal terminal marker, or a human stop are all ineligible: returning
		// here hands the outcome to the caller's normal branch.
		if attempt > maxResume || !turnHitIdleTimeout(ctx) {
			return ctx, cancel, output, steps, err
		}
		cancel() // release this attempt's timers before opening the fresh window
		r.logger.Warn("turn: idle-timeout resume",
			"attempt", attempt, "maxResume", maxResume, "idleCap", idle, "hardCap", hard)
	}
}

// resumeContinuationPrompt builds the user turn for an idle resume. The prior
// attempt went quiet and was reclaimed mid-work; rather than restart from the
// original prompt (which would redo the finished part), lead the model with what it
// had already produced and tell it to continue. A blank fragment (the turn stalled
// before emitting any text) falls back to the original task unchanged. A blank
// original (a history-driven turn such as the coordinator drain, whose prompt is "")
// drops the "Original task" tail so the model just continues from the fragment +
// whatever the history-aware runner already composed.
func resumeContinuationPrompt(original, fragment string) string {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return original
	}
	p := "Your previous attempt was interrupted — it went idle and was reclaimed by " +
		"the activity watchdog before finishing. Here is the partial work you had " +
		"produced:\n\n" + fragment + "\n\n---\n\nContinue from where you left off and " +
		"finish the remaining work. Do NOT repeat what is already done, and break the " +
		"rest into smaller steps so you keep emitting progress."
	if strings.TrimSpace(original) != "" {
		p += " Original task:\n\n" + original
	}
	return p
}

// formatMinutes renders a deadline the way the settings UI states it.
func formatMinutes(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d sn", int(d.Seconds()))
	}
	return fmt.Sprintf("%d dk", int(d.Minutes()))
}
