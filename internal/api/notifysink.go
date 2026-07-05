package api

import (
	"context"

	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// notifySink adapts the workspace event bus into a tools.NotifySink AND a
// tools.NavigateSink for one chat turn. notify raises a workspace-scoped "agent"
// event (OS toast); focus_view raises a "navigate" event open windows apply at
// once. Both flow through the same SSE pipeline as task/flow/artifact events.
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

// Navigate publishes a "navigate" event that open windows apply immediately to
// drive the UI to the requested view/entity (no toast). Empty entity hints fall
// back to this turn's session/agent so "focus_view chat" focuses this session.
func (s notifySink) Navigate(_ context.Context, spec tools.NavigateSpec) error {
	if s.emit == nil {
		return nil
	}
	target := map[string]string{"view": spec.View}
	sid := spec.SessionID
	if sid == "" {
		sid = s.sessionID
	}
	if sid != "" {
		target["sessionId"] = sid
	}
	aid := spec.AgentID
	if aid == "" {
		aid = s.agentID
	}
	if aid != "" {
		target["agentId"] = aid
	}
	s.emit(events.Event{Type: "navigate", Level: "info", Target: target})
	return nil
}
