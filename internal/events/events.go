// Package events provides a process-wide pub/sub bus for autonomous runtime
// notifications (task/schedule outcomes). The API
// layer streams these to the frontend over SSE so the UI can raise desktop
// notifications that deep-link to the relevant view.
package events

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is a single autonomous notification. Target carries navigation hints
// the frontend uses to deep-link on click (keys: view, sessionId, taskId,
// agentId). Level is one of "info", "success", "error".
type Event struct {
	Type          string            `json:"type"`
	Level         string            `json:"level"`
	WorkspaceID   string            `json:"workspaceId"`
	WorkspaceName string            `json:"workspaceName,omitempty"`
	Title         string            `json:"title"`
	Body          string            `json:"body"`
	Target        map[string]string `json:"target,omitempty"`
	Time          int64             `json:"time"`
	// Step carries an already-marshalled agent.TurnStep for a "session_step"
	// event: the live activity trace of an in-progress turn, broadcast so any
	// window viewing that session (Target["sessionId"]) — or an autonomous turn
	// with no per-request SSE of its own — can render thinking/tool steps as they
	// happen. Opaque JSON here (the events package never imports agent), exactly
	// like db.Message.Steps. Empty for ordinary notifications.
	Step json.RawMessage `json:"step,omitempty"`
	// Msg carries an already-marshalled db.Message for a "session_user_message"
	// event: a user-role message the runtime INJECTED mid-autonomous-flow (a worker
	// task-notification, a send_to_worker prompt, a coordination status/guard note).
	// Bridged to the session hub as KindUserMessage so a window watching the session
	// renders the note live and in order — assistant replies already bridge via
	// publishAutonomousReply, but these injected user messages did not, so the reply
	// appeared to answer a message the window never saw (_Docs/58). Opaque JSON here
	// (this package never imports db). Empty for everything else.
	Msg json.RawMessage `json:"msg,omitempty"`
	// Node carries an already-marshalled orchestration.NodeEvent for a
	// "flow_node" event: one flow node's lifecycle (start/done/error + output),
	// broadcast keyed by the flow run id (Target["flowRunId"]) so any window
	// viewing that run renders per-node progress live instead of polling. Opaque
	// JSON here (this package imports neither agent nor orchestration). Empty for
	// everything else.
	Node json.RawMessage `json:"node,omitempty"`
	// Log carries an already-marshalled logbuf.Entry for a "log" event: one
	// captured application log record, broadcast so the Logs screen can tail
	// live over SSE instead of polling. Opaque JSON here (this package never
	// imports logbuf). Empty for everything else.
	Log json.RawMessage `json:"log,omitempty"`
	// Data carries the structured payload of a WORKSPACE-STREAM event (a type
	// with the WorkspaceStreamPrefix, see types.go): a session lifecycle change,
	// a trajectory revision, a flow-run status, an armed schedule, an automation
	// fire. Such events never reach the fire-and-forget /api/events feed; the API
	// bridges them onto the ordered, replayable per-workspace hub stream
	// (GET /api/workspace/stream, _Docs/77 R3). Opaque JSON here so this package
	// keeps importing nothing. Empty for every other event.
	Data json.RawMessage `json:"data,omitempty"`
}

// Bus fans out events to every live subscriber. Sends are non-blocking: a slow
// subscriber drops events rather than stalling autonomous publishers.
type Bus struct {
	mu   sync.Mutex
	subs map[int]chan Event
	next int
}

// NewBus constructs an empty bus.
func NewBus() *Bus {
	return &Bus{subs: make(map[int]chan Event)}
}

// Subscribe registers a new listener and returns its id plus the receive
// channel. Call Unsubscribe with the id when done.
func (b *Bus) Subscribe() (int, <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan Event, 32)
	b.subs[id] = ch
	return id, ch
}

// Unsubscribe removes a listener and closes its channel.
func (b *Bus) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(ch)
	}
}

// Publish delivers an event to all subscribers. It stamps Time when unset and
// is safe to call on a nil bus (no-op) so zero-value wiring never panics.
func (b *Bus) Publish(e Event) {
	if b == nil {
		return
	}
	if e.Time == 0 {
		e.Time = time.Now().Unix()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default: // slow subscriber: drop rather than block the publisher
		}
	}
}
