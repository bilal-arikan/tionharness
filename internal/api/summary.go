package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

type summaryReq struct {
	Kind string `json:"kind"`
}

// summaryHeaders gives each summary kind a self-explanatory chat header so the
// resulting assistant message reads clearly on its own.
var summaryHeaders = map[string]string{
	agent.SummaryBoard: "🗂 **Görev panosu özeti**",
	agent.SummaryFlows: "🔀 **Akışlar özeti**",
	agent.SummaryTools: "🔌 **Araçlar**",
}

// handleSessionSummary produces an on-demand summary (board/flows), a tool
// listing or a forced conversation compaction for the session's agent, persists
// the result as an assistant message in the session, and returns that message.
// Powers the chat "/" commands.
func (s *Server) handleSessionSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()

	session, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}

	req, ok := bindJSON[summaryReq](w, r)
	if !ok {
		return
	}
	kind := strings.TrimSpace(req.Kind)

	// Resolve the header + body for this command. compact is handled specially;
	// the rest go through the model-summary path.
	var header, body string
	switch kind {
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
	// Cross-window sync: the new user+assistant pair lands in the transcript —
	// a sibling window viewing this session reloads so the slash command + its
	// output appear without a manual refresh. op="summary" / "message_added" so
	// the listener knows to refresh the transcript and update last-message
	// metadata.
	emitSessionChange(wsp, session.ID, "summary")
	writeJSON(w, http.StatusOK, map[string]any{"userMessage": userMsg, "replyMessage": msg})
}

// handleSessionHandoff performs a manual context reset (/handoff): it writes a
// handoff artifact for the session and spawns a FRESH session to continue the
// work in a clean window, then returns the new session id so the UI can switch to
// it. Unlike /compact (which folds in place and keeps the same session), this is
// the Anthropic "context reset" pattern. Powers the chat "/handoff" command.
func (s *Server) handleSessionHandoff(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()

	session, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}
	agentRow, err := wsp.DB.GetAgent(ctx, session.AgentID)
	if writeDBError(w, err, "agent not found") {
		return
	}

	// Record the command itself as a user message so the thread shows what was run.
	userMsg, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleUser,
		Text:      "/handoff",
	})
	if writeDBError(w, err, "session not found") {
		return
	}

	res, herr := wsp.Runtime.HandoffSession(ctx, session, agentRow, agent.HandoffOptions{
		Reason: agent.HandoffReasonManual,
	})
	if herr != nil {
		writeError(w, http.StatusInternalServerError, "handoff failed: "+herr.Error())
		return
	}

	// HandoffSession already dropped a tombstone (with the new session link) into
	// the old session; return it as the reply message so the chat renders it.
	//
	// Cross-window sync: Runtime.HandoffSession itself emits the "session"
	// events for both the old (op="handoff") and the new (op="create") sessions
	// — the same runtime call also backs the agent-driven handoff_session tool
	// and the auto-handoff path, so all three entry points get a refresh. The
	// handler therefore does NOT publish duplicates here.
	writeJSON(w, http.StatusOK, map[string]any{
		"userMessage":  userMsg,
		"newSessionId": res.NewSessionID,
		"agentName":    res.AgentName,
		"artifactId":   res.ArtifactID,
	})
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
	ctx = conversation.WithCompactPrompt(ctx, wsp.Runtime.CompactPromptTemplate())
	folded, summary, err := s.convo.ForceCompact(ctx, wsp.DB, provider, session, agentRow, history)
	if err != nil {
		return "", err
	}
	if folded == 0 {
		return "Sıkıştırılacak yeterli eski mesaj yok (son mesajlar zaten bağlam penceresinde tutuluyor).", nil
	}
	return fmt.Sprintf("%d mesaj kalıcı özete katlandı; bağlam penceresi küçültüldü.\n\n**Güncel özet:**\n\n%s", folded, summary), nil
}
