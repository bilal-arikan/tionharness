package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// errInstanceLocked is returned when another process already holds the data-dir
// lock. Bootstrap wraps it with a user-facing message.
var errInstanceLocked = errors.New("data directory is locked by another TionHarness instance")

// lockDataDir takes an exclusive, process-lifetime advisory lock on the data
// directory so a SECOND server process can never serve the same file store
// concurrently. The whole cross-window design (per-session hub, serial send-queue,
// coordSlot turn lock, interaction CAS, cron scheduler) lives in ONE process's
// memory; two processes over the same store means two of each — cross-process
// concurrent turns, double-fired schedules, duplicate boot re-dispatch, and
// last-writer-wins entity overwrites that corrupt the store (see _Docs/58, _Docs/30).
// The desktop app binds a ":0" port (no port collision to catch a double launch),
// so this store lock is the only guard against it.
//
// The lock auto-releases when this process exits — a crash included — so a stale
// lock never wedges a restart. Returns a clear, actionable error when it is held.
func lockDataDir(dir string) (*instanceLock, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir %q: %w", dir, err)
	}
	path := filepath.Join(dir, "instance.lock")
	l, err := acquireInstanceLock(path)
	if err != nil {
		return nil, fmt.Errorf("%w (%s): close the other instance first, or point this one at a different data directory: %v",
			errInstanceLocked, dir, err)
	}
	return l, nil
}
