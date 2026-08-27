package providers

import "strings"

// Approximate context-window sizes (tokens) by model family. Exact per-id tables
// rot fast (model IDs churn weekly), so knowledge is kept family-based and
// deliberately conservative: only families we are confident about are filled,
// everything else returns 0 ("unknown") so callers fall back rather than trust a
// guess. "Unknown" is not a licence for the caller to invent a large number:
// conversation.EffectiveBudget answers a 0 window with a conservative assumed
// window, so an unrecognised model gets a small budget, not the configured one.
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
	// The OpenAI GPT-5.6 line is tiered too: Sol and Terra ship ~1.05M
	// (1_048_576) while Luna stays at 400K. The frequently quoted 272K is NOT a
	// context limit — it is the long-context *pricing* threshold for Sol/Terra and,
	// separately, the Codex CLI's own fallback context_window for slugs it does not
	// recognise. We reuse that fallback for every other gpt-5.x / codex slug: it is
	// what the CLI itself assumes, so it can never over-promise.
	windowGPTLarge = 1_048_576 // GPT-5.6 Sol / Terra
	windowGPTLuna  = 400_000   // GPT-5.6 Luna
	windowGPTOther = 272_000   // other gpt-5.x / codex slugs (CLI fallback)
	// The GLM (Z.ai) line is tiered as well: glm-5.2 and glm-5.3 ship a 1M window
	// while every other GLM slug (glm-5, glm-5.1, glm-5-turbo, glm-4.7, glm-4.6)
	// stays at 200K. Expressed as 1_000_000 rather than 1_048_576 because docs.z.ai
	// quotes a round "1M", exactly like the MiniMax/DeepSeek/Gemini entries above —
	// only the GPT-5.6 large tier has a documented exact 1_048_576.
	windowGLMLarge = 1_000_000 // glm-5.2 / glm-5.3
	windowGLMOther = 200_000   // glm-5 / 5.1 / 5-turbo / 4.7 / 4.6
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
	maxOutDeepSeek      = 32_768 // DeepSeek V4 family (real ceiling 384K; kept well below)
	maxOutGemini        = 8_192  // Gemini family (conservative)
	// GPT-5.6's real ceiling is 128K output for every tier, so a single family
	// value is enough; 32_768 keeps the same safety margin as the Claude-capable
	// tier (≈1/4 of the ceiling) while staying far above the 4096 provider
	// fallback that would otherwise truncate ordinary answers.
	maxOutGPT = 32_768
	// GLM's real ceiling is 128K output across the whole family (docs.z.ai), so a
	// single family value suffices; 32_768 keeps the same ≈1/4-of-ceiling margin as
	// the GPT and Claude-capable tiers while staying far above the 4096 provider
	// fallback.
	maxOutGLM = 32_768
)

// gptFamily reports whether the slug belongs to the OpenAI GPT-5.x / Codex family.
// The gate is deliberately narrow: a bare "gpt" substring also matches gpt-4o,
// gpt-4.1, gpt-4o-mini and friends, whose windows are much smaller (128K for
// 4o-mini) than anything in the table below — claiming 272K for them is an
// OVER-estimate, the dangerous direction, since compaction would then fire too
// late. Only the slugs we actually verified are claimed; every other "gpt" model
// falls through to the unknown branch (0), matching the file's philosophy of
// filling only families we are confident about.
// The tier keywords ("sol", "terra", "luna") are short and would collide with
// unrelated names (e.g. "solar"), so tier matching is only ever done after this
// gate passes.
func gptFamily(m string) bool {
	return strings.Contains(m, "gpt-5") || strings.Contains(m, "gpt5") || strings.Contains(m, "codex")
}

// glmFamily reports whether the slug belongs to the Z.ai GLM family.
func glmFamily(m string) bool { return strings.Contains(m, "glm") }

// glmLargeWindow reports whether the GLM slug is one of the 1M-window tiers.
// Matching is on the full "glm-5.2"/"glm-5.3" token, never a "glm-5" prefix: the
// prefix would swallow 5.2/5.3 into the 200K tier (or, checked the other way,
// promote plain glm-5 / glm-5.1 to 1M). Both mistakes are silent, so this stays
// an explicit two-value check.
func glmLargeWindow(m string) bool {
	return strings.Contains(m, "glm-5.2") || strings.Contains(m, "glm-5.3")
}

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
	case gptFamily(m):
		return maxOutGPT
	case glmFamily(m):
		return maxOutGLM
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
	case gptFamily(m):
		// Tier order matters: "gpt-5.6-sol" must hit the large tier, not the
		// generic gpt fallback.
		switch {
		case strings.Contains(m, "sol"), strings.Contains(m, "terra"):
			return windowGPTLarge
		case strings.Contains(m, "luna"):
			return windowGPTLuna
		default:
			return windowGPTOther
		}
	case glmFamily(m):
		if glmLargeWindow(m) {
			return windowGLMLarge
		}
		return windowGLMOther
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
	case gptFamily(m):
		// Same rationale as the MiniMax/DeepSeek/Gemini long-context families: the
		// window is large but recall precision degrades over a big raw transcript,
		// so keep the conservative 0.35 share and let retrieval carry the rest.
		return 0.35
	case glmFamily(m):
		// Same rationale as the other long-context families: even the 1M tiers lose
		// recall precision over a big raw transcript, and the 200K tiers are simply
		// small — 0.35 fits both, with retrieval carrying the rest.
		return 0.35
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
