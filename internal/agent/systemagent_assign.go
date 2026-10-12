package agent

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// System role assignments (2026-10-07): a workspace may pick WHICH of a role's
// own system agents runs it — the locked built-in or a customisation derived from
// it — stored in ws-settings.json (systemAgentAssignments: SystemKey → agent id).
// An empty map is the old behaviour: the enabled customisation, else the built-in.
//
// Only system agents of the same role are accepted (user decision 2026-10-07):
// background jobs must never run on an ordinary agent the user did not set up
// for that job. To run a role on another model, customise its system agent in
// the agent library and assign the customisation.

// ErrRoleAssignmentKey rejects an assignment for an unknown or non-assignable role.
var ErrRoleAssignmentKey = errors.New("system role cannot be assigned")

// ErrRoleAssignmentAgent rejects an assignment whose agent cannot run the role.
var ErrRoleAssignmentAgent = errors.New("agent cannot run this system role")

// nonAssignableRoles are roles whose agent is chosen elsewhere. insight-applier is
// launched by a shipped automation that names its agent itself (automation
// defaults pin it by SystemKey), so a role assignment would have no effect.
var nonAssignableRoles = map[string]bool{"insight-applier": true}

// AssignableSystemRoles lists the SystemKeys a workspace may assign an agent to.
func AssignableSystemRoles() []string {
	out := make([]string, 0, len(systemAgentDefaults))
	for _, def := range systemAgentDefaults {
		if !nonAssignableRoles[def.SystemKey] {
			out = append(out, def.SystemKey)
		}
	}
	return out
}

// SetSystemRoleAssignments mirrors the workspace's role assignments (copied, so
// the caller may keep mutating its map).
func (r *Runtime) SetSystemRoleAssignments(m map[string]string) {
	cp := maps.Clone(m)
	r.roleAssignments.Store(&cp)
}

// SystemRoleAssignment returns the agent id assigned to key ("" = built-in).
func (r *Runtime) SystemRoleAssignment(key string) string {
	if p := r.roleAssignments.Load(); p != nil {
		return (*p)[key]
	}
	return ""
}

// roleAgentErr reports why agent a cannot run role key (nil when it can).
func roleAgentErr(key string, a db.Agent) error {
	if a.Deleted || a.Disabled {
		return fmt.Errorf("%w: %s is disabled or deleted", ErrRoleAssignmentAgent, a.Name)
	}
	if err := a.RunnableErr(); err != nil {
		return fmt.Errorf("%w: %v", ErrRoleAssignmentAgent, err)
	}
	if !a.System || a.SystemKey != key {
		return fmt.Errorf("%w: %s is not a system agent of role %s", ErrRoleAssignmentAgent, a.Name, key)
	}
	return nil
}

// CheckSystemRoleAssignment validates one assignment before it is persisted. An
// empty agentID (clear the assignment) is always valid for an assignable key.
func (r *Runtime) CheckSystemRoleAssignment(ctx context.Context, key, agentID string) error {
	if !slices.Contains(AssignableSystemRoles(), key) {
		return fmt.Errorf("%w: %q", ErrRoleAssignmentKey, key)
	}
	if agentID == "" {
		return nil
	}
	a, err := r.db.GetAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("%w: agent %s not found", ErrRoleAssignmentAgent, agentID)
	}
	return roleAgentErr(key, a)
}

// assignedRoleAgent returns the usable agent assigned to key. A stale assignment
// (agent deleted, archived, disabled since) falls back to the built-in silently
// at debug level: roles run on every turn and a dangling pick must not break them.
func (r *Runtime) assignedRoleAgent(key string) (db.Agent, bool) {
	id := r.SystemRoleAssignment(key)
	if id == "" || r.db == nil {
		return db.Agent{}, false
	}
	a, err := r.db.GetAgent(context.Background(), id)
	if err == nil {
		err = roleAgentErr(key, a)
	}
	if err != nil {
		r.logger.Debug("system role assignment unusable; using built-in", "systemKey", key, "agent", id, "error", err)
		return db.Agent{}, false
	}
	return a, true
}
