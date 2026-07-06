package api

// Session metadata + per-message feedback endpoints — the write side of the
// enriched session.jsonl fields (pin) and the user's quality rating on an
// assistant turn. Each mirrors the db setter and returns the updated value so the
// client can reconcile without a refetch.

import (
	"net/http"
	"strings"
)

type setPinReq struct {
	Pinned bool `json:"pinned"`
}

func (s *Server) handleSetSessionPin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[setPinReq](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	wsp := ws(r)
	database := wsp.DB
	if _, err := database.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	if err := database.SetSessionPinned(ctx, id, req.Pinned); writeDBError(w, err, "") {
		return
	}
	// Cross-window sync: a sibling window's sidebar re-sorts (pinned float to
	// top) the instant the pin flips here.
	emitSessionChange(wsp, id, "pin")
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "pinned": req.Pinned})
}

type setFeedbackReq struct {
	Rating int    `json:"rating"` // +1 | -1 | 0 (clear)
	Note   string `json:"note"`
}

func (s *Server) handleSetMessageFeedback(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	msgID := r.PathValue("msgId")
	req, ok := bindJSON[setFeedbackReq](w, r)
	if !ok {
		return
	}
	if req.Rating < -1 || req.Rating > 1 {
		writeError(w, http.StatusBadRequest, "rating must be -1, 0 or 1")
		return
	}
	ctx := r.Context()
	wsp := ws(r)
	database := wsp.DB
	if err := database.SetMessageFeedback(ctx, sessionID, msgID, req.Rating, strings.TrimSpace(req.Note)); writeDBError(w, err, "session or message not found") {
		return
	}
	// Cross-window sync: a sibling window viewing this session's transcript
	// re-fetches so the new thumb-up/down + note appear on the right message.
	// op="feedback" lets the listener reload only the transcript (not the
	// session list row's order/unread).
	emitSessionChange(wsp, sessionID, "feedback")
	writeJSON(w, http.StatusOK, map[string]any{"id": msgID, "rating": req.Rating, "note": strings.TrimSpace(req.Note)})
}
