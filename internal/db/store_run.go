package db

import (
	"context"
	"sort"
)

func (d *DB) persistRunLocked(r Run) error {
	d.runs[r.ID] = r
	return atomicWriteJSON(d.dir(dirRuns, r.ID+".json"), r)
}

// CreateRun starts a new run (typically in RunRunning state).
func (d *DB) CreateRun(ctx context.Context, r Run) (Run, error) {
	r.ID = newID()
	r.CreatedAt = now()
	r.UpdatedAt = r.CreatedAt
	if r.Status == "" {
		r.Status = RunPending
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return r, d.persistRunLocked(r)
}

// SetRunSession links a run to the transcript session (and assistant message)
// it produced, so the board can deep-link a run straight to its conversation.
func (d *DB) SetRunSession(ctx context.Context, runID, sessionID, messageID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.runs[runID]
	if !ok {
		return ErrNotFound
	}
	r.SessionID = sessionID
	r.MessageID = messageID
	r.UpdatedAt = now()
	return d.persistRunLocked(r)
}

// FinishRun records the terminal status, output and error of a run.
func (d *DB) FinishRun(ctx context.Context, runID, status, output, runErr string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	r, ok := d.runs[runID]
	if !ok {
		return ErrNotFound
	}
	r.Status = status
	r.Output = output
	r.Error = runErr
	r.UpdatedAt = now()
	return d.persistRunLocked(r)
}

// ListRuns returns runs for a task, newest first.
func (d *DB) ListRuns(ctx context.Context, taskID string) ([]Run, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Run, 0)
	for _, r := range d.runs {
		if r.TaskID == taskID {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// ListRunningRuns returns every run currently in the running state, across all
// tasks, oldest first. A running run means a task is executing on the board; its
// Trigger distinguishes manual/dependency/schedule-initiated work. Used by the
// activity endpoint to light the board/schedules nav indicators.
func (d *DB) ListRunningRuns(ctx context.Context) ([]Run, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Run, 0)
	for _, r := range d.runs {
		if r.Status == RunRunning {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

// GetRun loads a run by id.
func (d *DB) GetRun(ctx context.Context, id string) (Run, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	r, ok := d.runs[id]
	if !ok {
		return Run{}, ErrNotFound
	}
	return r, nil
}
