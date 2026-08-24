package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
	"github.com/bilal-arikan/tionharness/internal/workspace"
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

	// Validate the command up-front so an unknown kind fails cleanly BEFORE any
	// durable side effect (persisted user message / hub events).
	header, known := summaryHeader(kind)
	if !known {
		writeError(w, http.StatusBadRequest, "unknown summary kind: "+kind)
		return
	}

	// This command runs a direct provider.Complete outside runInboxWorker, so it
	// takes the session's admission slot like any other turn: it waits behind
	// whatever is running (a chat turn, a coordinator auto-turn — no two subprocesses
	// resuming the same claude-cli transcript at once), and a message sent meanwhile
	// stays WAITING in the tray behind it. A client disconnect while waiting bails
	// cleanly, before any side effect.
	releaseTurn, err := wsp.Runtime.ClaimSessionCommandTurn(ctx, session.ID, "/"+kind)
	if err != nil {
		return
	}
	defer releaseTurn()

	// For compact, snapshot the history BEFORE the "/compact" command message is
	// appended, so the fold boundary is computed over the real conversation — the
	// command bubble and its report are the freshest tail and must never be folded.
	var compactHistory []db.Message
	if kind == "compact" {
		compactHistory, err = wsp.DB.ListMessages(ctx, session.ID)
		if writeDBError(w, err, "session not found") {
			return
		}
	}

	// Event-source the command like a real turn (_Docs/58). Slash commands were the
	// last chat action still rendered optimistic-only, outside the hub: the user +
	// reply bubbles were client-local until the (often slow — compaction runs a
	// summarize LLM call) op finished and only THEN persisted, so a page refresh
	// mid-op made both bubbles vanish until it completed. Now the command message is
	// persisted and published FIRST, and a live "working" bubble rides the hub, so a
	// fresh subscriber replays the in-flight tail and both survive a refresh.
	userMsg, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleUser,
		Text:      "/" + kind,
	})
	if writeDBError(w, err, "session not found") {
		return
	}
	s.publishHub(wsp.ID, session.ID, sessionhub.KindUserMessage, userMsg, false)
	// A durable agent_start + one text step give every window a live ghost bubble
	// carrying the busy label while the op runs. Durable (not the ephemeral delta)
	// so a subscriber that joins/refreshes mid-op replays them.
	s.publishHub(wsp.ID, session.ID, sessionhub.KindAgentStart, map[string]any{"agentId": session.AgentID}, false)
	s.publishStep(wsp.ID, session.ID, mustJSON(map[string]any{"kind": "text", "text": summaryBusyLabel(kind)}))

	// Run the command. On failure, clear the "working" bubble in every window
	// (turn_error + commit); the persisted "/kind" user message stays as an honest
	// record that the command was attempted.
	body, err := s.runSummaryKind(ctx, wsp, session, kind, compactHistory)
	if err != nil {
		s.recordSummaryFailure(ctx, wsp, session, kind, err)
		s.logger.Warn("summary command failed", "session", session.ID, "kind", kind, "error", err)
		writeError(w, http.StatusInternalServerError, kind+" failed: "+err.Error())
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
	// Reply + turn_done + commit: every window swaps the live ghost for the
	// persisted report and stops showing "working"; the committed boundary advances
	// so a later fresh subscriber skips replaying this finished turn.
	s.publishHub(wsp.ID, session.ID, sessionhub.KindReply, msg, false)
	s.hub.Publish(wsp.ID, session.ID, sessionhub.KindTurnDone, mustJSON(map[string]any{"sessionTitle": ""}), false)
	s.hub.Commit(wsp.ID, session.ID)
	// Sibling windows NOT subscribed to this session's hub (e.g. the sessions list)
	// still refresh their last-message metadata via the global bus.
	emitSessionChange(wsp, session.ID, "summary")
	writeJSON(w, http.StatusOK, map[string]any{"userMessage": userMsg, "replyMessage": msg})
}

// recordSummaryFailure leaves a DURABLE record of a failed slash command. The
// live turn_error event alone is client-only: before this, a failing "/compact"
// wrote nothing to debug.jsonl and persisted no reply, so a page refresh showed
// a "/compact" user bubble with no answer at all — it looked like nothing had
// happened. Three things now happen instead:
//
//  1. a debug-journal error record (same shape ordinary turn/tool failures write),
//     so the failure shows up in the session debug view and in debug.jsonl;
//  2. a persisted assistant message carrying the provider error VERBATIM, so the
//     transcript stays honest across a refresh and the user can act on it;
//  3. the existing turn_error event, so live windows still clear the "working"
//     ghost immediately.
//
// The records are written on an uncancellable context: the failure is often a
// client disconnect or an aborted request, and that is exactly when the durable
// trace matters most.
func (s *Server) recordSummaryFailure(ctx context.Context, wsp *workspace.Workspace, session db.Session, kind string, cause error) {
	ctx = context.WithoutCancel(ctx)

	if err := wsp.DB.AppendDebugEvent(session.ID, db.DebugEvent{
		Type:    db.DebugError,
		AgentID: session.AgentID,
		Kind:    "command",
		Name:    "/" + kind,
		Err:     true,
		Error:   cause.Error(),
		Detail:  "slash command failed: /" + kind + ": " + cause.Error(),
	}, 0); err != nil {
		s.logger.Error("summary failure journal append failed", "session", session.ID, "kind", kind, "error", err)
	}

	// turn_error first: every open window drops the live "working" bubble before
	// the persisted failure reply lands in its place.
	s.publishHub(wsp.ID, session.ID, sessionhub.KindTurnError, map[string]any{"error": cause.Error(), "reason": "summary_failed"}, false)

	msg, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		AgentID:   session.AgentID,
		Text:      summaryFailureText(kind, cause),
		Steps:     "[]",
	})
	if err != nil {
		// Losing the durable record is the very bug this function exists to fix —
		// it must never be swallowed.
		s.logger.Error("persist summary failure message failed", "session", session.ID, "kind", kind, "error", err)
		s.hub.Commit(wsp.ID, session.ID)
		return
	}
	s.publishHub(wsp.ID, session.ID, sessionhub.KindReply, msg, false)
	s.hub.Commit(wsp.ID, session.ID)
	// Sibling windows (sessions list) refresh their last-message metadata.
	emitSessionChange(wsp, session.ID, "summary_failed")
}

// summaryFailureText renders the in-thread failure notice for a slash command.
// The underlying error is embedded verbatim and never softened: for the codex
// CLI it is the only actionable thing the user gets (e.g. "Your access token
// could not be refreshed because your refresh token was revoked").
func summaryFailureText(kind string, cause error) string {
	return "⚠️ **/" + kind + " başarısız oldu** — komut tamamlanamadı, oturumda hiçbir değişiklik yapılmadı.\n\n**Hata:**\n\n```\n" + cause.Error() + "\n```"
}

// summaryHeader resolves the self-explanatory chat header for a slash command,
// reporting known=false for an unrecognized kind (so the endpoint rejects it
// before any side effect).
func summaryHeader(kind string) (header string, known bool) {
	switch kind {
	case "compact":
		return "🗜 **Sohbet sıkıştırma**", true
	case "refresh-context":
		return "🔄 **Bağlam yenileme**", true
	default:
		h, ok := summaryHeaders[kind]
		return h, ok
	}
}

// summaryBusyLabel is the text shown in the live "working" ghost bubble while a
// slash command runs (mirrors the frontend's per-kind busy labels).
func summaryBusyLabel(kind string) string {
	switch kind {
	case "compact":
		return "⏳ Sohbet sıkıştırılıyor…"
	case "refresh-context":
		return "⏳ Bağlam snapshot'ı yenileniyor…"
	default:
		return "⏳ Özetleniyor…"
	}
}

// runSummaryKind executes one slash command and returns its assistant-message
// body. compact folds older history into the rolling summary; refresh-context
// drops the frozen prompt snapshot; the rest go through the model-summary path.
func (s *Server) runSummaryKind(ctx context.Context, wsp *workspace.Workspace, session db.Session, kind string, compactHistory []db.Message) (string, error) {
	switch kind {
	case "compact":
		return s.compactSession(ctx, wsp, session, compactHistory)
	case "refresh-context":
		// Prompt-epoch explicit adopt: drop the session's frozen prompt snapshot so
		// the next turn recomposes tools + static system from live state (a chosen
		// one-time cache re-write). No-op text when the feature is off.
		if wsp.Runtime.PromptEpochEnabled() {
			wsp.Runtime.RefreshPromptEpoch(ctx, session.ID)
			return "Statik bağlam snapshot'ı temizlendi: bir sonraki tur güncel araç kataloğu, skill listesi ve talimatlarla yeniden derlenecek (bilinçli tek seferlik cache yeniden yazımı).", nil
		}
		return "Prompt-epoch (donmuş bağlam snapshot'ı) bu workspace'te kapalı; her tur zaten canlı durumdan derleniyor — yenilenecek bir snapshot yok.", nil
	default:
		return wsp.Runtime.Summarize(ctx, session.AgentID, kind)
	}
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

	// Takes the session's admission slot like /compact: /handoff spawns a fresh
	// session outside runInboxWorker, so it waits for any in-flight turn (chat,
	// coordinator auto-turn, wake) and holds the slot while it runs — a message sent
	// during the handoff stays WAITING in the tray (and re-dispatches on the OLD
	// session after release, though the UI usually follows the switch to the new
	// one). A client disconnect while waiting bails before any side effect.
	releaseTurn, err := wsp.Runtime.ClaimSessionCommandTurn(ctx, session.ID, "/handoff")
	if err != nil {
		return
	}
	defer releaseTurn()

	// Record the command itself as a user message so the thread shows what was run.
	userMsg, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleUser,
		Text:      "/handoff",
	})
	if writeDBError(w, err, "session not found") {
		return
	}
	// Event-source the command on the OLD session's hub like /compact (_Docs/58):
	// persist + publish the "/handoff" bubble and a live "working" ghost BEFORE the
	// (potentially slow — writes a handoff artifact, spawns a fresh session) op, so
	// a page refresh mid-op replays the in-flight tail instead of losing both bubbles.
	s.publishHub(wsp.ID, session.ID, sessionhub.KindUserMessage, userMsg, false)
	s.publishHub(wsp.ID, session.ID, sessionhub.KindAgentStart, map[string]any{"agentId": session.AgentID}, false)
	s.publishStep(wsp.ID, session.ID, mustJSON(map[string]any{"kind": "text", "text": "⏳ Context reset — handoff yazılıyor ve temiz oturum başlatılıyor…"}))

	res, herr := wsp.Runtime.HandoffSession(ctx, session, agentRow, agent.HandoffOptions{
		Reason: agent.HandoffReasonManual,
	})
	if herr != nil {
		// The running-workers guard is a user-actionable precondition, not a server
		// fault: record the reason in-thread (so the chat shows why nothing happened,
		// right under the "/handoff" the user just ran) and return 409 instead of a
		// generic 500 toast.
		var rwErr *agent.RunningWorkersError
		if errors.As(herr, &rwErr) {
			notice, aerr := wsp.DB.AddMessage(ctx, db.Message{
				SessionID: session.ID,
				Role:      providers.RoleAssistant,
				AgentID:   session.AgentID,
				Text:      "⚠️ **Handoff yapılmadı** — " + rwErr.Error(),
				Steps:     "[]",
			})
			if writeDBError(w, aerr, "session not found") {
				return
			}
			// The session stays put (no fresh window) — swap the live ghost for the
			// persisted notice on the hub so every window renders it and stops showing
			// "working".
			s.publishHub(wsp.ID, session.ID, sessionhub.KindReply, notice, false)
			s.hub.Publish(wsp.ID, session.ID, sessionhub.KindTurnDone, mustJSON(map[string]any{"sessionTitle": ""}), false)
			s.hub.Commit(wsp.ID, session.ID)
			emitSessionChange(wsp, session.ID, "message_added")
			// 200 (not 409) with a blocked flag: the frontend's fetch wrapper throws
			// on any non-2xx and would surface a generic "HTTP 409" toast, burying the
			// friendly in-thread notice we just wrote. A blocked command is a normal,
			// expected outcome here — the reply message IS the user-facing result.
			writeJSON(w, http.StatusOK, map[string]any{
				"userMessage":  userMsg,
				"replyMessage": notice,
				"blocked":      true,
			})
			return
		}
		// Hard failure: clear the live "working" bubble in every window; the persisted
		// "/handoff" user message stays as an honest record it was attempted.
		s.publishHub(wsp.ID, session.ID, sessionhub.KindTurnError, map[string]any{"error": herr.Error(), "reason": "handoff_failed"}, false)
		s.hub.Commit(wsp.ID, session.ID)
		writeError(w, http.StatusInternalServerError, "handoff failed: "+herr.Error())
		return
	}

	// HandoffSession already dropped a tombstone (with the new session link) into
	// the old session; publish it on the old session's hub as the reply (+ turn_done)
	// so a window still on the old session swaps the ghost for it — the sending
	// window switches to the fresh session, but a sibling window or a return visit
	// renders the durable tombstone. publishAutonomousReply reads the old session's
	// last (assistant) message, which is exactly that tombstone.
	s.publishAutonomousReply(wsp.ID, session.ID)
	s.hub.Publish(wsp.ID, session.ID, sessionhub.KindTurnDone, mustJSON(map[string]any{"sessionTitle": ""}), false)
	s.hub.Commit(wsp.ID, session.ID)
	// Cross-window sync: Runtime.HandoffSession itself emits the "session"
	// events for both the old (op="handoff") and the new (op="create") sessions
	// — the same runtime call also backs the agent-driven handoff_session tool
	// and the auto-handoff path, so all three entry points get a refresh.
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
func (s *Server) compactSession(ctx context.Context, wsp *workspace.Workspace, session db.Session, history []db.Message) (string, error) {
	agentRow, err := wsp.DB.GetAgent(ctx, session.AgentID)
	if err != nil {
		return "", err
	}
	provider, err := s.providers.Get(agentRow.ProviderRef())
	if err != nil {
		return "", err
	}
	// Out-of-loop path: pin this app's CLI homes before ForceCompact's direct
	// provider.Complete, mirroring guardedComplete (else the CLI falls back to the
	// ambient home and can fail auth even when TionHarness is logged in).
	if err := wsp.Runtime.PinCLIHome(provider); err != nil {
		return "", err
	}
	// history is the pre-command snapshot captured by the caller (before the
	// "/compact" user message was appended), so the fold boundary matches the real
	// conversation.
	ctx = conversation.WithCompactPrompt(ctx, wsp.Runtime.CompactPromptTemplate())
	ctx = conversation.WithAttachmentRoot(ctx, wsp.SandboxRoot())
	ctx = conversation.WithClaudeHome(ctx, wsp.Runtime.ClaudeHomeDir())
	folded, summary, err := s.convo.ForceCompact(ctx, wsp.DB, provider, session, agentRow, history)
	if err != nil {
		return "", err
	}
	if folded == 0 {
		return "Sıkıştırılacak yeterli eski mesaj yok (son mesajlar zaten bağlam penceresinde tutuluyor).", nil
	}
	return fmt.Sprintf("%d mesaj kalıcı özete katlandı; bağlam penceresi küçültüldü.\n\n**Güncel özet:**\n\n%s", folded, summary), nil
}
