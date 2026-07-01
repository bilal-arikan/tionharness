package db

import (
	"context"
)

func (d *DB) persistTaskLocked(t Task) error {
	return dbPersistLocked(d, d.tasks, dirTasks, t.ID, t)
}

// CreateTask inserts a new task and returns the stored row.
func (d *DB) CreateTask(ctx context.Context, t Task) (Task, error) {
	t.ID = d.nextID(idTask)
	t.CreatedAt = now()
	t.UpdatedAt = t.CreatedAt
	if t.BoardState == "" {
		t.BoardState = BoardTodo
	}
	if t.Dependencies == "" {
		t.Dependencies = "[]"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return t, d.persistTaskLocked(t)
}

// GetTask loads a task by id.
func (d *DB) GetTask(ctx context.Context, id string) (Task, error) {
	return dbGet(d, d.tasks, id)
}

// ListTasks returns all tasks, newest first.
func (d *DB) ListTasks(ctx context.Context) ([]Task, error) {
	return dbList(d, d.tasks, func(a, b Task) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// UpdateTask edits the mutable fields of a task (title/description/prompt/owner/state).
func (d *DB) UpdateTask(ctx context.Context, t Task) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.tasks[t.ID]
	if !ok {
		return ErrNotFound
	}
	cur.Title = t.Title
	cur.Description = t.Description
	cur.Prompt = t.Prompt
	cur.OwnerAgentID = t.OwnerAgentID
	cur.FlowID = t.FlowID
	cur.BoardState = t.BoardState
	if t.Dependencies != "" {
		cur.Dependencies = t.Dependencies
	}
	cur.Priority = t.Priority
	cur.Tags = t.Tags
	cur.Progress = t.Progress
	cur.StartDate = t.StartDate
	cur.DueDate = t.DueDate
	cur.UpdatedAt = now()
	return d.persistTaskLocked(cur)
}

// MoveTask changes only a task's board state (kanban drag/drop).
func (d *DB) MoveTask(ctx context.Context, id, boardState string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, ok := d.tasks[id]
	if !ok {
		return ErrNotFound
	}
	t.BoardState = boardState
	t.UpdatedAt = now()
	return d.persistTaskLocked(t)
}

// DeleteTask removes a task and all of its runs.
func (d *DB) DeleteTask(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.tasks[id]; !ok {
		return ErrNotFound
	}
	delete(d.tasks, id)
	if err := removeFile(d.dir(dirTasks, id+".json")); err != nil {
		return err
	}
	// Cascade: remove runs belonging to this task.
	for rid, r := range d.runs {
		if r.TaskID == id {
			delete(d.runs, rid)
			_ = removeFile(d.dir(dirRuns, rid+".json"))
		}
	}
	return nil
}
