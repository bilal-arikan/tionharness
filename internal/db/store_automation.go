package db

import (
	"context"
	"strings"
)

// normalizeTags trims, drops empties, and de-duplicates a tag slice while
// preserving first-seen order. Returns nil for an all-empty input so a session
// with no tags stores nothing (omitempty). Shared by every SetXxxTags path.
func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (d *DB) persistAutomationLocked(a Automation) error {
	return dbPersistLocked(d, d.automations, dirAutomations, a.ID, a)
}

// CreateAutomation inserts a new tag-triggered automation.
func (d *DB) CreateAutomation(ctx context.Context, a Automation) (Automation, error) {
	a.ID = d.nextID(idAutomation)
	a.CreatedAt = now()
	a.UpdatedAt = a.CreatedAt
	a.TriggerTag = strings.TrimSpace(a.TriggerTag)
	a.SpawnTags = normalizeTags(a.SpawnTags)
	d.mu.Lock()
	defer d.mu.Unlock()
	return a, d.persistAutomationLocked(a)
}

// GetAutomation loads an automation by id.
func (d *DB) GetAutomation(ctx context.Context, id string) (Automation, error) {
	return dbGet(d, d.automations, id)
}

// ListAutomations returns all automations, newest first.
func (d *DB) ListAutomations(ctx context.Context) ([]Automation, error) {
	return dbList(d, d.automations, func(a, b Automation) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// ListEnabledAutomations returns only enabled, non-archived automations (for
// the event engine).
func (d *DB) ListEnabledAutomations(ctx context.Context) ([]Automation, error) {
	return dbFilter(d, d.automations,
		func(a Automation) bool { return a.Enabled && !a.Archived },
		func(a, b Automation) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// SetAutomationArchived archives or restores an automation. Archiving leaves
// Enabled untouched so a restore brings the rule back exactly as it was; the
// engine's guard chain and ListEnabledAutomations both exclude archived rows.
func (d *DB) SetAutomationArchived(ctx context.Context, id string, archived bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.automations[id]
	if !ok {
		return ErrNotFound
	}
	if a.Archived == archived {
		return nil
	}
	a.Archived = archived
	a.UpdatedAt = now()
	return d.persistAutomationLocked(a)
}

// UpdateAutomation replaces the mutable configuration of an automation. Enabled
// and the runtime bookkeeping (iteration count, last fire) are left untouched —
// use SetAutomationEnabled / RecordAutomationFire / ResetAutomationCount for those.
//
// It takes every configuration field from the incoming value and preserves ONLY
// identity + bookkeeping from the stored row. This replaced a field-by-field copy
// that silently dropped any struct field it forgot to list: the counter fields and
// SessionMode were both lost on update while create persisted them fine. Whole-
// value replacement means a newly added field can never be silently dropped again;
// the caller (REST/tool) already loads the current row and patches onto it, so the
// incoming value carries the full merged configuration.
func (d *DB) UpdateAutomation(ctx context.Context, a Automation) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.automations[a.ID]
	if !ok {
		return ErrNotFound
	}
	// Preserve identity + runtime state; these have their own setters and must not
	// change through a config edit.
	a.Enabled = cur.Enabled
	a.IterationCount = cur.IterationCount
	a.LastFiredAt = cur.LastFiredAt
	a.LastSessionID = cur.LastSessionID
	a.LastError = cur.LastError
	a.CreatedAt = cur.CreatedAt
	a.CreatedBy = cur.CreatedBy
	a.TriggerTag = strings.TrimSpace(a.TriggerTag)
	a.SpawnTags = normalizeTags(a.SpawnTags)
	a.UpdatedAt = now()
	return d.persistAutomationLocked(a)
}

// SetAutomationEnabled toggles an automation on or off. Enabling resets the
// iteration counter so a re-enabled loop starts its budget fresh.
func (d *DB) SetAutomationEnabled(ctx context.Context, id string, enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.automations[id]
	if !ok {
		return ErrNotFound
	}
	if enabled && !a.Enabled {
		a.IterationCount = 0
		a.LastError = ""
	}
	a.Enabled = enabled
	a.UpdatedAt = now()
	return d.persistAutomationLocked(a)
}

// ResetAutomationCount clears the iteration counter (and last error) so a
// maxed-out automation can run again without re-toggling it.
func (d *DB) ResetAutomationCount(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.automations[id]
	if !ok {
		return ErrNotFound
	}
	a.IterationCount = 0
	a.LastError = ""
	a.UpdatedAt = now()
	return d.persistAutomationLocked(a)
}

// RecordAutomationFire increments the iteration counter and records the outcome
// of a fire (the spawned session id, and any error). Called by the automation
// engine after each attempt. Does not bump UpdatedAt (a fire is activity, not an
// edit — the config is unchanged).
func (d *DB) RecordAutomationFire(ctx context.Context, id, spawnedSessionID, fireErr string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.automations[id]
	if !ok {
		return ErrNotFound
	}
	a.IterationCount++
	a.LastFiredAt = now()
	a.LastSessionID = spawnedSessionID
	a.LastError = fireErr
	return d.persistAutomationLocked(a)
}

// DeleteAutomation removes an automation.
func (d *DB) DeleteAutomation(ctx context.Context, id string) error {
	d.mu.Lock()
	err := dbDeleteLocked(d, d.automations, dirAutomations, id)
	d.mu.Unlock()
	if err == nil {
		_ = d.ClearAutomationFires(id)
	}
	return err
}
