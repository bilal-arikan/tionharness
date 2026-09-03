package providers

import "strings"

// claudeAliasModels maps the claude-cli model aliases (the values a system agent
// stores: "haiku"/"sonnet"/"opus"/"fable") onto the concrete Messages API ids
// the first-party anthropic kind serves. The CLI resolves these aliases itself;
// the API does not, so a call re-routed from the CLI to the API must translate.
var claudeAliasModels = map[string]string{
	"haiku":  "claude-haiku-4-5-20251001",
	"sonnet": "claude-sonnet-5",
	"opus":   "claude-opus-5",
	"fable":  "claude-fable-5",
}

// NativeClaudeModel returns the Messages API model id for a claude-cli alias.
// A concrete "claude-*" id passes through unchanged; an empty or unknown value
// falls back to the cheapest tier (haiku), which is what every auxiliary system
// agent suggests anyway. Only meaningful for the anthropic kind.
func NativeClaudeModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if id, ok := claudeAliasModels[m]; ok {
		return id
	}
	if strings.HasPrefix(m, "claude-") {
		return strings.TrimSpace(model)
	}
	return claudeAliasModels["haiku"]
}
