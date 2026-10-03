package db

import "slices"

// SessionRuntime is the persistent part of a sidebar runtime row. Running
// state is deliberately absent: live registries change without store writes.
type SessionRuntime struct {
	SessionID                string
	LastStatus               string
	CoordinatorSessionID     string
	RootCoordinatorSessionID string
}

type runtimeSessionsSnapshot struct {
	gen  uint64
	rows []SessionRuntime
}

// RuntimeSessions reuses a compact projection until the store changes.
func (d *DB) RuntimeSessions() []SessionRuntime {
	if snap := d.runtimeSessions.Load(); snap != nil && snap.gen == d.mutGen.Load() {
		return slices.Clone(snap.rows)
	}
	d.runtimeSessionsMu.Lock()
	defer d.runtimeSessionsMu.Unlock()
	if snap := d.runtimeSessions.Load(); snap != nil && snap.gen == d.mutGen.Load() {
		return slices.Clone(snap.rows)
	}
	d.mu.RLock()
	gen := d.mutGen.Load()
	type orderedRow struct {
		row     SessionRuntime
		pinned  bool
		updated int64
	}
	ordered := make([]orderedRow, 0)
	for _, raw := range d.sessions {
		sess := normalizeSessionMeta(raw)
		row := SessionRuntime{SessionID: sess.ID, CoordinatorSessionID: sess.CoordinatorSessionID, RootCoordinatorSessionID: sess.RootCoordinator()}
		if sess.SourceID != "" {
			switch sess.Kind {
			case "task":
				row.LastStatus = d.tasks[sess.SourceID].LastRunStatus
			}
		}
		if row.LastStatus != "" || row.CoordinatorSessionID != "" {
			ordered = append(ordered, orderedRow{row: row, pinned: sess.Pinned, updated: sess.UpdatedAt})
		}
	}
	d.mu.RUnlock()
	// Preserve the existing sidebar order without sorting under the store lock.
	slices.SortFunc(ordered, func(a, b orderedRow) int {
		if a.pinned != b.pinned {
			if a.pinned {
				return -1
			}
			return 1
		}
		if a.updated != b.updated {
			if a.updated > b.updated {
				return -1
			}
			return 1
		}
		if a.row.SessionID > b.row.SessionID {
			return -1
		}
		if a.row.SessionID < b.row.SessionID {
			return 1
		}
		return 0
	})
	rows := make([]SessionRuntime, len(ordered))
	for i, row := range ordered {
		rows[i] = row.row
	}
	d.runtimeSessions.Store(&runtimeSessionsSnapshot{gen: gen, rows: rows})
	return slices.Clone(rows)
}
