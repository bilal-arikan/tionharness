package db

import (
	"context"
)

func (d *DB) persistScheduleLocked(sc Schedule) error {
	return dbPersistLocked(d, d.schedules, dirSchedules, sc.ID, sc)
}

// CreateSchedule inserts a new schedule.
func (d *DB) CreateSchedule(ctx context.Context, sc Schedule) (Schedule, error) {
	sc.ID = d.nextID(idSchedule)
	sc.CreatedAt = now()
	sc.UpdatedAt = sc.CreatedAt
	d.mu.Lock()
	defer d.mu.Unlock()
	return sc, d.persistScheduleLocked(sc)
}

// GetSchedule loads a schedule by id.
func (d *DB) GetSchedule(ctx context.Context, id string) (Schedule, error) {
	return dbGet(d, d.schedules, id)
}

// ListSchedules returns all schedules, newest first.
func (d *DB) ListSchedules(ctx context.Context) ([]Schedule, error) {
	return dbList(d, d.schedules, func(a, b Schedule) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// ListEnabledSchedules returns only enabled schedules (for the scheduler boot),
// newest first.
func (d *DB) ListEnabledSchedules(ctx context.Context) ([]Schedule, error) {
	return dbFilter(d, d.schedules,
		func(sc Schedule) bool { return sc.Enabled },
		func(a, b Schedule) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// UpdateSchedule edits the mutable fields of a schedule (agent/cron/task/prompt).
// Delivery bookkeeping and the enabled flag are left untouched.
func (d *DB) UpdateSchedule(ctx context.Context, sc Schedule) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.schedules[sc.ID]
	if !ok {
		return ErrNotFound
	}
	// Name is assigned unconditionally like every other mutable field: guarding it
	// on non-empty would make "clear the name" a silent no-op, and callers that
	// want partial updates (the agent tool) already read-modify-write the row.
	cur.Name = sc.Name
	cur.AgentID = sc.AgentID
	cur.CronExpr = sc.CronExpr
	cur.Prompt = sc.Prompt
	cur.FlowID = sc.FlowID
	cur.SessionMode = sc.SessionMode
	cur.ExpiresAt = sc.ExpiresAt
	cur.UpdatedAt = now()
	return d.persistScheduleLocked(cur)
}

// SetScheduleTags replaces a schedule's free-form tags without touching its
// other fields or its delivery bookkeeping.
func (d *DB) SetScheduleTags(ctx context.Context, id string, tags []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	sc, ok := d.schedules[id]
	if !ok {
		return ErrNotFound
	}
	sc.Tags = normalizeTags(tags)
	sc.UpdatedAt = now()
	return d.persistScheduleLocked(sc)
}

// SetScheduleEnabled toggles a schedule on or off.
func (d *DB) SetScheduleEnabled(ctx context.Context, id string, enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	sc, ok := d.schedules[id]
	if !ok {
		return ErrNotFound
	}
	sc.Enabled = enabled
	sc.UpdatedAt = now()
	return d.persistScheduleLocked(sc)
}

// SetScheduleDelivery records the outcome of the latest fire and the next run time.
func (d *DB) SetScheduleDelivery(ctx context.Context, id, status, deliveryErr string, nextRunAt int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	sc, ok := d.schedules[id]
	if !ok {
		return ErrNotFound
	}
	sc.LastDeliveryStatus = status
	sc.LastDeliveryError = deliveryErr
	sc.LastRunAt = now()
	sc.NextRunAt = nextRunAt
	sc.UpdatedAt = now()
	return d.persistScheduleLocked(sc)
}

// GetOrCreateKindSession returns the agent's dedicated session of the given
// kind (e.g. "schedule"), creating it once if absent. Used to log scheduled
// prompt deliveries in a stable, discoverable thread.
func (d *DB) GetOrCreateKindSession(ctx context.Context, agentID, kind, title string) (Session, error) {
	return d.getOrCreateKindSession(agentID, kind, title)
}

// DeleteSchedule removes a schedule.
func (d *DB) DeleteSchedule(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return dbDeleteLocked(d, d.schedules, dirSchedules, id)
}
