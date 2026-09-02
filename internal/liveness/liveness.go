// Package liveness defines the ONE server-side answer to "what is this
// workspace doing right now?": which sessions are running, queued, waiting on a
// human or on flow input, or idle-but-waiting for their workers — and how much
// spawn capacity is left. The runtime composes it (agent.Runtime.Liveness) from
// the registries that already exist (turn admission slots, tracked autonomous
// invokes, the api's chat-run probe, the spawn queue, durable asks, waiting
// flow runs, coordinator slots); every consumer — the sidebar chips, the
// network graph, the executions feed, the activity poll, the workspace live
// view — reads this snapshot instead of merging registries by hand
// (_Docs/77 R2).
//
// It is a projection, computed on demand; nothing writes to it and it holds no
// lock across callers.
package liveness

import "sort"

// State is a session's liveness class. Ordered by "how alive": a session
// appears once, under the most active state that applies.
type State string

const (
	// Running: a turn holds the session's admission slot or a tracked
	// autonomous invoke is executing in it.
	Running State = "running"
	// Queued: work is admitted but not yet running — a coordinator drain owed a
	// turn by a pending worker note.
	Queued State = "queued"
	// WaitingAsk: the turn is parked on a durable ask (a question / permission
	// prompt awaiting a human).
	WaitingAsk State = "waiting_ask"
	// WaitingInput: the session's flow run is durably suspended at an
	// await-input node.
	WaitingInput State = "waiting_input"
	// AwaitingWorkers: a coordinator that is idle itself while at least one of
	// its workers runs (the sidebar's "awaiting-workers" chip).
	AwaitingWorkers State = "awaiting_workers"
)

// Entry is one live session.
type Entry struct {
	SessionID string `json:"sessionId"`
	State     State  `json:"state"`
	// Reason names the source in a compact, greppable form:
	// "turn:user", "turn:worker", "run", "turn:chat" (api probe),
	// "ask:SAK3", "flow:RUN88", "workers:2", "coord:drain-pending".
	Reason string `json:"reason,omitempty"`
	// Since is when the entry entered its state (unix seconds); 0 when the
	// source does not track it.
	Since int64 `json:"since,omitempty"`
	// Waiting is the number of turns queued behind the running one (admission
	// queue depth), for the "sırada N" hint.
	Waiting int `json:"waiting,omitempty"`
}

// Capacity is the workspace's spawn/turn budget at snapshot time.
type Capacity struct {
	SpawnActive    int  `json:"spawnActive"`
	SpawnMax       int  `json:"spawnMax"`
	QueueDepth     int  `json:"queueDepth"`
	QueueMax       int  `json:"queueMax"`
	BusyTurns      int  `json:"busyTurns"`
	AutonomyPaused bool `json:"autonomyPaused"`
}

// Snapshot is the workspace's liveness picture at one instant.
type Snapshot struct {
	Entries  []Entry  `json:"entries"`
	Capacity Capacity `json:"capacity"`
	At       int64    `json:"at"`
}

// Get returns the entry for a session, ok=false when the session is idle.
func (s Snapshot) Get(sessionID string) (Entry, bool) {
	for _, e := range s.Entries {
		if e.SessionID == sessionID {
			return e, true
		}
	}
	return Entry{}, false
}

// Is reports whether the session is in the given state.
func (s Snapshot) Is(sessionID string, st State) bool {
	e, ok := s.Get(sessionID)
	return ok && e.State == st
}

// RunningSet is the legacy "which sessions are working right now" set: every
// session that is Running or AwaitingWorkers. It is what the previous
// runningSessionIDs merge produced (a coordinator waiting on its workers was
// promoted into the live scope), so existing consumers keep their semantics.
func (s Snapshot) RunningSet() map[string]bool {
	out := make(map[string]bool, len(s.Entries))
	for _, e := range s.Entries {
		if e.State == Running || e.State == AwaitingWorkers {
			out[e.SessionID] = true
		}
	}
	return out
}

// WithRunning returns a copy of the snapshot with the given sessions recorded as
// Running under reason (a source the composer could not see, e.g. the api's own
// streamed chat runs). Existing entries keep precedence per Builder.Add.
func (s Snapshot) WithRunning(ids []string, reason string) Snapshot {
	if len(ids) == 0 {
		return s
	}
	var b Builder
	for _, e := range s.Entries {
		b.Add(e)
	}
	for _, id := range ids {
		b.Add(Entry{SessionID: id, State: Running, Reason: reason})
	}
	return b.Snapshot(s.Capacity, s.At)
}

// Count returns how many sessions are in the given state.
func (s Snapshot) Count(st State) int {
	n := 0
	for _, e := range s.Entries {
		if e.State == st {
			n++
		}
	}
	return n
}

// Builder accumulates entries by session, keeping the MOST ACTIVE state when a
// session is reported by several sources (a running turn beats an ask parked
// from an earlier turn; a coordinator busy with its own turn is Running, not
// AwaitingWorkers).
type Builder struct {
	entries map[string]Entry
}

// rank orders states from most to least active.
func rank(st State) int {
	switch st {
	case Running:
		return 0
	case Queued:
		return 1
	case WaitingAsk:
		return 2
	case WaitingInput:
		return 3
	case AwaitingWorkers:
		return 4
	}
	return 5
}

// Add records an entry; a less active state never overrides a more active one
// already recorded for the same session, and an equally active later entry
// keeps the first (its Reason).
func (b *Builder) Add(e Entry) {
	if e.SessionID == "" {
		return
	}
	if b.entries == nil {
		b.entries = map[string]Entry{}
	}
	cur, ok := b.entries[e.SessionID]
	if !ok || rank(e.State) < rank(cur.State) {
		b.entries[e.SessionID] = e
		return
	}
	// Same-or-lower activity: keep the existing entry, but let a source that
	// knows the queue depth or start time fill those in.
	if rank(e.State) == rank(cur.State) {
		if cur.Since == 0 {
			cur.Since = e.Since
		}
		if cur.Waiting == 0 {
			cur.Waiting = e.Waiting
		}
		b.entries[e.SessionID] = cur
	}
}

// Has reports whether the session already has an entry.
func (b *Builder) Has(sessionID string) bool {
	_, ok := b.entries[sessionID]
	return ok
}

// Snapshot finalizes the builder into a sorted (by session id) snapshot.
func (b *Builder) Snapshot(cap Capacity, at int64) Snapshot {
	out := make([]Entry, 0, len(b.entries))
	for _, e := range b.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SessionID < out[j].SessionID })
	return Snapshot{Entries: out, Capacity: cap, At: at}
}
