package db

import (
	"context"
	"sort"
)

// ListRunningRuns returns every run currently in the running state, oldest
// first. Used by the activity endpoint to light the board/schedules nav
// indicators. Runs are produced by external execution paths (flows, schedules);
// the board itself no longer runs tasks.
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
