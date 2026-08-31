package db

import (
	"encoding/json"
	"log/slog"
	"os"
	"sort"
)

// inflightFile is the per-session sidecar holding the assistant turn that is
// currently streaming. It lives next to session.jsonl and is written via the
// atomic tmp→rename helper on a throttle while the reply is generated, then
// removed the instant the turn returns (success or handled failure alike).
//
// Its sole purpose is crash recovery: if the process dies mid-turn (a dev
// rebuild, an OOM, a power loss), the streamed-but-unpersisted reply would
// otherwise be lost — the assistant message is only appended to session.jsonl
// after generation completes. The orphaned sidecar lets the next boot
// reconstruct a partial-but-saved message instead of leaving a "vanished" turn.
const inflightFile = "inflight.json"

// InflightTurn is the on-disk snapshot of a streaming assistant reply. Steps is
// the already-marshalled TurnStep[] JSON (the db layer treats it as opaque, just
// like Message.Steps) so this package never needs to import the agent package.
type InflightTurn struct {
	MessageID string `json:"messageId"`
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId"`
	StartedAt int64  `json:"startedAt"`
	// Text is the partial answer accumulated from streaming deltas so far.
	Text string `json:"text"`
	// Steps is the partial activity trace as a JSON array string (may be "[]").
	Steps string `json:"steps"`
}

// inflightPath returns the sidecar path for a session.
func (d *DB) inflightPath(sessionID string) string {
	return d.dir(dirSessions, sessionID, inflightFile)
}

// WriteInflight atomically persists the current streaming snapshot for a turn.
// Safe to call frequently (throttled by the caller); writes a separate file, so
// it never touches the session.jsonl append hot path and needs no store lock.
func (d *DB) WriteInflight(t InflightTurn) error {
	if t.SessionID == "" {
		return nil
	}
	if t.Steps == "" {
		t.Steps = "[]"
	}
	return atomicWriteJSON(d.inflightPath(t.SessionID), t)
}

// ClearInflight removes a session's sidecar. A missing file is not an error —
// the turn completed cleanly and there was nothing to recover.
func (d *DB) ClearInflight(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	err := os.Remove(d.inflightPath(sessionID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ReadInflight exposes a session's current streaming snapshot (the partial reply
// text + trace being written on a throttle while a turn runs). Returns ok=false
// when there is no live turn. Used by the API so a page reloaded MID-TURN can
// restore the in-progress assistant bubble (agent, steps-so-far) instead of
// showing a bare "thinking" dot until the turn finishes. Distinct from
// recoverInflight, which only materialises CRASH-orphaned sidecars at boot.
func (d *DB) ReadInflight(sessionID string) (InflightTurn, bool, error) {
	return d.readInflight(sessionID)
}

// readInflight loads a session's sidecar, returning ok=false when absent.
func (d *DB) readInflight(sessionID string) (InflightTurn, bool, error) {
	b, err := os.ReadFile(d.inflightPath(sessionID))
	if os.IsNotExist(err) {
		return InflightTurn{}, false, nil
	}
	if err != nil {
		return InflightTurn{}, false, err
	}
	var t InflightTurn
	if err := json.Unmarshal(b, &t); err != nil {
		return InflightTurn{}, false, err
	}
	return t, true, nil
}

// recoverInflight materialises orphaned sidecars left by a crash mid-turn. For
// each loaded session with an inflight.json: if its message was already
// persisted (the crash happened in the tiny window after appending the reply but
// before clearing the sidecar) the file is simply dropped; otherwise the partial
// reply is appended to session.jsonl as an interrupted assistant message so it
// survives the reload. Idempotent via the pre-allocated MessageID.
//
// Must be called after loadSessions (it relies on the in-memory message lists)
// and before serving. Best-effort: a bad sidecar is logged-by-return but never
// aborts boot for other sessions.
func (d *DB) recoverInflight() error {
	// Probe every session's sidecar CONCURRENTLY and outside the store lock. In
	// the common case (a clean shutdown) all of these are misses, but a miss is
	// still a cold file open — one per session — and on Windows that is ~15 ms
	// each, so a 100-session workspace paid over a second here for nothing. The
	// mutation pass below stays serial and locked.
	d.mu.RLock()
	ids := make([]string, 0, len(d.sessions))
	for id := range d.sessions {
		ids = append(ids, id)
	}
	d.mu.RUnlock()
	// Map iteration is random; sort so recovery order (and therefore the ids of
	// any recovered messages) is reproducible across boots.
	sort.Strings(ids)

	type sidecar struct {
		id string
		t  InflightTurn
		ok bool
	}
	found, err := parallelLoad(ids, func(id string) (sidecar, error) {
		t, ok, rerr := d.readInflight(id)
		if rerr != nil {
			// Unreadable sidecar: nothing to recover from it, same as before. It is
			// left on disk rather than deleted, so the next boot retries.
			slog.Warn("inflight sidecar unreadable", "component", "db", "session", id, "error", rerr)
			return sidecar{id: id}, nil
		}
		return sidecar{id: id, t: t, ok: ok}, nil
	})
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	for _, f := range found {
		if !f.ok {
			continue // no sidecar: nothing to recover
		}
		sessionID, t := f.id, f.t
		s, exists := d.sessions[sessionID]
		if !exists {
			continue
		}
		// Already persisted? (crash between append and clear) → just drop it.
		alreadyPersisted := false
		for _, m := range d.messages[sessionID] {
			if m.ID == t.MessageID {
				alreadyPersisted = true
				break
			}
		}
		if !alreadyPersisted && t.MessageID != "" {
			m := Message{
				ID:          t.MessageID,
				SessionID:   sessionID,
				Role:        "assistant",
				AgentID:     t.AgentID,
				Text:        t.Text,
				Steps:       t.Steps,
				ToolCalls:   "[]",
				Interrupted: true,
				CreatedAt:   now(),
			}
			if m.Steps == "" {
				m.Steps = "[]"
			}
			d.messages[sessionID] = append(d.messages[sessionID], m)
			s.MessageCount++
			s.UpdatedAt = m.CreatedAt
			s.Unread = true
			d.sessions[sessionID] = s
			// Append the recovered line; tolerate write failure (the sidecar stays
			// and we retry next boot). No transcript lock: recovery runs inside
			// load(), single-threaded, before the DB is published.
			_ = d.appendMessageLine(sessionID, m)
		}
		_ = os.Remove(d.inflightPath(sessionID))
	}
	return nil
}
