package agent

import (
	"log/slog"
	"strings"
	"sync"
)

var systemAgentModelSkipLogs sync.Map

// adoptSystemAgentModel copies a system agent's preferred model only when the
// calling provider can serve it. Claude aliases and IDs are specific to the
// Anthropic API and Claude CLI providers.
func adoptSystemAgentModel(logger *slog.Logger, systemKey, provider, currentModel, suggestedModel string) string {
	if !isClaudeModel(suggestedModel) || provider == "" || provider == "anthropic" || provider == "claude-cli" {
		return suggestedModel
	}

	logKey := systemKey + "\x00" + provider + "\x00" + suggestedModel
	if _, loaded := systemAgentModelSkipLogs.LoadOrStore(logKey, struct{}{}); !loaded && logger != nil {
		logger.Warn("system agent model incompatible with calling provider; keeping caller model",
			"systemKey", systemKey, "provider", provider, "skippedModel", suggestedModel)
	}
	return currentModel
}

func isClaudeModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "haiku" || model == "sonnet" || model == "opus" || strings.HasPrefix(model, "claude-")
}
