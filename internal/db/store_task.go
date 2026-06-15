package db

import (
	"context"
	"database/sql"
	"errors"
)

const taskColumns = `id, title, description, prompt, owner_agent_id, board_state,
	dependencies, last_run_id, last_run_status, last_run_at, created_at, updated_at`

func scanTask(s interface{ Scan(...any) error }, t *Task) error {
	var owner sql.NullString
	if err := s.Scan(&t.ID, &t.Title, &t.Description, &t.Prompt, &owner, &t.BoardState,
		&t.Dependencies, &t.LastRunID, &t.LastRunStatus, &t.LastRunAt,
		&t.CreatedAt, &t.UpdatedAt); err != nil {
		return err
	}
	t.OwnerAgentID = owner.String
	return nil
}

// CreateTask inserts a new task and returns the stored row.
func (d *DB) CreateTask(ctx context.Context, t Task) (Task, error) {
	t.ID = newID()
	t.CreatedAt = now()
	t.UpdatedAt = t.CreatedAt
	if t.BoardState == "" {
		t.BoardState = BoardTodo
	}
	if t.Dependencies == "" {
		t.Dependencies = "[]"
	}
	_, err := d.ExecContext(ctx, `INSERT INTO tasks
		(id, title, description, prompt, owner_agent_id, board_state, dependencies, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Title, t.Description, t.Prompt, nullable(t.OwnerAgentID), t.BoardState,
		t.Dependencies, t.CreatedAt, t.UpdatedAt)
	return t, err
}

// GetTask loads a task by id.
func (d *DB) GetTask(ctx context.Context, id string) (Task, error) {
	var t Task
	err := scanTask(d.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id), &t)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// ListTasks returns all tasks, newest first.
func (d *DB) ListTasks(ctx context.Context) ([]Task, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Task
	for rows.Next() {
		var t Task
		if err := scanTask(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateTask edits the mutable fields of a task (title/description/prompt/owner/state).
func (d *DB) UpdateTask(ctx context.Context, t Task) error {
	res, err := d.ExecContext(ctx, `UPDATE tasks
		SET title = ?, description = ?, prompt = ?, owner_agent_id = ?, board_state = ?,
		    dependencies = ?, updated_at = ?
		WHERE id = ?`,
		t.Title, t.Description, t.Prompt, nullable(t.OwnerAgentID), t.BoardState,
		t.Dependencies, now(), t.ID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// MoveTask changes only a task's board state (kanban drag/drop).
func (d *DB) MoveTask(ctx context.Context, id, boardState string) error {
	res, err := d.ExecContext(ctx, `UPDATE tasks SET board_state = ?, updated_at = ? WHERE id = ?`,
		boardState, now(), id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// SetTaskLastRun records the outcome of the latest run on the task.
func (d *DB) SetTaskLastRun(ctx context.Context, taskID, runID, status string, boardState string) error {
	_, err := d.ExecContext(ctx, `UPDATE tasks
		SET last_run_id = ?, last_run_status = ?, last_run_at = ?, board_state = ?, updated_at = ?
		WHERE id = ?`,
		runID, status, now(), boardState, now(), taskID)
	return err
}

// DeleteTask removes a task (its runs cascade).
func (d *DB) DeleteTask(ctx context.Context, id string) error {
	res, err := d.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// ---- shared helpers ----

// nullable converts an empty string to a SQL NULL (for optional FK columns).
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func mustAffect(res interface{ RowsAffected() (int64, error) }) error {
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
