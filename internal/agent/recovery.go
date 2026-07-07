package agent

import (
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// contReason tags why the loop continued to another iteration (a non-terminal
// transition). Stored on loopState so tests can assert a recovery path fired
// without inspecting message contents — the pattern claude-code's query loop
// uses with its State.transition field.
type contReason string

const (
	contToolUse        contReason = "tool_use"                   // model requested tools
	contMaxTokenResume contReason = "max_output_tokens_recovery" // resume after output cap
	contCompactRetry   contReason = "reactive_compact_retry"     // context overflow → compact → retry
	contProviderRetry  contReason = "provider_retry"             // transient provider fault → backoff → retry
)

// termReason tags why the loop returned (a terminal transition). Surfaced as the
// Reason on a StepError/StepRecovery so a persisted trace explains itself.
type termReason string

const (
	termCompleted         termReason = "completed"
	termMaxIters          termReason = "max_tool_iterations"
	termProviderErr       termReason = "provider_error"
	termCancelled         termReason = "cancelled"
	termMaxTokenExhausted termReason = "max_output_tokens_exhausted"
)

// loopState carries the single-shot recovery guards across loop iterations. The
// guards make every recovery path fire at most its allotted number of times, so
// a stuck model can never spin forever inside one turn.
type loopState struct {
	maxTokenRetries int        // 0..cfg.maxTokenLimit
	compacted       bool       // reactive compaction is one-shot per turn
	providerRetries int        // 0..cfg.maxProviderRetries (transient provider faults)
	lastContinue    contReason // why the previous iteration continued ("" on first)
}

// recoveryConfig is the resolved, settings-driven policy the loop hands to
// decideRecovery each iteration. Kept separate from loopState (the mutable
// guards) so the decision stays a pure function of (result, guards, policy).
type recoveryConfig struct {
	maxTokenLimit      int  // resume budget after the output cap (0 = resume disabled)
	reactiveCompact    bool // whether context-overflow compaction-and-retry is allowed
	maxProviderRetries int  // retry budget for transient provider faults (0 = retry disabled)
}

// decision is the output of the pure recovery analysis: it tells the loop body
// what to do next without performing any side effects itself. Exactly one of the
// outcomes is meaningful — cont (continue after injecting), compact (fold then
// retry), or a terminal (term set, optionally err) — keeping the loop's apply
// step a simple switch.
type decision struct {
	cont    bool               // continue to the next iteration as-is/after inject
	compact bool               // run reactive compaction, then continue
	reason  contReason         // machine tag for a cont/compact transition
	inject  *providers.Message // message to append before the next iteration
	backoff time.Duration      // ctx-aware wait before the retry (provider_retry only)
	term    termReason         // terminal tag when neither cont nor compact
	err     error              // non-nil → propagate as the loop's error return
}

// decideRecovery is the pure heart of A1: given the latest model result (resp)
// or provider error (callErr) plus the current guard state, it decides whether
// the loop continues (and how) or terminates (and why). It mutates nothing — the
// caller applies the decision and advances loopState — so it is exhaustively
// table-testable in isolation from the provider and the message plumbing.
func decideRecovery(resp *providers.Response, callErr error, st loopState, cfg recoveryConfig) decision {
	if callErr != nil {
		switch cls := classifyProviderError(callErr); {
		// Context overflow is recoverable once per turn by compacting the
		// in-flight history and retrying — when enabled.
		case cls == errContextOverflow && cfg.reactiveCompact && !st.compacted:
			return decision{compact: true, reason: contCompactRetry, err: callErr}
		// A user stop must terminate immediately — retrying a cancelled call
		// would resurrect a turn the user just killed.
		case cls == errCancelled:
			return decision{term: termCancelled, err: callErr}
		// Transient provider faults (429/5xx/timeout) are retried with the SAME
		// request after a jittered backoff, bounded by the per-turn budget.
		// Deterministic classes (auth/billing/unknown) never enter this path.
		case cls.retryable() && st.providerRetries < cfg.maxProviderRetries:
			return decision{cont: true, reason: contProviderRetry, backoff: retryBackoff(st.providerRetries), err: callErr}
		default:
			return decision{term: termProviderErr, err: callErr}
		}
	}
	// The model stopped because it ran into the output-token cap mid-answer.
	// Resume it (up to the configured budget) so the full answer is produced
	// across several capped calls instead of being silently truncated.
	if resp != nil && resp.StopReason == providers.StopMaxTok {
		if st.maxTokenRetries < cfg.maxTokenLimit {
			return decision{cont: true, reason: contMaxTokenResume, inject: resumeMessage()}
		}
		return decision{term: termMaxTokenExhausted}
	}
	return decision{term: termCompleted}
}

// resumeMessage is the meta user-turn injected to continue an answer cut off by
// the output-token cap. Phrased (in English, the model-facing language) to make
// the model pick up mid-thought without apologising or recapping.
func resumeMessage() *providers.Message {
	return &providers.Message{
		Role: providers.RoleUser,
		Text: "Output token limit reached. Continue exactly where you left off — " +
			"no apology, no recap. Pick up mid-thought if that is where the cut " +
			"happened, and break the remaining work into smaller pieces.",
	}
}

// recoveryText is the human-readable (Turkish, UI-facing) explanation shown on a
// StepRecovery card for a given continuation reason.
func recoveryText(r contReason) string {
	switch r {
	case contMaxTokenResume:
		return "Çıktı token limitine ulaşıldı; yanıt kaldığı yerden sürdürülüyor."
	case contCompactRetry:
		return "Bağlam taşması algılandı; eski mesajlar özetlenip tur yeniden denendi."
	case contProviderRetry:
		return "Geçici sağlayıcı hatası (aşırı yük/zaman aşımı); kısa bekleme sonrası yeniden denendi."
	default:
		return "Tur kurtarma yoluna girdi."
	}
}

// isContextOverflow reports whether a provider error signals the prompt exceeded
// the model's context window (Anthropic: "prompt is too long: N tokens > …";
// OpenAI-compatible: "context_length_exceeded"). Matched conservatively so an
// unrelated failure is never mistaken for a recoverable overflow.
func isContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "too long"):
		return true
	case strings.Contains(msg, "context_length_exceeded"):
		return true
	case strings.Contains(msg, "context length"):
		return true
	case strings.Contains(msg, "maximum context"):
		return true
	default:
		return false
	}
}
