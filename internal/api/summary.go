package api

import (
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

type summaryReq struct {
	Kind string `json:"kind"`
}

// summaryHeaders gives each summary kind a self-explanatory chat header so the
// resulting assistant message reads clearly on its own.
var summaryHeaders = map[string]string{
	agent.SummaryMemory: "🧠 **Hafıza özeti**",
	agent.SummaryBoard:  "🗂 **Görev panosu özeti**",
	agent.SummaryFlows:  "🔀 **Akışlar özeti**",
	agent.SummaryTools:  "🔌 **Araçlar**",
}

// handleSessionSummary produces an on-demand summary (memory/board/flows) or a
// tool listing for the session's agent, persists it as an assistant message in
// the session, and returns that message. Powers the chat "/" commands.
func (s *Server) handleSessionSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()

	session, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}

	var req summaryReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	kind := strings.TrimSpace(req.Kind)
	header, ok := summaryHeaders[kind]
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown summary kind: "+kind)
		return
	}

	summary, err := wsp.Runtime.Summarize(ctx, session.AgentID, kind)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "summary failed: "+err.Error())
		return
	}

	msg, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		AgentID:   session.AgentID,
		Text:      header + "\n\n" + summary,
		Steps:     "[]",
	})
	if writeDBError(w, err, "session not found") {
		return
	}
	writeJSON(w, http.StatusOK, msg)
}
