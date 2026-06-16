// Package events provides a process-wide pub/sub bus for autonomous runtime
// notifications (agent heartbeat failures, task/schedule outcomes). The API
// layer streams these to the frontend over SSE so the UI can raise desktop
// notifications that deep-link to the relevant view.
package events

import (
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
