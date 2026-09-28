package db

import (
	"context"
	"fmt"
)

// MarkAgentCatalog allows independent customizations of the same system role in
// the library. Role uniqueness is enforced when assigning them to a workspace.
func (d *DB) MarkAgentCatalog() { d.isAgentCatalog = true }

// PrepareAgentCatalog preserves role profiles while legacy seed repairs still
// operate on local rows. Attach imports those repaired rows before runtime boot.
func (d *DB) PrepareAgentCatalog(workspaceID string) { d.catalogWorkspaceID = workspaceID }

// SetCatalogRoleCheck is wired once, before workspace runtimes start.
func (d *DB) SetCatalogRoleCheck(check func(string) error) { d.catalogRoleCheck = check }

func (d *DB) CheckCatalogRole(catalogID string) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, local := range d.agents {
		if local.CatalogID != catalogID || local.Deleted {
			continue
		}
		a := d.resolveAgentLocked(local)
		if a.System && !a.Locked && d.systemRoleTakenLocked(a.SystemKey, a.ID) {
			return ErrSystemRoleTaken
		}
	}
	return nil
}

// AttachAgentCatalog imports legacy rows without changing local IDs or history.
// Origin keys make interrupted imports safe to retry. Existing links always
// read from the catalog, never overwrite it with a stale workspace snapshot.
func (d *DB) AttachAgentCatalog(catalog *DB, workspaceID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.agentCatalog, d.catalogWorkspaceID = catalog, workspaceID
	for id, a := range d.agents {
		if a.Deleted {
			continue
		}
		if _, err := d.importCatalogAgentLocked(id, map[string]bool{}); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) importCatalogAgentLocked(id string, visiting map[string]bool) (string, error) {
	a, ok := d.agents[id]
	if !ok {
		return "", ErrAgentParentNotFound
	}
	if a.CatalogID != "" {
		if _, err := d.agentCatalog.GetAgent(context.Background(), a.CatalogID); err != nil {
			return "", fmt.Errorf("catalog agent %s missing: %w", a.CatalogID, err)
		}
		return a.CatalogID, nil
	}
	if visiting[id] {
		return "", ErrAgentParentCycle
	}
	visiting[id] = true
	defer delete(visiting, id)
	origin := d.catalogWorkspaceID + "/" + id
	d.agentCatalog.mu.RLock()
	for _, candidate := range d.agentCatalog.agents {
		if (a.Locked && candidate.Locked && a.SystemKey == candidate.SystemKey) || candidate.CatalogOrigin == origin {
			a.CatalogID = candidate.ID
			break
		}
	}
	d.agentCatalog.mu.RUnlock()
	if a.CatalogID == "" {
		copy := a
		copy.CatalogOrigin = origin
		copy.Archived, copy.ArchivedAt = false, 0
		if a.ParentID != "" {
			parent, err := d.importCatalogAgentLocked(a.ParentID, visiting)
			if err != nil {
				return "", err
			}
			copy.ParentID = parent
		}
		created, err := d.agentCatalog.createAgent(context.Background(), copy)
		if err != nil {
			return "", err
		}
		a.CatalogID = created.ID
	}
	if err := d.persistAgentLocked(a); err != nil {
		return "", err
	}
	return a.CatalogID, nil
}

func (d *DB) catalogTarget(id string) (*DB, string) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.agentCatalog == nil {
		return nil, ""
	}
	return d.agentCatalog, d.agents[id].CatalogID
}

// catalogAgentLocked projects shared profile values onto a local assignment.
// Lifecycle and historical IDs belong to the workspace; settings belong to the library.
func (d *DB) catalogAgentLocked(local Agent) (Agent, bool) {
	if d.agentCatalog == nil || local.CatalogID == "" {
		return Agent{}, false
	}
	shared, err := d.agentCatalog.GetAgent(context.Background(), local.CatalogID)
	if err != nil {
		return Agent{}, false
	}
	shared.ID, shared.CatalogID = local.ID, local.CatalogID
	shared.CreatedBy, shared.CreatedAt = local.CreatedBy, local.CreatedAt
	deletedInCatalog := shared.Deleted
	if !deletedInCatalog {
		shared.Deleted, shared.DeletedAt = local.Deleted, local.DeletedAt
	}
	shared.Archived, shared.ArchivedAt = local.Archived, local.ArchivedAt
	shared.CatalogDetached = local.CatalogDetached || deletedInCatalog
	parent := shared.ParentID
	shared.CatalogParentID = parent
	shared.ParentID = ""
	for _, candidate := range d.agents {
		if parent != "" && !candidate.Deleted && candidate.CatalogID == parent {
			shared.ParentID = candidate.ID
			break
		}
	}
	return shared, true
}

func (d *DB) updateCatalogAgent(ctx context.Context, id string, p AgentProfilePatch) (Agent, error) {
	catalog, target := d.catalogTarget(id)
	if p.ParentID != nil && *p.ParentID != "" {
		_, parent := d.catalogTarget(*p.ParentID)
		if parent == "" {
			return Agent{}, ErrAgentParentNotFound
		}
		p.ParentID = &parent
	}
	if _, err := catalog.UpdateAgent(ctx, target, p); err != nil {
		return Agent{}, err
	}
	return d.GetAgent(ctx, id)
}

// AssignCatalogAgent is idempotent and restores a previously detached assignment.
func (d *DB) AssignCatalogAgent(ctx context.Context, catalogID string) (Agent, error) {
	if d.agentCatalog == nil {
		return Agent{}, fmt.Errorf("agent catalog unavailable")
	}
	d.agentCatalog.catalogProfileMu.Lock()
	defer d.agentCatalog.catalogProfileMu.Unlock()
	shared, err := d.agentCatalog.GetAgent(ctx, catalogID)
	if err != nil {
		return Agent{}, err
	}
	if shared.Deleted {
		return Agent{}, ErrNotFound
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, local := range d.agents {
		if local.CatalogID != catalogID {
			continue
		}
		if !local.Deleted {
			return d.resolveAgentLocked(local), nil
		}
		if local.CatalogDetached {
			if shared.System && !shared.Locked && !shared.Disabled && d.systemRoleTakenLocked(shared.SystemKey, local.ID) {
				return Agent{}, ErrSystemRoleTaken
			}
			local.CatalogDetached, local.Deleted, local.DeletedAt = false, false, 0
			if err := d.persistAgentLocked(local); err != nil {
				return Agent{}, err
			}
			return d.resolveAgentLocked(local), nil
		}
	}
	if shared.System && !shared.Locked && !shared.Disabled && d.systemRoleTakenLocked(shared.SystemKey, "") {
		return Agent{}, ErrSystemRoleTaken
	}
	// The counter has its own lock and can be used while holding the store lock.
	shared.ID = d.nextID(idAgent)
	shared.CatalogID, shared.ParentID = catalogID, ""
	shared.CatalogOrigin = ""
	if err := d.persistAgentLocked(shared); err != nil {
		return Agent{}, err
	}
	return d.resolveAgentLocked(shared), nil
}

// DetachCatalogAgent keeps historical identity and never deletes tasks or chats.
func (d *DB) DetachCatalogAgent(ctx context.Context, catalogID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, a := range d.agents {
		if a.CatalogID != catalogID || a.Deleted {
			continue
		}
		if a.Locked {
			return ErrAgentLocked
		}
		a.CatalogDetached, a.Deleted, a.DeletedAt = true, true, now()
		return d.persistAgentLocked(a)
	}
	return nil
}
