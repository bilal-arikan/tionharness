package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/workspace"
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

// handleSessionSummary produces an on-demand summary (memory/board/flows), a
// tool listing, a reflection (dream cycle) or a forced conversation compaction
// for the session's agent, persists the result as an assistant message in the
// session, and returns that message. Powers the chat "/" commands.
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

	// Resolve the header + body for this command. reflect/compact are handled
	// specially; the rest go through the model-summary path.
	var header, body string
	switch kind {
	case "reflect":
		header = "✦ **Yansıma (dream cycle)**"
		reflection, rerr := wsp.Runtime.Reflect(ctx, session.AgentID)
		if rerr != nil {
			writeError(w, http.StatusInternalServerError, "reflect failed: "+rerr.Error())
			return
		}
		body = reflection.Content
	case "compact":
		header = "🗜 **Sohbet sıkıştırma**"
		cbody, cerr := s.compactSession(ctx, wsp, session)
		if cerr != nil {
			writeError(w, http.StatusInternalServerError, "compact failed: "+cerr.Error())
			return
		}
		body = cbody
	default:
		h, ok := summaryHeaders[kind]
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown summary kind: "+kind)
			return
		}
		header = h
		summary, serr := wsp.Runtime.Summarize(ctx, session.AgentID, kind)
		if serr != nil {
			// Log before returning the 500: the funnel logs provider errors, but a
			// data-gathering failure would otherwise leave only the HTTP response —
			// match the title endpoints, which always log a degraded result.
			s.logger.Warn("summary generation failed", "session", session.ID, "kind", kind, "error", serr)
			writeError(w, http.StatusInternalServerError, "summary failed: "+serr.Error())
			return
		}
		body = summary
	}

	// Persist the command itself as a user message so the conversation shows what
	// was run (rendered in the command style), then the assistant's result.
	userMsg, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleUser,
		Text:      "/" + kind,
	})
	if writeDBError(w, err, "session not found") {
		return
	}

	msg, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		AgentID:   session.AgentID,
		Text:      header + "\n\n" + body,
		Steps:     "[]",
	})
	if writeDBError(w, err, "session not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"userMessage": userMsg, "replyMessage": msg})
}

// compactSession forces a conversation compaction now: it folds older history
// into the rolling summary (via the conversation Manager) and returns a short
// human-readable report for the chat.
func (s *Server) compactSession(ctx context.Context, wsp *workspace.Workspace, session db.Session) (string, error) {
	agentRow, err := wsp.DB.GetAgent(ctx, session.AgentID)
	if err != nil {
		return "", err
	}
	provider, err := s.providers.Get(agentRow.Provider)
	if err != nil {
		return "", err
	}
	history, err := wsp.DB.ListMessages(ctx, session.ID)
	if err != nil {
		return "", err
	}
	folded, summary, err := s.convo.ForceCompact(ctx, wsp.DB, provider, session, agentRow, history)
	if err != nil {
		return "", err
	}
	if folded == 0 {
		return "Sıkıştırılacak yeterli eski mesaj yok (son mesajlar zaten bağlam penceresinde tutuluyor).", nil
	}
	return fmt.Sprintf("%d mesaj kalıcı özete katlandı; bağlam penceresi küçültüldü.\n\n**Güncel özet:**\n\n%s", folded, summary), nil
}
