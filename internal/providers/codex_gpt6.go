package providers

import "strings"

// gpt6Tier identifies the verified GPT-6 models without changing the behavior
// of older GPT families or guessing capabilities for future model names.
func gpt6Tier(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	m = strings.TrimPrefix(m, "openai/")
	m = strings.Replace(m, "gpt6-", "gpt-6-", 1)
	m = strings.Replace(m, "gpt6.1-", "gpt-6.1-", 1)
	switch m {
	case "gpt-6-astra":
		return "astra"
	case "gpt-6-sol", "gpt-6.1-sol":
		return "sol"
	case "gpt-6-luna":
		return "luna"
	default:
		return ""
	}
}

// codexGPT6ThinkingTiers reflects the Codex 0.159.0 model catalog verified on
// 2026-09-30, including GPT-6.1 Sol. API levels are not the CLI capability list:
// none is not offered here, and ultra is offered only for Astra and Sol.
func codexGPT6ThinkingTiers(tier string) []string {
	tiers := []string{"low", "medium", "high", "xhigh", "max"}
	if tier != "luna" {
		tiers = append(tiers, "ultra")
	}
	return tiers
}

// codexGPT6ContextWindow is the verified CLI catalog window, not the larger
// published API ceiling. Recheck this value when upgrading Codex; TionHarness
// does not currently discover per-instance model context overrides.
const codexGPT6ContextWindow = 272_000
