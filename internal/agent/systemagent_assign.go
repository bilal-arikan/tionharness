package agent

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// System role assignments (2026-10-07): a workspace may pick WHICH agent runs a
// system role — the same idea as the Insight screen's analysis-agent picker,
// generalised to every assignable role and stored in ws-settings.json
// (systemAgentAssignments: SystemKey → agent id). An empty map is the old
// behaviour: every role runs on its customisation or locked built-in.
//
// What an assignment means depends on the role class:
//
//   - utility roles (titler, compaction, summarizer, lesson/insight analysis,
//     optimizers, stall judge) keep their ROLE CONTRACT — prompt, tool list, id,
//     SystemKey — and only borrow the assigned agent's provider, instance and
//     model, pinned (pinsProvider), so the call runs exactly there. A regular
//     agent's own soul would break the role's output format ({{summary}}
//     placeholders, strict JSON verdicts), so it is never used.
//   - worker roles (subagent-*) are replaced WHOLESALE: spawn_worker / run_subagent
//     with the profile name runs the assigned agent with its own prompt, tools and
//     model. This grants nothing a coordinator could not already do by naming the
//     agent directly; it only changes what the profile name points at.
//
// Picking the role's own built-in or customisation (System && SystemKey == key)
// simply selects that row for every role class.

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

// isWorkerRole reports whether key is a built-in worker profile role.
func isWorkerRole(key string) bool {
	id, ok := strings.CutPrefix(key, "subagent-")
	if !ok {
		return false
	}
	_, known := defaultSubagentProfiles[id]
	return known
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
	if a.System && a.SystemKey != key {
		return fmt.Errorf("%w: %s is the built-in agent of another role (%s)", ErrRoleAssignmentAgent, a.Name, a.SystemKey)
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

// assignedWorker returns the regular agent assigned to a worker profile target
// ("explore", "coder", …), or false when the profile runs on its built-in.
func (r *Runtime) assignedWorker(profileID string) (db.Agent, bool) {
	a, ok := r.assignedRoleAgent("subagent-" + profileID)
	if !ok || a.System {
		return db.Agent{}, false
	}
	return a, true
}

// withRoleExecutor overlays the assigned agent's transport onto the role's own
// agent: provider/instance/model become the assigned agent's and are marked as
// overrides so systemAgentExecutor treats them as an explicit pin.
func withRoleExecutor(role, assigned db.Agent) db.Agent {
	out := role
	out.Provider = assigned.Provider
	out.ProviderInstanceID = assigned.ProviderInstanceID
	if out.Provider == "" {
		// An agent with no provider runs on the keyless claude-cli default.
		out.Provider = "claude-cli"
	}
	out.Model = assigned.Model
	out.Overrides = slices.Clone(role.Overrides)
	for _, f := range []string{"provider", "model"} {
		if !slices.Contains(out.Overrides, f) {
			out.Overrides = append(out.Overrides, f)
		}
	}
	return out
}
