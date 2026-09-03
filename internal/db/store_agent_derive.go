package db

import (
	"context"
	"strings"
)

// DeriveAgentOptions configures DeriveAgent.
type DeriveAgentOptions struct {
	// Name of the child. Empty picks "<parent> (kopya)".
	Name string
	// BindRole makes the child the workspace's customisation of the parent's
	// system role: it carries System+SystemKey, so FindAgentBySystemKey resolves
	// the role to it while it is enabled. Requires a system parent, and refuses
	// (ErrSystemRoleTaken) when another enabled customisation already exists.
	BindRole bool
	// CreatedBy records the acting agent for a self-management derive ("" = user).
	CreatedBy string
}

// DeriveAgent creates a child that inherits EVERY field from parentID (no
// overrides yet). Its raw row is seeded with the parent's effective values so
// the on-disk file reads sensibly, but those values are a cache: the resolver
// keeps following the parent until a field is overridden.
func (d *DB) DeriveAgent(ctx context.Context, parentID string, opts DeriveAgentOptions) (Agent, error) {
	d.mu.RLock()
	raw, ok := d.agents[parentID]
	if !ok || raw.Deleted {
		d.mu.RUnlock()
		return Agent{}, ErrAgentParentNotFound
	}
	parent := d.resolveAgentLocked(raw)
	if opts.BindRole && d.systemRoleTakenLocked(parent.SystemKey, "") {
		d.mu.RUnlock()
		return Agent{}, ErrSystemRoleTaken
	}
	d.mu.RUnlock()

	if opts.BindRole && (!parent.System || parent.SystemKey == "") {
		return Agent{}, ErrAgentParentNotFound
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = parent.Name + " (kopya)"
	}
	child := Agent{
		Name:      name,
		ParentID:  parentID,
		CreatedBy: opts.CreatedBy,
	}
	copyInheritable(&child, parent)
	if opts.BindRole {
		child.System = true
		child.SystemKey = parent.SystemKey
	}
	return d.CreateAgent(ctx, child)
}
