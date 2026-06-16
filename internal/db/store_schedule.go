package db

import (
	"context"
	"sort"
)

func (d *DB) persistScheduleLocked(sc Schedule) error {
	d.schedules[sc.ID] = sc
	return atomicWriteJSON(d.dir(dirSchedules, sc.ID+".json"), sc)
}

// CreateSchedule inserts a new schedule.
func (d *DB) CreateSchedule(ctx context.Context, sc Schedule) (Schedule, error) {
	sc.ID = newID()
	sc.CreatedAt = now()
	d.mu.Lock()
	defer d.mu.Unlock()
	return sc, d.persistScheduleLocked(sc)
}

// GetSchedule loads a schedule by id.
func (d *DB) GetSchedule(ctx context.Context, id string) (Schedule, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	sc, ok := d.schedules[id]
	if !ok {
		return Schedule{}, ErrNotFound
	}
	return sc, nil
}

// ListSchedules returns all schedules, newest first.
func (d *DB) ListSchedules(ctx context.Context) ([]Schedule, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Schedule, 0, len(d.schedules))
	for _, sc := range d.schedules {
		out = append(out, sc)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// ListEnabledSchedules returns only enabled schedules (for the scheduler boot).
func (d *DB) ListEnabledSchedules(ctx context.Context) ([]Schedule, error) {
	all, err := d.ListSchedules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Schedule, 0, len(all))
	for _, sc := range all {
		if sc.Enabled {
			out = append(out, sc)
		}
	}
	return out, nil
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
	if _, ok := d.schedules[id]; !ok {
		return ErrNotFound
	}
	delete(d.schedules, id)
	return removeFile(d.dir(dirSchedules, id+".json"))
}
