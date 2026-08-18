package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/sessionhub"
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
	// Attempts counts how many times the serial worker has dispatched this turn.
	// It advances the instant the head is popped into the in-flight slot (before the
	// turn runs), so a turn that wedges the process on every boot is eventually
	// dropped by the poison guard instead of re-running forever. See _Docs/58.
	Attempts int `json:"attempts,omitempty"`
}

// sessionInbox is one session's FIFO command queue plus its serial-worker flag.
// Everything a session submits — user turns while a turn is already running —
// funnels through here so exactly one turn runs at a time, in order.
type sessionInbox struct {
	items []inboxItem
	// inflight is the head the worker has popped and is currently running. It is
	// held here (and persisted separately from the WAITING tail) until the turn
	// completes, so a mid-turn process death — even a hard kill before the turn
	// writes its own inflight sidecar — still re-dispatches this exact turn at boot
	// instead of losing it. nil when no turn is in flight.
	inflight *inboxItem
	seen     map[string]bool // clientMsgId dedupe (idempotent enqueue)
	running  bool            // a worker goroutine is draining this queue
	// closing freezes the serial worker during session teardown: it stops popping NEW
	// turns (a turn already in flight is stopped explicitly) WITHOUT discarding the
	// queued tail, so a delete that aborts (a subprocess that would not die) can unfreeze
	// and resume instead of losing the user's queued messages. See session_teardown.go.
	closing bool
	wsID    string // owning workspace (for persistence + dispatch)
}

// inboxStore holds every session's queue. Server-wide (like chatRuns), so it is
// keyed by scopeKey(workspaceID, sessionID) — NOT by session id alone: ids are
// allocated per workspace store, so "SES1" exists in every workspace and a bare
// key made two different sessions share one queue (and, through the shared entry's
// wsID, persisted one workspace's queue into another's store).
type inboxStore struct {
	mu       sync.Mutex
	sessions map[string]*sessionInbox
}

func newInboxStore() *inboxStore {
	return &inboxStore{sessions: make(map[string]*sessionInbox)}
}

func (i *inboxStore) lock()   { i.mu.Lock() }
func (i *inboxStore) unlock() { i.mu.Unlock() }

// at returns a session's queue, or nil when it has none. Caller holds the lock.
func (i *inboxStore) at(wsID, sessionID string) *sessionInbox {
	return i.sessions[scopeKey(wsID, sessionID)]
}

// enqueueMessage adds a turn to a session's queue (idempotent on clientMsgID),
// persists it, broadcasts the new queue to every window, and kicks the serial
// worker. Returns false when the id was already seen (a duplicate submit).
func (s *Server) enqueueMessage(wsID string, req chatReq, clientMsgID string) bool {
	if clientMsgID == "" {
		clientMsgID = uuid.NewString()
	}
	// Carry the resolved id on the request too, so the worker's runChatTurn stamps it
	// onto this turn's terminal hub events — the correlation a /chat + /chat/stream
	// queue observer uses to recognise its own turn's completion (chat_queue.go).
	req.ClientMsgID = clientMsgID
	s.inbox.lock()
	ib := s.inbox.at(wsID, req.SessionID)
	if ib == nil {
		ib = &sessionInbox{seen: make(map[string]bool), wsID: wsID}
		s.inbox.sessions[scopeKey(wsID, req.SessionID)] = ib
	}
	if ib.seen[clientMsgID] {
		s.inbox.unlock()
		return false
	}
	ib.seen[clientMsgID] = true
	ib.items = append(ib.items, inboxItem{ClientMsgID: clientMsgID, Req: req, WorkspaceID: wsID, EnqueuedAt: time.Now().Unix()})
	s.inbox.unlock()
	s.flushInbox(wsID, req.SessionID)
	s.kickInbox(wsID, req.SessionID)
	return true
}

// enqueueMessageFront enqueues a turn and immediately promotes it to the head of
// the WAITING queue — for a message that is older than whatever is already queued
// (the steer_undelivered fallback: guidance the user typed to redirect the turn
// that just ended). Best effort by construction: if the worker dispatches the queue
// between the two steps the promotion simply finds nothing to move.
func (s *Server) enqueueMessageFront(wsID string, req chatReq) {
	clientMsgID := uuid.NewString()
	if s.enqueueMessage(wsID, req, clientMsgID) {
		s.moveQueuedToFront(wsID, req.SessionID, clientMsgID)
	}
}

// withInbox runs fn under the inbox lock against a session's queue and, when fn
// reports a change, flushes the new state (durable persist + hub broadcast) from a
// single fresh snapshot. fn must ONLY mutate the passed inbox — it must not persist,
// publish, or re-lock. Returns what fn returned (false when the session has no
// queue, so fn never ran). Centralises the lock/mutate/flush dance every queue
// mutator shares.
func (s *Server) withInbox(wsID, sessionID string, fn func(*sessionInbox) bool) bool {
	s.inbox.lock()
	ib := s.inbox.at(wsID, sessionID)
	changed := false
	if ib != nil {
		changed = fn(ib)
	}
	s.inbox.unlock()
	if changed {
		s.flushInbox(wsID, sessionID)
	}
	return changed
}

// cancelQueued removes a WAITING message from a session's queue (the in-flight
// head is already popped, so a running turn is unaffected — that is what stop is
// for). Returns whether anything was removed.
func (s *Server) cancelQueued(wsID, sessionID, clientMsgID string) bool {
	var dropped []inboxItem
	removed := s.withInbox(wsID, sessionID, func(ib *sessionInbox) bool {
		for idx, it := range ib.items {
			if it.ClientMsgID == clientMsgID {
				dropped = append(dropped, it)
				ib.items = append(ib.items[:idx:idx], ib.items[idx+1:]...)
				return true
			}
		}
		return false
	})
	// The cancelled turn's uploads are now unreachable — remove them from disk.
	s.purgeQueuedAttachments(wsID, dropped)
	return removed
}

// clearQueued drops ALL waiting messages from a session's queue (the in-flight
// head is already popped, so a running turn is unaffected). Returns how many were
// removed.
func (s *Server) clearQueued(wsID, sessionID string) int {
	n := 0
	var dropped []inboxItem
	s.withInbox(wsID, sessionID, func(ib *sessionInbox) bool {
		n = len(ib.items)
		if n == 0 {
			return false
		}
		dropped = append(dropped, ib.items...)
		ib.items = nil
		return true
	})
	s.purgeQueuedAttachments(wsID, dropped)
	return n
}

// moveQueuedToFront promotes a waiting message to the head of the queue so it
// dispatches next ("send next"). Returns whether it was found + moved.
func (s *Server) moveQueuedToFront(wsID, sessionID, clientMsgID string) bool {
	return s.withInbox(wsID, sessionID, func(ib *sessionInbox) bool {
		for idx, it := range ib.items {
			if it.ClientMsgID == clientMsgID {
				ib.items = append(ib.items[:idx:idx], ib.items[idx+1:]...)
				ib.items = append([]inboxItem{it}, ib.items...)
				return true
			}
		}
		return false
	})
}

// kickInbox starts the serial worker for a session if one is not already running.
func (s *Server) kickInbox(wsID, sessionID string) {
	s.inbox.lock()
	ib := s.inbox.at(wsID, sessionID)
	if ib == nil || ib.running || ib.closing || len(ib.items) == 0 {
		s.inbox.unlock()
		return
	}
	ib.running = true
	s.inbox.unlock()
	go s.runInboxWorker(wsID, sessionID)
}

// runInboxWorker drains a session's queue one turn at a time. Each lap it first
// claims the session's RUNTIME turn slot, then pops the head into the durable
// in-flight slot (so the persisted queue keeps that turn until it completes AND
// shows the remaining WAITING tail), runs it under a watchdog, clears the slot and
// releases the runtime slot, then loops. A crash while the head runs is reclaimed
// at boot from the in-flight slot; the WAITING tail stays durable in inbox.json.
func (s *Server) runInboxWorker(wsID, sessionID string) {
	for {
		if s.endInboxDrain(wsID, sessionID) {
			return
		}
		// Claim the runtime turn slot BEFORE popping. That slot is the session's other
		// serializer — a coordinator auto-turn, a worker notification, a scheduler wake
		// all hold it without passing through this queue — and the turn we are about to
		// dispatch blocks on it anyway (runChatTurn). Popping first meant the message
		// left the WAITING tail (vanishing from every window's queue tray) and then
		// waited, invisible and no longer cancellable, until the coordinator released
		// the slot — surfacing in the transcript only when some worker's reply happened
		// to end that turn. Waiting HERE keeps it WAITING until it can really run.
		// See _Docs/58-QUEUE-SENKRON.md.
		release := s.claimRuntimeTurn(wsID, sessionID)
		item, ok := s.popInboxHead(wsID, sessionID)
		if !ok {
			// Drained or frozen while we waited for the runtime slot.
			release()
			continue
		}
		// Persist with the head moved into the durable in-flight slot: if the process
		// dies mid-turn (even a hard kill before the turn writes its own inflight
		// sidecar), boot re-dispatches this exact item instead of losing it.
		s.flushInbox(wsID, sessionID)

		// Poison guard: a turn that has already wedged the process too many times is
		// dropped (with a visible turn_error) so it can never block the queue forever.
		if item.Attempts > maxInboxAttempts {
			release()
			s.dropPoisonedInflight(wsID, sessionID, item)
			continue
		}

		// The queue is keyed by workspace, so the item's workspace IS this worker's —
		// resolve it strictly (no default-workspace fallback: dispatching a turn into
		// the wrong store is worse than not running it, and shows up as an error).
		wsp := s.workspaceByID(wsID)
		if wsp == nil {
			release()
			s.clearInflight(wsID, sessionID)
			continue
		}
		// The turn slot is already ours: tell runChatTurn not to claim it again (it
		// would deadlock behind itself).
		item.Req.turnSlotHeld = true
		// Server-driven turn: no single owning client, so clientGone never fires —
		// an interactive ask_user waits for an answer from ANY window (via the
		// interaction CAS) instead of bailing. All UI rides the hub (write=nil).
		// runQueuedTurn adds a panic barrier AND a watchdog so ONE bad turn can never
		// kill the worker goroutine or wedge the queue by never returning.
		s.runQueuedTurn(wsp, sessionID, item.Req)
		release()
		s.clearInflight(wsID, sessionID)
	}
}

// endInboxDrain ends the serial drain when the session's queue is empty (or frozen
// by a teardown), clearing the running flag under the SAME lock that observes the
// emptiness so a message enqueued in that instant can never find running=true with
// no worker left to drain it. Returns true when the worker should exit.
func (s *Server) endInboxDrain(wsID, sessionID string) bool {
	s.inbox.lock()
	ib := s.inbox.at(wsID, sessionID)
	if ib != nil && !ib.closing && len(ib.items) > 0 {
		s.inbox.unlock()
		return false
	}
	if ib != nil {
		ib.running = false
		// closing means a teardown froze the queue: leave items + inflight + the
		// dedupe set intact so an aborted delete can resume exactly where it
		// paused. Only a genuine drain (empty queue) resets them.
		if !ib.closing {
			ib.inflight = nil
			// Queue drained: reset the dedupe set so it can't grow without bound
			// across a long-lived session (a re-submit of an old id after this is a
			// genuinely new turn).
			ib.seen = make(map[string]bool)
		}
	}
	s.inbox.unlock()
	s.flushInbox(wsID, sessionID)
	return true
}

// popInboxHead moves the head of a session's queue into the durable in-flight slot
// and returns it. Reports false when the queue emptied or froze meanwhile (the
// worker waits for the runtime turn slot between the two, so the queue it saw is
// not necessarily the queue it pops from). Attempts advances here — the instant the
// turn is committed to run — so the poison guard counts dispatches, not waits.
func (s *Server) popInboxHead(wsID, sessionID string) (inboxItem, bool) {
	s.inbox.lock()
	defer s.inbox.unlock()
	ib := s.inbox.at(wsID, sessionID)
	if ib == nil || ib.closing || len(ib.items) == 0 {
		return inboxItem{}, false
	}
	item := ib.items[0]
	ib.items = ib.items[1:]
	item.Attempts++
	inflight := item
	ib.inflight = &inflight
	return item, true
}

// claimRuntimeTurn blocks until the session's runtime turn slot is free and claims
// it as a user turn, returning the release. A session whose workspace/runtime can't
// be resolved (a store that vanished) yields a no-op release rather than wedging the
// queue — the turn then fails its own way, visibly.
func (s *Server) claimRuntimeTurn(wsID, sessionID string) func() {
	wsp := s.workspaceByID(wsID)
	if wsp == nil || wsp.Runtime == nil {
		return func() {}
	}
	return wsp.Runtime.BeginSessionUserTurn(sessionID)
}

// queueView is the client-facing shape of one waiting queue entry.
type queueView struct {
	ClientMsgID string `json:"clientMsgId"`
	Text        string `json:"text"`
	EnqueuedAt  int64  `json:"enqueuedAt"`
}

// flushInbox persists the session's durable queue (in-flight head + WAITING tail)
// to its inbox.json sidecar AND broadcasts the current WAITING queue to every
// window — both derived from ONE locked snapshot. The old persist-then-publish pair
// took the inbox lock twice, so another goroutine could mutate the queue between the
// two critical sections and leave the bytes on disk disagreeing with the queue shown
// in every window. Snapshotting once closes that gap; both I/O steps still run
// outside the lock. An empty queue clears the sidecar.
func (s *Server) flushInbox(wsID, sessionID string) {
	s.inbox.lock()
	ib := s.inbox.at(wsID, sessionID)
	var snapshot persistedInbox
	view := make([]queueView, 0)
	inflightID := ""
	var inflightView *queueView
	if ib != nil {
		snapshot.Items = append([]inboxItem{}, ib.items...)
		if ib.inflight != nil {
			cp := *ib.inflight
			snapshot.Inflight = &cp
			inflightID = ib.inflight.ClientMsgID
			inflightView = &queueView{
				ClientMsgID: cp.ClientMsgID,
				Text:        cp.Req.Message,
				EnqueuedAt:  cp.EnqueuedAt,
			}
		}
		for _, it := range ib.items {
			view = append(view, queueView{ClientMsgID: it.ClientMsgID, Text: it.Req.Message, EnqueuedAt: it.EnqueuedAt})
		}
	}
	s.inbox.unlock()

	// The sidecar goes to the queue's OWN workspace store, resolved strictly: the
	// old default-workspace fallback could write one workspace's queue into another
	// workspace's session folder (the ids collide).
	wsp := s.workspaceByID(wsID)
	if wsp != nil && wsp.DB != nil {
		if snapshot.Inflight == nil && len(snapshot.Items) == 0 {
			_ = wsp.DB.ClearInbox(sessionID)
		} else if data, err := json.Marshal(snapshot); err == nil {
			_ = wsp.DB.WriteInbox(sessionID, data)
		}
	}
	s.publishQueue(wsID, sessionID, view, inflightID, inflightView)
}

// publishQueue broadcasts a session's complete turn queue to every window: the
// user's own durable send-queue (staged / dispatched) PLUS the runtime admission
// queue (what actually holds the session and what is queued behind it — a
// coordinator's worker notification, a wake, a peer delivery). The two used to be
// invisible to each other, which is how a dispatched message could sit waiting on
// something the UI never showed (_Docs/58).
//
// It is called from flushInbox (queue mutated) AND from the admission-queue
// observer bridge (runtime state changed), so the view stays live either way. The
// runtime part is re-read here rather than carried on the event, so a burst of
// changes coalesces into one read.
func (s *Server) publishQueue(wsID, sessionID string, view []queueView, inflightID string, inflightView *queueView) {
	payload := map[string]any{
		// inflightClientMsgId lets a queue observer (the /chat + /chat/stream handlers)
		// tell "my message was dispatched and is running" (== inflight) from "my message
		// was cancelled/cleared before running" (absent from both) — the latter never
		// produces a terminal event, so the observer must close instead of hanging.
		// inflight carries its TEXT too, so the UI can keep showing the dispatched
		// message (as "gönderiliyor") in the tray until its user bubble appears in the
		// transcript — no window where the message is nowhere to be seen.
		"queue":               view,
		"inflightClientMsgId": inflightID,
		"inflight":            inflightView,
	}
	if wsp := s.workspaceByID(wsID); wsp != nil && wsp.Runtime != nil {
		snap := wsp.Runtime.TurnQueue().Snapshot(sessionID)
		payload["turns"] = snap
	}
	s.publishHub(wsID, sessionID, sessionhub.KindQueueUpdate, payload, false)
}

// republishQueue re-broadcasts a session's queue view from the CURRENT inbox state
// without touching the disk sidecar — the admission-queue observer path, where only
// the runtime half changed. Splitting this from flushInbox keeps a per-turn-boundary
// event from rewriting inbox.json every time.
func (s *Server) republishQueue(wsID, sessionID string) {
	s.inbox.lock()
	ib := s.inbox.at(wsID, sessionID)
	view := make([]queueView, 0)
	inflightID := ""
	var inflightView *queueView
	if ib != nil {
		if ib.inflight != nil {
			cp := *ib.inflight
			inflightID = cp.ClientMsgID
			inflightView = &queueView{ClientMsgID: cp.ClientMsgID, Text: cp.Req.Message, EnqueuedAt: cp.EnqueuedAt}
		}
		for _, it := range ib.items {
			view = append(view, queueView{ClientMsgID: it.ClientMsgID, Text: it.Req.Message, EnqueuedAt: it.EnqueuedAt})
		}
	}
	s.inbox.unlock()
	s.publishQueue(wsID, sessionID, view, inflightID, inflightView)
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
			pi := decodeInbox(data)
			// The in-flight head was interrupted by the crash/restart before it
			// completed. Put it back at the FRONT so it runs first, right where the
			// worker left off; its Attempts counter already advanced, so the poison
			// guard eventually gives up on a turn that wedges the process every boot.
			items := pi.Items
			if pi.Inflight != nil {
				items = append([]inboxItem{*pi.Inflight}, items...)
			}
			if len(items) == 0 {
				continue
			}
			s.inbox.lock()
			ib := &sessionInbox{seen: make(map[string]bool), wsID: wsp.ID}
			for _, it := range items {
				// A sidecar written before queues were workspace-scoped may carry a
				// foreign WorkspaceID (the shared-entry bug): the store the file lives
				// in is authoritative, so re-stamp it here rather than dispatching the
				// turn into another workspace on this boot.
				it.WorkspaceID = wsp.ID
				ib.items = append(ib.items, it)
				ib.seen[it.ClientMsgID] = true
			}
			s.inbox.sessions[scopeKey(wsp.ID, sess.ID)] = ib
			s.inbox.unlock()
			// Rewrite the sidecar in the current object shape (in-flight slot cleared,
			// the reclaimed head now a normal WAITING entry) so a second crash before
			// dispatch doesn't double-count it as both in-flight and waiting.
			s.flushInbox(wsp.ID, sess.ID)
			s.kickInbox(wsp.ID, sess.ID)
		}
	}
}

// recoverAutonomousTurns reclaims worker/spawn/coordinator turns orphaned by a
// crash/restart, per workspace. Unlike inbox turns (durably queued), autonomous
// turns run as fire-and-forget goroutines with no sidecar, so a restart leaves the
// worker session with no reply AND its coordinator waiting forever. The per-runtime
// pass records an interrupted reply and, for a worker, injects a synthetic killed
// notification so the coordinator resumes. Best-effort; runs once at startup.
func (s *Server) recoverAutonomousTurns() {
	for _, meta := range s.workspaces.List() {
		wsp, err := s.workspaces.Get(meta.ID)
		if err != nil || wsp == nil || wsp.Runtime == nil {
			continue
		}
		wsp.Runtime.RecoverOrphanedTurns(context.Background())
		// AFTER the orphan pass, never before: a sub-coordinator that still owes its
		// coordinator a report usually gets one more turn out of that pass (its
		// recovered workers notify it), and should report for itself. Re-arming the
		// backstop here lets it do exactly that — the backstop only fires for a node
		// that is still quiet once the grace period elapses.
		wsp.Runtime.RecoverPendingReports(context.Background())
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
	if queued {
		// Instant feedback: name a still-untitled session from a snippet of this
		// prompt right away, so the UI drops "new chat" without waiting for the
		// first-turn LLM auto-title. Best-effort — never blocks the reply.
		s.maybeSnippetTitle(r.Context(), ws(r), req.SessionID, req.Message)
	}
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
	removed := s.cancelQueued(ws(r).ID, sessionID, msgID)
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}

// handleClearQueue drops every waiting message from a session's queue.
func (s *Server) handleClearQueue(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": s.clearQueued(ws(r).ID, sessionID)})
}

// handleMoveQueuedFront promotes a waiting message to dispatch next.
func (s *Server) handleMoveQueuedFront(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	msgID := r.PathValue("msgId")
	if sessionID == "" || msgID == "" {
		writeError(w, http.StatusBadRequest, "session id and message id required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"moved": s.moveQueuedToFront(ws(r).ID, sessionID, msgID)})
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
	info, live := s.runs.sessionRunInfo(ws(r).ID, sessionID)
	var run *chatRun
	if live {
		run = s.runs.get(info.RunID)
	}
	if run == nil {
		// No chat run: the turn may still be an AUTONOMOUS one (schedule / wake /
		// spawn / coordination), which the runtime tracks separately and which never
		// enters chatRuns. "stop" can cancel those; "steer" has no such channel.
		if req.Action == "stop" && ws(r).Runtime.CancelSession(sessionID) {
			// A cancelled autonomous turn leaves no trace of WHY it stopped — the
			// context just dies. Record the cause in the transcript.
			s.recordAutonomousStop(ws(r), sessionID)
			writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
			return
		}
		if !live {
			writeError(w, http.StatusNotFound, "no in-flight turn for this session")
			return
		}
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
		// (see agent/toolloop.go drainSteer). CLI providers (claude-cli, codex-cli)
		// run their own subprocess loop with no such drain point, so instead stash
		// the guidance on the run; on claude-cli it is delivered at the next tool
		// boundary as the Interaction MCP permission tool's additionalContext (see
		// callPermission). If the turn ends with no tool call, runChatTurn enqueues
		// the leftover as the next message (steer_undelivered fallback).
		if run.providerOf() == "claude-cli" || run.providerOf() == "codex-cli" {
			// In "auto" (bypass) mode the CLI never calls the permission-prompt tool,
			// so there is no boundary to carry the steer. codex has NO such boundary in
			// ANY mode (see steerableForTurn), so it always falls into this branch. — it would only surface at
			// turn end as a re-queued message. Tell the client it's unsupported so it
			// queues the message and shows a hint, instead of us pretending it landed.
			if !run.steerableFor() {
				writeJSON(w, http.StatusOK, map[string]string{"result": "unsupported"})
				return
			}
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
