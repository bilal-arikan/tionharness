package providers

import "strings"

// Approximate context-window sizes (tokens) by model family. Exact per-id tables
// rot fast (model IDs churn weekly), so knowledge is kept family-based and
// deliberately conservative: only families we are confident about are filled,
// everything else returns 0 ("unknown") so callers fall back rather than trust a
// guess. The 1M Claude tier is opt-in (beta), so the Claude family reports its
// 200K base.
const (
	// Claude 4.x/5 is NOT one size: Opus 4.8, Sonnet 4.6 and Fable 5 ship a 1M
	// window (the long-context surcharge was dropped in 2026), while Haiku 4.5
	// stays at 200K. So the Claude family must be matched per-tier, not with one
	// flat value.
	windowClaudeOpusSonnet = 1_000_000 // Opus 4.8 / Sonnet 4.6
	windowFable            = 1_000_000 // Fable 5 (1M standard, no beta opt-in)
	windowHaiku            = 200_000   // Haiku 4.5
	windowClaudeOther      = 200_000   // generic Claude fallback (conservative)
	windowMiniMax          = 1_000_000 // MiniMax M-series (M3 ≈ 1,048,576, ≥512K guaranteed)
	windowDeepSeek         = 1_000_000 // DeepSeek V4 family ("1M context")
	windowGemini           = 1_000_000 // Gemini long-context family
)

// Per-family generation caps (max output tokens), used to fill Request.MaxTokens
// when a caller leaves it unset. Same philosophy as the context-window table:
// family-based, deliberately conservative. Each value sits comfortably BELOW the
// family's known maximum so a new family member can never produce a request the
// API rejects (an over-cap max_tokens is a hard 400, unlike an over-estimated
// context window which only shifts compaction timing) — yet well ABOVE the
// providers' 4096 fallback so ordinary answers finish in one call instead of
// being truncated and resumed by the turn-recovery loop. Raising max_tokens has
// no cost or rate-limit downside (billing is per actual output token), so the
// only constraint is staying within the real ceiling.
const (
	maxOutClaudeCapable = 32_768 // Opus / Sonnet 4.x / Fable 5 (real ceiling 64–128K)
	maxOutClaudeSmall   = 16_384 // Haiku / generic Claude
	maxOutMiniMax       = 32_768 // MiniMax M-series (M3 ceiling ≈ 512K)
	maxOutDeepSeek      = 8_192  // DeepSeek family (conservative)
	maxOutGemini        = 8_192  // Gemini family (conservative)
)

// MaxOutputFor returns the model-aware generation cap (max output tokens) for a
// provider/model, or 0 when the family is unknown. It is the single source of
// truth the agent loop uses to fill Request.MaxTokens so the output cap tracks
// the model's real capacity rather than the providers' conservative 4096
// fallback — which otherwise truncates answers mid-stream and forces the
// turn-recovery (A1) loop to resume far more often than necessary. Matching is by
// model family, mirroring ContextWindowFor; order matters so the more specific
// tier (haiku) is checked before the broad opus/sonnet/claude rules. provider is
// accepted for future disambiguation. Unknown families return 0 so the caller
// falls back to the provider default.
func MaxOutputFor(provider, model string) int {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		// claude-cli "default": the CLI manages its own output cap, so the value
		// is moot — return unknown and let the caller leave MaxTokens unset.
		return 0
	}
	switch {
	case strings.Contains(m, "minimax"):
		return maxOutMiniMax
	case strings.Contains(m, "deepseek"):
		return maxOutDeepSeek
	case strings.Contains(m, "gemini"):
		return maxOutGemini
	case strings.Contains(m, "haiku"):
		return maxOutClaudeSmall
	case strings.Contains(m, "opus"), strings.Contains(m, "sonnet"), strings.Contains(m, "fable"):
		return maxOutClaudeCapable
	case strings.Contains(m, "claude"):
		return maxOutClaudeSmall
	default:
		return 0
	}
}

// ContextWindowFor returns the approximate context-window size in tokens for a
// provider/model, or 0 when unknown. It is the single source of truth used by
// the catalog (UI display) and tool-output threshold scaling (CG-9). provider is
// accepted for future disambiguation; matching is currently by model family.
// Order matters: the more specific tier (haiku) is checked before the broad
// opus/sonnet/claude rules.
func ContextWindowFor(provider, model string) int {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		// claude-cli "default": resolves to the live session model — unknown here.
		return 0
	}
	switch {
	case strings.Contains(m, "minimax"):
		return windowMiniMax
	case strings.Contains(m, "deepseek"):
		return windowDeepSeek
	case strings.Contains(m, "gemini"):
		return windowGemini
	case strings.Contains(m, "haiku"):
		return windowHaiku
	case strings.Contains(m, "opus"), strings.Contains(m, "sonnet"):
		return windowClaudeOpusSonnet
	case strings.Contains(m, "fable"):
		return windowFable
	case strings.Contains(m, "claude"):
		return windowClaudeOther
	default:
		return 0
	}
}

// AdaptiveBudgetFraction returns the share of a model's context window to keep as
// raw live transcript before compaction, chosen per family. It is a context-rot
// mitigation: a bigger raw window grows the n² attention surface and erodes recall
// precision (a gradient, not a cliff — see _Docs/17 §12), so long-context-reliable
// families get a larger share while smaller/unknown models stay conservative.
// Durability of folded-away detail is carried by the retrieval layer (memory /
// conversation_search / core blocks), NOT by the raw window size — so a smaller
// fraction loses no recall, it only shifts recovery from "huge window" to "cheap
// retrieval". A fraction of 0 in settings/Manager means "auto" and resolves here.
// Returns 0 for unknown families so the caller falls back to its own default; for
// those the window is also unknown (0), so the fraction is moot anyway.
func AdaptiveBudgetFraction(provider, model string) float64 {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return 0
	}
	switch {
	case strings.Contains(m, "minimax"):
		return 0.35
	case strings.Contains(m, "deepseek"):
		return 0.35
	case strings.Contains(m, "gemini"):
		return 0.35
	case strings.Contains(m, "haiku"):
		return 0.40
	case strings.Contains(m, "opus"), strings.Contains(m, "sonnet"), strings.Contains(m, "fable"):
		return 0.45
	case strings.Contains(m, "claude"):
		return 0.40
	default:
		return 0
	}
}
