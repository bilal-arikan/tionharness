package db

import (
	"context"
	"errors"
)

// ErrSystemAgentArchive refuses archiving an agent that backs a built-in role:
// runtime jobs (titles, summaries, subagent profiles) resolve to it, so
// archiving it would break them instead of just hiding a roster entry.
var ErrSystemAgentArchive = errors.New("system agent cannot be archived; disable it or derive a copy instead")

// SetAgentArchived archives (archived=true) or restores an agent. Archiving is
// the reversible counterpart of DeleteAgent: nothing is cascaded — schedules,
// automations and sessions keep pointing at the agent, they just fail with an
// ErrArchived error while it stays archived. A deleted agent cannot be
// archived or restored (ErrNotFound), and system agents are refused.
// Re-applying the current state is a no-op that still returns the row.
func (d *DB) SetAgentArchived(ctx context.Context, id string, archived bool) (Agent, error) {
	return d.mutateAgentLockedErr(id, func(a *Agent) error {
		if a.Deleted {
			return ErrNotFound
		}
		if a.System || a.Locked {
			return ErrSystemAgentArchive
		}
		if a.Archived == archived {
			return nil
		}
		a.Archived = archived
		if archived {
			a.ArchivedAt = now()
		} else {
			a.ArchivedAt = 0
		}
		return nil
	})
}
