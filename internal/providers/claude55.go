package providers

import "strings"

// claude55Tier matches only verified Claude 5.5 IDs, including OpenRouter's
// dotted IDs. Future versions must not inherit these wire-format rules.
func claude55Tier(model string) string {
	m := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "anthropic/")
	switch m {
	case "claude-opus-5-5", "claude-opus-5.5":
		return "opus"
	case "claude-sonnet-5-5", "claude-sonnet-5.5":
		return "sonnet"
	default:
		return ""
	}
}

// fableModel keeps the existing retention hints and refusal fallback scoped to
// Fable/Mythos. Always-on thinking alone does not imply those requirements.
func fableModel(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "fable") || strings.Contains(m, "mythos")
}
