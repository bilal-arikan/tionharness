package providers

import "strings"

// Approximate context-window sizes (tokens) by model family. Exact per-id tables
// rot fast (model IDs churn weekly), so knowledge is kept family-based and
// deliberately conservative: only families we are confident about are filled,
// everything else returns 0 ("unknown") so callers fall back rather than trust a
// guess. The 1M Claude tier is opt-in (beta), so the Claude family reports its
// 200K base.
const (
	// Claude 4.x is NOT one size: Opus 4.8 and Sonnet 4.6 ship a 1M window (the
	// long-context surcharge was dropped in 2026), while Haiku 4.5 stays at 200K.
	// So the Claude family must be matched per-tier, not with one flat value.
	windowClaudeOpusSonnet = 1_000_000 // Opus 4.8 / Sonnet 4.6
	windowHaiku            = 200_000   // Haiku 4.5
	windowClaudeOther      = 200_000   // Fable / generic Claude fallback (conservative)
	windowMiniMax          = 1_000_000 // MiniMax M-series (M3 ≈ 1,048,576, ≥512K guaranteed)
	windowDeepSeek         = 1_000_000 // DeepSeek V4 family ("1M context")
	windowGemini           = 1_000_000 // Gemini long-context family
)

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
	case strings.Contains(m, "fable"), strings.Contains(m, "claude"):
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
	case strings.Contains(m, "opus"), strings.Contains(m, "sonnet"):
		return 0.45
	case strings.Contains(m, "fable"), strings.Contains(m, "claude"):
		return 0.40
	default:
		return 0
	}
}
