package db

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// ErrSystemAgentDelete explains the supported alternative to deleting a
// built-in agent. Callers may use errors.Is to map it to an API conflict.
var ErrSystemAgentDelete = errors.New("built-in agent cannot be deleted; derive a copy to customise it")

// SystemAgentDefinition is the persistence-layer representation of a built-in
// agent default. Callers own the registry and pass definitions into the store,
// keeping db independent of the agent package.
type SystemAgentDefinition struct {
	SystemKey      string
	Name           string
	Description    string
	SystemPrompt   string
	SuggestedModel string
	AllowedTools   string

	// Provider is the provider instance id the system agent runs on. An empty
	// value seeds the keyless claude-cli default, which is what the read-time
	// backfill in models.go already resolves it to.
	Provider string

	// Avatar (single emoji/glyph) and Color (hex) are the canonical visual
	// identity of a built-in agent, so the same system agent looks the same in
	// every workspace.
	Avatar string
	Color  string
}

// canonicalAgent renders the definition as the exact row a locked built-in
// carries. Every boot re-imposes this, so the built-in can never drift from
// the compiled registry.
func (def SystemAgentDefinition) canonicalAgent() Agent {
	provider := def.Provider
	if provider == "" {
		provider = "claude-cli"
	}
	allowed := def.AllowedTools
	if allowed == "" {
		allowed = "[]"
	}
	return Agent{
		Name:               def.Name,
		Soul:               def.SystemPrompt,
		Identity:           def.Description,
		Provider:           provider,
		ProviderInstanceID: provider,
		Model:              def.SuggestedModel,
		ThinkingLevel:      LegacyThinkingLevelFor(provider),
		PermissionMode:     "auto",
		AllowedTools:       allowed,
		BlockedTools:       "[]",
		ToolOverrides:      "{}",
		Skills:             []string{},
		Avatar:             def.Avatar,
		Color:              def.Color,
		System:             true,
		SystemKey:          def.SystemKey,
		Locked:             true,
	}
}

// imposeCanonical writes the canonical values onto a locked row, keeping only
// its identity (ID, timestamps, CreatedBy).
func imposeCanonical(a *Agent, canonical Agent) {
	id, createdAt, createdBy := a.ID, a.CreatedAt, a.CreatedBy
	*a = canonical
	a.ID, a.CreatedAt, a.CreatedBy = id, createdAt, createdBy
}

// FindAgentBySystemKey returns the agent that currently SERVES the system role
// `key`: the enabled workspace customisation when one exists, else the locked
// built-in, else (a store from before built-ins were locked) whatever
// customisation is present even if disabled. The result is resolved.
func (d *DB) FindAgentBySystemKey(key string) (*Agent, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var locked, enabledChild, anyChild *Agent
	for _, a := range d.agents {
		if !a.System || a.SystemKey != key || a.Deleted {
			continue
		}
		row := a
		switch {
		case row.Locked:
			if locked == nil {
				locked = &row
			}
		case !row.Disabled:
			if enabledChild == nil || row.CreatedAt < enabledChild.CreatedAt {
				enabledChild = &row
			}
		default:
			if anyChild == nil {
				anyChild = &row
			}
		}
	}
	for _, pick := range []*Agent{enabledChild, locked, anyChild} {
		if pick != nil {
			found := d.resolveAgentLocked(*pick)
			return &found, true
		}
	}
	return nil, false
}

// FindBuiltinAgentBySystemKey returns the locked built-in row for key.
func (d *DB) FindBuiltinAgentBySystemKey(key string) (*Agent, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, a := range d.agents {
		if a.System && a.Locked && a.SystemKey == key && !a.Deleted {
			found := d.resolveAgentLocked(a)
			return &found, true
		}
	}
	return nil, false
}

// EnsureSystemAgents seeds the LOCKED built-in row for every definition and
// re-imposes the canonical values on rows that already exist, so the built-in
// always equals the compiled registry. It is idempotent.
//
// Migration from the previous model (one editable system row per key): a row
// that still equals its definition is converted in place into the built-in
// (same ID — its sessions keep resolving). A row the user customised keeps its
// ID and its edits and becomes a CHILD of a freshly created built-in, carrying
// exactly the fields that differ as overrides; the role keeps resolving to it
// (FindAgentBySystemKey prefers the enabled customisation).
func (d *DB) EnsureSystemAgents(ctx context.Context, defs ...SystemAgentDefinition) error {
	if legacy, ok := d.FindAgentBySystemKey("compactor"); ok {
		if _, err := d.mutateAgentLocked(legacy.ID, func(a *Agent) {
			a.SystemKey = "overview-summarizer"
		}); err != nil {
			return err
		}
	}
	for _, def := range defs {
		if err := d.adoptLegacyWorkerAgent(def); err != nil {
			return err
		}
		if err := d.ensureSystemAgent(ctx, def); err != nil {
			return err
		}
	}
	return nil
}

// adoptLegacyWorkerAgent migrates a materialised "worker:<profile>" agent: it
// becomes the role's system row when none exists, or is disabled when one does
// so there is never a second target.
func (d *DB) adoptLegacyWorkerAgent(def SystemAgentDefinition) error {
	if !strings.HasPrefix(def.SystemKey, "subagent-") {
		return nil
	}
	legacyName := "worker:" + strings.TrimPrefix(def.SystemKey, "subagent-")
	legacy, legacyFound := d.findLiveAgentByName(legacyName)
	if !legacyFound {
		return nil
	}
	if _, systemFound := d.FindAgentBySystemKey(def.SystemKey); systemFound {
		_, err := d.mutateAgentLocked(legacy.ID, func(a *Agent) { a.Disabled = true })
		return err
	}
	_, err := d.mutateAgentLocked(legacy.ID, func(a *Agent) {
		a.Name = def.Name
		a.Identity = def.Description
		a.AllowedTools = def.AllowedTools
		a.System = true
		a.SystemKey = def.SystemKey
	})
	return err
}

func (d *DB) ensureSystemAgent(ctx context.Context, def SystemAgentDefinition) error {
	canonical := def.canonicalAgent()

	d.mu.Lock()
	var locked *Agent
	var children []Agent
	for _, a := range d.agents {
		if !a.System || a.SystemKey != def.SystemKey || a.Deleted {
			continue
		}
		row := a
		if row.Locked {
			if locked == nil {
				locked = &row
			}
			continue
		}
		children = append(children, row)
	}

	if locked != nil {
		// Repair: collapse a MIGRATION ARTIFACT child back into the built-in. The
		// first migration kept every pre-existing editable row as a child whenever
		// it differed from the definition — but those rows differed only because
		// the definition had moved under them (stale prompt text, provider/model
		// cloned by an old spawn path), not because the user customised them in
		// this model. A child OLDER than its locked parent can only be such an
		// artifact (a deliberate customisation is derived after the built-in
		// exists), so it becomes the built-in in place — keeping its id, which is
		// what its worker sessions reference — and the migration-created locked
		// row, which nothing references yet, is removed.
		if collapsed, err := d.collapseLegacyChildLocked(*locked, canonical); err != nil {
			d.mu.Unlock()
			return err
		} else if collapsed {
			d.mu.Unlock()
			return nil
		}
		// Re-impose: the built-in is owned by code. Skip the write when nothing
		// changed so a boot does not rewrite every workspace.
		next := *locked
		imposeCanonical(&next, canonical)
		if len(diffInheritable(*locked, next)) > 0 || locked.Name != next.Name || locked.Disabled || locked.ParentID != "" || len(locked.Overrides) > 0 {
			next.UpdatedAt = now()
			if err := d.persistAgentLocked(next); err != nil {
				d.mu.Unlock()
				return err
			}
		}
		d.mu.Unlock()
		return nil
	}

	// No built-in yet. The OLDEST legacy row becomes the built-in IN PLACE: it
	// keeps its id (its sessions keep resolving) and takes the canonical values —
	// whatever edits it carried are dropped, because in this model a role is
	// customised by deriving a child, never by editing the built-in. Any further
	// non-locked row for the same key (a disabled "worker:<profile>" duplicate)
	// is re-parented under it so there is never a second root for the role.
	if len(children) == 0 {
		d.mu.Unlock()
		_, err := d.CreateAgent(ctx, canonical)
		return err
	}
	sort.SliceStable(children, func(i, j int) bool { return children[i].CreatedAt < children[j].CreatedAt })
	root := children[0]
	imposeCanonical(&root, canonical)
	root.UpdatedAt = now()
	if err := d.persistAgentLocked(root); err != nil {
		d.mu.Unlock()
		return err
	}
	d.mu.Unlock()
	for _, child := range children[1:] {
		if child.ParentID != "" {
			continue // already in a tree
		}
		overrides := diffInheritable(normalizeLegacySystemRow(child, canonical), canonical)
		if _, err := d.mutateAgentLocked(child.ID, func(a *Agent) {
			a.ParentID = root.ID
			a.Overrides = normalizeOverrides(overrides)
			set := overrideSet(a.Overrides)
			for _, f := range inheritableFields {
				if !set[f.Key] {
					f.copy(a, &canonical)
				}
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

// normalizeLegacySystemRow fills the fields an older seed left EMPTY (visual
// identity, reasoning level, provider) with the canonical value, so "never set"
// is not mistaken for a user edit.
func normalizeLegacySystemRow(row Agent, canonical Agent) Agent {
	if row.Avatar == "" {
		row.Avatar = canonical.Avatar
	}
	if row.Color == "" {
		row.Color = canonical.Color
	}
	if row.ThinkingLevel == "" {
		row.ThinkingLevel = canonical.ThinkingLevel
	}
	if row.Provider == "" {
		row.Provider = canonical.Provider
		row.ProviderInstanceID = canonical.ProviderInstanceID
	}
	if row.PermissionMode == "" {
		row.PermissionMode = canonical.PermissionMode
	}
	// An adopted "worker:<profile>" row never carried the prompt/description
	// (they were read from the registry); empty means "not customised".
	if row.Soul == "" {
		row.Soul = canonical.Soul
	}
	if row.Identity == "" {
		row.Identity = canonical.Identity
	}
	return row
}

func (d *DB) findLiveAgentByName(name string) (Agent, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, a := range d.agents {
		if !a.Deleted && !a.System && a.Name == name {
			return a, true
		}
	}
	return Agent{}, false
}

// collapseLegacyChildLocked implements the repair described in
// ensureSystemAgent: when exactly one non-locked child of `locked` predates it,
// that child is turned into the canonical built-in in place and the locked row
// is removed. Nothing happens when the locked row is referenced by a session
// (it is not a fresh migration artifact then) or when the tree is ambiguous.
// Caller holds d.mu for writing. Reports whether a collapse was performed.
func (d *DB) collapseLegacyChildLocked(locked Agent, canonical Agent) (bool, error) {
	var older []Agent
	for _, a := range d.agents {
		if a.Deleted || a.Locked || a.ParentID != locked.ID || !a.System || a.SystemKey != locked.SystemKey {
			continue
		}
		if a.CreatedAt < locked.CreatedAt {
			older = append(older, a)
		}
	}
	if len(older) != 1 || d.agentReferencedBySessionLocked(locked.ID) {
		return false, nil
	}
	child := older[0]
	imposeCanonical(&child, canonical)
	child.UpdatedAt = now()
	if err := d.persistAgentLocked(child); err != nil {
		return false, err
	}
	delete(d.agents, locked.ID)
	if err := removeFile(d.dir(dirAgents, locked.ID+".json")); err != nil {
		return false, err
	}
	return true, nil
}

// agentReferencedBySessionLocked reports whether any session names id as its
// agent or as a participant. Caller holds d.mu.
func (d *DB) agentReferencedBySessionLocked(id string) bool {
	for _, s := range d.sessions {
		if s.AgentID == id {
			return true
		}
		for _, p := range s.Participants {
			if p == id {
				return true
			}
		}
	}
	return false
}
