package db

import (
	"context"
	"fmt"
)

// ---- Session asks (Durable Ask, MVP) ----
//
// The file-store analog of the flow await-input waiting model: one JSON file per
// suspended ask, a CAS claim guaranteeing a single answerer wins, and a waiting
// list for the timeout sweeper + boot restore.

func (d *DB) persistSessionAskLocked(a SessionAsk) error {
	return dbPersistLocked(d, d.sessionAsks, dirSessionAsks, a.ID, a)
}

// CreateSessionAsk parks a new prompt in the waiting state and returns the stored
// row (with its assigned id).
func (d *DB) CreateSessionAsk(ctx context.Context, a SessionAsk) (SessionAsk, error) {
	a.ID = d.nextID(idSessionAsk)
	a.CreatedAt = now()
	a.UpdatedAt = a.CreatedAt
	if a.Status == "" {
		a.Status = SessionAskWaiting
	}
	if a.Kind == "" {
		a.Kind = "ask"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return a, d.persistSessionAskLocked(a)
}

// GetSessionAsk loads an ask by id.
func (d *DB) GetSessionAsk(ctx context.Context, id string) (SessionAsk, error) {
	return dbGet(d, d.sessionAsks, id)
}

// ListWaitingSessionAsks returns still-waiting asks (for the sweeper + boot
// restore), oldest suspend first.
func (d *DB) ListWaitingSessionAsks(ctx context.Context) ([]SessionAsk, error) {
	return dbFilter(d, d.sessionAsks,
		func(a SessionAsk) bool { return a.Status == SessionAskWaiting },
		func(a, b SessionAsk) bool { return a.CreatedAt < b.CreatedAt }), nil
}

// ClaimSessionAsk atomically transitions an ask waiting → resolved and stamps the
// winning answer, so exactly one answerer wins the race (concurrent input from
// multiple windows/peers). Returns ErrNotFound if the ask is missing and a plain
// error if it is not currently waiting (already answered, cancelled, or timed out).
func (d *DB) ClaimSessionAsk(ctx context.Context, id, answer string) (SessionAsk, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.sessionAsks[id]
	if !ok {
		return SessionAsk{}, ErrNotFound
	}
	if a.Status != SessionAskWaiting {
		return SessionAsk{}, fmt.Errorf("session ask %s is not waiting (status %q)", id, a.Status)
	}
	a.Status = SessionAskResolved
	a.Answer = answer
	a.UpdatedAt = now()
	if err := d.persistSessionAskLocked(a); err != nil {
		return SessionAsk{}, err
	}
	return a, nil
}

// CloseSessionAsk CAS-transitions a still-waiting ask to a terminal non-answer
// status (cancelled/timeout). Idempotent: closing an already-terminal ask is a
// no-op that returns false (the caller lost the race to a real answer).
func (d *DB) CloseSessionAsk(ctx context.Context, id, status string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	a, ok := d.sessionAsks[id]
	if !ok {
		return false, ErrNotFound
	}
	if a.Status != SessionAskWaiting {
		return false, nil
	}
	a.Status = status
	a.UpdatedAt = now()
	return true, d.persistSessionAskLocked(a)
}

// DeleteSessionAsk removes an ask row and its file (GC of terminal asks).
func (d *DB) DeleteSessionAsk(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.sessionAsks[id]; !ok {
		return ErrNotFound
	}
	delete(d.sessionAsks, id)
	return removeFile(d.dir(dirSessionAsks, id+".json"))
}
