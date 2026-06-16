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
