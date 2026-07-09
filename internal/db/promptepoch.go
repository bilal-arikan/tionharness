package db

import "os"

// promptEpochFile is the per-session sidecar holding the frozen prompt-prefix
// snapshot (static system + tool defs per agent). It lives next to session.jsonl,
// like inflight.json, and is written via the atomic tmp→rename helper. The db
// layer treats the payload as opaque JSON — the agent package owns the schema —
// so this package never needs to import provider types.
const promptEpochFile = "prompt_epoch.json"

// promptEpochPath returns the sidecar path for a session.
func (d *DB) promptEpochPath(sessionID string) string {
	return d.dir(dirSessions, sessionID, promptEpochFile)
}

// WritePromptEpoch atomically persists a session's prompt-epoch snapshot. The
// value is marshalled as-is (the agent package passes its own file struct).
func (d *DB) WritePromptEpoch(sessionID string, v any) error {
	if sessionID == "" {
		return nil
	}
	return atomicWriteJSON(d.promptEpochPath(sessionID), v)
}

// ReadPromptEpoch loads a session's prompt-epoch sidecar, returning ok=false
// when absent.
func (d *DB) ReadPromptEpoch(sessionID string) ([]byte, bool, error) {
	if sessionID == "" {
		return nil, false, nil
	}
	b, err := os.ReadFile(d.promptEpochPath(sessionID))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// ClearPromptEpoch removes a session's prompt-epoch sidecar. A missing file is
// not an error.
func (d *DB) ClearPromptEpoch(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	err := os.Remove(d.promptEpochPath(sessionID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
