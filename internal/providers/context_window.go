package providers

import "strings"

// Approximate context-window sizes (tokens) by model family. Exact per-id tables
// rot fast (model IDs churn weekly), so knowledge is kept family-based and
// deliberately conservative: only families we are confident about are filled,
// everything else returns 0 ("unknown") so callers fall back rather than trust a
// guess. The 1M Claude tier is opt-in (beta), so the Claude family reports its
// 200K base.
const (
	windowClaude   = 200_000   // Claude 4.x base (1M is the opt-in beta tier)
	windowMiniMax  = 1_000_000 // MiniMax M-series (M3 ≈ 1,048,576, ≥512K guaranteed)
	windowDeepSeek = 1_000_000 // DeepSeek V4 family ("1M context")
	windowGemini   = 1_000_000 // Gemini long-context family
)

// ContextWindowFor returns the approximate context-window size in tokens for a
// provider/model, or 0 when unknown. It is the single source of truth used by
// the catalog (UI display) and tool-output threshold scaling (CG-9). provider is
// accepted for future disambiguation; matching is currently by model family.
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
	case strings.Contains(m, "claude"), strings.Contains(m, "fable"),
		m == "opus", m == "sonnet", m == "haiku":
		return windowClaude
	default:
		return 0
	}
}
