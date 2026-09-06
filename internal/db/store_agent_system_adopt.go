package db

import "context"

// Adopting a per-workspace customisation into the app-global layer.
//
// Before built-ins were editable, "customise" derived a child bound to the same
// SystemKey and that child served the role for this workspace. Those rows are
// exactly the copies this model removes, so on boot each one is folded INTO the
// app-global layer: its overrides become the built-in's customisation, and the
// child stops competing for the role.
//
// The child row itself is kept, not deleted: sessions reference it as their
// agent, and history must keep resolving the name and avatar those turns were
// written with. It is disabled and unbound from the role instead, which is the
// same "put away, never destroyed" treatment a deleted agent gets.

// adoptWorkspaceCustomisation folds this workspace's customisation child for
// `def` into the app-global layer, then retires the child.
//
// Runs before the built-in is re-imposed, so the very same boot seeds the
// adopted values. It is idempotent: once the children are retired there is
// nothing left to adopt, and a role that already has a stored customisation is
// never overwritten by a second workspace's copy.
func (d *DB) adoptWorkspaceCustomisation(ctx context.Context, def SystemAgentDefinition) error {
	g := d.GlobalSystemAgentOverrideLayer()
	if g == nil {
		return nil
	}
	child, ok := d.customisationChild(def.SystemKey)
	if !ok {
		return nil
	}
	// First adoption wins. A second workspace's copy of the same role cannot be
	// merged automatically — silently overwriting the customisation already in
	// force everywhere would be worse than leaving this copy to be retired.
	if _, exists := g.Get(def.SystemKey); !exists {
		canonical := def.canonicalAgent()
		effective := d.resolveAgent(child)
		// Trust the effective values over the stored override list: a row from the
		// migration era can carry keys whose value never actually diverged.
		keys := diffInheritable(effective, canonical)
		name := ""
		if effective.Name != canonical.Name {
			name = effective.Name
		}
		if len(keys) > 0 || name != "" {
			if err := g.Put(def.SystemKey, keys, effective, name); err != nil {
				return err
			}
		}
	}
	return d.retireCustomisationChild(child.ID)
}

// customisationChild returns this workspace's non-locked system row bound to
// systemKey — the pre-existing "Özelleştir" copy — if one is still live.
func (d *DB) customisationChild(systemKey string) (Agent, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, a := range d.agents {
		if a.Deleted || a.Locked || !a.System || a.SystemKey != systemKey || a.Disabled {
			continue
		}
		return a, true
	}
	return Agent{}, false
}

// resolveAgent resolves an agent's effective values (inheritance applied).
func (d *DB) resolveAgent(a Agent) Agent {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.resolveAgentLocked(a)
}

// retireCustomisationChild takes a copy out of service without destroying it:
// it stops serving the system role and stops appearing in rosters, while the
// sessions it ran keep resolving their author.
func (d *DB) retireCustomisationChild(id string) error {
	_, err := d.mutateAgentLocked(id, func(a *Agent) {
		a.Disabled = true
		// Unbind from the role so FindAgentBySystemKey can never fall back to it
		// (its "any child, even disabled" last resort would otherwise resurrect the
		// copy and defeat the whole adoption).
		a.SystemKey = ""
		a.System = false
	})
	return err
}
