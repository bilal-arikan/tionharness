package api

import (
	"net/http"
)

func (s *Server) handleRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ws(r).Runtime.Status())
}

type setHeartbeatReq struct {
	Enabled     bool   `json:"enabled"`
	IntervalSec int    `json:"intervalSec"`
	Prompt      string `json:"prompt"`
}

// handleSetHeartbeat persists heartbeat config and starts/stops the worker.
func (s *Server) handleSetHeartbeat(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	wsp := ws(r)

	var req setHeartbeatReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if _, err := wsp.DB.GetAgent(r.Context(), agentID); err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	if err := wsp.DB.UpdateHeartbeat(r.Context(), agentID, req.Enabled, req.IntervalSec, req.Prompt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if req.Enabled {
		_ = wsp.Runtime.Start(agentID, req.IntervalSec)
	} else {
		wsp.Runtime.Stop(agentID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agentId": agentID,
		"enabled": req.Enabled,
		"status":  wsp.Runtime.Status(),
	})
}

// handleWake triggers an immediate tick for a running agent.
func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if err := ws(r).Runtime.Wake(agentID); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"agentId": agentID, "result": "woken"})
}
