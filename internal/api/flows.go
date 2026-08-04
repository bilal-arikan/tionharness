package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
)

func (s *Server) handleListFlows(w http.ResponseWriter, r *http.Request) {
	flows, err := ws(r).DB.ListFlows(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if flows == nil {
		flows = []db.Flow{}
	}
	writeJSON(w, http.StatusOK, flows)
}

// handleGetFlow returns a single flow by id (used by the chat "Akış olarak gör"
// to resolve a flow session back to its real graph). 404 if the flow is gone.
func (s *Server) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	flow, err := ws(r).DB.GetFlow(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, flow)
}

type flowReq struct {
	Name  string               `json:"name"`
	Graph *orchestration.Graph `json:"graph"`
	Emoji string               `json:"emoji"` // optional cosmetic glyph (create only; edited via /emoji)
}

// marshalGraph validates and serialises a graph, defaulting to an empty object.
func marshalGraph(g *orchestration.Graph) (string, error) {
	if g == nil {
		return "{}", nil
	}
	// Validate only when there is something to validate (allow draft saves with
	// no start node yet).
	if g.Start != "" {
		if err := g.Validate(); err != nil {
			return "", err
		}
	}
	data, err := json.Marshal(g)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Server) handleCreateFlow(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[flowReq](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	graph, err := marshalGraph(req.Graph)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid graph: "+err.Error())
		return
	}
	flow, err := ws(r).DB.CreateFlow(r.Context(), db.Flow{
		Name:  req.Name,
		Graph: graph,
		Emoji: strings.TrimSpace(req.Emoji),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, flow)
}

func (s *Server) handleUpdateFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[flowReq](w, r)
	if !ok {
		return
	}
	graph, err := marshalGraph(req.Graph)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid graph: "+err.Error())
		return
	}
	if err := ws(r).DB.UpdateFlow(r.Context(), db.Flow{
		ID:    id,
		Name:  req.Name,
		Graph: graph,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	flow, _ := ws(r).DB.GetFlow(r.Context(), id)
	writeJSON(w, http.StatusOK, flow)
}

type flowEmojiReq struct {
	Emoji string `json:"emoji"`
}

// handleSetFlowEmoji replaces a flow's cosmetic emoji only (independent of the
// name/graph save), so changing the glyph never round-trips the whole graph.
func (s *Server) handleSetFlowEmoji(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[flowEmojiReq](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if _, err := ws(r).DB.GetFlow(ctx, id); writeDBError(w, err, "flow not found") {
		return
	}
	if err := ws(r).DB.SetFlowEmoji(ctx, id, strings.TrimSpace(req.Emoji)); writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "emoji": strings.TrimSpace(req.Emoji)})
}

// handleFlowPath returns the absolute path of a flow's on-disk JSON file.
func (s *Server) handleFlowPath(w http.ResponseWriter, r *http.Request) {
	path, err := ws(r).DB.FlowPath(r.PathValue("id"))
	if writeDBError(w, err, "flow not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// handleRevealFlow opens the folder holding the flow's JSON file in the OS file
// manager (Windows: Explorer, highlighting the file) on the local desktop.
func (s *Server) handleRevealFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path, err := ws(r).DB.FlowPath(id)
	if writeDBError(w, err, "flow not found") {
		return
	}
	// Detached from r.Context() so the fire-and-forget launch isn't killed when
	// the handler returns. explorer.exe returns non-zero even on success, so only
	// a failure to *start* the process is a real error.
	if err := exec.Command("explorer.exe", "/select,"+path).Start(); err != nil {
		s.logger.Warn("reveal flow folder failed", "flow", id, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

func (s *Server) handleDeleteFlow(w http.ResponseWriter, r *http.Request) {
	if err := ws(r).DB.DeleteFlow(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "deleted"})
}

type runFlowReq struct {
	Input string `json:"input"`
}

// handleRunFlow executes a flow synchronously and returns the finished run
// (with its trace). Manual runs are user-initiated, so not budget-gated. The run
// is also recorded into the flow's transcript session so it shows up in the
// unified executions feed and the streamable transcript viewer.
func (s *Server) handleRunFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req runFlowReq
	_ = decodeJSON(r, &req)

	// Detach from the request lifecycle: a client disconnect must not cancel
	// in-flight flow nodes ("context canceled"). The flow finishes and persists
	// regardless. Mirrors the chat turn's context.WithoutCancel durability.
	runCtx := context.WithoutCancel(r.Context())
	run, sessionID, err := ws(r).Runtime.RunFlowRecorded(runCtx, id, req.Input, false, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run":       run,
		"sessionId": sessionID,
	})
}

// handleRunFlowStream is the SSE variant of handleRunFlow: it streams each node's
// start/finish live, records the run into the flow's transcript session, and ends
// with the session id. Events:
//
//	node    → orchestration.NodeEvent   (per node start/done)
//	reply   → { run, sessionId }        (terminal, success)
//	error   → { error }                 (terminal, setup failure)
func (s *Server) handleRunFlowStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()

	var req runFlowReq
	_ = decodeJSON(r, &req)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	var mu sync.Mutex
	sse := func(event string, data any) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	obs := func(ev orchestration.NodeEvent) { sse("node", ev) }
	// Detach flow execution from the request: a client disconnect mid-run (the SSE
	// connection dropping, navigation, the "Tekrar çalıştır" caller going away)
	// must not cancel in-flight nodes with "context canceled". SSE writes to a gone
	// client simply no-op; the run still completes and persists.
	runCtx := context.WithoutCancel(ctx)
	run, sessionID, err := wsp.Runtime.RunFlowRecorded(runCtx, id, req.Input, false, obs)
	if err != nil {
		sse("error", map[string]any{"error": err.Error()})
		return
	}
	sse("reply", map[string]any{"run": run, "sessionId": sessionID})
}

// handleListFlowRuns lists flow runs (GET /api/flow-runs), optionally narrowed to
// one flow with ?flowId=. With ?rootOnly=true the subflow/spawn children of a
// composed flow are left out, so one click on a composed flow contributes one row
// instead of a burst of near-identical ones; the children stay reachable through
// /api/flow-runs/{id}/tree. The filter is opt-in so existing callers that expect
// every run keep working unchanged.
func (s *Server) handleListFlowRuns(w http.ResponseWriter, r *http.Request) {
	flowID := r.URL.Query().Get("flowId")
	list := ws(r).DB.ListFlowRuns
	if r.URL.Query().Get("rootOnly") == "true" {
		list = ws(r).DB.ListRootFlowRuns
	}
	runs, err := list(r.Context(), flowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if runs == nil {
		runs = []db.FlowRun{}
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleGetFlowRun(w http.ResponseWriter, r *http.Request) {
	run, err := ws(r).DB.GetFlowRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "flow run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// handleFlowRunTree returns every run in one composed flow's tree — the root run
// plus every subflow/spawn descendant at any depth — breadth-first, so a parent
// always precedes its children (GET /api/flow-runs/{id}/tree).
//
// The id may be ANY member of the tree, not only its root. The UI has whatever
// run the user clicked selected, which is routinely a child, and "show me this
// run's tree" must not depend on which member was picked; one read normalises the
// id via RootOf(). This endpoint is also the resync path after a dropped SSE
// connection, where the client knows a run id but not necessarily the root.
func (s *Server) handleFlowRunTree(w http.ResponseWriter, r *http.Request) {
	run, err := ws(r).DB.GetFlowRun(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "flow run not found")
		return
	}
	runs, err := ws(r).DB.ListFlowRunTree(r.Context(), run.RootOf())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if runs == nil {
		runs = []db.FlowRun{}
	}
	writeJSON(w, http.StatusOK, runs)
}

// handleFlowRunNodeSteps returns one node's captured tool/thinking steps for a
// run (GET /api/flow-runs/{id}/nodes/{nodeId}/steps), read from the per-node
// sidecar. A node with no steps (or a run from before step capture) yields [] —
// the run inspector then just shows the input/output bubbles.
func (s *Server) handleFlowRunNodeSteps(w http.ResponseWriter, r *http.Request) {
	steps, err := ws(r).Runtime.ReadFlowNodeSteps(r.PathValue("id"), r.PathValue("nodeId"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if steps == nil {
		steps = []agent.TurnStep{}
	}
	writeJSON(w, http.StatusOK, steps)
}

// handleResumeFlowRun delivers input to a run suspended at an await-input node and
// resumes it (POST /api/flow-runs/{id}/input). Only a "waiting" run accepts input;
// the waiting→running CAS makes concurrent input from multiple windows safe (the
// losers get 409). The engine then continues past the await with the input as {{last}}.
func (s *Server) handleResumeFlowRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req runFlowReq // reuses {input}
	_ = decodeJSON(r, &req)
	runCtx := context.WithoutCancel(r.Context())
	run, err := ws(r).Runtime.ResumeWaitingFlow(runCtx, id, req.Input)
	if err != nil {
		// Not-waiting / already-resumed / missing → conflict (idempotent for clients).
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run})
}
