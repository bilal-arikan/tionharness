package db

import (
	"errors"
	"fmt"
)

// persistTranscriptEditLocked publishes a rewrite only after persistence. The
// caller holds both transcript and store locks. A failed transcript replacement
// leaves the canonical messages intact and restores the previous header.
func (d *DB) persistTranscriptEditLocked(s Session, msgs []Message) error {
	previous := d.messages[s.ID]
	d.messages[s.ID] = msgs
	if err := d.writeSessionFileLocked(s); err != nil {
		d.messages[s.ID] = previous
		if restoreErr := d.writeSessionHeaderLocked(d.sessions[s.ID]); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore transcript header: %w", restoreErr))
		}
		return err
	}
	d.sessions[s.ID] = s
	d.markMutatedLocked()
	return nil
}
