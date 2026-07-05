package conversation

import "github.com/bilal-arikan/tionswarm/internal/providers"

// Model-aware transcript budget (Option B). A flat default budget wastes a large
// model's context window: a 200K–1M model could keep far more history before
// compaction than a 12K budget allows. EffectiveBudget lifts the budget toward a
// fraction of the model's window when that window is known, bounded so cost stays
// sane and never dropping below the configured value. Both knobs (fraction, ceil)
// are settings-configurable (ContextBudgetFraction / ContextBudgetCeil) and
// default to the values below.
const (
	// defaultBudgetWindowFraction is the *fallback* share of a model's window kept
	// as raw transcript before compacting, used only when neither settings nor the
	// per-family adaptive table (providers.AdaptiveBudgetFraction) supplies one — in
	// practice only for families whose window is also unknown, where the fraction is
	// moot. A fraction of 0 in settings/Manager means "auto" → the adaptive table
	// picks a family-appropriate value (see _Docs/17 §12).
	defaultBudgetWindowFraction = 0.4
	// defaultBudgetAutoCeil caps the auto-derived budget (tokens) so a 1M-window
	// model can't silently run runaway-expensive turns. For 1M models this ceiling
	// is the operative number (window*fraction exceeds it), so it is the main knob
	// for "how much of a big window we actually use". Lowered 512K→256K (2026-06-25,
	// _Docs/17 §12): 256K keeps the live window in the gradient's high-precision zone
	// (~¼ of 512K's n² attention surface) while the retrieval layer carries durability
	// of folded detail. Users who want more raise it from Settings (or
	// TIONSWARM_CONTEXT_BUDGET_CEIL); MaxContextTokens is the floor.
	defaultBudgetAutoCeil = 262144
)

// EffectiveBudget returns the transcript token budget for an agent's model. The
// configured value (settings.MaxContextTokens / TIONSWARM_MAX_CONTEXT_TOKENS) is
// the floor; when the model's context window is known we allow a larger budget —
// clamp(window * fraction, configured, ceil) — so big-context models aren't
// pinned to the small default. An unknown window (0) falls back to configured, so
// behaviour is unchanged for models without metadata. fraction/ceil are the live
// settings values; non-positive values fall back to the package defaults.
func EffectiveBudget(provider, model string, configured int, fraction float64, ceil int) int {
	if configured <= 0 {
		configured = defaultMaxTokens
	}
	if ceil <= 0 {
		ceil = defaultBudgetAutoCeil
	}
	window := providers.ContextWindowFor(provider, model)
	if window <= 0 {
		return configured
	}
	// fraction<=0 means "auto": pick a family-appropriate share (context-rot aware),
	// falling back to the package default only for families the table doesn't cover.
	if fraction <= 0 {
		fraction = providers.AdaptiveBudgetFraction(provider, model)
		if fraction <= 0 {
			fraction = defaultBudgetWindowFraction
		}
	}
	derived := int(float64(window) * fraction)
	if derived > ceil {
		derived = ceil
	}
	if derived < configured {
		derived = configured
	}
	return derived
}
