package db

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// ---- Agents ----

func (d *DB) persistAgentLocked(a Agent) error {
	d.agents[a.ID] = a
	return atomicWriteJSON(d.dir(dirAgents, a.ID+".json"), a)
}

// mutateAgentLocked loads an agent under the write lock, applies fn to it, bumps
// UpdatedAt, and persists it — centralizing the lock/lookup/mutate/persist dance
// shared by every agent mutator. Returns ErrNotFound when the agent is absent.
func (d *DB) mutateAgentLocked(id string, fn func(*Agent)) (Agent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.agents[id]
	if !ok {
		return Agent{}, ErrNotFound
	}
	fn(&a)
	a.UpdatedAt = now()
	if err := d.persistAgentLocked(a); err != nil {
		return Agent{}, err
	}
	return a, nil
}

// CreateAgent inserts a new agent and returns the stored row.
func (d *DB) CreateAgent(ctx context.Context, a Agent) (Agent, error) {
	a.ID = newID()
	a.CreatedAt = now()
	a.UpdatedAt = a.CreatedAt
	if a.Capabilities == "" {
		a.Capabilities = "[]"
	}
	if a.PlanningMode == "" {
		a.PlanningMode = "standard"
	}
	if a.AllowedTools == "" {
		a.AllowedTools = "[]"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return a, d.persistAgentLocked(a)
}

// GetAgent loads an agent by id.
func (d *DB) GetAgent(ctx context.Context, id string) (Agent, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	a, ok := d.agents[id]
	if !ok {
		return Agent{}, ErrNotFound
	}
	return a, nil
}

// ListAgents returns all agents, newest first.
func (d *DB) ListAgents(ctx context.Context) ([]Agent, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Agent, 0, len(d.agents))
	for _, a := range d.agents {
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// AgentProfilePatch carries the editable identity fields for UpdateAgent. A nil
// pointer leaves that field untouched, so callers can do partial updates.
type AgentProfilePatch struct {
	Name         *string
	Soul         *string
	Identity     *string
	Provider     *string
	Model        *string
	PlanningMode *string
	Avatar       *string
	Color        *string
}

// UpdateAgent applies a partial profile patch to an existing agent and persists
// it. Only non-nil patch fields are written.
func (d *DB) UpdateAgent(ctx context.Context, agentID string, p AgentProfilePatch) (Agent, error) {
	return d.mutateAgentLocked(agentID, func(a *Agent) {
		if p.Name != nil {
			a.Name = *p.Name
		}
		if p.Soul != nil {
			a.Soul = *p.Soul
		}
		if p.Identity != nil {
			a.Identity = *p.Identity
		}
		if p.Provider != nil {
			a.Provider = *p.Provider
		}
		if p.Model != nil {
			a.Model = *p.Model
		}
		if p.PlanningMode != nil {
			a.PlanningMode = *p.PlanningMode
		}
		if p.Avatar != nil {
			a.Avatar = *p.Avatar
		}
		if p.Color != nil {
			a.Color = *p.Color
		}
	})
}

// UpdateHeartbeat sets the autonomous wake configuration for an agent.
func (d *DB) UpdateHeartbeat(ctx context.Context, agentID string, enabled bool, intervalSec int, prompt string) error {
	_, err := d.mutateAgentLocked(agentID, func(a *Agent) {
		a.HeartbeatEnabled = enabled
		a.HeartbeatIntervalSec = intervalSec
		a.HeartbeatPrompt = prompt
	})
	return err
}

// GetOrCreateHeartbeatSession returns the dedicated heartbeat session for an
// agent, creating it if absent. Heartbeat output is logged here for visibility.
func (d *DB) GetOrCreateHeartbeatSession(ctx context.Context, agentID string) (Session, error) {
	return d.getOrCreateKindSession(agentID, "heartbeat", "♥ Heartbeat")
}

// ---- Sessions ----

func (d *DB) persistSessionLocked(s Session) error {
	d.sessions[s.ID] = s
	return d.writeSessionFileLocked(s)
}

// mutateSessionLocked loads a session under the write lock, applies fn, and
// persists it. Unlike the agent variant it does not touch UpdatedAt, leaving
// that to fn — some session mutations (e.g. rolling summary) are not "edits".
// Returns ErrNotFound when the session is absent.
func (d *DB) mutateSessionLocked(id string, fn func(*Session)) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	if !ok {
		return ErrNotFound
	}
	fn(&s)
	return d.persistSessionLocked(s)
}

// writeSessionFileLocked (re)writes a session's JSONL file: line 1 is the
// session header, the remaining lines are its messages in order.
func (d *DB) writeSessionFileLocked(s Session) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil { // header line
		return err
	}
	for _, m := range d.messages[s.ID] {
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	return atomicWriteBytes(d.dir(dirSessions, s.ID, "session.jsonl"), buf.Bytes())
}

// CreateSession inserts a new session.
func (d *DB) CreateSession(ctx context.Context, s Session) (Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.createSessionLocked(s)
}

func (d *DB) createSessionLocked(s Session) (Session, error) {
	s.ID = newID()
	s.CreatedAt = now()
	s.UpdatedAt = s.CreatedAt
	if s.Kind == "" {
		s.Kind = "chat"
	}
	if s.State == "" {
		s.State = "active"
	}
	d.messages[s.ID] = nil
	return s, d.persistSessionLocked(s)
}

func (d *DB) getOrCreateKindSession(agentID, kind, title string) (Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range d.sessions {
		if s.AgentID == agentID && s.Kind == kind {
			return s, nil
		}
	}
	return d.createSessionLocked(Session{AgentID: agentID, Kind: kind, Title: title})
}

// SetSessionSummary persists the rolling compaction summary for a session.
func (d *DB) SetSessionSummary(ctx context.Context, sessionID, summary string, msgCount int) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Summary = summary
		s.SummaryMsgCount = msgCount
	})
}

// SetSessionTitle persists a (re)generated title for a session.
func (d *DB) SetSessionTitle(ctx context.Context, sessionID, title string) error {
	return d.mutateSessionLocked(sessionID, func(s *Session) {
		s.Title = title
		s.UpdatedAt = now()
	})
}

// GetSession loads a session by id.
func (d *DB) GetSession(ctx context.Context, id string) (Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	s, ok := d.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	return s, nil
}

// ListSessions returns sessions for an agent (or all if agentID is empty),
// most recently updated first.
func (d *DB) ListSessions(ctx context.Context, agentID string) ([]Session, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Session, 0, len(d.sessions))
	for _, s := range d.sessions {
		if agentID == "" || s.AgentID == agentID {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// ---- Messages ----

// AddMessage appends a message to a session and bumps the session counter.
func (d *DB) AddMessage(ctx context.Context, m Message) (Message, error) {
	m.ID = newID()
	m.CreatedAt = now()
	if m.ToolCalls == "" {
		m.ToolCalls = "[]"
	}
	if m.Steps == "" {
		m.Steps = "[]"
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[m.SessionID]
	if !ok {
		return m, ErrNotFound
	}
	d.messages[m.SessionID] = append(d.messages[m.SessionID], m)
	s.MessageCount++
	s.UpdatedAt = m.CreatedAt
	d.sessions[s.ID] = s
	// Hot path: append only the new message line (O(1)) instead of rewriting the
	// whole conversation file (which was O(n) per message → O(n²) per session).
	// The header line keeps a stale MessageCount/UpdatedAt on disk; both are
	// recomputed from the message lines on load and refreshed by the next full
	// rewrite (title/summary change).
	return m, d.appendMessageLocked(s.ID, m)
}

// appendMessageLocked appends a single encoded message line to a session's
// JSONL file. The header line is written at session creation, so the file
// already exists with its header as line 1.
func (d *DB) appendMessageLocked(sessionID string, m Message) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return err
	}
	path := d.dir(dirSessions, sessionID, "session.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// ListMessages returns messages for a session in chronological order.
func (d *DB) ListMessages(ctx context.Context, sessionID string) ([]Message, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	msgs := d.messages[sessionID]
	out := make([]Message, len(msgs))
	copy(out, msgs)
	return out, nil
}

// ---- session loading (boot) ----

// loadSessions reads every sessions/<id>/session.jsonl file: the first line is
// the session header, the rest are its messages.
func (d *DB) loadSessions() error {
	entries, err := os.ReadDir(d.dir(dirSessions))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(d.dir(dirSessions), e.Name(), "session.jsonl")
		s, msgs, err := readSessionFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if s.ID == "" { // empty/headerless file — nothing usable
			continue
		}
		d.sessions[s.ID] = s
		d.messages[s.ID] = msgs
	}
	return nil
}

func readSessionFile(path string) (Session, []Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // allow large message lines
	var lines [][]byte
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		b := make([]byte, len(line)) // scanner reuses its buffer; copy out
		copy(b, line)
		lines = append(lines, b)
	}
	if err := sc.Err(); err != nil {
		return Session{}, nil, err
	}
	if len(lines) == 0 {
		return Session{}, nil, nil
	}

	var s Session
	if err := json.Unmarshal(lines[0], &s); err != nil {
		return Session{}, nil, err // header corruption is fatal
	}
	msgs := make([]Message, 0, len(lines)-1)
	for i := 1; i < len(lines); i++ {
		var m Message
		if err := json.Unmarshal(lines[i], &m); err != nil {
			// A torn trailing line (crash mid-append) is tolerated by dropping it;
			// corruption on any earlier line is real and fatal.
			if i == len(lines)-1 {
				break
			}
			return Session{}, nil, err
		}
		msgs = append(msgs, m)
	}
	// The append hot-path leaves the header's counters stale; recompute them from
	// the actual message lines so in-memory state is always authoritative.
	s.MessageCount = len(msgs)
	if n := len(msgs); n > 0 && msgs[n-1].CreatedAt > s.UpdatedAt {
		s.UpdatedAt = msgs[n-1].CreatedAt
	}
	return s, msgs, nil
}
