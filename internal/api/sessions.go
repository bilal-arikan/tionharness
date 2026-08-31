package api

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
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
	q := r.URL.Query()
	agentID := q.Get("agentId")
	sessions, err := ws(r).DB.ListSessions(r.Context(), agentID)
	if writeDBError(w, err, "") {
		return
	}
	if sessions == nil {
		sessions = []db.Session{}
	}

	// Filters: kind (legacy producer kind), category, executionType and state
	// (active|archived). With any of limit/offset/sort present the response is
	// the standard {items,total,offset,limit,hasMore} envelope; without them it
	// stays the legacy full unwrapped list so existing UI clients keep working.
	limit, offset, field, asc, listing, err := listQueryParams(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kind := strings.TrimSpace(q.Get("kind"))
	category := strings.TrimSpace(q.Get("category"))
	executionType := strings.TrimSpace(q.Get("executionType"))
	state := strings.TrimSpace(q.Get("state"))
	matches := make([]db.Session, 0, len(sessions))
	for _, s := range sessions {
		if kind != "" && s.Kind != kind {
			continue
		}
		if category != "" && s.Category != category {
			continue
		}
		if executionType != "" && s.ExecutionType != executionType {
			continue
		}
		if state != "" && s.State != state {
			continue
		}
		matches = append(matches, s)
	}

	if !listing {
		writeJSON(w, http.StatusOK, matches)
		return
	}
	less, err := tools.SortByField(matches, field, asc,
		func(s db.Session) int64 { return s.UpdatedAt },
		func(s db.Session) int64 { return s.CreatedAt },
		func(s db.Session) string { return s.Title },
		func(s db.Session) string { return s.ID },
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sort.SliceStable(matches, less)
	page, total := tools.SlicePage(matches, offset, limit)
	pageJSONResponse(w, page, total, offset, limit)
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
	// One message by id — never the whole transcript. Copying every turn's trace
	// to return one of them was the single most expensive way to serve this.
	m, err := ws(r).DB.FindMessage(r.Context(), sessionID, msgID)
	if errors.Is(err, db.ErrMessageNotFound) {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	if writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"steps": m.Steps})
}

// handleActiveSessions returns the session ids that currently have an in-flight
// turn. Turns are detached from the client connection, so after a page reload the
// frontend queries this to restore the "thinking" indicator for any turn still
// running server-side. The set comes from runningSessionIDs — streamed chat turns,
// autonomous runtime invokes (spawn / worker / schedule / flow / inbox) and any
// turn holding the session's admission slot — so a spawned session running purely
// in the runtime (never registered in s.runs) also restores its indicator. Scoped
// to the active workspace so a foreign workspace's in-flight turns are not leaked
// to (nor restored by) this one. Sorted for a stable reply.
func (s *Server) handleActiveSessions(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	running := s.runningSessionIDs(wsp)
	ids := make([]string, 0, len(running))
	for id := range running {
		ids = append(ids, id)
	}
	sort.Strings(ids)
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
	if s.rejectReadOnlySession(w, r, sessionID, "a transcript rewind") {
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
	// Own the session-scoped delete lock across BOTH runtime teardown and durable
	// deletion. A second delete must not enter teardown while this request is in the
	// lifecycle-hook/DB gap and reopen state owned by the first request.
	unlockDelete := s.teardownLocks.lock(scopeKey(wsp.ID, id))
	defer unlockDelete()
	// Tear down this session's LIVE runtime (in-flight turn + its claude-cli subprocess,
	// warm pooled processes, the inbox worker, any autonomous worker) BEFORE removing it.
	// Fail closed: if a live process/turn cannot be stopped, abort with 409 so we never
	// strand a process pointing at a session that no longer exists (the session is left
	// fully intact — queue restored, worker resumed).
	prepared, err := s.prepareSessionRuntimeLocked(wsp, id, sessionTeardownGrace)
	if err != nil {
		s.logger.Error("session delete aborted: could not tear down live runtime", "session", id, "error", err)
		writeError(w, http.StatusConflict, "session has live processes that could not be stopped; not deleted: "+err.Error())
		return
	}
	runtimeCommitted := false
	defer func() {
		s.finishSessionRuntime(wsp, id, prepared, runtimeCommitted)
	}()
	// SessionEnd lifecycle hook (Claude Code parity): fire BEFORE the delete so a
	// cleanup hook can still read the session's files. Fire-and-forget audit.
	wsp.Runtime.RunLifecycleHooks(r.Context(), id, db.HookSessionEnd, agent.LifecycleExtras{Trigger: "delete"})
	var deleteErr error
	if s.deleteSession != nil {
		deleteErr = s.deleteSession(r.Context(), wsp, id)
	} else {
		deleteErr = wsp.DB.DeleteSession(r.Context(), id)
	}
	if writeDBError(w, deleteErr, "session not found") {
		return
	}
	runtimeCommitted = true
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
