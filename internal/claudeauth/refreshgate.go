package claudeauth

import (
	"sync"
	"time"
)

// refreshgate.go serializes claude-cli launches that are about to REFRESH the
// OAuth token in a shared claude-home.
//
// The problem it solves is specific and was observed live (a 3-level coordinator
// tree, _Docs/47): TionSwarm runs many `claude` subprocesses concurrently against
// ONE per-workspace claude-home. OAuth refresh tokens are single-use, so when
// several processes find the access token expired at the same moment they all
// refresh with the same refresh token — the first wins, the rest get
// invalid_grant, and the CLI reacts by CLEARING <home>/.credentials.json
// (accessToken:"", refreshToken:"", expiresAt:0). Every later turn in that
// workspace then fails "not logged in", permanently, because the credential
// self-heal only runs on workspace open.
//
// The fix is deliberately narrow: serialize ONLY the window in which a refresh is
// actually due. While the access token is comfortably valid — the overwhelmingly
// common case, and the whole point of a coordinator tree — launches stay fully
// parallel and this costs one small file read.

// refreshMargin is how long before expiry a token counts as "about to refresh".
// Generous on purpose: the CLI refreshes proactively, and a launch that slips
// past the real expiry mid-startup is exactly the race we are avoiding.
const refreshMargin = 10 * time.Minute

// refreshWatchWindow caps how long the gate stays closed behind one launch. The
// holder is released as soon as the credential file's expiry actually advances
// (the refresh landed); this is only the backstop for a launch that never
// refreshes at all — a failed start, a turn that dies early, a CLI build that
// does not rewrite the file. Without a cap one such launch would block every
// sibling in the workspace.
//
// A var, not a const, purely so the test for that backstop can shrink it: waiting
// out the real window would add half a minute to every suite run.
var refreshWatchWindow = 30 * time.Second

// refreshPoll is how often the watcher re-reads the credential file while waiting
// for the refresh to land.
const refreshPoll = 250 * time.Millisecond

var gate struct {
	mu    sync.Mutex
	homes map[string]*sync.Mutex // one lock per claude-home dir
}

// homeLock returns (creating if needed) the launch lock for one claude-home.
func homeLock(homeDir string) *sync.Mutex {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.homes == nil {
		gate.homes = map[string]*sync.Mutex{}
	}
	m, ok := gate.homes[homeDir]
	if !ok {
		m = &sync.Mutex{}
		gate.homes[homeDir] = m
	}
	return m
}

// SerializeRefresh blocks until it is safe for THIS process to launch a claude-cli
// subprocess against homeDir, then returns — the caller launches immediately.
//
// When no refresh is due it returns at once and concurrent launches are unaffected.
// When one IS due it admits a single launcher and holds the gate closed behind it
// until that launcher's refresh lands in the credential file (or refreshWatchWindow
// elapses), so siblings start against the NEW token instead of racing for the
// single-use refresh token.
//
// Safe to call with an empty homeDir (no isolated home → nothing to serialize) and
// for a home whose credentials cannot be read (treated as "refresh due", which is
// the conservative choice: an unreadable or wiped file is exactly the state a
// concurrent refresh left behind, and serializing the recovery is harmless).
func SerializeRefresh(homeDir string) {
	if homeDir == "" || !refreshDue(homeDir) {
		return
	}
	lock := homeLock(homeDir)
	lock.Lock()
	// A sibling may have refreshed while we waited for the lock — in that case
	// there is nothing left to serialize and we must not hold the gate closed.
	if !refreshDue(homeDir) {
		lock.Unlock()
		return
	}
	before := expiryOf(homeDir)
	// Released asynchronously: the caller launches NOW (that is the launch we are
	// admitting), and the gate opens once its refresh is observable to siblings.
	go func() {
		defer lock.Unlock()
		deadline := time.Now().Add(refreshWatchWindow)
		for time.Now().Before(deadline) {
			time.Sleep(refreshPoll)
			if expiryOf(homeDir) != before {
				return // the refresh landed; siblings may proceed against the new token
			}
		}
	}()
}

// refreshDue reports whether a launch against homeDir is likely to trigger an
// OAuth refresh: the access token is missing, has no expiry, or expires within
// refreshMargin.
func refreshDue(homeDir string) bool {
	cred, err := ReadCredentials(homeDir)
	if err != nil {
		return true // unreadable/missing: assume the worst and serialize
	}
	if cred.AccessToken == "" || cred.ExpiresAt == 0 {
		return true // wiped or never logged in
	}
	return time.Now().Add(refreshMargin).After(time.UnixMilli(cred.ExpiresAt))
}

// expiryOf returns the credential's expiry stamp (unix ms), or 0 when it cannot be
// read. Used as the change signal the refresh watcher waits on.
func expiryOf(homeDir string) int64 {
	cred, err := ReadCredentials(homeDir)
	if err != nil {
		return 0
	}
	return cred.ExpiresAt
}
