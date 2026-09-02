package db

import "context"

// Pin setters (Rota F3). A pinned automation / schedule / hook is exempt from
// every automatic curator pass; the flag is the user's "keep this" mark and is
// independent of Enabled / Archived.

// SetAutomationPinned pins or unpins an automation.
func (d *DB) SetAutomationPinned(ctx context.Context, id string, pinned bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.automations[id]
	if !ok {
		return ErrNotFound
	}
	if a.Pinned == pinned {
		return nil
	}
	a.Pinned = pinned
	a.UpdatedAt = now()
	return d.persistAutomationLocked(a)
}

// SetSchedulePinned pins or unpins a schedule.
func (d *DB) SetSchedulePinned(ctx context.Context, id string, pinned bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	sc, ok := d.schedules[id]
	if !ok {
		return ErrNotFound
	}
	if sc.Pinned == pinned {
		return nil
	}
	sc.Pinned = pinned
	sc.UpdatedAt = now()
	return d.persistScheduleLocked(sc)
}

// SetHookPinned pins or unpins a hook.
func (d *DB) SetHookPinned(ctx context.Context, id string, pinned bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	h, ok := d.hooks[id]
	if !ok {
		return ErrNotFound
	}
	if h.Pinned == pinned {
		return nil
	}
	h.Pinned = pinned
	return d.persistHookLocked(h)
}
