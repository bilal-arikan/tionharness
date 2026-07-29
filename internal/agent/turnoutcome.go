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
						"Kalan işi daha küçük parçalara böl ya da limiti yükselt (TIONSWARM_MAX_TOOL_ITERS).",
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

// formatMinutes renders a deadline the way the settings UI states it.
func formatMinutes(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d sn", int(d.Seconds()))
	}
	return fmt.Sprintf("%d dk", int(d.Minutes()))
}
