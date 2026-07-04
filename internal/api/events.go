package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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
			b, _ := json.Marshal(e)
			// Live turn-activity steps ride the same feed but under a distinct SSE
			// event name so the frontend routes them to the transcript renderer
			// instead of the notification/badge path (which reacts to `notify`).
			name := "notify"
			if e.Type == "session_step" {
				name = "step"
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
			flusher.Flush()
		}
	}
}
