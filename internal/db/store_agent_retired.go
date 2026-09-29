package db

import (
	"context"
	"sort"
)

// RetireSystemAgents removes obsolete built-in roles from active workspaces.
// Their rows remain available for historical sessions, while associated work
// and schedules are cancelled through the normal agent deletion path.
func (d *DB) RetireSystemAgents(ctx context.Context, keys ...string) error {
	retired := make(map[string]bool, len(keys))
	for _, key := range keys {
		retired[key] = true
	}

	d.mu.RLock()
	rows := make([]Agent, 0)
	for _, a := range d.agents {
		if a.System && retired[a.SystemKey] && !a.Deleted {
			rows = append(rows, a)
		}
	}
	d.mu.RUnlock()

	// Remove customisations before their locked parent so child reparenting can
	// still resolve the parent while the role is retired.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Locked != rows[j].Locked {
			return !rows[i].Locked
		}
		return rows[i].ID < rows[j].ID
	})
	for _, a := range rows {
		if err := d.deleteAgent(ctx, a.ID, true); err != nil {
			return err
		}
	}
	return nil
}
