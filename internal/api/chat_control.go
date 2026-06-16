package api

import (
	"context"
	"net/http"
	"sync"
)

// chatRun is the live control handle for one in-flight streaming chat turn.
type chatRun struct {
	cancel context.CancelFunc
	steer  chan string
	// answer delivers a reply to a blocked ask_user tool call. Buffered (1) so
	// the control endpoint never blocks; only one question is outstanding at a
	// time because the tool loop runs synchronously.
	answer chan string
}

// chatRuns is the registry of active streaming turns, keyed by run id, so the
// control endpoint can stop or steer a turn while it is running.
type chatRuns struct {
	mu   sync.Mutex
	runs map[string]*chatRun
}

func newChatRuns() *chatRuns { return &chatRuns{runs: make(map[string]*chatRun)} }

// register creates a control handle for a run and returns it.
func (c *chatRuns) register(id string, cancel context.CancelFunc) *chatRun {
	run := &chatRun{cancel: cancel, steer: make(chan string, 16), answer: make(chan string, 1)}
	c.mu.Lock()
	c.runs[id] = run
	c.mu.Unlock()
	return run
}

func (c *chatRuns) unregister(id string) {
	c.mu.Lock()
	delete(c.runs, id)
	c.mu.Unlock()
}

func (c *chatRuns) get(id string) *chatRun {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runs[id]
}

type chatControlReq struct {
	RunID  string `json:"runId"`
	Action string `json:"action"` // "stop" | "steer" | "answer"
	Text   string `json:"text"`
}

// handleChatControl stops, steers or answers an in-flight streaming turn. "stop"
// cancels the run's context (ending the stream); "steer" delivers live guidance
// the tool loop folds in before its next model call; "answer" delivers a reply
// to a blocked ask_user tool call.
func (s *Server) handleChatControl(w http.ResponseWriter, r *http.Request) {
	var req chatControlReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	run := s.runs.get(req.RunID)
	if run == nil {
		writeError(w, http.StatusNotFound, "run not found (already finished?)")
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
		select {
		case run.steer <- req.Text:
		default: // buffer full — drop rather than block the request
		}
	case "answer":
		if req.Text == "" {
			writeError(w, http.StatusBadRequest, "answer text is required")
			return
		}
		select {
		case run.answer <- req.Text:
		default: // no question waiting (or already answered) — drop
		}
	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+req.Action)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}
