package agent

import (
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// ResolveSystemAgent returns the agent that serves system role key: the
// workspace's role assignment when one is set (systemagent_assign.go), otherwise
// the enabled workspace agent for key, falling back to the canonical built-in
// definition when the workspace agent is unavailable.
//
// A worker role assigned to a regular agent is NOT reflected here — callers use
// this for the profile's prompt and allowlist contract; the spawn paths consult
// assignedWorker themselves.
func (r *Runtime) ResolveSystemAgent(key string) (db.Agent, bool, error) {
	if assigned, ok := r.assignedRoleAgent(key); ok {
		if assigned.System {
			return assigned, false, nil
		}
		if !isWorkerRole(key) {
			role, fallback, err := r.resolveRoleAgent(key)
			if err != nil {
				return db.Agent{}, false, err
			}
			return withRoleExecutor(role, assigned), fallback, nil
		}
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
