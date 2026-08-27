package conversation

import (
	"log/slog"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

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
	// TIONHARNESS_CONTEXT_BUDGET_CEIL); MaxContextTokens is the floor.
	defaultBudgetAutoCeil = 262144
	// budgetWindowShare caps EVERY budget — including one lifted by the configured
	// floor — at this share of the model's real context window. Without it the floor
	// silently outranks physics: with the shipped default (MaxContextTokens =
	// 800000) a 200K model was told to keep 800K of transcript, so compaction could
	// never fire before the API rejected the request and every long turn ended in
	// the context-overflow recovery path instead of a clean fold. The budget is
	// weighed against messages PLUS the fixed per-turn overhead (see Compact), so
	// the remaining 20% is what the reply and the estimator's error margin live in.
	budgetWindowShare = 0.8
	// unknownModelWindow is the context window ASSUMED for a model family
	// providers.ContextWindowFor does not recognise. An unknown model used to
	// inherit the configured floor verbatim, i.e. an 800K transcript budget for a
	// model that may really have 32K — wrong in the dangerous direction, and
	// invisible until the API refused the turn. 128K is the smallest window still
	// plausible for a current model, so assuming it under-promises rather than
	// over-promises; the model is also logged once (see noteUnknownModelWindow) so
	// the missing family entry is fixable instead of silent.
	unknownModelWindow = 128_000
)

// unknownModelWindowSeen dedupes the unknown-model warning: EffectiveBudget runs on
// every turn, so without it one unrecognised model would flood the Logs screen.
var unknownModelWindowSeen sync.Map

// noteUnknownModelWindow logs, once per provider/model, that the family is unknown
// and the budget therefore falls back to the conservative assumed window. The empty
// model is exempt: that is claude-cli's "default" alias, where the concrete model is
// resolved inside the CLI and an unknown window here is expected, not a gap.
func noteUnknownModelWindow(provider, model string, budget int) {
	if model == "" {
		return
	}
	if _, dup := unknownModelWindowSeen.LoadOrStore(provider+"/"+model, struct{}{}); dup {
		return
	}
	slog.Warn("unknown model context window; using safe transcript budget",
		"provider", provider, "model", model,
		"assumedWindow", unknownModelWindow, "budget", budget)
}

// capToWindow bounds a budget at budgetWindowShare of window. window must be > 0.
func capToWindow(budget, window int) int {
	limit := int(float64(window) * budgetWindowShare)
	if budget > limit {
		return limit
	}
	return budget
}

// EffectiveBudget returns the transcript token budget for an agent's model. The
// configured value (settings.MaxContextTokens / TIONHARNESS_MAX_CONTEXT_TOKENS) is
// the floor; when the model's context window is known we allow a larger budget —
// clamp(window * fraction, configured, ceil) — so big-context models aren't
// pinned to the small default. Every result is then capped at budgetWindowShare of
// the model's window, so the floor can lift the budget but never above the model's
// real capacity. An unknown window (0) falls back to the configured value capped by
// the conservative unknownModelWindow, and the model is logged once. fraction/ceil
// are the live settings values; non-positive values fall back to the package
// defaults.
func EffectiveBudget(provider, model string, configured int, fraction float64, ceil int) int {
	if configured <= 0 {
		configured = defaultMaxTokens
	}
	if ceil <= 0 {
		ceil = defaultBudgetAutoCeil
	}
	window := providers.ContextWindowFor(provider, model)
	if window <= 0 {
		// Unknown family: fall back to the conservative assumed window instead of
		// trusting the configured floor, which knows nothing about this model.
		safe := capToWindow(configured, unknownModelWindow)
		noteUnknownModelWindow(provider, model, safe)
		return safe
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
	// The floor lifts, but never past what the model can actually hold.
	return capToWindow(derived, window)
}
