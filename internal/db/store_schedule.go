package db

import (
	"context"
	"database/sql"
	"errors"
)

const scheduleColumns = `id, agent_id, task_id, cron_expr, prompt, next_run_at, last_run_at,
	last_delivery_status, last_delivery_error, enabled, created_at`

func scanSchedule(s interface{ Scan(...any) error }, sc *Schedule) error {
	var task sql.NullString
	var nextRun sql.NullInt64
	if err := s.Scan(&sc.ID, &sc.AgentID, &task, &sc.CronExpr, &sc.Prompt, &nextRun,
		&sc.LastRunAt, &sc.LastDeliveryStatus, &sc.LastDeliveryError, &sc.Enabled,
		&sc.CreatedAt); err != nil {
		return err
	}
	sc.TaskID = task.String
	sc.NextRunAt = nextRun.Int64
	return nil
}

// CreateSchedule inserts a new schedule.
func (d *DB) CreateSchedule(ctx context.Context, sc Schedule) (Schedule, error) {
	sc.ID = newID()
	sc.CreatedAt = now()
	_, err := d.ExecContext(ctx, `INSERT INTO schedules
		(id, agent_id, task_id, cron_expr, prompt, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sc.ID, sc.AgentID, nullable(sc.TaskID), sc.CronExpr, sc.Prompt, sc.Enabled, sc.CreatedAt)
	return sc, err
}

// GetSchedule loads a schedule by id.
func (d *DB) GetSchedule(ctx context.Context, id string) (Schedule, error) {
	var sc Schedule
	err := scanSchedule(d.QueryRowContext(ctx, `SELECT `+scheduleColumns+` FROM schedules WHERE id = ?`, id), &sc)
	if errors.Is(err, sql.ErrNoRows) {
		return sc, ErrNotFound
	}
	return sc, err
}

// ListSchedules returns all schedules, newest first.
func (d *DB) ListSchedules(ctx context.Context) ([]Schedule, error) {
	return d.querySchedules(ctx, `SELECT `+scheduleColumns+` FROM schedules ORDER BY created_at DESC`)
}

// ListEnabledSchedules returns only enabled schedules (for the scheduler boot).
func (d *DB) ListEnabledSchedules(ctx context.Context) ([]Schedule, error) {
	return d.querySchedules(ctx, `SELECT `+scheduleColumns+` FROM schedules WHERE enabled = 1`)
}

func (d *DB) querySchedules(ctx context.Context, query string, args ...any) ([]Schedule, error) {
	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Schedule
	for rows.Next() {
		var sc Schedule
		if err := scanSchedule(rows, &sc); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// SetScheduleEnabled toggles a schedule on or off.
func (d *DB) SetScheduleEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := d.ExecContext(ctx, `UPDATE schedules SET enabled = ? WHERE id = ?`, enabled, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// SetScheduleDelivery records the outcome of the latest fire and the next run time.
func (d *DB) SetScheduleDelivery(ctx context.Context, id, status, deliveryErr string, nextRunAt int64) error {
	_, err := d.ExecContext(ctx, `UPDATE schedules
		SET last_delivery_status = ?, last_delivery_error = ?, last_run_at = ?, next_run_at = ?
		WHERE id = ?`, status, deliveryErr, now(), nextRunAt, id)
	return err
}

// GetOrCreateKindSession returns the agent's dedicated session of the given
// kind (e.g. "schedule"), creating it once if absent. Used to log scheduled
// prompt deliveries in a stable, discoverable thread.
func (d *DB) GetOrCreateKindSession(ctx context.Context, agentID, kind, title string) (Session, error) {
	var s Session
	err := scanSession(d.QueryRowContext(ctx, `SELECT `+sessionColumns+`
		FROM sessions WHERE agent_id = ? AND kind = ? LIMIT 1`, agentID, kind), &s)
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}
	return d.CreateSession(ctx, Session{AgentID: agentID, Kind: kind, Title: title})
}

// DeleteSchedule removes a schedule.
func (d *DB) DeleteSchedule(ctx context.Context, id string) error {
	res, err := d.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}
