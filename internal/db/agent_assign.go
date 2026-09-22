package db

// AssignableErr reports whether the agent may be written as the target of a
// schedule, an automation or a task owner, or nil when it may. currentID is the
// agent the record already points at ("" on create).
//
// An archived agent can never run (RunnableErr), so pointing a record at one
// is refused up front with the same archive.ErrArchived error instead of being
// accepted and failing only at fire time. Keeping the already-stored target is
// allowed even when that agent is archived: an edit form resends the whole
// record, and renaming or pausing a schedule must not require first moving it
// to another agent. Such a record still refuses to run until the agent is
// restored.
func (a Agent) AssignableErr(currentID string) error {
	if currentID != "" && a.ID == currentID {
		return nil
	}
	return a.RunnableErr()
}
