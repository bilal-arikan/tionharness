package db

import (
	"context"
	"sort"
)

func (d *DB) persistHookLocked(h Hook) error {
	d.hooks[h.ID] = h
	return atomicWriteJSON(d.dir(dirHooks, h.ID+".json"), h)
}

// CreateHook inserts a new hook config and returns the stored row.
func (d *DB) CreateHook(ctx context.Context, h Hook) (Hook, error) {
	h.ID = newID()
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
	d.mu.RLock()
	defer d.mu.RUnlock()
	h, ok := d.hooks[id]
	if !ok {
		return Hook{}, ErrNotFound
	}
	return h, nil
}

// ListHooks returns all hooks, newest first.
func (d *DB) ListHooks(ctx context.Context) ([]Hook, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Hook, 0, len(d.hooks))
	for _, h := range d.hooks {
		out = append(out, h)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// ListEnabledHooksByEvent returns only enabled hooks for the given event,
// oldest first so they fire in a stable, creation-ordered chain.
func (d *DB) ListEnabledHooksByEvent(ctx context.Context, event string) ([]Hook, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Hook, 0, len(d.hooks))
	for _, h := range d.hooks {
		if h.Enabled && h.Event == event {
			out = append(out, h)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
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
	if _, ok := d.hooks[id]; !ok {
		return ErrNotFound
	}
	delete(d.hooks, id)
	return removeFile(d.dir(dirHooks, id+".json"))
}
