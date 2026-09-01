package db

import (
	"encoding/json"
	"os"
	"strconv"
	"time"
)

// inboxFile is the per-session durable command queue: user messages submitted
// while a turn is already running (or before the worker picked them up) wait
// here so they survive a reload/crash and are re-dispatched at the next boot.
// It lives next to session.jsonl and is written atomically on every change,
// removed when the queue drains. The payload is opaque JSON owned by the api
// layer (a []queuedMessage), kept raw here so this package stays decoupled from
// the turn-request shape — exactly like inflight's Steps. See _Docs/58.
const inboxFile = "inbox.json"

// inboxPath returns the queue sidecar path for a session.
func (d *DB) inboxPath(sessionID string) string {
	return d.dir(dirSessions, sessionID, inboxFile)
}

// WriteInbox atomically persists a session's queued-message list (opaque JSON).
// An empty/nil payload clears it (a drained queue leaves no sidecar).
func (d *DB) WriteInbox(sessionID string, data []byte) error {
	if sessionID == "" {
		return nil
	}
	if len(data) == 0 {
		return d.ClearInbox(sessionID)
	}
	// atomicWriteJSON marshals its argument; a json.RawMessage marshals to itself,
	// so the opaque payload lands byte-for-byte.
	return atomicWriteJSON(d.inboxPath(sessionID), json.RawMessage(data))
}

// ReadInbox returns a session's persisted queue payload, ok=false when absent.
func (d *DB) ReadInbox(sessionID string) ([]byte, bool, error) {
	b, err := os.ReadFile(d.inboxPath(sessionID))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// QuarantineInbox moves a session's queue sidecar aside as
// inbox.json.corrupt-<unix> instead of deleting it, so a payload the api layer
// could not parse stays recoverable by hand — the queued user messages inside it
// are the only copy. Naming mirrors the settings store's quarantine. It returns
// the destination path ("" when there was no sidecar); a missing file is not an
// error.
func (d *DB) QuarantineInbox(sessionID string) (string, error) {
	if sessionID == "" {
		return "", nil
	}
	src := d.inboxPath(sessionID)
	dest := src + ".corrupt-" + strconv.FormatInt(time.Now().Unix(), 10)
	if err := os.Rename(src, dest); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return dest, nil
}

// ClearInbox removes a session's queue sidecar. A missing file is not an error.
func (d *DB) ClearInbox(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	err := os.Remove(d.inboxPath(sessionID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
