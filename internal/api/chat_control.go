package api

import (
	"context"
	"net/http"
	"sync"

	"github.com/google/uuid"

	"github.com/bilal/swarmgo/internal/tools"
)

// chatRun is the live control handle for one in-flight streaming chat turn.
type chatRun struct {
	cancel context.CancelFunc
	steer  chan string
	// answer delivers a reply to a blocked ask_user tool call. Buffered (1) so
	// the control endpoint never blocks; only one question is outstanding at a
	// time because the tool loop runs synchronously.
	answer chan string
	// done is closed when the turn finishes (unregister), unblocking any
	// Interaction MCP tool call still waiting on this run.
	done chan struct{}
	// token is the per-run opaque secret a CLI subprocess presents (Bearer) so
	// its Interaction MCP calls correlate back to this turn.
	token string
	// sessionID is the chat session this turn belongs to, so the UI can ask
	// "is a turn in flight for session X?" after a page reload (turns are
	// detached from the client connection and keep running server-side).
	sessionID string

	// mu serialises SSE writes: the stream handler goroutine and the Interaction
	// MCP handler goroutine both emit steps onto the same ResponseWriter. It also
	// guards artifacts (swapped per responding agent in a multi-agent turn).
	mu        sync.Mutex
	write     func(event string, data any) // installed by the stream handler; nil once the turn ends
	artifacts tools.ArtifactSink           // current agent's artifact sink, for Interaction MCP create/update
	grants    *tools.PermissionGrants      // session "Always allow" set, for the CLI permission-prompt tool
}

// setGrants installs the session's permission grants so the Interaction MCP
// permission-prompt tool can honour "Always allow" across turns.
func (r *chatRun) setGrants(g *tools.PermissionGrants) {
	r.mu.Lock()
	r.grants = g
	r.mu.Unlock()
}

// grantStore returns the session's permission grants (nil if none installed).
func (r *chatRun) grantStore() *tools.PermissionGrants {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.grants
}

// setArtifacts installs the artifact sink for the currently responding agent so
// the Interaction MCP create_artifact/update_artifact tools persist to it.
func (r *chatRun) setArtifacts(sink tools.ArtifactSink) {
	r.mu.Lock()
	r.artifacts = sink
	r.mu.Unlock()
}

// artifactSink returns the current artifact sink (nil if none installed).
func (r *chatRun) artifactSink() tools.ArtifactSink {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.artifacts
}

// emit writes one SSE event through the run's writer under the lock, so the
// stream handler and the Interaction MCP server never race on the ResponseWriter.
// A no-op once the turn has ended (write cleared).
func (r *chatRun) emit(event string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.write != nil {
		r.write(event, data)
	}
}

// setWrite installs the SSE writer for this run.
func (r *chatRun) setWrite(fn func(event string, data any)) {
	r.mu.Lock()
	r.write = fn
	r.mu.Unlock()
}

// clearWrite drops the writer so late Interaction MCP emits are silently ignored.
func (r *chatRun) clearWrite() {
	r.mu.Lock()
	r.write = nil
	r.mu.Unlock()
}

// chatRuns is the registry of active streaming turns, keyed by run id, so the
// control endpoint can stop or steer a turn while it is running.
type chatRuns struct {
	mu   sync.Mutex
	runs map[string]*chatRun
}

func newChatRuns() *chatRuns { return &chatRuns{runs: make(map[string]*chatRun)} }

// register creates a control handle for a run (with a fresh per-run token) and
// returns it. sessionID ties the run to its chat session for activeSessionIDs.
func (c *chatRuns) register(id, sessionID string, cancel context.CancelFunc) *chatRun {
	run := &chatRun{
		cancel:    cancel,
		steer:     make(chan string, 16),
		answer:    make(chan string, 1),
		done:      make(chan struct{}),
		token:     uuid.NewString(),
		sessionID: sessionID,
	}
	c.mu.Lock()
	c.runs[id] = run
	c.mu.Unlock()
	return run
}

// activeSessionIDs returns the distinct session ids that currently have a turn
// in flight. The frontend uses this after a reload to restore the "thinking"
// indicator for turns that are still running detached on the server.
func (c *chatRuns) activeSessionIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := make(map[string]struct{}, len(c.runs))
	ids := make([]string, 0, len(c.runs))
	for _, run := range c.runs {
		if run.sessionID == "" {
			continue
		}
		if _, ok := seen[run.sessionID]; ok {
			continue
		}
		seen[run.sessionID] = struct{}{}
		ids = append(ids, run.sessionID)
	}
	return ids
}

func (c *chatRuns) unregister(id string) {
	c.mu.Lock()
	run := c.runs[id]
	delete(c.runs, id)
	c.mu.Unlock()
	if run != nil {
		close(run.done)
	}
}

func (c *chatRuns) get(id string) *chatRun {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runs[id]
}

// byToken resolves a run by its per-run Bearer token (Interaction MCP correlation).
func (c *chatRuns) byToken(token string) *chatRun {
	if token == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, run := range c.runs {
		if run.token == token {
			return run
		}
	}
	return nil
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
