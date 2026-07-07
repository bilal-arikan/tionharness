package providers

import "strings"

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
