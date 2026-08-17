package db

import (
	"context"
	"errors"
)

// This file holds the NARROW transcript readers.
//
// ListMessages copies a session's ENTIRE message slice on every call, and most
// callers were paying that for a question far smaller than "give me everything":
// the last message, the last handful, one message by id, or a scan that keeps
// almost nothing. A long-running session is megabytes of Steps/ToolCalls JSON,
// so the copy is the dominant cost of endpoints that then throw ~all of it away.
//
// These readers answer those questions without the full copy. They are also the
// seam the lazy/paged store needs later (see the Madde 5 plan): once transcripts
// load on demand, THESE signatures are the ones that can be served from a file
// tail without materialising the whole session — ListMessages cannot.

// ErrMessageNotFound is returned when a session exists but holds no message with
// the requested id. Deliberately distinct from ErrNotFound (unknown session) so a
// caller can tell "no such conversation" from "no such message in it" instead of
// collapsing both into one 404.
var ErrMessageNotFound = errors.New("message not found")

// ListMessagesTail returns the last n messages of a session in chronological
// order, plus the index of the first returned message in the full transcript
// (0 when the whole transcript fits in n, so the caller can tell a tail from a
// complete listing). n <= 0 returns the whole transcript, matching ListMessages.
//
// Unlike ListMessages, an unknown session is ErrNotFound rather than an empty
// slice: a caller asking for a tail has a specific session in mind, and silently
// handing back "no messages" is how a wrong session id reads as an empty chat.
func (d *DB) ListMessagesTail(ctx context.Context, sessionID string, n int) ([]Message, int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.sessions[sessionID]; !ok {
		return nil, 0, ErrNotFound
	}
	msgs := d.messages[sessionID]
	from := 0
	if n > 0 && len(msgs) > n {
		from = len(msgs) - n
	}
	tail := msgs[from:]
	out := make([]Message, len(tail))
	copy(out, tail)
	return out, from, nil
}

// LastMessage returns a session's final message. ok is false when the session
// exists but has no messages yet; an unknown session is ErrNotFound.
func (d *DB) LastMessage(ctx context.Context, sessionID string) (Message, bool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.sessions[sessionID]; !ok {
		return Message{}, false, ErrNotFound
	}
	msgs := d.messages[sessionID]
	if len(msgs) == 0 {
		return Message{}, false, nil
	}
	return msgs[len(msgs)-1], true, nil
}

// FindMessage returns one message by id. ErrNotFound when the session is
// unknown, ErrMessageNotFound when the session has no such message.
func (d *DB) FindMessage(ctx context.Context, sessionID, messageID string) (Message, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.sessions[sessionID]; !ok {
		return Message{}, ErrNotFound
	}
	msgs := d.messages[sessionID]
	for i := range msgs {
		if msgs[i].ID == messageID {
			return msgs[i], nil
		}
	}
	return Message{}, ErrMessageNotFound
}

// StreamMessages walks a session's transcript oldest-first, handing each message
// to fn. Returning false from fn stops the walk early. This is the reader for
// "scan everything, keep almost nothing" callers: it never allocates a copy of
// the transcript, so a scan that extracts a handful of steps costs the scan and
// nothing else.
//
// fn runs WHILE THE STORE READ LOCK IS HELD, because the store mutates message
// records in place (SetMessageFeedback) and an unlocked walk would race with it.
// Two obligations follow, and neither is optional:
//   - fn must NOT call back into the store. Go's RWMutex is not reentrant, so a
//     nested write (or a read with a writer already queued) deadlocks.
//   - fn must be cheap. It is on the critical path of every message append in
//     this workspace.
//
// The Message handed to fn is a copy, so retaining it after the callback is safe.
func (d *DB) StreamMessages(ctx context.Context, sessionID string, fn func(Message) bool) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if _, ok := d.sessions[sessionID]; !ok {
		return ErrNotFound
	}
	for _, m := range d.messages[sessionID] {
		if !fn(m) {
			return nil
		}
	}
	return nil
}
