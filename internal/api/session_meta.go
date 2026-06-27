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
	var req setPinReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	ctx := r.Context()
	database := ws(r).DB
	if _, err := database.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	if err := database.SetSessionPinned(ctx, id, req.Pinned); writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "pinned": req.Pinned})
}

type setFeedbackReq struct {
	Rating int    `json:"rating"` // +1 | -1 | 0 (clear)
	Note   string `json:"note"`
}

func (s *Server) handleSetMessageFeedback(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	msgID := r.PathValue("msgId")
	var req setFeedbackReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Rating < -1 || req.Rating > 1 {
		writeError(w, http.StatusBadRequest, "rating must be -1, 0 or 1")
		return
	}
	ctx := r.Context()
	database := ws(r).DB
	if err := database.SetMessageFeedback(ctx, sessionID, msgID, req.Rating, strings.TrimSpace(req.Note)); writeDBError(w, err, "session or message not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": msgID, "rating": req.Rating, "note": strings.TrimSpace(req.Note)})
}
