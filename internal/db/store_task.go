package db

import (
	"context"
	"sort"
)

func (d *DB) persistTaskLocked(t Task) error {
	d.tasks[t.ID] = t
	return atomicWriteJSON(d.dir(dirTasks, t.ID+".json"), t)
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
	d.mu.Lock()
	defer d.mu.Unlock()
	return t, d.persistTaskLocked(t)
}

// GetTask loads a task by id.
func (d *DB) GetTask(ctx context.Context, id string) (Task, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	t, ok := d.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}
	return t, nil
}

// ListTasks returns all tasks, newest first.
func (d *DB) ListTasks(ctx context.Context) ([]Task, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Task, 0, len(d.tasks))
	for _, t := range d.tasks {
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
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
