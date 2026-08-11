// Package turnqueue is the per-session TURN ADMISSION queue: the one place that
// decides which turn runs on a session and in what order.
//
// A session can be driven from many directions — a message the user queued, a
// coordinator auto-turn triggered by a worker's <task-notification>, a scheduler
// wake, a peer inbox delivery, a spawn opening prompt, a /compact command. Every
// one of them runs a turn against the SAME transcript, so exactly one may run at a
// time. Before this package that ordering lived in two disconnected places: a
// durable FIFO in internal/api (visible in the UI, cancellable) and an anonymous
// mutex in internal/agent. A message could leave the first and wait, invisible and
// uncancellable, in the second — the "queued message vanished, then appeared when a
// worker replied" bug (_Docs/58).
//
// Now there is one queue and both layers hold it:
//
//	internal/api  → durable staging (inbox.json: what to run, survives a crash)
//	turnqueue     → admission order (who runs next, and what is waiting behind)
//
// The queue is deliberately NOT durable: it orders turns that are already in
// flight, and its entries are reconstructed at boot by the callers that own
// durability (the send-queue's inbox.json, RecoverOrphanedTurns for autonomous
// turns). What it adds over a mutex is order (FIFO baton passing, no wake-up
// scramble), cancellability (a caller that gives up never wedges the slot), and
// observability (Snapshot → the UI can show everything waiting on a session, not
// just the user's own messages).
package turnqueue

import (
	"context"
	"sync"
)

// Kind labels who wants the turn. It exists for observability — the queue itself
// is strictly first-come-first-served, with no priority by kind (a priority scheme
// is how the coordinator starved the user in the first place).
type Kind string

const (
	KindUser        Kind = "user"        // a message the user sent/queued
	KindCommand     Kind = "command"     // a slash command (/compact, /handoff)
	KindCoordinator Kind = "coordinator" // an auto-turn from a worker notification
	KindWorker      Kind = "worker"      // a worker session running its task
	KindWake        Kind = "wake"        // scheduler wake / scheduled prompt
	KindPeer        Kind = "peer"        // peer inbox delivery (agent-to-agent)
	KindSpawn       Kind = "spawn"       // spawned session's opening turn
	KindAutomation  Kind = "automation"  // automation delivery
)

// Entry is one turn holding or waiting for a session's slot.
type Entry struct {
	Kind  Kind   `json:"kind"`
	Label string `json:"label,omitempty"`
	// Since is the unix second the entry started running (Running) or started
	// waiting (Waiting) — the UI shows "N sn bekliyor".
	Since int64 `json:"since"`
}

// Snapshot is a session's admission state for the UI / views: what holds the slot
// and what is queued behind it, oldest first.
type Snapshot struct {
	Running *Entry  `json:"running,omitempty"`
	Waiting []Entry `json:"waiting,omitempty"`
}

// waiter is one blocked caller. ch is closed when the baton is handed to it
// (granted); dropped marks a caller that gave up (ctx cancelled) so the handoff
// skips it.
type waiter struct {
	entry   Entry
	ch      chan struct{}
	granted bool
	dropped bool
}

type slot struct {
	running *Entry
	waitq   []*waiter
}

// Queue owns the admission slots for one workspace's sessions.
type Queue struct {
	mu       sync.Mutex
	sessions map[string]*slot
	// now returns the current unix second; swappable so tests get stable stamps.
	now func() int64
	// observe is called (outside the lock) whenever a session's admission state
	// changes, so the API layer can push a fresh queue view to every window.
	observe func(sessionID string)
}

// New builds an empty queue. now may be nil (defaults to wall clock).
func New(now func() int64) *Queue {
	if now == nil {
		panic("turnqueue: now func is required")
	}
	return &Queue{sessions: make(map[string]*slot), now: now}
}

// Observe registers the change callback (see Queue.observe). Call once at wiring
// time, before any Acquire.
func (q *Queue) Observe(fn func(sessionID string)) { q.observe = fn }

func (q *Queue) notify(sessionID string) {
	if q.observe != nil {
		q.observe(sessionID)
	}
}

// slotFor returns (creating if needed) a session's slot. Callers must hold mu.
func (q *Queue) slotFor(sessionID string) *slot {
	s := q.sessions[sessionID]
	if s == nil {
		s = &slot{}
		q.sessions[sessionID] = s
	}
	return s
}

// Acquire blocks until this caller owns the session's turn slot, then returns the
// release func (idempotent — deferring it is always safe). It joins the FIFO even
// when the slot is free but someone is already queued: taking a free slot ahead of
// an older waiter is exactly the barging this queue exists to prevent. On ctx
// cancellation it returns a non-nil error and a no-op release, having removed
// itself from the queue.
func (q *Queue) Acquire(ctx context.Context, sessionID string, kind Kind, label string) (release func(), err error) {
	entry := Entry{Kind: kind, Label: label, Since: q.now()}

	q.mu.Lock()
	s := q.slotFor(sessionID)
	if s.running == nil && liveWaiters(s) == 0 {
		s.waitq = nil
		e := entry
		s.running = &e
		q.mu.Unlock()
		q.notify(sessionID)
		return q.releaseFunc(sessionID), nil
	}
	w := &waiter{entry: entry, ch: make(chan struct{})}
	s.waitq = append(s.waitq, w)
	q.mu.Unlock()
	q.notify(sessionID)

	select {
	case <-w.ch:
		// The baton (and s.running) is already ours — handoffLocked installed it.
		return q.releaseFunc(sessionID), nil
	case <-ctx.Done():
		q.mu.Lock()
		if w.granted {
			// The baton arrived in the same instant we gave up: pass it straight on,
			// or the slot would read "busy" with nobody holding it — a permanent wedge
			// for every later turn on this session.
			s := q.slotFor(sessionID)
			s.running = nil
			q.handoffLocked(s)
			q.mu.Unlock()
			q.notify(sessionID)
			return func() {}, ctx.Err()
		}
		w.dropped = true
		q.mu.Unlock()
		q.notify(sessionID)
		return func() {}, ctx.Err()
	}
}

// releaseFunc builds the idempotent release for a held slot.
func (q *Queue) releaseFunc(sessionID string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			q.mu.Lock()
			s := q.slotFor(sessionID)
			s.running = nil
			q.handoffLocked(s)
			q.mu.Unlock()
			q.notify(sessionID)
		})
	}
}

// handoffLocked passes the slot to the oldest live waiter, installing it as the
// running entry. When nobody is waiting the slot is left free. Callers must hold mu
// AND must have cleared s.running first. This is the ONLY way the slot changes
// hands, so a waiter can never be stranded behind a slot that reads free.
func (q *Queue) handoffLocked(s *slot) {
	for len(s.waitq) > 0 {
		w := s.waitq[0]
		s.waitq = s.waitq[1:]
		if w.dropped {
			continue
		}
		w.granted = true
		e := w.entry
		e.Since = q.now() // it starts RUNNING now; Since stops meaning "waiting since"
		s.running = &e
		close(w.ch)
		return
	}
}

func liveWaiters(s *slot) int {
	n := 0
	for _, w := range s.waitq {
		if !w.dropped {
			n++
		}
	}
	return n
}

// Busy reports whether a turn currently holds the session's slot.
func (q *Queue) Busy(sessionID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := q.sessions[sessionID]
	return s != nil && s.running != nil
}

// Waiting counts the turns queued behind the running one.
func (q *Queue) Waiting(sessionID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := q.sessions[sessionID]
	if s == nil {
		return 0
	}
	return liveWaiters(s)
}

// Snapshot returns the session's admission state for the UI, oldest waiter first.
func (q *Queue) Snapshot(sessionID string) Snapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := q.sessions[sessionID]
	if s == nil {
		return Snapshot{}
	}
	var snap Snapshot
	if s.running != nil {
		e := *s.running
		snap.Running = &e
	}
	for _, w := range s.waitq {
		if !w.dropped {
			snap.Waiting = append(snap.Waiting, w.entry)
		}
	}
	return snap
}

// Forget drops a session's slot state (session deleted, or a flow coordinator node
// finished). A no-op while a turn is running or queued — dropping live state would
// strand its waiters.
func (q *Queue) Forget(sessionID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := q.sessions[sessionID]
	if s == nil || s.running != nil || liveWaiters(s) > 0 {
		return
	}
	delete(q.sessions, sessionID)
}
