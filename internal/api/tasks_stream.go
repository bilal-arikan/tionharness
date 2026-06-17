package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"

	"github.com/bilal/swarmgo/internal/agent"
)

// handleRunTaskStream runs a task "now" over Server-Sent Events, emitting each
// activity step as it occurs so the board / activity feed shows live progress —
// the same step protocol as the chat stream. The run is registered so it appears
// "running" in the executions feed and can be cancelled. Events:
//
//	meta  → { runId, taskId, sessionId }   (once)
//	step  → a single agent.TurnStep        (zero or more)
//	reply → { run }                        (terminal, success)
//	error → { error }                      (terminal, setup failure)
//
// A provider error during the run is captured in the returned run (status
// failure), not as an error event — mirroring RunTask.
func (s *Server) handleRunTaskStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	task, err := wsp.DB.GetTask(r.Context(), id)
	if writeDBError(w, err, "task not found") {
		return
	}

	// Resolve the transcript session up front so the run can be registered against
	// it: that powers the executions feed's live "running" flag and lets a "stop"
	// control cancel it. Detach generation from the client connection so a page
	// refresh doesn't abort the run mid-flight (it persists regardless).
	session, _ := wsp.Runtime.TaskSession(r.Context(), task)
	runID := uuid.NewString()
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()
	_ = s.runs.register(runID, session.ID, cancel)
	defer s.runs.unregister(runID)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// A flow-backed task's observer fires from parallel goroutines, so guard the
	// writer.
	var mu sync.Mutex
	sse := func(event string, data any) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	sse("meta", map[string]any{"runId": runID, "taskId": id, "sessionId": session.ID})

	onStep := func(st agent.TurnStep) { sse("step", st) }
	run, runErr := wsp.Runtime.RunTaskStream(ctx, id, "manual", onStep)
	if runErr != nil {
		sse("error", map[string]any{"error": runErr.Error()})
		return
	}
	sse("reply", map[string]any{"run": run})
}
