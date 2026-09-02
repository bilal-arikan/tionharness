package db

import (
	"context"
)

func (d *DB) persistHookLocked(h Hook) error {
	return dbPersistLocked(d, d.hooks, dirHooks, h.ID, h)
}

// CreateHook inserts a new hook config and returns the stored row.
func (d *DB) CreateHook(ctx context.Context, h Hook) (Hook, error) {
	h.ID = d.nextID(idHook)
	h.CreatedAt = now()
	if h.Type == "" {
		h.Type = "command"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return h, d.persistHookLocked(h)
}

// GetHook loads a hook by id.
func (d *DB) GetHook(ctx context.Context, id string) (Hook, error) {
	return dbGet(d, d.hooks, id)
}

// ListHooks returns all hooks, newest first.
func (d *DB) ListHooks(ctx context.Context) ([]Hook, error) {
	return dbList(d, d.hooks, func(a, b Hook) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// ListEnabledHooksByEvent returns only enabled hooks for the given event,
// oldest first so they fire in a stable, creation-ordered chain.
func (d *DB) ListEnabledHooksByEvent(ctx context.Context, event string) ([]Hook, error) {
	return dbFilter(d, d.hooks,
		func(h Hook) bool { return h.Enabled && !h.Archived && h.Event == event },
		func(a, b Hook) bool { return a.CreatedAt < b.CreatedAt }), nil
}

// SetHookArchived archives or restores a hook (archived hooks never run and are
// hidden from the default list; Enabled is left as-is for a faithful restore).
func (d *DB) SetHookArchived(ctx context.Context, id string, archived bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	h, ok := d.hooks[id]
	if !ok {
		return ErrNotFound
	}
	if h.Archived == archived {
		return nil
	}
	h.Archived = archived
	return d.persistHookLocked(h)
}

// RecordHookFire bumps a hook's usage telemetry after its command ran.
// Best-effort bookkeeping: a missing hook (deleted mid-run) is not an error.
func (d *DB) RecordHookFire(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	h, ok := d.hooks[id]
	if !ok {
		return nil
	}
	h.FireCount++
	h.LastFiredAt = now()
	return d.persistHookLocked(h)
}

// UpdateHook edits the mutable fields of a hook.
func (d *DB) UpdateHook(ctx context.Context, h Hook) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.hooks[h.ID]
	if !ok {
		return ErrNotFound
	}
	cur.Event = h.Event
	cur.Matcher = h.Matcher
	cur.Type = h.Type
	cur.Command = h.Command
	cur.TimeoutSec = h.TimeoutSec
	cur.Enabled = h.Enabled
	return d.persistHookLocked(cur)
}

// SetHookEnabled toggles a hook on or off.
func (d *DB) SetHookEnabled(ctx context.Context, id string, enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	h, ok := d.hooks[id]
	if !ok {
		return ErrNotFound
	}
	h.Enabled = enabled
	return d.persistHookLocked(h)
}

// DeleteHook removes a hook config.
func (d *DB) DeleteHook(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return dbDeleteLocked(d, d.hooks, dirHooks, id)
}
