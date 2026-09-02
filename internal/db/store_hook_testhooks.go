package db

import "context"

// SetHookCreatedAtForTest rewrites a hook's creation time so a test can age it
// (the curator's "never fired in N days" rule). Test-only, like
// SetFlowRunCreatedAtForTest.
func (d *DB) SetHookCreatedAtForTest(ctx context.Context, id string, createdAt int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	h, ok := d.hooks[id]
	if !ok {
		return ErrNotFound
	}
	h.CreatedAt = createdAt
	return d.persistHookLocked(h)
}
