package db

import (
	"context"
	"fmt"
)

// store_agentmsg.go owns the AgentMessage delivery receipts (models_agentmsg.go).
// One file per receipt under <root>/agent-messages/, same shape as every other
// entity store in this package.

func (d *DB) persistAgentMessageLocked(m AgentMessage) error {
	return dbPersistLocked(d, d.agentMessages, dirAgentMsgs, m.ID, m)
}

// CreateAgentMessage records a delivery receipt and returns it with its id. The
// caller must have already decided Status (accepted/held/refused/dropped); this
// layer only validates that the status is one of the four.
func (d *DB) CreateAgentMessage(ctx context.Context, m AgentMessage) (AgentMessage, error) {
	switch m.Status {
	case DeliveryAccepted, DeliveryRefused, DeliveryDropped:
	default:
		return AgentMessage{}, fmt.Errorf("invalid delivery status %q", m.Status)
	}
	m.ID = d.nextID(idAgentMsg)
	m.CreatedAt = now()
	m.UpdatedAt = m.CreatedAt
	d.mu.Lock()
	defer d.mu.Unlock()
	return m, d.persistAgentMessageLocked(m)
}

// GetAgentMessage loads one receipt by id.
func (d *DB) GetAgentMessage(ctx context.Context, id string) (AgentMessage, error) {
	return dbGet(d, d.agentMessages, id)
}

// ListAgentMessagesFrom returns the receipts for messages a given sender sent,
// newest first — the sender's "what happened to my messages" view.
func (d *DB) ListAgentMessagesFrom(ctx context.Context, fromAgentID string) ([]AgentMessage, error) {
	if fromAgentID == "" {
		return nil, fmt.Errorf("fromAgentId is required")
	}
	return dbFilter(d, d.agentMessages,
		func(m AgentMessage) bool { return m.FromAgentID == fromAgentID },
		func(a, b AgentMessage) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// MarkAgentMessageDropped downgrades an already-recorded receipt to "dropped"
// with a reason. Used when a delivery was accepted by policy but then failed to
// be carried out, so the sender's receipt reflects reality instead of claiming
// an acceptance that never happened.
func (d *DB) MarkAgentMessageDropped(ctx context.Context, id, reason string) (AgentMessage, error) {
	if reason == "" {
		return AgentMessage{}, fmt.Errorf("a drop reason is required")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	m, ok := d.agentMessages[id]
	if !ok {
		return AgentMessage{}, ErrNotFound
	}
	m.Status = DeliveryDropped
	m.Reason = reason
	m.UpdatedAt = now()
	if err := d.persistAgentMessageLocked(m); err != nil {
		return AgentMessage{}, err
	}
	return m, nil
}
