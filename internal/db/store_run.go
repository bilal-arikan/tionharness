package db

import (
	"context"
	"database/sql"
	"errors"
)

const runColumns = `id, task_id, agent_id, status, trigger, output, error, created_at, updated_at`

func scanRun(s interface{ Scan(...any) error }, r *Run) error {
	var task, agent sql.NullString
	if err := s.Scan(&r.ID, &task, &agent, &r.Status, &r.Trigger, &r.Output,
		&r.Error, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return err
	}
	r.TaskID = task.String
	r.AgentID = agent.String
	return nil
}

// CreateRun starts a new run row (typically in RunRunning state).
func (d *DB) CreateRun(ctx context.Context, r Run) (Run, error) {
	r.ID = newID()
	r.CreatedAt = now()
	r.UpdatedAt = r.CreatedAt
	if r.Status == "" {
		r.Status = RunPending
	}
	_, err := d.ExecContext(ctx, `INSERT INTO runs
		(id, task_id, agent_id, status, trigger, output, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, nullable(r.TaskID), nullable(r.AgentID), r.Status, r.Trigger,
		r.Output, r.Error, r.CreatedAt, r.UpdatedAt)
	return r, err
}

// FinishRun records the terminal status, output and error of a run.
func (d *DB) FinishRun(ctx context.Context, runID, status, output, runErr string) error {
	res, err := d.ExecContext(ctx, `UPDATE runs
		SET status = ?, output = ?, error = ?, updated_at = ?
		WHERE id = ?`, status, output, runErr, now(), runID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// ListRuns returns runs for a task, newest first.
func (d *DB) ListRuns(ctx context.Context, taskID string) ([]Run, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+runColumns+` FROM runs WHERE task_id = ? ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var r Run
		if err := scanRun(rows, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRun loads a run by id.
func (d *DB) GetRun(ctx context.Context, id string) (Run, error) {
	var r Run
	err := scanRun(d.QueryRowContext(ctx, `SELECT `+runColumns+` FROM runs WHERE id = ?`, id), &r)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}
