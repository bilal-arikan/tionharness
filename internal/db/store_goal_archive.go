package db

import "context"

// SetGoalArchived maps the shared archive/unarchive pair onto a goal's status,
// which already carries the archived state (GoalStatusArchived). Archiving
// moves the goal to "archived"; unarchiving an archived goal returns it to
// "draft" — the same transition the Goals screen offers — so a restored goal
// never resumes autonomous evolution without the user re-activating it.
// Unarchiving a goal that is not archived is a no-op. by is recorded in the
// goal's revision history. A missing goal is ErrNotFound.
//
// Both the REST archive routes and the set_archived agent tool go through
// here, so the transition rule lives in one place.
func (d *DB) SetGoalArchived(ctx context.Context, id string, archived bool, by string) (Goal, error) {
	g, err := d.GetGoal(ctx, id)
	if err != nil {
		return Goal{}, err
	}
	switch {
	case archived:
		return d.SetGoalStatus(ctx, id, GoalStatusArchived, by)
	case g.Status == GoalStatusArchived:
		return d.SetGoalStatus(ctx, id, GoalStatusDraft, by)
	}
	return g, nil
}
