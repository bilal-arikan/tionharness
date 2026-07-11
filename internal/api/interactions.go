package api

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/sessionhub"
)

// pendingInteraction is one outstanding human-in-the-loop prompt (ask_user /
// permission / plan approval) for a session. Because a session can be open in
// several windows, the prompt is broadcast to all of them via the hub
// (interaction_open) and any window may answer; the FIRST answer wins via an
// atomic compare-and-swap on state, and the loser is told it was already
// resolved. This is the generic "resolve-once" primitive (Faz 2 of
// _Docs/58-QUEUE-SENKRON.md) that replaces the old per-request-SSE ask channel.
type pendingInteraction struct {
	id        string
	sessionID string
	kind      string // ask | permission | plan
	// state is 0 while open, 1 once resolved (CAS target — first writer wins).
	state int32
	// answer carries the winning reply to the blocked tool call. Buffered (1) so
	// the resolver never blocks; only one resolve can ever succeed (CAS gate).
	answer chan string
}

// interactionStore holds every session's outstanding interactions, keyed by
// session id then interaction id.
type interactionStore struct {
	mu        sync.Mutex
	bySession map[string]map[string]*pendingInteraction
}

func newInteractionStore() *interactionStore {
	return &interactionStore{bySession: make(map[string]map[string]*pendingInteraction)}
}

func (st *interactionStore) add(pi *pendingInteraction) {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.bySession[pi.sessionID]
	if m == nil {
		m = make(map[string]*pendingInteraction)
		st.bySession[pi.sessionID] = m
	}
	m[pi.id] = pi
}

func (st *interactionStore) get(sessionID, id string) *pendingInteraction {
	st.mu.Lock()
	defer st.mu.Unlock()
	if m := st.bySession[sessionID]; m != nil {
		return m[id]
	}
	return nil
}

func (st *interactionStore) remove(sessionID, id string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if m := st.bySession[sessionID]; m != nil {
		delete(m, id)
		if len(m) == 0 {
			delete(st.bySession, sessionID)
		}
	}
}

// openInteraction registers a new prompt, broadcasts interaction_open on the
// session hub (so EVERY window renders the card), and returns the handle the
// tool call blocks on. payload is the card descriptor (question/options/tool/…),
// merged with the assigned interaction id.
func (s *Server) openInteraction(sessionID, kind string, payload map[string]any) *pendingInteraction {
	pi := &pendingInteraction{
		id:        uuid.NewString(),
		sessionID: sessionID,
		kind:      kind,
		answer:    make(chan string, 1),
	}
	s.interactions.add(pi)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["id"] = pi.id
	payload["kind"] = kind
	s.publishHub(sessionID, sessionhub.KindInteractionOpen, payload, false)
	return pi
}

// resolveInteraction is the compare-and-swap answer path. It succeeds for the
// FIRST caller only; a second concurrent answer (another window) returns false.
// On success it delivers the answer to the blocked tool call and broadcasts
// interaction_resolved so every window closes the card and records the answer.
func (s *Server) resolveInteraction(sessionID, id, answer, by string) bool {
	pi := s.interactions.get(sessionID, id)
	if pi == nil {
		return false
	}
	if !atomic.CompareAndSwapInt32(&pi.state, 0, 1) {
		return false // already resolved — the loser of a concurrent answer race
	}
	select {
	case pi.answer <- answer:
	default:
	}
	s.interactions.remove(sessionID, id)
	s.publishHub(sessionID, sessionhub.KindInteractionResolve, map[string]any{
		"id":         id,
		"answer":     answer,
		"resolvedBy": by,
	}, false)
	return true
}

// waitInteraction blocks until the prompt is answered by some window, the turn's
// context is cancelled (stop), or the client that started the turn is gone. On
// any non-answer exit it CAS-closes the interaction and broadcasts a resolved
// (cancelled) event so no window is left showing a dead card. Returns the answer
// or the terminating error.
func (s *Server) waitInteraction(ctx, clientGone context.Context, pi *pendingInteraction) (string, error) {
	select {
	case ans := <-pi.answer:
		return ans, nil
	case <-clientGone.Done():
		s.cancelInteraction(pi, "client_gone")
		return "", clientGone.Err()
	case <-ctx.Done():
		s.cancelInteraction(pi, "stopped")
		return "", ctx.Err()
	}
}

// waitInteractionCLI is the claude-cli variant of waitInteraction: it blocks on
// the answer, the turn ending (run.done), request cancellation, or the ask
// timeout. reason is "" on a real answer, else one of "done" | "ctx" | "timeout"
// so the CLI tool can return its provider-specific fallback text. Non-answer
// exits CAS-close the interaction so no window is left showing a dead card.
func (s *Server) waitInteractionCLI(ctx context.Context, run *chatRun, pi *pendingInteraction) (string, string) {
	select {
	case ans := <-pi.answer:
		return ans, ""
	case <-run.done:
		s.cancelInteraction(pi, "turn_ended")
		return "", "done"
	case <-ctx.Done():
		s.cancelInteraction(pi, "cancelled")
		return "", "ctx"
	case <-time.After(askTimeout):
		s.cancelInteraction(pi, "timeout")
		return "", "timeout"
	}
}

// cancelInteraction closes an interaction that no one answered (turn stopped /
// client gone) and tells every window to drop its card. Idempotent via the CAS.
func (s *Server) cancelInteraction(pi *pendingInteraction, reason string) {
	if !atomic.CompareAndSwapInt32(&pi.state, 0, 1) {
		return
	}
	s.interactions.remove(pi.sessionID, pi.id)
	s.publishHub(pi.sessionID, sessionhub.KindInteractionResolve, map[string]any{
		"id":        pi.id,
		"cancelled": true,
		"reason":    reason,
	}, false)
}

type interactionAnswerReq struct {
	// Answer is the reply for a single-question ask / permission choice. For a
	// multi-question ask_user the client sends AnswersJSON instead.
	Answer string `json:"answer"`
	// AnswersJSON is a JSON array of answers for a multi-question ask; passed
	// through verbatim to the waiting tool (which folds it via FormatMultiAnswer).
	AnswersJSON string `json:"answersJson,omitempty"`
	// ClientID identifies the answering window (for the resolvedBy attribution
	// shown to the other windows). Optional.
	ClientID string `json:"clientId,omitempty"`
}

// handleInteractionAnswer resolves a pending interaction via compare-and-swap.
// The first window to answer wins (200); a concurrent second answer gets 409 so
// its UI can simply close the (already resolved) card.
func (s *Server) handleInteractionAnswer(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	iid := r.PathValue("iid")
	if sessionID == "" || iid == "" {
		writeError(w, http.StatusBadRequest, "session id and interaction id required")
		return
	}
	req, ok := bindJSON[interactionAnswerReq](w, r)
	if !ok {
		return
	}
	answer := req.Answer
	if req.AnswersJSON != "" {
		answer = req.AnswersJSON
	}
	if s.resolveInteraction(sessionID, iid, answer, req.ClientID) {
		writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
		return
	}
	// Not found or already resolved: from the client's perspective the card is
	// simply gone. 409 lets it distinguish "someone beat me to it" from success.
	writeError(w, http.StatusConflict, "interaction already resolved")
}
