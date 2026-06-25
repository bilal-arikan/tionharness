package api

import (
	"context"

	"github.com/bilal-arikan/swarmgo/internal/events"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// notifySink adapts the workspace event bus into a tools.NotifySink for one chat
// turn. An agent-raised notify call becomes a workspace-scoped "agent" event that
// flows through the same SSE pipeline as task/flow/artifact notifications, so open
// windows (per device prefs) raise an OS toast that deep-links to the session.
type notifySink struct {
	sessionID string
	agentID   string
	// emit publishes the workspace-scoped event. Optional (nil-safe) — a turn with
	// no runtime emitter simply doesn't surface the toast.
	emit func(events.Event)
}

// newNotifySink builds a sink bound to the given session/agent. emit may be nil
// (the notify call then becomes a silent no-op).
func newNotifySink(sessionID, agentID string, emit func(events.Event)) notifySink {
	return notifySink{sessionID: sessionID, agentID: agentID, emit: emit}
}

// Notify publishes the agent notification as an "agent" event (the type reserved
// for general agent notifications in the frontend NOTIFY_TYPES, mutable per
// device). Best-effort and nil-safe.
func (s notifySink) Notify(_ context.Context, spec tools.NotifySpec) error {
	if s.emit == nil {
		return nil
	}
	level := spec.Level
	if level == "" {
		level = "info"
	}
	s.emit(events.Event{
		Type:  "agent",
		Level: level,
		Title: spec.Title,
		Body:  spec.Body,
		Target: map[string]string{
			"view":      "chat",
			"sessionId": s.sessionID,
			"agentId":   s.agentID,
		},
	})
	return nil
}
