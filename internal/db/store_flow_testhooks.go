package db

// SetFlowRunCreatedAtForTest rewrites a run's CreatedAt in memory (not on disk)
// so tests can make retention ordering deterministic without sleeping. Test
// support only — production code never changes a run's creation time.
func (d *DB) SetFlowRunCreatedAtForTest(id string, createdAt int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r, ok := d.flowRuns[id]; ok {
		r.CreatedAt = createdAt
		d.flowRuns[id] = r
	}
}
