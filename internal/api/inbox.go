package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/sessionhub"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// inboxItem is one queued user turn awaiting dispatch. The whole chatReq is
// carried so the serial worker can run it exactly as the direct path would;
// WorkspaceID resolves the owning workspace at dispatch (the store is
// server-wide). See _Docs/58-QUEUE-SENKRON.md Faz 3.
type inboxItem struct {
	ClientMsgID string  `json:"clientMsgId"`
	Req         chatReq `json:"req"`
	WorkspaceID string  `json:"workspaceId"`
	EnqueuedAt  int64   `json:"enqueuedAt"`
}

// sessionInbox is one session's FIFO command queue plus its serial-worker flag.
// Everything a session submits — user turns while a turn is already running —
// funnels through here so exactly one turn runs at a time, in order.
type sessionInbox struct {
	items   []inboxItem
	seen    map[string]bool // clientMsgId dedupe (idempotent enqueue)
	running bool            // a worker goroutine is draining this queue
	wsID    string          // owning workspace (for persistence + dispatch)
}

// inboxStore holds every session's queue. Server-wide (like chatRuns), keyed by
// session id.
type inboxStore struct {
	mu       sync.Mutex
	sessions map[string]*sessionInbox
}

func newInboxStore() *inboxStore {
	return &inboxStore{sessions: make(map[string]*sessionInbox)}
}

func (i *inboxStore) lock()   { i.mu.Lock() }
func (i *inboxStore) unlock() { i.mu.Unlock() }

// enqueueMessage adds a turn to a session's queue (idempotent on clientMsgID),
// persists it, broadcasts the new queue to every window, and kicks the serial
// worker. Returns false when the id was already seen (a duplicate submit).
func (s *Server) enqueueMessage(wsID string, req chatReq, clientMsgID string) bool {
	if clientMsgID == "" {
		clientMsgID = uuid.NewString()
	}
	s.inbox.lock()
	ib := s.inbox.sessions[req.SessionID]
	if ib == nil {
		ib = &sessionInbox{seen: make(map[string]bool), wsID: wsID}
		s.inbox.sessions[req.SessionID] = ib
	}
	if ib.wsID == "" {
		ib.wsID = wsID
	}
	if ib.seen[clientMsgID] {
		s.inbox.unlock()
		return false
	}
	ib.seen[clientMsgID] = true
	ib.items = append(ib.items, inboxItem{ClientMsgID: clientMsgID, Req: req, WorkspaceID: wsID, EnqueuedAt: time.Now().Unix()})
	s.inbox.unlock()
	s.persistInbox(req.SessionID)
	s.publishQueueUpdate(req.SessionID)
	s.kickInbox(req.SessionID)
	return true
}

// cancelQueued removes a WAITING message from a session's queue (the in-flight
// head is already popped, so a running turn is unaffected — that is what stop is
// for). Returns whether anything was removed.
func (s *Server) cancelQueued(sessionID, clientMsgID string) bool {
	s.inbox.lock()
	ib := s.inbox.sessions[sessionID]
	removed := false
	if ib != nil {
		for idx, it := range ib.items {
			if it.ClientMsgID == clientMsgID {
				ib.items = append(ib.items[:idx:idx], ib.items[idx+1:]...)
				removed = true
				break
			}
		}
	}
	s.inbox.unlock()
	if removed {
		s.persistInbox(sessionID)
		s.publishQueueUpdate(sessionID)
	}
	return removed
}

// clearQueued drops ALL waiting messages from a session's queue (the in-flight
// head is already popped, so a running turn is unaffected). Returns how many were
// removed.
func (s *Server) clearQueued(sessionID string) int {
	s.inbox.lock()
	ib := s.inbox.sessions[sessionID]
	n := 0
	if ib != nil {
		n = len(ib.items)
		ib.items = nil
	}
	s.inbox.unlock()
	if n > 0 {
		s.persistInbox(sessionID)
		s.publishQueueUpdate(sessionID)
	}
	return n
}

// moveQueuedToFront promotes a waiting message to the head of the queue so it
// dispatches next ("send next"). Returns whether it was found + moved.
func (s *Server) moveQueuedToFront(sessionID, clientMsgID string) bool {
	s.inbox.lock()
	ib := s.inbox.sessions[sessionID]
	moved := false
	if ib != nil {
		for idx, it := range ib.items {
			if it.ClientMsgID == clientMsgID {
				ib.items = append(ib.items[:idx:idx], ib.items[idx+1:]...)
				ib.items = append([]inboxItem{it}, ib.items...)
				moved = true
				break
			}
		}
	}
	s.inbox.unlock()
	if moved {
		s.persistInbox(sessionID)
		s.publishQueueUpdate(sessionID)
	}
	return moved
}

// kickInbox starts the serial worker for a session if one is not already running.
func (s *Server) kickInbox(sessionID string) {
	s.inbox.lock()
	ib := s.inbox.sessions[sessionID]
	if ib == nil || ib.running || len(ib.items) == 0 {
		s.inbox.unlock()
		return
	}
	ib.running = true
	s.inbox.unlock()
	go s.runInboxWorker(sessionID)
}

// runInboxWorker drains a session's queue one turn at a time. It pops the head
// (marking it in-flight, so the persisted queue shows only WAITING turns), runs
// it to completion via the hub-publishing turn runner, then loops. A crash while
// the head runs is covered by the turn's own inflight sidecar; the WAITING tail
// stays durable in inbox.json.
func (s *Server) runInboxWorker(sessionID string) {
	for {
		s.inbox.lock()
		ib := s.inbox.sessions[sessionID]
		if ib == nil || len(ib.items) == 0 {
			if ib != nil {
				ib.running = false
				// Queue drained: reset the dedupe set so it can't grow without bound
				// across a long-lived session (a re-submit of an old id after this is a
				// genuinely new turn).
				ib.seen = make(map[string]bool)
			}
			s.inbox.unlock()
			s.persistInbox(sessionID)
			s.publishQueueUpdate(sessionID)
			return
		}
		item := ib.items[0]
		ib.items = ib.items[1:]
		s.inbox.unlock()
		s.persistInbox(sessionID)
		s.publishQueueUpdate(sessionID)

		wsp := s.workspaces.Default()
		if item.WorkspaceID != "" {
			if w, err := s.workspaces.Get(item.WorkspaceID); err == nil {
				wsp = w
			}
		}
		if wsp == nil {
			continue
		}
		// Server-driven turn: no single owning client, so clientGone never fires —
		// an interactive ask_user waits for an answer from ANY window (via the
		// interaction CAS) instead of bailing. All UI rides the hub (write=nil).
		// Recover from a panic in the turn so ONE bad turn can never kill the worker
		// goroutine and leave the session's queue wedged (running=true, undrained).
		s.runTurnGuarded(wsp, item.Req)
	}
}

// runTurnGuarded runs one queued turn with a panic barrier so a crash in the turn
// (a provider bug, a nil deref) surfaces as a logged error + a hub turn_error
// instead of killing the serial worker and wedging the session's queue.
func (s *Server) runTurnGuarded(wsp *workspace.Workspace, req chatReq) {
	defer func() {
		if r := recover(); r != nil {
			if s.logger != nil {
				s.logger.Error("queued turn panicked", "session", req.SessionID, "panic", r)
			}
			s.publishHub(req.SessionID, sessionhub.KindTurnError, map[string]any{
				"error":  "turn crashed",
				"reason": "panic",
			}, false)
			s.hub.Commit(req.SessionID)
		}
	}()
	s.runChatTurn(context.Background(), wsp, req, nil)
}

// persistInbox writes the session's WAITING queue to its inbox.json sidecar (or
// clears it when empty), in the owning workspace's store.
func (s *Server) persistInbox(sessionID string) {
	s.inbox.lock()
	ib := s.inbox.sessions[sessionID]
	var items []inboxItem
	wsID := ""
	if ib != nil {
		items = append([]inboxItem{}, ib.items...)
		wsID = ib.wsID
	}
	s.inbox.unlock()
	wsp := s.workspaces.Default()
	if wsID != "" {
		if w, err := s.workspaces.Get(wsID); err == nil {
			wsp = w
		}
	}
	if wsp == nil || wsp.DB == nil {
		return
	}
	if len(items) == 0 {
		_ = wsp.DB.ClearInbox(sessionID)
		return
	}
	if data, err := json.Marshal(items); err == nil {
		_ = wsp.DB.WriteInbox(sessionID, data)
	}
}

// queueView is the client-facing shape of one waiting queue entry.
type queueView struct {
	ClientMsgID string `json:"clientMsgId"`
	Text        string `json:"text"`
	EnqueuedAt  int64  `json:"enqueuedAt"`
}

// publishQueueUpdate broadcasts the session's current WAITING queue to every
// window on the hub, so a message queued in one window shows in all of them.
func (s *Server) publishQueueUpdate(sessionID string) {
	s.inbox.lock()
	ib := s.inbox.sessions[sessionID]
	out := make([]queueView, 0)
	if ib != nil {
		for _, it := range ib.items {
			out = append(out, queueView{ClientMsgID: it.ClientMsgID, Text: it.Req.Message, EnqueuedAt: it.EnqueuedAt})
		}
	}
	s.inbox.unlock()
	s.publishHub(sessionID, sessionhub.KindQueueUpdate, map[string]any{"queue": out}, false)
}

// recoverInboxes re-enqueues every session's persisted WAITING queue at boot and
// kicks its worker, so messages submitted before a crash/restart still run. Best
// effort: an unreadable sidecar is skipped. Runs once at startup.
func (s *Server) recoverInboxes() {
	for _, meta := range s.workspaces.List() {
		wsp, err := s.workspaces.Get(meta.ID)
		if err != nil || wsp == nil || wsp.DB == nil {
			continue
		}
		sessions, err := wsp.DB.ListSessions(context.Background(), "")
		if err != nil {
			continue
		}
		for _, sess := range sessions {
			data, ok, err := wsp.DB.ReadInbox(sess.ID)
			if err != nil || !ok {
				continue
			}
			var items []inboxItem
			if json.Unmarshal(data, &items) != nil || len(items) == 0 {
				continue
			}
			s.inbox.lock()
			ib := &sessionInbox{seen: make(map[string]bool), wsID: wsp.ID}
			for _, it := range items {
				ib.items = append(ib.items, it)
				ib.seen[it.ClientMsgID] = true
			}
			s.inbox.sessions[sess.ID] = ib
			s.inbox.unlock()
			s.publishQueueUpdate(sess.ID)
			s.kickInbox(sess.ID)
		}
	}
}

// ---- HTTP handlers ----

// handleEnqueueMessage is the queue-based submit: it adds the turn to the
// session's FIFO inbox and returns immediately (the turn runs server-side, its
// UI streamed to every window over the hub). The path session id is authoritative.
func (s *Server) handleEnqueueMessage(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[chatReq](w, r)
	if !ok {
		return
	}
	req.SessionID = r.PathValue("id")
	if req.SessionID == "" || (len(req.Attachments) == 0 && req.Message == "") {
		writeError(w, http.StatusBadRequest, "sessionId and message (or attachments) are required")
		return
	}
	queued := s.enqueueMessage(ws(r).ID, req, req.ClientMsgID)
	writeJSON(w, http.StatusOK, map[string]any{"queued": queued, "clientMsgId": req.ClientMsgID})
}

// handleCancelQueued drops a not-yet-dispatched message from a session's queue.
func (s *Server) handleCancelQueued(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	msgID := r.PathValue("msgId")
	if sessionID == "" || msgID == "" {
		writeError(w, http.StatusBadRequest, "session id and message id required")
		return
	}
	removed := s.cancelQueued(sessionID, msgID)
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}

// handleClearQueue drops every waiting message from a session's queue.
func (s *Server) handleClearQueue(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": s.clearQueued(sessionID)})
}

// handleMoveQueuedFront promotes a waiting message to dispatch next.
func (s *Server) handleMoveQueuedFront(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	msgID := r.PathValue("msgId")
	if sessionID == "" || msgID == "" {
		writeError(w, http.StatusBadRequest, "session id and message id required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"moved": s.moveQueuedToFront(sessionID, msgID)})
}

type sessionControlReq struct {
	Action string `json:"action"` // "stop" | "steer"
	Text   string `json:"text"`
}

// handleSessionControl stops or steers a session's in-flight turn WITHOUT the
// client needing the runId — the queue runs turns server-side, so control is
// session-scoped now. It resolves the session's live run and forwards to it.
func (s *Server) handleSessionControl(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	req, ok := bindJSON[sessionControlReq](w, r)
	if !ok {
		return
	}
	info, live := s.runs.sessionRunInfo(sessionID)
	if !live {
		writeError(w, http.StatusNotFound, "no in-flight turn for this session")
		return
	}
	run := s.runs.get(info.RunID)
	if run == nil {
		writeError(w, http.StatusNotFound, "run already finished")
		return
	}
	switch req.Action {
	case "stop":
		run.cancel()
	case "steer":
		if req.Text == "" {
			writeError(w, http.StatusBadRequest, "steer text is required")
			return
		}
		// Native providers drain the steer CHANNEL between tool-loop iterations
		// (see agent/toolloop.go drainSteer). claude-cli runs its own subprocess
		// loop with no such drain point, so instead stash the guidance on the run;
		// it is delivered at the next tool boundary as the Interaction MCP permission
		// tool's additionalContext (see callPermission). If the turn ends with no
		// tool call, runChatTurn enqueues the leftover as the next message
		// (steer_undelivered fallback).
		if run.providerOf() == "claude-cli" {
			run.setSteer(req.Text)
			writeJSON(w, http.StatusOK, map[string]string{"result": "steered"})
			return
		}
		select {
		case run.steer <- req.Text:
		default:
		}
	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+req.Action)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}
