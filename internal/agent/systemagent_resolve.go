package agent

import (
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// ResolveSystemAgent returns the agent that serves system role key: the
// workspace's role assignment when one is set and usable (always a system agent
// of this role, systemagent_assign.go), otherwise the enabled workspace agent for
// key, falling back to the canonical built-in definition when the workspace
// agent is unavailable.
func (r *Runtime) ResolveSystemAgent(key string) (db.Agent, bool, error) {
	if assigned, ok := r.assignedRoleAgent(key); ok {
		return assigned, false, nil
	}
	return r.resolveRoleAgent(key)
}

// resolveRoleAgent is the assignment-free resolution: enabled customisation or
// locked built-in row, else the compiled definition.
func (r *Runtime) resolveRoleAgent(key string) (db.Agent, bool, error) {
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
		Provider:     def.Provider,
		AllowedTools: def.AllowedTools,
		System:       true,
		SystemKey:    def.SystemKey,
	}, true, nil
}
