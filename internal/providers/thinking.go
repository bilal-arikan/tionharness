package providers

import (
	"fmt"
	"strings"
)

// validThinkingLevels is the single definition of the reasoning-tier tokens the
// system accepts, in ramp order. Everything that stores or requests a level
// (agent rows, the agent API, the per-turn chat override) validates against
// this list instead of carrying its own copy.
//
// The empty string is deliberately NOT a member. It used to be stored as a
// third state next to "off" and meant two different things at runtime — no
// thinking on the native API path, but "high" effort on the CLI path — so it is
// now rejected at write time and backfilled for legacy rows (see
// db.BackfillThinkingLevels).
var validThinkingLevels = []string{"off", "low", "medium", "high", "xhigh", "max"}

// ValidThinkingLevels returns the accepted reasoning-tier tokens in ramp order.
func ValidThinkingLevels() []string {
	out := make([]string, len(validThinkingLevels))
	copy(out, validThinkingLevels)
	return out
}

// IsValidThinkingLevel reports whether level is one of the accepted tokens. The
// empty string is not.
func IsValidThinkingLevel(level string) bool {
	for _, v := range validThinkingLevels {
		if v == level {
			return true
		}
	}
	return false
}

// ValidateThinkingLevel checks a requested reasoning level twice: that the token
// itself is known, and that it is meaningful on the given model per
// ThinkingTiersFor. A tier outside the model's set would be a silent no-op (or,
// on the always-on class, a dropped field), so it is reported as an error rather
// than accepted and ignored. Bare family aliases and an empty model id land in
// the "alias" class, which offers the full ramp — nothing is rejected there.
func ValidateThinkingLevel(model, level string) error {
	if level == "" {
		return fmt.Errorf("thinkingLevel is required (one of: %s)", strings.Join(validThinkingLevels, ", "))
	}
	if !IsValidThinkingLevel(level) {
		return fmt.Errorf("unknown thinkingLevel %q (one of: %s)", level, strings.Join(validThinkingLevels, ", "))
	}
	tiers := ThinkingTiersFor(model)
	for _, t := range tiers {
		if t == level {
			return nil
		}
	}
	return fmt.Errorf("thinkingLevel %q is not supported by model %q (supported: %s)",
		level, model, strings.Join(tiers, ", "))
}

// Thinking wire formats on the Anthropic Messages API differ by model class:
//
//   - Adaptive class (Fable/Mythos 5, Opus 4.7/4.8, Sonnet 5): the legacy
//     {type:"enabled", budget_tokens:N} shape is REMOVED and returns 400.
//     Thinking is requested as {type:"adaptive"} and depth is steered with
//     output_config.effort. Within this class Fable/Mythos are always-on:
//     an explicit {type:"disabled"} also 400s, so "off" omits the field.
//   - Everything else (Opus/Sonnet 4.6 and older, Haiku, and non-Claude
//     endpoints that speak the Anthropic protocol such as MiniMax): the legacy
//     enabled+budget shape still applies.

// UsesAdaptiveThinking reports whether the model rejects the legacy
// enabled+budget_tokens thinking shape and requires {type:"adaptive"} +
// output_config.effort instead.
func UsesAdaptiveThinking(model string) bool {
	if AlwaysOnThinking(model) {
		return true
	}
	m := strings.ToLower(model)
	for _, s := range []string{"opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8", "sonnet-5"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// AlwaysOnThinking reports whether the model runs with thinking permanently on
// (the Fable/Mythos 5 class): both the legacy enabled shape AND an explicit
// {type:"disabled"} return 400 there, so "off" must omit the thinking field
// entirely (the server thinks anyway).
func AlwaysOnThinking(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "fable") || strings.Contains(m, "mythos")
}

// SupportsTaskBudget reports whether the model accepts the (beta) task-budget
// directive (output_config.task_budget + the task-budgets beta header): the
// model sees a running token countdown for the whole agentic loop and paces
// itself. Supported on exactly the adaptive-thinking class (Fable/Mythos 5,
// Opus 4.7/4.8, Sonnet 5).
func SupportsTaskBudget(model string) bool { return UsesAdaptiveThinking(model) }

// SupportsStructuredOutputs reports whether the model accepts
// output_config.format (JSON-schema-constrained replies). Per the capability
// matrix: Fable/Mythos 5, Opus 4.8, Sonnet 5, Haiku 4.5, plus legacy Opus
// 4.5/4.1 — notably NOT Opus 4.6/4.7 or Sonnet 4.6.
func SupportsStructuredOutputs(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "fable") || strings.Contains(m, "mythos") {
		return true
	}
	for _, s := range []string{"opus-4-8", "opus-4.8", "sonnet-5", "haiku-4-5", "haiku-4.5", "opus-4-5", "opus-4.5", "opus-4-1", "opus-4.1"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// SupportsSystemInMessages reports whether the model accepts mid-conversation
// {"role":"system"} entries in the messages array (the cache-safe, non-spoofable
// operator channel). Claude Opus 4.8 only.
func SupportsSystemInMessages(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "opus-4-8") || strings.Contains(m, "opus-4.8")
}

// SupportsDynamicWebTools reports whether the model accepts the _20260209 web
// search/fetch variants (dynamic filtering): the Claude 4.6+ class — Opus
// 4.6/4.7/4.8, Sonnet 4.6, Sonnet 5, Fable/Mythos. Older models use the basic
// variants instead.
func SupportsDynamicWebTools(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "fable") || strings.Contains(m, "mythos") {
		return true
	}
	for _, s := range []string{
		"opus-4-6", "opus-4.6", "opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8",
		"sonnet-4-6", "sonnet-4.6", "sonnet-5",
	} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// SupportsProgrammaticTools reports whether the model supports programmatic
// tool calling (code_execution_20260120 + allowed_callers): Claude Opus 4.5+
// and Sonnet 4.5+ (incl. Sonnet 5) and the Fable/Mythos class.
func SupportsProgrammaticTools(model string) bool {
	m := strings.ToLower(model)
	if strings.Contains(m, "fable") || strings.Contains(m, "mythos") {
		return true
	}
	for _, s := range []string{
		"opus-4-5", "opus-4.5", "opus-4-6", "opus-4.6", "opus-4-7", "opus-4.7", "opus-4-8", "opus-4.8",
		"sonnet-4-5", "sonnet-4.5", "sonnet-4-6", "sonnet-4.6", "sonnet-5",
	} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// ThinkingClass classifies how a model handles extended reasoning. It is the
// single classifier the UI reads twice: ThinkingTiersFor gates which tier
// buttons are active, and the client uses the class to explain WHY an inactive
// tier is inactive. One of:
//
//   - "always-on": thinking cannot be disabled (Fable/Mythos) — {type:"disabled"}
//     400s and "off" merely omits the field, so "off" is dropped.
//   - "adaptive": the full effort ramp incl. xhigh/max, reaching the model as
//     output_config.effort (Opus 4.7/4.8, Sonnet 5).
//   - "non-thinking": the model does not reason at all, so only "off" is
//     meaningful (DeepSeek V4 Flash — "düşünmeyen mod").
//   - "legacy": a concrete model that reasons but has no distinct xhigh/max wire
//     form, so those clamp down to high (legacy Claude, MiniMax — whose
//     reasoning_effort tops out at "high" — DeepSeek Pro, …).
//   - "alias": a bare family alias / custom / empty id ("opus", "sonnet",
//     "Varsayılan") whose concrete model is unknown here; offered the full ramp
//     and clamped provider-side.
func ThinkingClass(model string) string {
	switch {
	case AlwaysOnThinking(model):
		return "always-on"
	case UsesAdaptiveThinking(model):
		return "adaptive"
	case isNonThinking(model):
		return "non-thinking"
	case hasConcreteVersion(model):
		return "legacy"
	default:
		return "alias"
	}
}

// isNonThinking reports whether the model has no extended-reasoning mode at all,
// so every tier but "off" is a no-op. DeepSeek's V4 Flash tier is the sole
// current case; its Pro sibling reasons and is left in the "legacy" class.
func isNonThinking(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "deepseek") && strings.Contains(m, "flash")
}

// ThinkingTiersFor returns the reasoning tiers a model meaningfully supports, as
// the stable tokens the UI pickers use: "off","low","medium","high","xhigh",
// "max". The set is derived from ThinkingClass so the composer / agent pickers
// can grey out tiers that would be a silent no-op on the selected model (they
// are shown disabled with a reason, not hidden).
func ThinkingTiersFor(model string) []string {
	switch ThinkingClass(model) {
	case "always-on":
		return []string{"low", "medium", "high", "xhigh", "max"}
	case "adaptive", "alias":
		return []string{"off", "low", "medium", "high", "xhigh", "max"}
	case "non-thinking":
		return []string{"off"}
	default: // legacy
		return []string{"off", "low", "medium", "high"}
	}
}

// hasConcreteVersion reports whether the model id carries a version digit, i.e.
// it names a concrete model rather than a bare family alias ("opus", "sonnet").
// Bare aliases resolve to whatever the provider currently points them at (often
// the adaptive flagship), so they get the full ramp rather than the clamped
// legacy set.
func hasConcreteVersion(model string) bool {
	return strings.ContainsAny(model, "0123456789")
}

// EffortForThinkingBudget maps a legacy thinking token budget (as produced by
// the agent's ThinkingLevel) to the output_config.effort value used by
// adaptive-class models. 0 means "no override" (server default). The xhigh/max
// tiers exist only on the adaptive class; the legacy enabled+budget path clamps
// those budgets down instead (see thinkingFor).
func EffortForThinkingBudget(budget int) string {
	switch {
	case budget <= 0:
		return ""
	case budget <= 2048:
		return "low"
	case budget <= 8192:
		return "medium"
	case budget <= 16384:
		return "high"
	case budget <= 32768:
		return "xhigh"
	default:
		return "max"
	}
}
