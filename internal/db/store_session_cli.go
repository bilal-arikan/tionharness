// Session fields owned by the claude-cli transport: the resumable CLI session id and the native-compaction bookkeeping around it.
package db

import (
	"context"
	"errors"
)

// SetSessionCLIResume records the claude-cli resume state for a session: the
// (rotated) CLI session id to --resume next turn, and how many of the session's
// messages the CLI has already seen (so the next turn sends only the delta). Does
// not bump UpdatedAt — bookkeeping must not reorder the session list.
func (d *DB) SetSessionCLIResume(ctx context.Context, sessionID, cliSessionID string, sentMsgCount int) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.CLISessionID = cliSessionID
		s.CLISentMsgCount = sentMsgCount
	})
}

// BeginSessionCLINativeCompaction durably marks the external CLI state as
// potentially changing. The CLI must not be invoked if this write fails.
func (d *DB) BeginSessionCLINativeCompaction(ctx context.Context, sessionID string) error {
	return d.mutateSessionAfterWriteLocked(sessionID, func(s *Session) error {
		if s.CLINativeCompactionPending {
			return errors.New("CLI native compaction recovery is pending")
		}
		s.CLINativeCompactionPending = true
		return nil
	})
}

// SetSessionCLICompactionState atomically commits the CLI's rotated resume
// target, both transcript counters and recovery-marker clearance. A failed temp
// write or rename leaves the durable and in-memory marker set, forcing a cold
// next turn rather than reusing the old resume id and delta cursor.
func (d *DB) SetSessionCLICompactionState(ctx context.Context, sessionID, cliSessionID string, msgCount int) error {
	return d.mutateSessionAfterWriteLocked(sessionID, func(s *Session) error {
		s.CLISessionID = cliSessionID
		s.CLISentMsgCount = msgCount
		if msgCount > s.CLICompactMsgCount {
			s.CLICompactMsgCount = msgCount
		}
		s.CLINativeCompactionPending = false
		return nil
	})
}

// RetireSessionCLINativeCompactionRecovery atomically discards stale external
// resume authority when the pending recovery cannot run through a compatible
// resumer. A failed write leaves the old state and marker untouched.
func (d *DB) RetireSessionCLINativeCompactionRecovery(ctx context.Context, sessionID string) error {
	return d.mutateSessionAfterWriteLocked(sessionID, func(s *Session) error {
		s.CLISessionID = ""
		s.CLISentMsgCount = 0
		s.CLICompactMsgCount = 0
		s.CLINativeCompactionPending = false
		return nil
	})
}

// SetSessionCLICompactBoundary records the transcript length the provider's own
// context was compacted at (see Session.CLICompactMsgCount). Monotonic: a later
// compaction always moves the boundary forward, and a stale/smaller value is
// ignored rather than rewinding the baseline. Does not bump UpdatedAt —
// bookkeeping must not reorder the session list.
func (d *DB) SetSessionCLICompactBoundary(ctx context.Context, sessionID string, msgCount int) error {
	return d.mutateSessionAfterWriteLocked(sessionID, func(s *Session) error {
		if msgCount > s.CLICompactMsgCount {
			s.CLICompactMsgCount = msgCount
		}
		return nil
	})
}
