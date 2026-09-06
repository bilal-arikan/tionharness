package db

import (
	"context"
	"fmt"
)

// Editing a BUILT-IN system agent.
//
// A built-in row is re-imposed from the compiled registry on every boot, so an
// edit written to the row itself would survive only until the next start. The
// supported edit therefore goes to the app-global override layer
// (GlobalSystemAgentOverrides): the changed units are pinned there, and every
// workspace's copy of the built-in — this one included, immediately — is brought
// back in line with the layer. One editable target, no derived copy, and the
// change applies installation-wide.

// lockedSystemAgent returns the live locked built-in row for id.
func (d *DB) lockedSystemAgent(id string) (Agent, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	a, ok := d.agents[id]
	if !ok || a.Deleted || !a.Locked || !a.System || a.SystemKey == "" {
		return Agent{}, false
	}
	return a, true
}

// updateBuiltinSystemAgent applies a patch to a built-in by storing it in the
// app-global layer and re-imposing the result on this workspace's row.
func (d *DB) updateBuiltinSystemAgent(ctx context.Context, locked Agent, p AgentProfilePatch) (Agent, error) {
	g := d.GlobalSystemAgentOverrideLayer()
	if g == nil {
		// Nothing durable to write to: refuse rather than accept an edit the next
		// boot would silently revert.
		return Agent{}, ErrAgentLocked
	}
	if err := ValidateOverrideKeys(p.ResetFields); err != nil {
		return Agent{}, err
	}
	// A built-in serves a system role and is never disabled or re-parented; those
	// are properties of the compiled registry, not of a customisation.
	if p.Disabled != nil && *p.Disabled {
		return Agent{}, fmt.Errorf("built-in system agent cannot be disabled")
	}
	if p.ParentID != nil && *p.ParentID != "" {
		return Agent{}, fmt.Errorf("built-in system agent cannot be re-parented")
	}

	// Start from what the role currently resolves to, so an edit builds on the
	// customisation already stored rather than on the compiled default.
	edited := locked
	name := locked.Name
	if ov, ok := g.Get(locked.SystemKey); ok && ov.Name != "" {
		name = ov.Name
	}
	pinned := overrideSet(locked.Overrides)
	if p.Name != nil {
		name = *p.Name
		edited.Name = name
	}
	applyInheritablePatch(&edited, p, func(key string) { pinned[key] = true })
	for _, key := range p.ResetFields {
		delete(pinned, key)
	}
	return d.storeBuiltinCustomisation(ctx, locked.SystemKey, edited, keysOf(pinned), name)
}

// editBuiltinSystemAgent is the non-patch entry point: it applies `edit` to the
// role's current row and pins `keys`. Used by the tool-access writers, which
// shape several struct fields at once rather than passing a profile patch.
func (d *DB) editBuiltinSystemAgent(ctx context.Context, locked Agent, keys []string, edit func(*Agent)) error {
	g := d.GlobalSystemAgentOverrideLayer()
	if g == nil {
		return ErrAgentLocked
	}
	edited := locked
	edit(&edited)
	pinned := overrideSet(locked.Overrides)
	for _, k := range keys {
		pinned[k] = true
	}
	name := locked.Name
	if ov, ok := g.Get(locked.SystemKey); ok && ov.Name != "" {
		name = ov.Name
	}
	_, err := d.storeBuiltinCustomisation(ctx, locked.SystemKey, edited, keysOf(pinned), name)
	return err
}

// storeBuiltinCustomisation pins the edited units in the app-global layer and
// re-imposes the result on this workspace's built-in row.
func (d *DB) storeBuiltinCustomisation(ctx context.Context, systemKey string, edited Agent, keys []string, name string) (Agent, error) {
	g := d.GlobalSystemAgentOverrideLayer()
	if g == nil {
		return Agent{}, ErrAgentLocked
	}
	// A field edited back to its compiled value is not a customisation: drop it so
	// the role keeps tracking the registry there.
	base := d.compiledCanonicalFor(systemKey)
	keys = pinnedDiffering(keys, edited, base)
	storedName := ""
	if name != base.Name {
		storedName = name
	}
	if err := g.Put(systemKey, keys, edited, storedName); err != nil {
		return Agent{}, fmt.Errorf("store system agent customisation: %w", err)
	}
	// Bring THIS workspace's row in line right away; other workspaces pick the
	// change up through their own re-impose (open now, or on the next boot).
	return d.reimposeBuiltinLocked(ctx, systemKey)
}

// keysOf returns the set's members as a slice.
func keysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k, on := range set {
		if on {
			out = append(out, k)
		}
	}
	return out
}

// pinnedDiffering keeps only the keys whose value in `edited` actually differs
// from `base`, so resetting a field by re-typing its default un-pins it.
func pinnedDiffering(keys []string, edited, base Agent) []string {
	set := overrideSet(keys)
	out := make([]string, 0, len(keys))
	for _, f := range inheritableFields {
		if set[f.Key] && !f.equal(&edited, &base) {
			out = append(out, f.Key)
		}
	}
	return out
}

// ClearBuiltinSystemAgentOverrides drops a built-in's customisation, so the role
// follows the compiled definition again in every workspace.
func (d *DB) ClearBuiltinSystemAgentOverrides(ctx context.Context, agentID string) (Agent, error) {
	locked, ok := d.lockedSystemAgent(agentID)
	if !ok {
		return Agent{}, ErrNotFound
	}
	g := d.GlobalSystemAgentOverrideLayer()
	if g == nil {
		return Agent{}, ErrAgentLocked
	}
	if _, err := g.Clear(locked.SystemKey); err != nil {
		return Agent{}, fmt.Errorf("clear system agent customisation: %w", err)
	}
	return d.reimposeBuiltinLocked(ctx, locked.SystemKey)
}

// reimposeBuiltinLocked rewrites this workspace's built-in row for systemKey
// from the compiled definition plus the current app-global customisation.
func (d *DB) reimposeBuiltinLocked(ctx context.Context, systemKey string) (Agent, error) {
	def, ok := d.systemDefinition(systemKey)
	if !ok {
		return Agent{}, fmt.Errorf("no compiled definition for system role %q", systemKey)
	}
	canonical := d.canonicalFor(def)
	var out Agent
	d.mu.Lock()
	for _, a := range d.agents {
		if a.Deleted || !a.Locked || !a.System || a.SystemKey != systemKey {
			continue
		}
		next := a
		imposeCanonical(&next, canonical)
		next.UpdatedAt = now()
		if err := d.persistAgentLocked(next); err != nil {
			d.mu.Unlock()
			return Agent{}, err
		}
		out = d.resolveAgentLocked(next)
		break
	}
	d.mu.Unlock()
	if out.ID == "" {
		return Agent{}, ErrNotFound
	}
	return out, nil
}
