package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

func now() int64 { return time.Now().Unix() }

func newID() string { return uuid.NewString() }

// ---- Agents ----

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
	_, err := d.ExecContext(ctx, `INSERT INTO agents
		(id, name, soul, identity, provider, model, capabilities, planning_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Soul, a.Identity, a.Provider, a.Model, a.Capabilities, a.PlanningMode, a.CreatedAt, a.UpdatedAt)
	return a, err
}

const agentColumns = `id, name, soul, identity, provider, model, capabilities, planning_mode,
	heartbeat_enabled, heartbeat_interval_sec, heartbeat_prompt,
	daily_call_limit, daily_token_limit, created_at, updated_at`

func scanAgent(s interface{ Scan(...any) error }, a *Agent) error {
	return s.Scan(&a.ID, &a.Name, &a.Soul, &a.Identity, &a.Provider, &a.Model,
		&a.Capabilities, &a.PlanningMode,
		&a.HeartbeatEnabled, &a.HeartbeatIntervalSec, &a.HeartbeatPrompt,
		&a.DailyCallLimit, &a.DailyTokenLimit,
		&a.CreatedAt, &a.UpdatedAt)
}

// GetAgent loads an agent by id.
func (d *DB) GetAgent(ctx context.Context, id string) (Agent, error) {
	var a Agent
	err := scanAgent(d.QueryRowContext(ctx, `SELECT `+agentColumns+` FROM agents WHERE id = ?`, id), &a)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// ListAgents returns all agents, newest first.
func (d *DB) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+agentColumns+` FROM agents ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Agent
	for rows.Next() {
		var a Agent
		if err := scanAgent(rows, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateHeartbeat sets the autonomous wake configuration for an agent.
func (d *DB) UpdateHeartbeat(ctx context.Context, agentID string, enabled bool, intervalSec int, prompt string) error {
	res, err := d.ExecContext(ctx, `UPDATE agents
		SET heartbeat_enabled = ?, heartbeat_interval_sec = ?, heartbeat_prompt = ?, updated_at = ?
		WHERE id = ?`, enabled, intervalSec, prompt, now(), agentID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetOrCreateHeartbeatSession returns the dedicated heartbeat session for an
// agent, creating it if absent. Heartbeat output is logged here for visibility.
func (d *DB) GetOrCreateHeartbeatSession(ctx context.Context, agentID string) (Session, error) {
	var s Session
	err := scanSession(d.QueryRowContext(ctx, `SELECT `+sessionColumns+`
		FROM sessions WHERE agent_id = ? AND kind = 'heartbeat' LIMIT 1`, agentID), &s)
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}
	return d.CreateSession(ctx, Session{AgentID: agentID, Kind: "heartbeat", Title: "♥ Heartbeat"})
}

// ---- Sessions ----

// CreateSession inserts a new session.
func (d *DB) CreateSession(ctx context.Context, s Session) (Session, error) {
	s.ID = newID()
	s.CreatedAt = now()
	s.UpdatedAt = s.CreatedAt
	if s.Kind == "" {
		s.Kind = "chat"
	}
	if s.State == "" {
		s.State = "active"
	}
	_, err := d.ExecContext(ctx, `INSERT INTO sessions
		(id, agent_id, kind, title, message_count, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.AgentID, s.Kind, s.Title, s.MessageCount, s.State, s.CreatedAt, s.UpdatedAt)
	return s, err
}

const sessionColumns = `id, agent_id, kind, title, message_count, state,
	summary, summary_msg_count, created_at, updated_at`

func scanSession(s interface{ Scan(...any) error }, sess *Session) error {
	return s.Scan(&sess.ID, &sess.AgentID, &sess.Kind, &sess.Title, &sess.MessageCount,
		&sess.State, &sess.Summary, &sess.SummaryMsgCount, &sess.CreatedAt, &sess.UpdatedAt)
}

// SetSessionSummary persists the rolling compaction summary for a session.
func (d *DB) SetSessionSummary(ctx context.Context, sessionID, summary string, msgCount int) error {
	res, err := d.ExecContext(ctx, `UPDATE sessions
		SET summary = ?, summary_msg_count = ? WHERE id = ?`, summary, msgCount, sessionID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// GetSession loads a session by id.
func (d *DB) GetSession(ctx context.Context, id string) (Session, error) {
	var s Session
	err := scanSession(d.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id), &s)
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}

// ListSessions returns sessions for an agent (or all if agentID is empty).
func (d *DB) ListSessions(ctx context.Context, agentID string) ([]Session, error) {
	query := `SELECT ` + sessionColumns + ` FROM sessions`
	args := []any{}
	if agentID != "" {
		query += ` WHERE agent_id = ?`
		args = append(args, agentID)
	}
	query += ` ORDER BY updated_at DESC`

	rows, err := d.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var s Session
		if err := scanSession(rows, &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- Messages ----

// AddMessage appends a message to a session and bumps the session counter.
func (d *DB) AddMessage(ctx context.Context, m Message) (Message, error) {
	m.ID = newID()
	m.CreatedAt = now()
	if m.ToolCalls == "" {
		m.ToolCalls = "[]"
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return m, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `INSERT INTO session_messages
		(id, session_id, role, text, tool_calls, reasoning_content, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.SessionID, m.Role, m.Text, m.ToolCalls, m.ReasoningContent, m.CreatedAt); err != nil {
		return m, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions
		SET message_count = message_count + 1, updated_at = ?
		WHERE id = ?`, m.CreatedAt, m.SessionID); err != nil {
		return m, err
	}
	return m, tx.Commit()
}

// ListMessages returns messages for a session in chronological order.
func (d *DB) ListMessages(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, session_id, role, text, tool_calls, reasoning_content, created_at
		FROM session_messages WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Text, &m.ToolCalls, &m.ReasoningContent, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
