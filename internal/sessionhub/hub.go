// Package sessionhub is the server-authoritative, per-session event log that
// every window watching a session subscribes to. It replaces the old
// owner/non-owner split (the owning window streamed its own SSE while other
// windows recovered via the inflight snapshot + polling) with a single model:
// each window is just a subscriber to one ordered, replayable event stream.
//
// Two classes of event ride the hub (see _Docs/58-QUEUE-SENKRON.md):
//
//   - DURABLE events (user_message, step, reply, interaction_open/resolved,
//     session_update, turn_done/error, queue_update) get a per-session
//     MONOTONIC seq, are kept in a bounded ring buffer, and can be replayed to a
//     reconnecting client from its cursor. A client that detects a seq gap
//     (missed frame) reconnects with since=lastSeq to gap-fill.
//   - EPHEMERAL events (delta, tool_delta, typing) carry seq 0, are NOT ringed,
//     and are best-effort live only — the full text lands on the durable reply.
//
// The seq counter is in-memory (per process). Across a restart it resets, so
// every stream carries an Epoch (minted once at boot): when a client presents a
// cursor from a previous epoch the endpoint tells it to reset (full resync via
// listMessages) instead of trusting a stale seq.
//
// SCOPE: every method is keyed by (workspaceID, sessionID), never by the session
// id alone. Session ids are allocated per workspace STORE ("SES1" exists in every
// workspace), so a bare-id hub would fan one workspace's live turn out to a window
// watching a same-numbered session in another workspace — and replay its in-flight
// tail on subscribe. See _Docs/58-QUEUE-SENKRON.md.
package sessionhub

import (
	"encoding/json"
	"sync"
	"time"
)

// Durable event kinds. Kept as string constants so the db/agent packages never
// need to import this one (payloads are opaque JSON, exactly like the bus).
const (
	KindUserMessage        = "user_message"
	KindStep               = "step"
	KindReply              = "reply"
	KindAgentStart         = "agent_start"
	KindInteractionOpen    = "interaction_open"
	KindInteractionResolve = "interaction_resolved"
	KindSessionUpdate      = "session_update"
	KindTurnDone           = "turn_done"
	KindTurnError          = "turn_error"
	KindQueueUpdate        = "queue_update"
	KindPresence           = "presence"

	// Ephemeral kinds (seq 0, not ringed).
	KindDelta     = "delta"
	KindToolDelta = "tool_delta"
	KindTombstone = "tombstone"
	KindTyping    = "typing"
)

// Event is one item on a session's stream. Payload is already-marshalled JSON
// (a TurnStep, a Message, an interaction descriptor, …) kept opaque here so this
// package imports neither agent nor db.
type Event struct {
	Seq       int64           `json:"seq"`
	SessionID string          `json:"sessionId"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Time      int64           `json:"time"`
}

// scopeKey namespaces a session id by its workspace. NUL is the separator because
// neither id can contain it, so no (ws, session) pair can alias another.
func scopeKey(wsID, sessionID string) string { return wsID + "\x00" + sessionID }

// sessionState is the per-session seq counter + ring buffer + subscriber set.
type sessionState struct {
	seq int64
	// committed is the seq of the last COMPLETED turn (reply / turn_done): every
	// event at or below it is already in the persisted transcript, so a FRESH
	// subscriber (which loads listMessages separately) only needs the in-flight
	// tail replayed (seq > committed) instead of the whole ring.
	committed int64
	ring      []Event // durable events only, capped at Hub.ringCap
	subs      map[int]chan Event
	// lastPublish stamps the newest event of ANY kind — ephemeral token deltas
	// included, because a turn streaming text is alive even though its deltas never
	// bump seq. This is the liveness signal the queue's idle watchdog reads, so it
	// deliberately does not care whether the event was retained.
	lastPublish time.Time
}

// Hub fans per-session events out to live subscribers and retains a bounded
// replay window per session. states is keyed by scopeKey(workspaceID, sessionID).
type Hub struct {
	mu      sync.Mutex
	epoch   string
	ringCap int
	states  map[string]*sessionState
	nextSub int
}

// New constructs a hub. epoch is a per-boot identity (e.g. a uuid) so clients
// can tell a restarted server from a live one; ringCap bounds the per-session
// replay window (0 → a sensible default).
func New(epoch string, ringCap int) *Hub {
	if ringCap <= 0 {
		ringCap = 512
	}
	return &Hub{
		epoch:   epoch,
		ringCap: ringCap,
		states:  make(map[string]*sessionState),
	}
}

// Epoch returns the per-boot identity stamped on every stream hello.
func (h *Hub) Epoch() string {
	if h == nil {
		return ""
	}
	return h.epoch
}

// stateLocked returns (creating if needed) the state for a scoped session. Caller
// holds h.mu.
func (h *Hub) stateLocked(key string) *sessionState {
	st := h.states[key]
	if st == nil {
		st = &sessionState{subs: make(map[int]chan Event)}
		h.states[key] = st
	}
	return st
}

// Publish records + broadcasts one event to the (workspace, session) scope.
// Durable events (ephemeral=false) get the next per-session seq and are appended
// to the ring; ephemeral events keep seq 0 and are only fanned out live. Returns
// the assigned seq (0 for ephemeral). Safe on a nil hub (no-op) so zero-value
// wiring never panics. An unresolved workspace (empty wsID) publishes NOTHING
// rather than falling into a shared scope — that is what leaked across workspaces.
// Event.SessionID stays the bare id: the scope is server-side, the wire contract
// with the client is unchanged.
func (h *Hub) Publish(wsID, sessionID, kind string, payload json.RawMessage, ephemeral bool) int64 {
	if h == nil || wsID == "" || sessionID == "" {
		return 0
	}
	ev := Event{SessionID: sessionID, Kind: kind, Payload: payload, Time: time.Now().Unix()}
	return h.publish(scopeKey(wsID, sessionID), ev, ephemeral, h.ringCap, false)
}

// publish is the scope-agnostic core behind Publish and PublishWorkspace. key is
// the already-namespaced scope; ringCap bounds this scope's retained window;
// autoCommit marks the appended event as committed at once (the workspace
// stream has no turn boundary, so nothing is ever "in flight" there).
func (h *Hub) publish(key string, ev Event, ephemeral bool, ringCap int, autoCommit bool) int64 {
	h.mu.Lock()
	st := h.stateLocked(key)
	st.lastPublish = time.Now()
	if !ephemeral {
		st.seq++
		ev.Seq = st.seq
		st.ring = append(st.ring, ev)
		if autoCommit {
			st.committed = st.seq
		}
		if len(st.ring) > ringCap {
			// Trim the oldest to bound the window, but NEVER evict an uncommitted
			// (in-flight) event — a fresh subscriber replays exactly those. So only
			// committed events (seq <= committed) are dropped; a single turn emitting
			// more than ringCap events keeps them all until it commits.
			excess := len(st.ring) - ringCap
			drop := 0
			for drop < excess && st.ring[drop].Seq <= st.committed {
				drop++
			}
			if drop > 0 {
				st.ring = st.ring[drop:]
			}
		}
	}
	// Non-blocking fan-out: a slow subscriber drops the frame rather than stalling
	// the publisher. The client detects the resulting seq gap and reconnects with
	// since=lastSeq to gap-fill from the ring.
	for _, ch := range st.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	h.mu.Unlock()
	return ev.Seq
}

// Subscribe registers a listener for a session and returns its id, receive
// channel, and the current head seq (so the caller can report where the live
// edge is). Call Unsubscribe with the id when done.
func (h *Hub) Subscribe(wsID, sessionID string) (int, <-chan Event, int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.stateLocked(scopeKey(wsID, sessionID))
	id := h.nextSub
	h.nextSub++
	ch := make(chan Event, 256)
	st.subs[id] = ch
	return id, ch, st.seq
}

// Unsubscribe removes a listener and closes its channel.
func (h *Hub) Unsubscribe(wsID, sessionID string, id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.states[scopeKey(wsID, sessionID)]
	if st == nil {
		return
	}
	if ch, ok := st.subs[id]; ok {
		delete(st.subs, id)
		close(ch)
	}
}

// Drop discards a session's state entirely: its seq counter, its ring buffer and
// its subscriber set. Without it the states map only ever grows — every session
// that ever published or was watched keeps a ring (up to ringCap events) for the
// life of the process, including the short-lived schedule/spawn/worker sessions
// an autonomous workspace creates and discards constantly.
//
// Live subscriber channels are closed, which is the right signal for a session
// that no longer exists: the stream handler reads with the two-value form and
// ends its loop on a closed channel. A later Unsubscribe for a dropped session
// finds no state and returns without double-closing.
//
// Call this only after the session is gone (or going) — a Publish or Subscribe
// afterwards silently recreates the state from scratch.
func (h *Hub) Drop(wsID, sessionID string) {
	if h == nil || wsID == "" || sessionID == "" {
		return
	}
	key := scopeKey(wsID, sessionID)
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.states[key]
	if st == nil {
		return
	}
	for id, ch := range st.subs {
		delete(st.subs, id)
		close(ch)
	}
	delete(h.states, key)
}

// Replay returns the durable events a (re)connecting client is missing.
//
//   - FRESH subscribe (since <= 0): the client has just loaded the persisted
//     transcript via listMessages, so it only needs the IN-FLIGHT tail — events
//     after the last committed turn (seq > committed). Completed turns are never
//     replayed, so opening a busy session no longer re-runs every past turn.
//   - RECONNECT (since > 0): gap-fill from the cursor (seq > since). ok=false when
//     the cursor fell before the retained ring window — the caller then tells the
//     client to reset (full resync) instead of silently losing a gap.
func (h *Hub) Replay(wsID, sessionID string, since int64) ([]Event, bool) {
	if h == nil {
		return nil, true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.states[scopeKey(wsID, sessionID)]
	if st == nil {
		return nil, since <= 0
	}
	fresh := since <= 0
	floor := since
	if fresh {
		floor = st.committed
	}
	if len(st.ring) == 0 {
		// Nothing retained: fine for a fresh subscribe (nothing in flight), a reset
		// for a positive cursor that expected history the ring no longer holds.
		return nil, fresh
	}
	oldest := st.ring[0].Seq
	if !fresh && since+1 < oldest {
		return nil, false // reconnect gap fell out of the ring → reset
	}
	out := make([]Event, 0, len(st.ring))
	for _, e := range st.ring {
		if e.Seq > floor {
			out = append(out, e)
		}
	}
	return out, true
}

// Commit marks a session's turn as completed: every retained event up to the
// current head is now in the persisted transcript, so a future fresh subscriber
// skips replaying it (only the next in-flight tail is replayed). Called by the
// turn runner after each reply and at turn end.
func (h *Hub) Commit(wsID, sessionID string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if st := h.states[scopeKey(wsID, sessionID)]; st != nil {
		st.committed = st.seq
	}
}

// Head returns the current head (latest durable seq) for a session, 0 if none.
func (h *Hub) Head(wsID, sessionID string) int64 {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if st := h.states[scopeKey(wsID, sessionID)]; st != nil {
		return st.seq
	}
	return 0
}

// LastActivity returns when the session last published an event of any kind
// (durable step OR ephemeral token delta), and whether such an event exists.
// It is the liveness probe behind the queue's idle watchdog: a turn that is
// still emitting is working, however long it has been running, while one that
// has gone completely silent is the wedge the watchdog exists to break.
func (h *Hub) LastActivity(wsID, sessionID string) (time.Time, bool) {
	if h == nil {
		return time.Time{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if st := h.states[scopeKey(wsID, sessionID)]; st != nil && !st.lastPublish.IsZero() {
		return st.lastPublish, true
	}
	return time.Time{}, false
}

// SubscriberCount returns how many live subscribers a session currently has —
// the raw signal behind presence ("this session is open in N windows").
func (h *Hub) SubscriberCount(wsID, sessionID string) int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if st := h.states[scopeKey(wsID, sessionID)]; st != nil {
		return len(st.subs)
	}
	return 0
}
