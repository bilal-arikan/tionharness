package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionharness/internal/events"
)

// Workspace event stream (_Docs/77 R3).
//
// GET /api/workspace/stream is the per-workspace counterpart of the per-session
// stream: one ordered, replayable log of structured lifecycle facts (sessions
// created/finished, flow runs, armed schedules, automation fires, trajectory
// revisions) for the workspace named by X-Workspace-Id. Same wire contract as
// handleSessionStream — hello/reset/hub frames, `since` cursor, `epoch` — so the
// frontend client is the same loop. Differences: a fresh subscriber replays
// nothing (it loads the current picture over REST), and the ring is larger.

// workspaceStreamPayload is the hub payload of every workspace event: the bus
// event's navigation hints plus its structured Data (shape per kind, see
// internal/agent/wsevents.go).
type workspaceStreamPayload struct {
	Target map[string]string `json:"target,omitempty"`
	Data   json.RawMessage   `json:"data,omitempty"`
	Level  string            `json:"level,omitempty"`
}

// bridgeWorkspaceEvent moves one workspace-stream bus event onto the hub's
// workspace scope. Events are stamped with their workspace by Runtime.publish;
// an unstamped one is dropped and logged rather than guessed into a workspace.
func (s *Server) bridgeWorkspaceEvent(e events.Event) {
	if s.hub == nil {
		return
	}
	if e.WorkspaceID == "" {
		if s.logger != nil {
			s.logger.Warn("workspace-stream event without workspace id; dropped", "type", e.Type)
		}
		return
	}
	payload := mustJSON(workspaceStreamPayload{Target: e.Target, Data: e.Data, Level: e.Level})
	s.hub.PublishWorkspace(e.WorkspaceID, events.WorkspaceStreamKind(e.Type), payload)
}

// handleWorkspaceStream serves the workspace's ordered event stream (SSE).
//
// Query params: since (last durable seq applied), epoch (the boot the cursor
// belongs to). Frames: hello {epoch, head, now} once; reset {head} when the
// cursor is unusable (epoch changed or fell out of the ring); hub → a
// sessionhub.Event whose Payload is a workspaceStreamPayload.
func (s *Server) handleWorkspaceStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	if s.hub == nil {
		writeError(w, http.StatusServiceUnavailable, "session hub unavailable")
		return
	}
	wsID := ws(r).ID

	since := int64(0)
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			since = n
		}
	}
	clientEpoch := r.URL.Query().Get("epoch")

	subID, ch, head := s.hub.SubscribeWorkspace(wsID)
	defer s.hub.UnsubscribeWorkspace(wsID, subID)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	writeFrame := func(name string, seq int64, data any) {
		b, _ := json.Marshal(data)
		if seq > 0 {
			fmt.Fprintf(w, "event: %s\nid: %d\ndata: %s\n\n", name, seq, b)
		} else {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
		}
		flusher.Flush()
	}

	writeFrame("hello", 0, map[string]any{
		"epoch": s.hub.Epoch(),
		"head":  head,
		"now":   time.Now().Unix(),
	})
	if clientEpoch != "" && clientEpoch != s.hub.Epoch() {
		writeFrame("reset", 0, map[string]any{"head": head})
	} else if replay, okReplay := s.hub.ReplayWorkspace(wsID, since); okReplay {
		for _, ev := range replay {
			writeFrame("hub", ev.Seq, ev)
		}
	} else {
		writeFrame("reset", 0, map[string]any{"head": head})
	}

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
		case ev, okCh := <-ch:
			if !okCh {
				return
			}
			writeFrame("hub", ev.Seq, ev)
		}
	}
}
