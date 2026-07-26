package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/sessionhub"
)

// chat_queue.go holds the two LEGACY chat entry points — POST /api/chat/stream
// (SSE) and POST /api/chat (JSON) — after the durable cutover (_Docs/58). Neither
// runs a turn inline anymore: both ENQUEUE onto the session's serial send-queue
// (persisted to inbox.json, crash-recoverable, never concurrent with another turn
// on the session) exactly like POST /sessions/{id}/messages, then observe the
// per-session hub for THIS turn's completion. The streaming handler relays the hub
// back in the legacy SSE frame shape so old clients / external automation still get
// a streamed reply; the JSON handler waits for the terminal event and returns the
// persisted reply. The frontend uses neither endpoint (it enqueues + renders from
// the hub directly), so these serve external automation (Doc 33) only.
//
// Correlation: each handler forces a FRESH clientMsgId (so the enqueue is never
// deduped as a duplicate) and closes on the terminal hub event (turn_done /
// turn_error) that carries that id. runChatTurn and the queue durability barriers
// stamp clientMsgId onto their terminal events for exactly this purpose.

// handleChatStream enqueues a turn durably, then relays the session hub back to the
// caller as legacy SSE frames until this turn's terminal event arrives. A client
// disconnect stops the relay but the turn keeps running durably in the worker.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[chatReq](w, r)
	if !ok {
		return
	}
	if req.SessionID == "" || (strings.TrimSpace(req.Message) == "" && len(req.Attachments) == 0) {
		writeError(w, http.StatusBadRequest, "sessionId and message (or attachments) are required")
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
	wsp := ws(r)

	clientMsgID := uuid.NewString()
	req.ClientMsgID = clientMsgID

	// Subscribe BEFORE enqueue so we cannot miss our turn's first frames.
	subID, ch, _ := s.hub.Subscribe(req.SessionID)
	defer s.hub.Unsubscribe(req.SessionID, subID)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	write := func(event string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	if !s.enqueueMessage(wsp.ID, req, clientMsgID) {
		write("error", map[string]any{"error": "could not enqueue message", "reason": "enqueue_failed"})
		return
	}

	ping := time.NewTicker(eventsPingInterval)
	defer ping.Stop()
	ctx := r.Context()
	lastSeq := int64(0)
	sawSelf := false
	frame := func(e sessionhub.Event) bool {
		if e.Kind == sessionhub.KindQueueUpdate {
			// Cancel detection: once we've seen our message live in the queue, its later
			// disappearance (without a terminal event) means another window cancelled or
			// cleared it — it will never run, so close instead of hanging.
			if queueHasMsg(e.Payload, clientMsgID) {
				sawSelf = true
			} else if sawSelf {
				write("error", map[string]any{"error": "message was cancelled from the queue", "reason": "cancelled", "clientMsgId": clientMsgID})
				return true
			}
			return false
		}
		return relayLegacyFrame(write, e, clientMsgID)
	}
	for {
		select {
		case <-ctx.Done():
			// Client navigated away; the turn keeps running durably in the worker.
			return
		case <-ping.C:
			// Safety net for a dropped TERMINAL frame: it is the last event of the turn,
			// so a fan-out drop leaves no later event to trigger gap detection — the
			// relay would hang. Reconcile any unseen durable events from the ring here.
			if lastSeq > 0 {
				if done := s.drainReplay(req.SessionID, &lastSeq, frame); done {
					return
				}
			}
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			// Gap-fill: the hub's non-blocking fan-out drops a frame to a slow consumer
			// rather than stalling — which could skip our terminal turn_done and hang the
			// relay. On a seq gap, replay the missed durable events from the ring first.
			if done := s.relayGapThenEvent(req.SessionID, ev, &lastSeq, frame); done {
				return
			}
		}
	}
}

// handleChat enqueues a turn durably, waits for THIS turn's terminal hub event,
// then returns the persisted assistant reply as JSON. A client disconnect leaves
// the turn running durably (nothing is returned).
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[chatReq](w, r)
	if !ok {
		return
	}
	if req.SessionID == "" || (strings.TrimSpace(req.Message) == "" && len(req.Attachments) == 0) {
		writeError(w, http.StatusBadRequest, "sessionId and message (or attachments) are required")
		return
	}
	if s.hub == nil {
		writeError(w, http.StatusServiceUnavailable, "session hub unavailable")
		return
	}
	wsp := ws(r)
	database := wsp.DB
	// Fail fast with a normal 404 before enqueuing anything.
	if _, err := database.GetSession(r.Context(), req.SessionID); err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	clientMsgID := uuid.NewString()
	req.ClientMsgID = clientMsgID
	subID, ch, _ := s.hub.Subscribe(req.SessionID)
	defer s.hub.Unsubscribe(req.SessionID, subID)
	if !s.enqueueMessage(wsp.ID, req, clientMsgID) {
		writeError(w, http.StatusInternalServerError, "could not enqueue message")
		return
	}

	ctx := r.Context()
	lastSeq := int64(0)
	var lastReply json.RawMessage
	sawSelf := false
	process := func(ev sessionhub.Event) (done bool) {
		switch ev.Kind {
		case sessionhub.KindQueueUpdate:
			// Cancel detection: our message vanishing from the queue after we've seen it
			// live (and with no terminal event) means another window cancelled/cleared it
			// before it ran — respond 409 instead of hanging.
			if queueHasMsg(ev.Payload, clientMsgID) {
				sawSelf = true
			} else if sawSelf {
				writeError(w, http.StatusConflict, "message was cancelled from the queue")
				return true
			}
		case sessionhub.KindReply:
			// Serial worker: the reply immediately preceding our turn_done is ours, so
			// return THIS payload rather than re-reading "the last assistant message"
			// from the DB (which a back-to-back next turn could already have overtaken).
			lastReply = ev.Payload
		case sessionhub.KindTurnDone:
			if payloadClientMsgID(ev.Payload) != clientMsgID {
				return false
			}
			s.writeChatReply(w, database, req.SessionID, ev.Payload, lastReply)
			return true
		case sessionhub.KindTurnError:
			if payloadClientMsgID(ev.Payload) != clientMsgID {
				return false
			}
			var p struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(ev.Payload, &p)
			detail := p.Error
			if detail == "" {
				detail = "turn failed"
			}
			writeError(w, http.StatusBadGateway, detail)
			return true
		}
		return false
	}
	// A ping ticker doubles as the dropped-terminal safety net (see drainReplay): a
	// non-stream client sends no data, so the ticks are internal only.
	ping := time.NewTicker(eventsPingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			// Client gone; the turn still completes durably. Nothing left to return.
			return
		case <-ping.C:
			if lastSeq > 0 {
				if done := s.drainReplay(req.SessionID, &lastSeq, process); done {
					return
				}
			}
		case ev, ok := <-ch:
			if !ok {
				writeError(w, http.StatusInternalServerError, "stream closed before reply")
				return
			}
			if done := s.relayGapThenEvent(req.SessionID, ev, &lastSeq, process); done {
				return
			}
		}
	}
}

// relayGapThenEvent applies fn to ev, first replaying any durable events missed
// since *lastSeq (a slow-consumer drop in the hub's non-blocking fan-out) so a
// terminal frame is never skipped. It advances *lastSeq and returns true as soon as
// fn signals completion (the terminal event was handled). Ephemeral events (seq 0)
// are passed straight through.
func (s *Server) relayGapThenEvent(sessionID string, ev sessionhub.Event, lastSeq *int64, fn func(sessionhub.Event) bool) bool {
	if ev.Seq > 0 && *lastSeq > 0 && ev.Seq > *lastSeq+1 {
		if replay, ok := s.hub.Replay(sessionID, *lastSeq); ok {
			for _, rev := range replay {
				if rev.Seq <= *lastSeq || rev.Seq >= ev.Seq {
					continue
				}
				if fn(rev) {
					return true
				}
				*lastSeq = rev.Seq
			}
		}
	}
	if ev.Seq > 0 {
		*lastSeq = ev.Seq
	}
	return fn(ev)
}

// drainReplay pulls every durable event past *lastSeq from the ring and applies fn,
// advancing *lastSeq. It is the ping-tick safety net: the terminal frame (turn_done /
// turn_error) is usually the LAST event of a turn, so if the hub dropped it to a slow
// consumer there is no later event to trigger relayGapThenEvent's gap detection — the
// observer would hang forever. Reconciling from the ring on each tick recovers it.
// Returns true as soon as fn signals completion. A no-op when nothing new is retained.
func (s *Server) drainReplay(sessionID string, lastSeq *int64, fn func(sessionhub.Event) bool) bool {
	replay, ok := s.hub.Replay(sessionID, *lastSeq)
	if !ok {
		return false
	}
	for _, rev := range replay {
		if rev.Seq <= *lastSeq {
			continue
		}
		if fn(rev) {
			return true
		}
		*lastSeq = rev.Seq
	}
	return false
}

// relayLegacyFrame maps one hub event to the legacy SSE frame shape the old
// /chat/stream clients expect. It returns true (terminal) only for OUR turn's
// turn_done / turn_error — a frame from a turn queued ahead of us carries a
// different clientMsgId and is dropped rather than closing the stream early.
func relayLegacyFrame(write func(string, any), ev sessionhub.Event, myClientMsgID string) (done bool) {
	switch ev.Kind {
	case sessionhub.KindUserMessage:
		write("meta", map[string]any{"userMessage": ev.Payload})
	case sessionhub.KindAgentStart:
		write("agent", ev.Payload)
	case sessionhub.KindStep, sessionhub.KindDelta, sessionhub.KindToolDelta:
		write("step", ev.Payload)
	case sessionhub.KindReply:
		write("reply", map[string]any{"replyMessage": ev.Payload})
	case sessionhub.KindTurnDone:
		if payloadClientMsgID(ev.Payload) == myClientMsgID {
			write("done", ev.Payload)
			return true
		}
	case sessionhub.KindTurnError:
		if payloadClientMsgID(ev.Payload) == myClientMsgID {
			write("error", ev.Payload)
			return true
		}
	}
	return false
}

// writeChatReply builds the legacy non-stream chat response for a completed queued
// turn. It prefers the assistant reply carried by the hub KindReply event we observed
// for THIS turn (replyPayload); only if that is missing does it fall back to scanning
// the transcript for the last assistant message (a back-to-back next turn can append a
// newer user message, so it scans from the end for the last assistant role rather than
// trusting the very last message). Turn-level extras the old inline path reported
// (contextTokens/compacted) are not reconstructed — the turn ran out-of-band.
func (s *Server) writeChatReply(w http.ResponseWriter, database *db.DB, sessionID string, turnDonePayload, replyPayload json.RawMessage) {
	var td struct {
		SessionTitle string `json:"sessionTitle"`
	}
	_ = json.Unmarshal(turnDonePayload, &td)
	resp := chatResp{SessionTitle: td.SessionTitle}

	var reply db.Message
	haveReply := false
	if len(replyPayload) > 0 {
		if err := json.Unmarshal(replyPayload, &reply); err == nil && reply.Role == providers.RoleAssistant {
			haveReply = true
		}
	}
	if !haveReply {
		if msgs, err := database.ListMessages(context.Background(), sessionID); err == nil {
			for i := len(msgs) - 1; i >= 0; i-- {
				if msgs[i].Role == providers.RoleAssistant {
					reply = msgs[i]
					haveReply = true
					break
				}
			}
		}
	}
	if haveReply {
		resp.ReplyMsg = reply
		resp.Reply = reply.Text
		resp.Model = reply.Model
		if reply.Usage != nil {
			resp.Usage = providers.Usage{
				InputTokens:      reply.Usage.InputTokens,
				OutputTokens:     reply.Usage.OutputTokens,
				CacheReadTokens:  reply.Usage.CacheReadTokens,
				CacheWriteTokens: reply.Usage.CacheWriteTokens,
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// queueHasMsg reports whether clientMsgID is still live in a queue_update snapshot —
// either WAITING in the queue or the currently-dispatched inflight head. A queue
// observer uses the transition present→absent (after it has confirmed the message was
// present at least once) to detect that another window cancelled/cleared its queued
// message before it ran, so it can close instead of waiting forever for a terminal
// event that will never come. On a parse error it conservatively reports present, so a
// malformed frame never triggers a false "cancelled".
func queueHasMsg(raw json.RawMessage, clientMsgID string) bool {
	if len(raw) == 0 {
		return true
	}
	var p struct {
		Queue []struct {
			ClientMsgID string `json:"clientMsgId"`
		} `json:"queue"`
		InflightClientMsgID string `json:"inflightClientMsgId"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return true
	}
	if p.InflightClientMsgID == clientMsgID {
		return true
	}
	for _, q := range p.Queue {
		if q.ClientMsgID == clientMsgID {
			return true
		}
	}
	return false
}

// payloadClientMsgID extracts the clientMsgId a terminal hub event carries so a
// queue observer can tell its own turn's completion from another queued turn's.
func payloadClientMsgID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var p struct {
		ClientMsgID string `json:"clientMsgId"`
	}
	_ = json.Unmarshal(raw, &p)
	return p.ClientMsgID
}
