package conversation

import "github.com/bilal-arikan/swarmgo/internal/providers"

// Model-aware transcript budget (Option B). A flat default budget wastes a large
// model's context window: a 200K–1M model could keep far more history before
// compaction than a 12K budget allows. EffectiveBudget lifts the budget toward a
// fraction of the model's window when that window is known, bounded so cost stays
// sane and never dropping below the configured value.
const (
	// budgetWindowFraction is the share of a model's context window we are willing
	// to spend on live transcript before compacting. The rest of the window is
	// headroom for tool output, the system prompt and the reply.
	budgetWindowFraction = 0.20
	// budgetAutoCeil caps the auto-derived budget (tokens) so a 1M-window model
	// can't silently run runaway-expensive turns. For 1M models this ceiling is the
	// operative number (window*fraction exceeds it), so it is the main knob for "how
	// much of a big window we actually use": 128K ≈ 12.8% of a 1M window. Users who
	// want more raise SWARMGO_MAX_CONTEXT_TOKENS explicitly (it is the floor).
	budgetAutoCeil = 128000
)

// EffectiveBudget returns the transcript token budget for an agent's model. The
// configured value (settings.MaxContextTokens / SWARMGO_MAX_CONTEXT_TOKENS) is
// the floor; when the model's context window is known we allow a larger budget —
// clamp(window * fraction, configured, ceil) — so big-context models aren't
// pinned to the small default. An unknown window (0) falls back to configured, so
// behaviour is unchanged for models without metadata.
func EffectiveBudget(provider, model string, configured int) int {
	if configured <= 0 {
		configured = defaultMaxTokens
	}
	window := providers.ContextWindowFor(provider, model)
	if window <= 0 {
		return configured
	}
	derived := int(float64(window) * budgetWindowFraction)
	if derived > budgetAutoCeil {
		derived = budgetAutoCeil
	}
	if derived < configured {
		derived = configured
	}
	return derived
}
