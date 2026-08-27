package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// systemKeyProfileID returns the built-in profile id behind a system agent key
// ("subagent-coder" -> "coder"), or "" when the agent is not a profile worker.
func systemKeyProfileID(agent db.Agent) string {
	if !agent.System {
		return ""
	}
	id := strings.TrimPrefix(agent.SystemKey, "subagent-")
	if id == agent.SystemKey {
		return ""
	}
	if _, ok := defaultSubagentProfiles[id]; !ok {
		return ""
	}
	return id
}

// applyProfileAllowlist re-asserts the CODE-side tool allowlist on a profile
// worker agent. The allowlist is a safety contract (see defaultSubagentProfiles),
// so the persisted row is treated as a cache of it, never as the authority: a
// hand-edited or stale record must not widen what the worker may touch.
//
// A non-profile agent is left untouched.
func applyProfileAllowlist(agent *db.Agent) error {
	id := systemKeyProfileID(*agent)
	if id == "" {
		return nil
	}
	allow, err := json.Marshal(defaultSubagentProfiles[id].AllowedTools)
	if err != nil {
		return fmt.Errorf("cannot render worker profile %q allowlist: %w", id, err)
	}
	agent.AllowedTools = string(allow)
	return nil
}

// applyProfileAllowlist is the Runtime-bound form used on the spawn path; it logs
// when a persisted row disagreed with the contract, since that means something
// wrote an allowlist we are about to override.
func (r *Runtime) applyProfileAllowlist(agent *db.Agent) error {
	before := agent.AllowedTools
	if err := applyProfileAllowlist(agent); err != nil {
		return err
	}
	if agent.AllowedTools != before {
		r.logger.Warn("profile worker allowlist re-asserted from code",
			"systemKey", agent.SystemKey, "stored", before, "effective", agent.AllowedTools)
	}
	return nil
}
