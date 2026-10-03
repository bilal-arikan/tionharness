package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bilal-arikan/tionharness/internal/events"
)

// eventsPingInterval keeps the SSE connection (and any proxies) alive between
// real events.
const eventsPingInterval = 25 * time.Second

// handleEvents streams autonomous runtime events (task and schedule outcomes)
// to the client over Server-Sent Events. The feed is global:
// every event carries its own workspaceId so the UI can deep-link on click.
//
//	notify → an events.Event (zero or more, as they occur)
//
// The stream stays open until the client disconnects.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	if s.bus == nil {
		writeError(w, http.StatusServiceUnavailable, "event bus unavailable")
		return
	}

	id, ch := s.bus.Subscribe()
	defer s.bus.Unsubscribe(id)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Open the stream so the client's EventSource fires `onopen` immediately.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ping := time.NewTicker(eventsPingInterval)
	defer ping.Stop()
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case e, ok := <-ch:
			if !ok {
				return
			}
			name, skip := sseEventName(e.Type)
			if skip {
				continue
			}
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
			flusher.Flush()
		}
	}
}

// sseEventName maps a bus event type to the SSE event name the global feed
// uses, or reports that the event must not ride this feed at all.
//
// Live turn-activity steps, per-node flow progress and log records ride the
// same feed under distinct names so the frontend routes them to the transcript
// renderer / run viewer / Logs tail instead of the notification/badge path
// (which reacts to `notify`). Workspace-stream types (events.WorkspaceStreamPrefix)
// are skipped: they are bridged onto the ordered per-workspace hub stream
// instead (handleWorkspaceStream), and putting them here would have every
// unknown type toast as a notification.
func sseEventName(typ string) (name string, skip bool) {
	if events.IsWorkspaceStream(typ) {
		return "", true
	}
	switch typ {
	case "session_step":
		return "step", false
	case "flow_node":
		return "flownode", false
	case "log":
		return "log", false
	case "process":
		// The process panel's refresh cue. Named apart from "notify" so an open
		// panel refetches while every other window ignores it (a spawned shell is
		// not news anyone needs a toast for).
		return "process", false
	}
	return "notify", false
}
