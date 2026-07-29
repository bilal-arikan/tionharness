package api

import (
	"net/http"
	"os/exec"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
)

type setStateReq struct {
	State string `json:"state"`
}

// handleSetSessionState sets a session's lifecycle state ("active"/"archived").
// Archiving drops it from the active sidebar list + cross-session context block
// but never deletes it; the user can restore it (state="active"). The same
// mutation an agent makes via archive_session.
func (s *Server) handleSetSessionState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[setStateReq](w, r)
	if !ok {
		return
	}
	state := strings.TrimSpace(req.State)
	if state != "active" && state != "archived" {
		writeError(w, http.StatusBadRequest, "state must be \"active\" or \"archived\"")
		return
	}
	ctx := r.Context()
	database := ws(r).DB
	if _, err := database.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	if err := database.SetSessionState(ctx, id, state); writeDBError(w, err, "") {
		return
	}
	// Keep the "archived" auto-tag in sync with the lifecycle state so an automation
	// can scan archived sessions: add it when archiving, drop it when restoring.
	if sess, err := database.GetSession(ctx, id); err == nil && ws(r).Runtime.AutoTagEnabled() {
		has := false
		out := make([]string, 0, len(sess.Tags))
		for _, t := range sess.Tags {
			if t == agent.TagArchived {
				has = true
				continue // dropped; re-added below only when archiving
			}
			out = append(out, t)
		}
		if state == "archived" {
			out = append(out, agent.TagArchived)
		}
		if has != (state == "archived") {
			_ = database.SetSessionTags(ctx, id, out)
		}
	}
	// Cross-window sync: other windows on the same workspace move the row
	// between Active/Archived filters immediately. Note: emitted AFTER the
	// tag reconcile so the "tags" op (if any) is observed first chronologically
	// and the "state" op is the authoritative one for the filter.
	emitSessionChange(ws(r), id, "state")
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "state": state})
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agentId")
	sessions, err := ws(r).DB.ListSessions(r.Context(), agentID)
	if writeDBError(w, err, "") {
		return
	}
	if sessions == nil {
		sessions = []db.Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

type createSessionReq struct {
	AgentID string `json:"agentId"`
	Title   string `json:"title"`
	// WorkingDir optionally pins this session's cwd. When omitted, the session
	// inherits the workspace's configured default working directory (Path), so a
	// fresh session starts already scoped to that folder.
	WorkingDir string `json:"workingDir"`
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[createSessionReq](w, r)
	if !ok {
		return
	}
	// agentId is the session's default agent. Optional: when omitted we fall
	// back to the workspace's first (newest) agent so a session can be created
	// session-first, then routed per-turn via "@mention".
	if req.AgentID == "" {
		agents, _ := ws(r).DB.ListAgents(r.Context())
		if len(agents) == 0 {
			writeError(w, http.StatusBadRequest, "no agents exist; create an agent first")
			return
		}
		req.AgentID = agents[0].ID
	} else if _, err := ws(r).DB.GetAgent(r.Context(), req.AgentID); err != nil {
		writeError(w, http.StatusBadRequest, "agent not found")
		return
	}

	// Seed the cwd: an explicit request value wins; otherwise inherit the
	// workspace's configured default working directory (Path) so every new
	// session starts already scoped to that folder.
	cwd := strings.TrimSpace(req.WorkingDir)
	if cwd == "" {
		cwd = strings.TrimSpace(ws(r).Settings().DefaultWorkingDir)
	}

	session, err := ws(r).DB.CreateSession(r.Context(), db.Session{
		AgentID:    req.AgentID,
		Title:      req.Title,
		WorkingDir: cwd,
	})
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("session created", "session", session.ID, "agent", req.AgentID)
	// Cross-window sync: a second window on the same workspace (or any other
	// listener) should immediately see the new row in its session list.
	emitSessionChange(ws(r), session.ID, "create")
	writeJSON(w, http.StatusCreated, session)
}

type titleResp struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type generateTitleReq struct {
	// Title, when set, is applied verbatim (manual rename) and no AI generation
	// happens. Source optionally overrides what the title is generated from; when
	// both are empty the session's existing conversation is used.
	Title  string `json:"title"`
	Source string `json:"source"`
}

// handleGenerateSessionTitle sets a session's title. With a `title` it renames
// directly (manual); otherwise it (re)generates from `source` or, by default,
// the opening of the session's conversation.
func (s *Server) handleGenerateSessionTitle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()

	session, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}

	var req generateTitleReq
	_ = decodeJSON(r, &req) // body is optional

	// Manual rename: apply the given title verbatim.
	if t := strings.TrimSpace(req.Title); t != "" {
		if err := wsp.DB.SetSessionTitle(ctx, id, t); writeDBError(w, err, "session not found") {
			return
		}
		emitSessionChange(wsp, id, "title")
		writeJSON(w, http.StatusOK, titleResp{ID: id, Title: t})
		return
	}

	source := strings.TrimSpace(req.Source)
	if source == "" {
		msgs, err := wsp.DB.ListMessages(ctx, id)
		if writeDBError(w, err, "") {
			return
		}
		source = titleSourceFromMessages(msgs)
	}
	if source == "" {
		writeError(w, http.StatusBadRequest, "no content to generate a title from")
		return
	}

	title, err := wsp.Runtime.TitleFor(ctx, session.AgentID, source)
	if err != nil {
		s.logger.Warn("session title generation degraded", "session", id, "error", err)
	}
	if err := wsp.DB.SetSessionTitle(ctx, id, title); writeDBError(w, err, "session not found") {
		return
	}
	emitSessionChange(wsp, id, "title")
	writeJSON(w, http.StatusOK, titleResp{ID: id, Title: title})
}

// titleSourceFromMessages composes a compact source string from the first few
// turns of a conversation for titling.
func titleSourceFromMessages(msgs []db.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteString("\n")
		if b.Len() > 1500 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	messages, err := ws(r).DB.ListMessages(r.Context(), sessionID)
	if writeDBError(w, err, "") {
		return
	}
	if messages == nil {
		messages = []db.Message{}
	}
	// Shrink the wire copy: a long transcript's bulk is tool output the chat
	// never paints. Trimmed steps carry their `*Truncated` flags, and the full
	// trace stays one click away via handleMessageSteps below. ListMessages
	// already returned a copy, so mutating in place cannot touch the store.
	for i := range messages {
		messages[i].Steps = trimStepsJSON(messages[i].Steps)
	}
	writeJSON(w, http.StatusOK, messages)
}

// handleMessageSteps returns ONE message's activity trace untrimmed. The
// transcript listing ships tool payloads cut to stepFieldCap; the chat calls
// this when the user asks to see a truncated turn in full.
func (s *Server) handleMessageSteps(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	msgID := r.PathValue("msgId")
	messages, err := ws(r).DB.ListMessages(r.Context(), sessionID)
	if writeDBError(w, err, "") {
		return
	}
	for _, m := range messages {
		if m.ID == msgID {
			writeJSON(w, http.StatusOK, map[string]string{"steps": m.Steps})
			return
		}
	}
	writeError(w, http.StatusNotFound, "message not found")
}

// handleActiveSessions returns the session ids that currently have an in-flight
// turn. Turns are detached from the client connection, so after a page reload the
// frontend queries this to restore the "thinking" indicator for any turn still
// running server-side. It unions the two authoritative registries — streamed chat
// turns (s.runs) AND autonomous runtime invokes (spawn / worker / schedule / flow
// / inbox), so a spawned session running purely in the runtime (never registered
// in s.runs) also restores its indicator. Scoped to the active workspace so a
// foreign workspace's in-flight turns are not leaked to (nor restored by) this one.
func (s *Server) handleActiveSessions(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	seen := map[string]struct{}{}
	ids := []string{}
	add := func(list []string) {
		for _, id := range list {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	add(s.runs.activeSessionIDs(wsp.ID))
	add(wsp.Runtime.ActiveSessionIDs())
	writeJSON(w, http.StatusOK, map[string][]string{"sessionIds": ids})
}

// handleDropSessionCLIProcess recycles the session's warm claude-cli process(es)
// (persistent-pool mode) so the next turn cold-restarts with a fresh process. The
// conversation/history is untouched — only the background process is dropped. A
// no-op (dropped:0) when no warm process exists.
func (s *Server) handleDropSessionCLIProcess(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dropped := ws(r).Runtime.DropWarmCLISession(id)
	writeJSON(w, http.StatusOK, map[string]int{"dropped": dropped})
}

// handleSessionInflight returns the session's in-progress streaming snapshot (the
// partial assistant reply — agent, text and trace so far — written on a throttle
// while the turn runs), or null when no turn is streaming. A page reloaded
// mid-turn fetches this to restore the in-progress bubble (agent name + steps)
// rather than showing a bare "thinking" dot until the turn completes. The turn is
// detached server-side, so the snapshot keeps growing after the reload and the
// authoritative message replaces it on completion.
func (s *Server) handleSessionInflight(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	t, ok, err := ws(r).DB.ReadInflight(sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "inflight read failed")
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// handleDeleteMessage removes a single message from a session (e.g. to prune a
// mistaken or test message). Rewrites the session's JSONL file.
func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	msgID := r.PathValue("msgId")
	wsp := ws(r)
	if err := wsp.DB.DeleteMessage(r.Context(), sessionID, msgID); writeDBError(w, err, "message not found") {
		return
	}
	// Cross-window sync: the active session's transcript in a sibling window
	// reloads so the dropped message disappears. The op hint lets the listener
	// skip the network call when the deleted message belongs to a non-active
	// session (the list update is enough).
	emitSessionChange(wsp, sessionID, "delete_message")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": msgID})
}

type rewindReq struct {
	MessageID string `json:"messageId"`
}

// handleRewindSession rewinds the conversation to a checkpoint: it removes the
// given message and every message after it, then rewrites the session's JSONL.
// This is a conversation-only rewind — file changes from past turns are NOT
// reverted (git remains the source of truth for code). The client reloads the
// transcript afterwards.
func (s *Server) handleRewindSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	wsp := ws(r)
	req, ok := bindJSON[rewindReq](w, r)
	if !ok {
		return
	}
	msgID := strings.TrimSpace(req.MessageID)
	if msgID == "" {
		writeError(w, http.StatusBadRequest, "messageId is required")
		return
	}
	removed, err := wsp.DB.DeleteMessagesFrom(r.Context(), sessionID, msgID)
	if writeDBError(w, err, "message not found") {
		return
	}
	s.logger.Info("session rewound", "session", sessionID, "from", msgID, "removed", removed)
	// Cross-window sync: the active session's transcript reloads (rewind can drop
	// the in-flight ghost bubble too — handled by the chat hook if the same
	// session is being viewed live).
	emitSessionChange(wsp, sessionID, "rewind")
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}

// handleMarkSessionRead clears a session's unread flag.
func (s *Server) handleMarkSessionRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ws(r).DB.MarkSessionRead(r.Context(), id); writeDBError(w, err, "session not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

// handleDeleteSession removes a session and its on-disk folder.
func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	// Tear down this session's LIVE runtime (in-flight turn + its claude-cli subprocess,
	// warm pooled processes, the inbox worker, any autonomous worker) BEFORE removing it.
	// Fail closed: if a live process/turn cannot be stopped, abort with 409 so we never
	// strand a process pointing at a session that no longer exists (the session is left
	// fully intact — queue restored, worker resumed).
	if err := s.teardownSessionRuntime(wsp, id); err != nil {
		s.logger.Error("session delete aborted: could not tear down live runtime", "session", id, "error", err)
		writeError(w, http.StatusConflict, "session has live processes that could not be stopped; not deleted: "+err.Error())
		return
	}
	// SessionEnd lifecycle hook (Claude Code parity): fire BEFORE the delete so a
	// cleanup hook can still read the session's files. Fire-and-forget audit.
	wsp.Runtime.RunLifecycleHooks(r.Context(), id, db.HookSessionEnd, agent.LifecycleExtras{Trigger: "delete"})
	if err := wsp.DB.DeleteSession(r.Context(), id); writeDBError(w, err, "session not found") {
		return
	}
	s.logger.Info("session deleted", "session", id)
	// Cross-window sync: a sibling window showing this session in its sidebar
	// drops the row immediately; the active session (if it was the deleted one)
	// falls back to the next chat session in App.tsx's session-list effect.
	emitSessionChange(wsp, id, "delete")
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}

// handleSessionPath returns the absolute folder holding the session's JSONL file.
func (s *Server) handleSessionPath(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path, err := ws(r).DB.SessionDir(id)
	if writeDBError(w, err, "session not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// handleRevealSession opens the session's folder in the OS file manager on the
// machine running the backend (local desktop app). Windows: Explorer.
func (s *Server) handleRevealSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path, err := ws(r).DB.SessionDir(id)
	if writeDBError(w, err, "session not found") {
		return
	}
	// Detached from r.Context() so it isn't killed when the handler returns.
	if err := exec.Command("explorer.exe", path).Start(); err != nil {
		// explorer.exe returns a non-zero exit code even on success; only a
		// failure to *start* the process is a real error.
		s.logger.Warn("reveal session folder failed", "session", id, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}
