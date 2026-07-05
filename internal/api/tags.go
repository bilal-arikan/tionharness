package api

import (
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/events"
)

// tagsReq is the body for every "set tags" endpoint: the full replacement tag
// set for a session, flow or schedule.
type tagsReq struct {
	Tags []string `json:"tags"`
}

// handleSetSessionTags replaces a session's tags. Tags organize sessions and
// drive tag-triggered automations. The same mutation an agent makes via
// set_session_tags. Emits a "session" change event for live UI refresh.
func (s *Server) handleSetSessionTags(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[tagsReq](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	wsp := ws(r)
	if _, err := wsp.DB.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	if err := wsp.DB.SetSessionTags(ctx, id, req.Tags); writeDBError(w, err, "") {
		return
	}
	// Refresh open session list + detail panel live (same event agent mutations use).
	wsp.Runtime.Emit(events.Event{
		Type:   "session",
		Level:  "info",
		Target: map[string]string{"sessionId": id},
	})
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "tags": req.Tags})
}

// handleSetFlowTags replaces a flow's tags (organizational only).
func (s *Server) handleSetFlowTags(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[tagsReq](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if _, err := ws(r).DB.GetFlow(ctx, id); writeDBError(w, err, "flow not found") {
		return
	}
	if err := ws(r).DB.SetFlowTags(ctx, id, req.Tags); writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "tags": req.Tags})
}

// handleSetScheduleTags replaces a schedule's tags (organizational only).
func (s *Server) handleSetScheduleTags(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[tagsReq](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if _, err := ws(r).DB.GetSchedule(ctx, id); writeDBError(w, err, "schedule not found") {
		return
	}
	if err := ws(r).DB.SetScheduleTags(ctx, id, req.Tags); writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "tags": req.Tags})
}
