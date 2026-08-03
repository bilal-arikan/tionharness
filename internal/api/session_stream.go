package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/sessionhub"
)

// handleSessionStream is the single server-authoritative event stream every
// window watching a session subscribes to (SSE). It replaces the old
// owner-streams-its-own-SSE + non-owner-polls-inflight split: a window opens
// this once and renders the transcript live from it, whoever started the turn.
//
// Query params:
//
//	since  the client's cursor (last durable seq it has applied); the endpoint
//	       replays durable events with seq > since before going live.
//	epoch  the boot-epoch the cursor belongs to; a mismatch (server restarted)
//	       makes the endpoint send a `reset` so the client does a full resync
//	       (listMessages) instead of trusting a seq that reset to zero.
//
// SSE frames:
//
//	hello → { epoch, head, now }        (once, first; now = server unix seconds)
//	reset → { head }                    (when the cursor is unusable → client resyncs)
//	hub   → a sessionhub.Event          (durable carry `id: <seq>`; ephemeral seq 0)
//
// The stream stays open until the client disconnects; a ping keeps proxies warm.
func (s *Server) handleSessionStream(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	if s.hub == nil {
		writeError(w, http.StatusServiceUnavailable, "session hub unavailable")
		return
	}
	// Confirm the session exists in this workspace before opening a long-lived
	// stream (fail fast with a normal JSON 404 rather than a dangling SSE).
	if _, err := ws(r).DB.GetSession(r.Context(), sessionID); err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	since := int64(0)
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			since = n
		}
	}
	clientEpoch := r.URL.Query().Get("epoch")

	subID, ch, head := s.hub.Subscribe(sessionID)
	defer func() {
		s.hub.Unsubscribe(sessionID, subID)
		// Presence dropped by one: tell the remaining windows.
		s.publishPresence(sessionID)
	}()
	// Presence: this window just joined — broadcast the new viewer count so every
	// window can show "open in N windows" / "another window is answering".
	s.publishPresence(sessionID)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	writeFrame := func(name string, seq int64, data any) {
		b, _ := json.Marshal(data)
		if seq > 0 {
			fmt.Fprintf(w, "event: %s\nid: %d\ndata: %s\n\n", name, seq, b)
		} else {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
		}
		flusher.Flush()
	}

	// Hello: tell the client the current epoch + live head so it can align its
	// cursor and detect a restarted server. `now` is this server's wall clock, so
	// the client can calibrate its elapsed-time counters against OUR clock before
	// the first hub frame arrives (turn start stamps are server-side).
	writeFrame("hello", 0, map[string]any{
		"epoch": s.hub.Epoch(),
		"head":  head,
		"now":   time.Now().Unix(),
	})

	// Gap-fill from the cursor, or ask the client to resync when it can't be trusted.
	if clientEpoch != "" && clientEpoch != s.hub.Epoch() {
		writeFrame("reset", 0, map[string]any{"head": head})
	} else if replay, okReplay := s.hub.Replay(sessionID, since); okReplay {
		for _, ev := range replay {
			writeFrame("hub", ev.Seq, ev)
		}
	} else {
		writeFrame("reset", 0, map[string]any{"head": head})
	}

	// Durable Ask restore: re-publish any still-waiting ask card for this session
	// (ephemeral → live to current subscribers, not added to the ring), so a card
	// survives a backend restart that cleared the in-memory hub. The disk row is the
	// source of truth. No-op when there are none.
	s.restoreWaitingAsks(ws(r), sessionID)

	ping := time.NewTicker(eventsPingInterval)
	defer ping.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			writeFrame("hub", ev.Seq, ev)
		}
	}
}

// publishStep is the server-side helper the chat turn uses to put one durable
// TurnStep onto a session's hub stream. step is the already-marshalled TurnStep
// JSON (opaque here). A no-op on a nil hub.
func (s *Server) publishStep(sessionID string, step json.RawMessage) {
	s.hub.Publish(sessionID, sessionhub.KindStep, step, false)
}

type typingReq struct {
	Active   bool   `json:"active"`
	ClientID string `json:"clientId"`
}

// handleTyping broadcasts a live "user is typing" signal to the OTHER windows
// viewing a session, as an ephemeral hub event (seq 0, not retained). The
// originating window tags its clientId so it can ignore its own echo. Feature:
// cross-window collaboration awareness (Faz 4+, _Docs/58).
func (s *Server) handleTyping(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	req, ok := bindJSON[typingReq](w, r)
	if !ok {
		return
	}
	s.hub.Publish(sessionID, sessionhub.KindTyping, mustJSON(map[string]any{
		"active":   req.Active,
		"clientId": req.ClientID,
	}), true)
	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}

// publishPresence broadcasts the current live viewer count for a session as an
// ephemeral hub event (seq 0, not retained) — the raw signal behind the
// "open in N windows" / "another window is answering" UI (Faz 4).
func (s *Server) publishPresence(sessionID string) {
	s.hub.Publish(sessionID, sessionhub.KindPresence, mustJSON(map[string]any{
		"count": s.hub.SubscriberCount(sessionID),
	}), true)
}

// mustJSON marshals v, returning null on error (never panics — presence/telemetry
// payloads must not break a stream).
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// publishHub marshals v and publishes it under kind. ephemeral events (delta,
// typing) carry seq 0 and are not retained for replay. A no-op on a nil hub or
// a marshal error.
func (s *Server) publishHub(sessionID, kind string, v any, ephemeral bool) {
	// A completed reply carries the turn's whole activity trace. Trim its
	// oversized tool payloads the same way the transcript listing does, so a
	// turn does not render one way live and a shorter way after a reload — and
	// so the fan-out to every open window stays small. Only the wire copy is
	// trimmed; the persisted message keeps the full trace.
	if kind == sessionhub.KindReply {
		if m, ok := v.(db.Message); ok {
			m.Steps = trimStepsJSON(m.Steps)
			v = m
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	s.hub.Publish(sessionID, kind, b, ephemeral)
}

// bridgeBusToHub mirrors AUTONOMOUS turns' live steps from the process-wide bus
// onto the per-session hub, so a window watching an autonomous session
// (scheduler/spawn/worker/wake) renders its activity from the same authoritative
// stream as an interactive turn. Interactive turns already publish to the hub
// directly, in order, from the chat stream handler — so those are skipped here
// (a session with a registered chatRun) to avoid double-publishing. Runs for the
// server's lifetime; the subscription is never torn down.
func (s *Server) bridgeBusToHub() {
	if s.bus == nil || s.hub == nil {
		return
	}
	_, ch := s.bus.Subscribe()
	for e := range ch {
		sid := e.Target["sessionId"]
		if sid == "" {
			continue
		}
		switch e.Type {
		case "session_step":
			if len(e.Step) == 0 {
				continue
			}
			// An INTERACTIVE turn already published this step to the hub in-order
			// (runChatTurn onStep → publishHub KindStep); its bus copy is tagged
			// origin=interactive purely so we can drop it here. Re-bridging it would
			// double-publish — and, worse, a straggler bus step processed AFTER the
			// run unregisters (defer) would re-arm the client's "conversing" indicator
			// that turn_done just cleared (stuck-live bug). Skip interactive steps
			// outright, regardless of run liveness. Only AUTONOMOUS turns
			// (coordinator/scheduler/spawn/worker/wake) reach the hub via this bridge:
			// their runtime onStep does NOT self-publish, and many carry no chatRun at
			// all, so liveness is not a reliable discriminator — origin is.
			if e.Target["origin"] == "interactive" {
				continue
			}
			s.hub.Publish(sid, sessionhub.KindStep, e.Step, false)
		case "session_user_message":
			// A runtime-injected user-role message (a worker task-notification, a
			// send_to_worker prompt, a coordination status/guard note) was just
			// persisted mid-autonomous-flow. Publish it to the hub as a durable
			// user_message so every window watching the coordinator/worker renders it
			// live and IN ORDER — the assistant reply that follows already bridges via
			// the completion case below, but without this the reply appeared to answer
			// a message the window never saw (looked like a duplicate reply out of
			// nowhere; only a reload restored order — _Docs/58). Unconditional (not
			// run-liveness gated): no interactive run publishes this injected message,
			// so there is nothing to double up with. Left UNcommitted here — the
			// following turn's turn_done commits it, and a fresh subscriber that joins
			// in between replays it (and dedupes by message id against listMessages).
			if len(e.Msg) == 0 {
				continue
			}
			s.hub.Publish(sid, sessionhub.KindUserMessage, e.Msg, false)
		case "chat", "spawned", "worker", "schedule", "flow", "automation":
			// An AUTONOMOUS turn (scheduler/spawn/worker/flow) finished: it publishes
			// no hub reply/turn_done of its own, so bridge a turn_done here — every
			// window watching the session then reloads the persisted turn. Skip wake
			// lifecycle phases (armed/start are not ends) and skip interactive turns
			// (their runChatTurn already emitted turn_done while the run was live).
			if ph := e.Target["phase"]; ph == "armed" || ph == "start" {
				continue
			}
			if _, live := s.runs.sessionRunInfo(sid); live {
				continue
			}
			// Symmetry with interactive turns: publish the persisted reply so windows
			// render it live, then turn_done. (Reload still backstops it.)
			s.publishAutonomousReply(sid, e.WorkspaceID)
			s.hub.Publish(sid, sessionhub.KindTurnDone, mustJSON(map[string]any{"sessionTitle": ""}), false)
			s.hub.Commit(sid)
		}
	}
}

// publishAutonomousReply pushes an autonomous turn's persisted final assistant
// message onto the hub as a reply, so a window watching the session renders it
// live (interactive turns already do this from runChatTurn). Best-effort: a no-op
// when the workspace/message can't be resolved or the last message isn't an
// assistant turn.
func (s *Server) publishAutonomousReply(sessionID, wsID string) {
	wsp := s.workspaces.Default()
	if wsID != "" {
		if w, err := s.workspaces.Get(wsID); err == nil {
			wsp = w
		}
	}
	if wsp == nil || wsp.DB == nil {
		return
	}
	msgs, err := wsp.DB.ListMessages(context.Background(), sessionID)
	if err != nil || len(msgs) == 0 {
		return
	}
	last := msgs[len(msgs)-1]
	if last.Role != "assistant" {
		return
	}
	s.publishHub(sessionID, sessionhub.KindReply, last, false)
}
