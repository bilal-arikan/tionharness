package providers

import "strings"

// MinAdaptiveThinkingBudget is the lowest thinking budget to send for a model
// that mandates always-on adaptive reasoning. It keeps "off"/"low" requests
// valid for those models without spending a large reasoning budget.
const MinAdaptiveThinkingBudget = 1024

// RequiresAdaptiveThinking reports whether a model rejects thinking:disabled and
// instead demands an always-on (adaptive) reasoning budget — the Claude Fable 5
// / Mythos 5 class. For these, omitting the thinking parameter makes the API
// return 400, so callers must send a (possibly minimal) budget instead.
//
// Opus / Sonnet / Haiku and every other model return false → unchanged behaviour
// (thinking stays opt-in via the agent's ThinkingLevel).
func RequiresAdaptiveThinking(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "fable") || strings.Contains(m, "mythos")
}
