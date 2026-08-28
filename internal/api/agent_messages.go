package api

import (
	"net/http"
	"strings"
)

// agent_messages.go exposes the inbound-message HOLD queue: when a recipient's
// inbound policy is "hold", a message addressed at it is parked with a durable
// receipt (db.AgentMessage) instead of being delivered. These endpoints are how
// a human sees what is waiting and either releases it (delivery happens now) or
// refuses it (the sender's receipt records why). See internal/agent/inbound.go.

// handleListHeldAgentMessages lists messages parked for approval, oldest first.
// Optional ?agentId= narrows the list to one recipient.
func (s *Server) handleListHeldAgentMessages(w http.ResponseWriter, r *http.Request) {
	agentID := strings.TrimSpace(r.URL.Query().Get("agentId"))
	list, err := ws(r).Runtime.ListHeldMessages(r.Context(), agentID)
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleReleaseAgentMessage approves a held message and delivers it now. The
// underlying CAS makes a double release impossible, and a delivery that fails
// after approval downgrades the receipt to "dropped" rather than reporting a
// success that did not happen.
func (s *Server) handleReleaseAgentMessage(w http.ResponseWriter, r *http.Request) {
	receipt, err := ws(r).Runtime.ReleaseHeldMessage(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "held message not found") {
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

// handleRefuseAgentMessage rejects a held message. The body is kept on the
// receipt so the refusal stays auditable.
func (s *Server) handleRefuseAgentMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason string `json:"reason"`
	}
	// A missing/!empty body is fine: the reason is optional and defaults.
	_ = decodeJSON(r, &req)
	receipt, err := ws(r).Runtime.RefuseHeldMessage(r.Context(), r.PathValue("id"), req.Reason)
	if writeDBError(w, err, "held message not found") {
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}
