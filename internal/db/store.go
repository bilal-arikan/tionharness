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
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.agents[agentID]
	if !ok {
		return Agent{}, ErrNotFound
	}
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
	a.UpdatedAt = now()
	if err := d.persistAgentLocked(a); err != nil {
		return Agent{}, err
	}
	return a, nil
}

// UpdateHeartbeat sets the autonomous wake configuration for an agent.
func (d *DB) UpdateHeartbeat(ctx context.Context, agentID string, enabled bool, intervalSec int, prompt string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.agents[agentID]
	if !ok {
		return ErrNotFound
	}
	a.HeartbeatEnabled = enabled
	a.HeartbeatIntervalSec = intervalSec
	a.HeartbeatPrompt = prompt
	a.UpdatedAt = now()
	return d.persistAgentLocked(a)
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
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return ErrNotFound
	}
	s.Summary = summary
	s.SummaryMsgCount = msgCount
	return d.persistSessionLocked(s)
}

// SetSessionTitle persists a (re)generated title for a session.
func (d *DB) SetSessionTitle(ctx context.Context, sessionID, title string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[sessionID]
	if !ok {
		return ErrNotFound
	}
	s.Title = title
	s.UpdatedAt = now()
	return d.persistSessionLocked(s)
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
	return m, d.writeSessionFileLocked(s)
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

	var s Session
	var msgs []Message
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // allow large message lines
	first := true
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if first {
			if err := json.Unmarshal(line, &s); err != nil {
				return Session{}, nil, err
			}
			first = false
			continue
		}
		var m Message
		if err := json.Unmarshal(line, &m); err != nil {
			return Session{}, nil, err
		}
		msgs = append(msgs, m)
	}
	return s, msgs, sc.Err()
}
