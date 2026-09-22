package providers

import "strings"

// Coarse-effort reasoning: DeepSeek V4.x and the Z.ai GLM-5.3 family.
//
// These third-party models steer reasoning depth with a three-level effort enum
// — low | high | max — rather than a token budget. Their Anthropic-compatible
// endpoints accept thinking.budget_tokens but IGNORE it (depth rides
// output_config.effort), and their OpenAI-compatible endpoints take a top-level
// reasoning_effort with the same three values. The legacy enabled+budget shape
// on its own therefore never changed anything: every request ran at the server
// default (DeepSeek "high", GLM-5.3 "max").
//
// Both vendors also reason by DEFAULT, so "off" has to be explicit. DeepSeek
// accepts thinking {type:"disabled"}; the GLM-5.3 family cannot stop reasoning at
// all (Z.ai documents that "disabled" fails the request), so "off" is served as
// the lowest effort there instead.

// UsesCoarseEffort reports whether the model's reasoning depth is the three-level
// effort enum described above.
func UsesCoarseEffort(model string) bool {
	m := strings.ToLower(model)
	return deepseekV4Family(m) || glm53Family(m)
}

// ForcedThinking reports whether reasoning cannot be switched off on the model:
// the GLM-5.3 family (glm-5.3, glm-5.3-flash, glm-5.3-flashx) thinks on every
// request, and an explicit {type:"disabled"} is an error rather than a no-op.
func ForcedThinking(model string) bool { return glm53Family(strings.ToLower(model)) }

// deepseekV4Family matches DeepSeek's hosted V4 generation: deepseek-flash
// (V4.1 Flash), the deepseek-v4-* ids (V4 Pro, plus the retired V4 Flash names
// that DeepSeek now routes to V4.1 Flash) and OpenRouter's
// "deepseek/deepseek-v4.1-flash". A bare "deepseek" match would also catch local
// R1 distills and the retired deepseek-chat / deepseek-reasoner aliases, none of
// which take this wire format.
func deepseekV4Family(m string) bool {
	return strings.Contains(m, "deepseek-flash") || strings.Contains(m, "deepseek-v4")
}

// glm53Family matches the GLM-5.3 line on any transport ("glm-5.3",
// "glm-5.3-flash", "glm-5.3-flashx", OpenRouter's "z-ai/glm-5.3"). GLM-5.2 and
// older keep the legacy class: they can disable reasoning and are not verified
// to honour output_config.effort on the Anthropic endpoint.
func glm53Family(m string) bool { return strings.Contains(m, "glm-5.3") }

// CoarseEffortForBudget maps a thinking-token budget (the agent's ThinkingLevel,
// see thinkingBudgetForLevel) onto the low/high/max enum. medium folds into high
// and xhigh/ultra into max — the nearest real level, the same folding Z.ai
// documents for its own wider enum. A zero budget means "off": "" (omit, the
// caller disables reasoning) unless forced is set, where reasoning cannot stop
// and the lowest level is the honest stand-in.
func CoarseEffortForBudget(budget int, forced bool) string {
	switch {
	case budget <= 0:
		if forced {
			return "low"
		}
		return ""
	case budget <= 2048:
		return "low"
	case budget <= 16384:
		return "high"
	default:
		return "max"
	}
}

// coarseEffortTiers is the picker ramp for the coarse-effort class: the three
// real levels, plus "off" where reasoning can actually be disabled.
func coarseEffortTiers(model string) []string {
	if ForcedThinking(model) {
		return []string{"low", "high", "max"}
	}
	return []string{"off", "low", "high", "max"}
}

// coarseEffortStorable is what may be STORED on a coarse-effort model: every tier
// CoarseEffortForBudget folds onto a real level. medium stays legal because it
// was offered while these models sat in the legacy class, so agents saved back
// then must remain saveable; "off" is legal on the forced family for the same
// reason the always-on class keeps it (it maps to the lowest effort). "ultra" is
// left out — it is a CLI-transport depth, never offered on these endpoints.
func coarseEffortStorable() []string {
	return []string{"off", "low", "medium", "high", "xhigh", "max"}
}
