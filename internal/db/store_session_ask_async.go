package db

import (
	"cmp"
	"context"
	"slices"
)

// TakeAsyncSessionAnswers claims undelivered replies once across concurrent
// native/MCP consumers and turn finalization. An empty agentID selects all agents.
func (d *DB) TakeAsyncSessionAnswers(ctx context.Context, sessionID, agentID string) ([]SessionAsk, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var pending []SessionAsk
	for _, a := range d.sessionAsks {
		if a.Async && a.SessionID == sessionID && (agentID == "" || a.AgentID == agentID) &&
			a.Status == SessionAskResolved && !a.AnswerDelivered {
			pending = append(pending, a)
		}
	}
	slices.SortFunc(pending, func(a, b SessionAsk) int {
		if c := cmp.Compare(a.UpdatedAt, b.UpdatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	var claimed []SessionAsk
	for _, a := range pending {
		a.AnswerDelivered = true
		if err := d.persistSessionAskLocked(a); err != nil {
			return claimed, err
		}
		claimed = append(claimed, a)
	}
	return claimed, nil
}

// CancelAsyncSessionAsks also closes replies that have not reached the model
// yet, so stopping a turn cannot immediately restart it through answer delivery.
func (d *DB) CancelAsyncSessionAsks(ctx context.Context, sessionID string) ([]SessionAsk, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var cancelled []SessionAsk
	for _, a := range d.sessionAsks {
		if !a.Async || a.SessionID != sessionID || a.AnswerDelivered ||
			(a.Status != SessionAskWaiting && a.Status != SessionAskResolved) {
			continue
		}
		a.Status = SessionAskCancelled
		a.UpdatedAt = now()
		if err := d.persistSessionAskLocked(a); err != nil {
			return cancelled, err
		}
		cancelled = append(cancelled, a)
	}
	return cancelled, nil
}
