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

// Context windows Codex itself serves, from the context_window field of the
// Codex 0.163.0 model catalog (codex-home/models_cache.json, fetched
// 2026-10-18). These are the CLI's windows, not the larger API ceilings: the
// same gpt-5.6-sol is 1.05M on the API but 272K through Codex. Recheck when
// upgrading Codex; TionHarness does not read per-instance overrides.
const (
	codexCLIWindowGPT61Sol = 500_000 // gpt-6.1-sol
	codexCLIWindowDefault  = 272_000 // every other Codex slug (GPT-6, GPT-5.6, codex-*)
)

// codexCLIContextWindow returns the Codex-served window for a GPT-family slug.
func codexCLIContextWindow(model string) int {
	m := strings.ToLower(strings.TrimSpace(model))
	m = strings.TrimPrefix(m, "openai/")
	m = strings.Replace(m, "gpt6.1-", "gpt-6.1-", 1)
	if m == "gpt-6.1-sol" {
		return codexCLIWindowGPT61Sol
	}
	return codexCLIWindowDefault
}
