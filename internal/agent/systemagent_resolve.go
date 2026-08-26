package agent

import (
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// ResolveSystemAgent returns the enabled workspace agent for key, falling back
// to the canonical built-in definition when the workspace agent is unavailable.
func (r *Runtime) ResolveSystemAgent(key string) (db.Agent, bool, error) {
	if workspaceAgent, ok := r.db.FindAgentBySystemKey(key); ok && !workspaceAgent.Disabled {
		return *workspaceAgent, false, nil
	}

	def, ok := SystemAgentDefault(key)
	if !ok {
		return db.Agent{}, false, fmt.Errorf("unknown system agent key %q", key)
	}

	r.logger.Info("system agent unavailable; using built-in fallback", "systemKey", key)
	return db.Agent{
		Name:         def.Name,
		Soul:         def.SystemPrompt,
		Identity:     def.Description,
		Model:        def.SuggestedModel,
		AllowedTools: def.AllowedTools,
		System:       true,
		SystemKey:    def.SystemKey,
	}, true, nil
}
