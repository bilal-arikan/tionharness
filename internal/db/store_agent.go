// Agent records: create, read, update (profile patches and override resets), soft delete, and the skill detachment that runs when a skill is removed.
package db

import (
	"cmp"
	"context"
	"slices"
)

func (d *DB) persistAgentLocked(a Agent) error {
	d.agents[a.ID] = a
	d.markMutatedLocked()
	return atomicWriteJSON(d.dir(dirAgents, a.ID+".json"), a)
}

// AgentPath returns the absolute path of an agent's on-disk JSON file (one
// file per agent under the workspace store's agents/ folder).
func (d *DB) AgentPath(agentID string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.agents[agentID]; !ok {
		return "", ErrNotFound
	}
	return d.dir(dirAgents, agentID+".json"), nil
}

// mutateAgentLocked loads an agent under the write lock, applies fn to it, bumps
// UpdatedAt, and persists it — centralizing the lock/lookup/mutate/persist dance
// shared by every agent mutator. Returns ErrNotFound when the agent is absent.
func (d *DB) mutateAgentLocked(id string, fn func(*Agent)) (Agent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.agents[id]
	if !ok {
		return Agent{}, ErrNotFound
	}
	fn(&a)
	a.UpdatedAt = now()
	if err := d.persistAgentLocked(a); err != nil {
		return Agent{}, err
	}
	// Resolve on the RETURNED value only (not before persistAgentLocked above):
	// callers should see the effective row (inheritance folded, populated
	// ProviderInstanceID), but a mutation that didn't touch a field must not
	// silently widen the on-disk write beyond what the patch actually changed
	// (_Docs/71 §3, "no bulk write").
	return d.resolveAgentLocked(a), nil
}

// mutateAgentLockedErr is mutateAgentLocked for mutators that validate under
// the lock: fn may refuse the write, in which case nothing is persisted.
func (d *DB) mutateAgentLockedErr(id string, fn func(*Agent) error) (Agent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.agents[id]
	if !ok {
		return Agent{}, ErrNotFound
	}
	if err := fn(&a); err != nil {
		return Agent{}, err
	}
	a.UpdatedAt = now()
	if err := d.persistAgentLocked(a); err != nil {
		return Agent{}, err
	}
	return d.resolveAgentLocked(a), nil
}

// CreateAgent inserts a new agent and returns the stored row.
func (d *DB) CreateAgent(ctx context.Context, a Agent) (Agent, error) {
	a.ID = d.nextID(idAgent)
	a.CreatedAt = now()
	a.UpdatedAt = a.CreatedAt
	if a.PermissionMode == "" {
		a.PermissionMode = "auto"
	}
	// ThinkingLevel has no valid empty value any more (see Agent.ThinkingLevel).
	// The API create path rejects a blank level outright; the remaining creation
	// paths (pack install, workspace templates, self-management create_agent,
	// system-agent seeding) may still omit it, and resolving it here with the
	// same rule the boot migration uses keeps them from writing a row that only
	// becomes valid after the next restart.
	if a.ThinkingLevel == "" {
		a.ThinkingLevel = LegacyThinkingLevelFor(a.Provider)
	}
	if a.AllowedTools == "" {
		a.AllowedTools = "[]"
	}
	if a.BlockedTools == "" {
		a.BlockedTools = "[]"
	}
	if a.ToolOverrides == "" {
		a.ToolOverrides = "{}"
	}
	if a.Skills == nil {
		a.Skills = []string{}
	}
	// MCPEnabled is intentionally NOT defaulted here. Booleans cannot tell
	// "caller didn't set" from "caller set false", so default-on at the DB
	// layer would silently override explicit opt-outs. Each creation path
	// (POST /api/agents, market/ingest pack install, workspace-template
	// seeding, self-management create_agent, e2e harness) is responsible for
	// flipping false→true before calling CreateAgent — see the per-caller
	// "default-on" comments next to each db.Agent literal. To turn tools off
	// for an existing agent, call UpdateAgentTools (POST /api/agents/{id}/tools).

	// Every creation path is expected to have already synced Provider/
	// ProviderInstanceID (agent.SyncProviderFields, _Docs/71 §2.5), but this
	// backfill is the last-resort invariant guard for a caller that forgot to —
	// a brand-new row is exactly where filling it in is safe to persist
	// (unlike an existing row, there is no "unrelated field" risk).
	a = a.backfillProviderInstance()

	d.mu.Lock()
	defer d.mu.Unlock()
	// Inheritance: the parent must exist; overrides must name real fields. A
	// brand-new row cannot form a cycle, so only existence is checked here.
	if err := d.checkParentLocked("", a.ParentID); err != nil {
		return Agent{}, err
	}
	if err := ValidateOverrideKeys(a.Overrides); err != nil {
		return Agent{}, err
	}
	if a.ParentID == "" {
		a.Overrides = nil
	} else {
		a.Overrides = normalizeOverrides(a.Overrides)
	}
	if err := d.persistAgentLocked(a); err != nil {
		return Agent{}, err
	}
	return d.resolveAgentLocked(a), nil
}

// DeleteAgent marks an agent deleted and drops the forward-looking records that
// can no longer fire without it:
//   - schedules bound to it (AgentID),
//   - automations targeting it (TargetAgentID cleared, not deleted),
//   - tasks it owns (OwnerAgentID) together with their runs.
//
// The agent row itself is KEPT (Deleted=true) and so are the sessions it owns.
// Conversations are history: destroying them to remove their author loses the
// user's record of what happened, and every consumer that resolves a message's
// agent id would fall back to a raw id (or, worse, to a different agent). The
// row survives so that history still renders the real name/avatar/colour with a
// "deleted" marker, while ListAgents hides it from rosters and pickers.
//
// Removing the schedules here only clears the persisted rows; callers that run a
// live cron registry (the API server) must reload the scheduler afterwards so
// the in-memory jobs drop too.
func (d *DB) DeleteAgent(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.agents[id]
	if !ok {
		return ErrNotFound
	}
	// A built-in (locked) row is owned by the compiled registry and re-seeded on
	// boot, so deleting it would only ever be undone. A workspace customisation
	// of a system role (System && !Locked) IS deletable: the role falls back to
	// the built-in, and the soft-deleted row keeps its sessions renderable.
	if a.Locked {
		return ErrSystemAgentDelete
	}
	a.Deleted = true
	a.DeletedAt = now()
	a.UpdatedAt = a.DeletedAt
	d.agents[id] = a
	d.markMutatedLocked()
	if err := atomicWriteJSON(d.dir(dirAgents, id+".json"), a); err != nil {
		return err
	}
	// Children keep their effective values: they are re-pointed at this agent's
	// own parent (or become roots), with the removed layer's contribution folded
	// into their overrides.
	if err := d.reparentChildrenLocked(a); err != nil {
		return err
	}
	// Cascade: schedules deliver prompts to this agent, so they can no longer fire.
	for scid, sc := range d.schedules {
		if sc.AgentID == id {
			delete(d.schedules, scid)
			d.markMutatedLocked()
			_ = removeFile(d.dir(dirSchedules, scid+".json"))
		}
	}
	// Cascade: automations targeting this agent lose their target so they don't
	// fire a dangling reference at runtime. The automation stays intact (name,
	// trigger, budget, history) and can be pointed at a new agent later.
	for aid, a := range d.automations {
		if a.TargetAgentID == id {
			a.TargetAgentID = ""
			a.UpdatedAt = now()
			if err := atomicWriteJSON(d.dir(dirAutomations, aid+".json"), a); err != nil {
				return err
			}
			d.automations[aid] = a
			d.markMutatedLocked()
		}
	}
	// Cascade: tasks owned by this agent — they cannot be delivered without an owner.
	for tid, t := range d.tasks {
		if t.OwnerAgentID == id {
			delete(d.tasks, tid)
			d.markMutatedLocked()
			_ = removeFile(d.dir(dirTasks, tid+".json"))
		}
	}
	return nil
}

// RemoveSkillFromAgents strips a skill slug from every agent's skill selection
// and persists the agents that changed. It is called after a skill is deleted so
// no agent keeps a dangling reference to a skill that no longer exists. Returns
// the number of agents that were updated.
func (d *DB) RemoveSkillFromAgents(ctx context.Context, slug string) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	updated := 0
	for aid, a := range d.agents {
		if len(a.Skills) == 0 {
			continue
		}
		kept := make([]string, 0, len(a.Skills))
		for _, s := range a.Skills {
			if s != slug {
				kept = append(kept, s)
			}
		}
		if len(kept) == len(a.Skills) {
			continue // slug not referenced by this agent
		}
		a.Skills = kept
		a.UpdatedAt = now()
		if err := d.persistAgentLocked(a); err != nil {
			return updated, err
		}
		d.agents[aid] = a
		d.markMutatedLocked()
		updated++
	}
	return updated, nil
}

// GetAgent loads an agent by id.
func (d *DB) GetAgent(ctx context.Context, id string) (Agent, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	a, ok := d.agents[id]
	if !ok {
		return Agent{}, ErrNotFound
	}
	return d.resolveAgentLocked(a), nil
}

// ListAgents returns the live agents, newest first. Deleted agents are excluded:
// this backs every server-side "pick an agent" path (defaults, budget rollups,
// market publish, graph), none of which may ever select a deleted one.
func (d *DB) ListAgents(ctx context.Context) ([]Agent, error) {
	return d.listAgents(false)
}

// ListAgentsWithDeleted also returns agents marked deleted (flagged, so the
// caller can tell them apart). The UI roster endpoint needs them: a past
// conversation must still render its author's real name and avatar with a
// "deleted" marker rather than falling back to a raw id — while the client
// filters them out of its own pickers.
func (d *DB) ListAgentsWithDeleted(ctx context.Context) ([]Agent, error) {
	return d.listAgents(true)
}

func (d *DB) listAgents(includeDeleted bool) ([]Agent, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Agent, 0, len(d.agents))
	for _, a := range d.agents {
		if a.Deleted && !includeDeleted {
			continue
		}
		out = append(out, d.resolveAgentLocked(a))
	}
	slices.SortStableFunc(out, func(a, b Agent) int {
		return cmp.Compare(b.CreatedAt, a.CreatedAt)
	})
	return out, nil
}

// AgentProfilePatch carries the editable identity fields for UpdateAgent. A nil
// pointer leaves that field untouched, so callers can do partial updates.
type AgentProfilePatch struct {
	Name     *string
	Soul     *string
	Identity *string
	Provider *string
	// ProviderInstanceID patches Agent.ProviderInstanceID directly. Most
	// callers should go through agent.SyncProviderFields instead of setting
	// this (or Provider) by hand — see its doc comment for why.
	ProviderInstanceID *string
	Model              *string
	ThinkingLevel      *string
	// NativeWebSearch toggles the provider's own web search (see
	// Agent.NativeWebSearch). Pointer so an explicit false (turn it off) is
	// distinguishable from "not in this patch"; the stored field is itself a
	// pointer whose nil means enabled, so a patch can only set it, never unset it.
	NativeWebSearch *bool
	// NativeShell opts a claude-cli agent into the CLI's own shell family next to
	// the bridged one (see Agent.NativeShell). Pointer for the same reason.
	NativeShell    *bool
	PermissionMode *string
	Avatar         *string
	Color          *string
	// Skills is the agent's ordered skill-slug selection. Non-nil replaces the
	// whole list (an empty slice clears it).
	Skills   *[]string
	Disabled *bool
	// CoordinatorMode / CoordinatorWorkflow are the agent's coordinator DEFAULTS
	// for the sessions it opens (see Agent.CoordinatorMode). Pointers so an
	// explicit false is distinguishable from "not in this patch"; changing them
	// never touches sessions that already exist.
	CoordinatorMode     *bool
	CoordinatorWorkflow *string
	// CoordinatorPrompt is the agent's coordinator-only prompt block (see
	// Agent.CoordinatorPrompt). Pointer so clearing it ("") is distinguishable
	// from "not in this patch".
	CoordinatorPrompt *string
	// ParentID re-parents the agent (see Agent.ParentID); "" detaches it into a
	// root that keeps its effective values. Validated for existence and cycles.
	ParentID *string
	// ResetFields lists override keys (InheritableFieldKeys) to drop so those
	// fields inherit again. Applied after the field writes above, so a patch
	// cannot both set and reset the same field meaningfully — reset wins.
	ResetFields []string
}

// UpdateAgent applies a partial profile patch to an existing agent and persists
// it. Only non-nil patch fields are written.
//
// Inheritance rules (see agent_inherit.go):
//   - a Locked (built-in) system agent is edited THROUGH the app-global override
//     layer: the patched units are pinned there and re-imposed on every
//     workspace's copy of the built-in, so the edit reaches the whole
//     installation and derives no per-workspace copy. Without a layer attached
//     the built-in stays read-only (ErrAgentLocked), because an edit written only
//     to this workspace's row would be silently reverted by the next boot;
//   - on a child, every field the patch touches becomes an OVERRIDE, and
//     ResetFields drops overrides (the field goes back to inheriting; its raw
//     value is refreshed from the parent so the on-disk row stays readable);
//   - ParentID "" → X turns a root into a child that keeps behaving exactly as
//     before (every field becomes an override, to be reset one by one);
//     X → "" materialises the effective values into a root; X → Y keeps the
//     override set and re-resolves the rest against Y.
func (d *DB) UpdateAgent(ctx context.Context, agentID string, p AgentProfilePatch) (Agent, error) {
	if err := ValidateOverrideKeys(p.ResetFields); err != nil {
		return Agent{}, err
	}
	// A built-in is owned by the compiled registry, so its edit is stored in the
	// app-global layer rather than in this workspace's row; the write below then
	// re-imposes it everywhere.
	if locked, ok := d.lockedSystemAgent(agentID); ok {
		return d.updateBuiltinSystemAgent(ctx, locked, p)
	}
	return d.mutateAgentLockedErr(agentID, func(a *Agent) error {
		if a.Locked {
			return ErrAgentLocked
		}
		// Parent change first, so the field writes below are classified against
		// the tree the agent ends up in.
		if p.ParentID != nil && *p.ParentID != a.ParentID {
			if err := d.checkParentLocked(a.ID, *p.ParentID); err != nil {
				return err
			}
			switch {
			case a.ParentID == "":
				// root → child: keep behaviour, own everything until reset.
				a.ParentID = *p.ParentID
				a.Overrides = InheritableFieldKeys()
			case *p.ParentID == "":
				// child → root: freeze the effective values.
				*a = materialize(d.resolveAgentLocked(*a))
			default:
				a.ParentID = *p.ParentID
			}
		}
		// A disabled customisation coming back must not compete with another
		// enabled customisation of the same role.
		if p.Disabled != nil && !*p.Disabled && a.Disabled && a.System && a.SystemKey != "" {
			if d.systemRoleTakenLocked(a.SystemKey, a.ID) {
				return ErrSystemRoleTaken
			}
		}
		mark := func(key string) {
			if a.ParentID != "" {
				a.Overrides = withOverride(a.Overrides, key)
			}
		}
		if p.Name != nil {
			a.Name = *p.Name
		}
		if p.Disabled != nil {
			a.Disabled = *p.Disabled
		}
		applyInheritablePatch(a, p, mark)
		if len(p.ResetFields) > 0 && a.ParentID != "" {
			d.resetOverridesLocked(a, p.ResetFields)
		}
		if a.ParentID == "" {
			a.Overrides = nil
		}
		return nil
	})
}

// resetOverridesLocked drops the named overrides from a and refreshes their
// raw values from the parent's effective row. Caller holds d.mu.
func (d *DB) resetOverridesLocked(a *Agent, keys []string) {
	parent, ok := d.agents[a.ParentID]
	var parentEffective Agent
	if ok {
		parentEffective = d.resolveAgentLocked(parent)
	}
	for _, key := range keys {
		a.Overrides = withoutOverride(a.Overrides, key)
		if !ok {
			continue
		}
		for _, f := range inheritableFields {
			if f.Key == key {
				f.copy(a, &parentEffective)
			}
		}
	}
}

// ClearAgentOverrides makes a child inherit every field again. A root agent is
// returned unchanged; a locked built-in refuses with ErrAgentLocked.
func (d *DB) ClearAgentOverrides(ctx context.Context, agentID string) (Agent, error) {
	return d.mutateAgentLockedErr(agentID, func(a *Agent) error {
		if a.Locked {
			return ErrAgentLocked
		}
		if a.ParentID == "" {
			return nil
		}
		d.resetOverridesLocked(a, InheritableFieldKeys())
		return nil
	})
}
