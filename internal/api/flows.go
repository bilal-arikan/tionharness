package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/tools"
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
	q := r.URL.Query()
	limit, offset, field, asc, listing, err := listQueryParams(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	wantTags := tools.SplitTags(q.Get("tags"))
	matches := make([]db.Flow, 0, len(flows))
	for _, f := range flows {
		if len(wantTags) > 0 && !tools.HasAllTags(f.Tags, wantTags) {
			continue
		}
		matches = append(matches, f)
	}
	if !listing {
		writeJSON(w, http.StatusOK, matches)
		return
	}
	if field != "" {
		less, err := tools.SortByField(matches, field, asc,
			func(f db.Flow) int64 { return f.UpdatedAt },
			func(f db.Flow) int64 { return f.CreatedAt },
			func(f db.Flow) string { return f.Name },
			func(f db.Flow) string { return f.ID })
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		sort.SliceStable(matches, less)
	}
	page, total := tools.SlicePage(matches, offset, limit)
	pageJSONResponse(w, page, total, offset, limit)
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

// validateFlowGraphAgents runs the semantic (agent-existence + provider-ready)
// check on top of marshalGraph's structural check, so a graph referencing a
// missing or unrunnable agent is rejected at save time instead of only
// surfacing when the flow is run. Mirrors marshalGraph's draft-save allowance:
// a nil graph or one with no start node yet is not checked. rt is nil in the
// (rare) case a workspace's runtime has not finished booting; skip rather than
// panic — RunFlow re-checks preconditions right before executing anyway.
func validateFlowGraphAgents(ctx context.Context, rt *agent.Runtime, g *orchestration.Graph) error {
	if rt == nil || g == nil || g.Start == "" {
		return nil
	}
	return rt.ValidateFlowGraph(ctx, *g)
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
	if err := validateFlowGraphAgents(r.Context(), ws(r).Runtime, req.Graph); err != nil {
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
	var req struct {
		Name  *string              `json:"name"`
		Graph *orchestration.Graph `json:"graph"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	cur, err := ws(r).DB.GetFlow(r.Context(), id)
	if writeDBError(w, err, "flow not found") {
		return
	}
	if req.Name != nil {
		cur.Name = *req.Name
	}
	if req.Graph != nil {
		graph, err := marshalGraph(req.Graph)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid graph: "+err.Error())
			return
		}
		if err := validateFlowGraphAgents(r.Context(), ws(r).Runtime, req.Graph); err != nil {
			writeError(w, http.StatusBadRequest, "invalid graph: "+err.Error())
			return
		}
		cur.Graph = graph
	}
	if err := ws(r).DB.UpdateFlow(r.Context(), cur); err != nil {
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

// handleDeleteFlowRun removes a finished run and its whole tree (subflow/spawn
// descendants, state deltas, per-node step sidecars). 404 for an unknown id,
// 409 while any member of the tree is still running or waiting (_Docs/77 R8).
func (s *Server) handleDeleteFlowRun(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	deleted, err := wsp.Runtime.DeleteFlowRun(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, db.ErrNotFound):
		writeError(w, http.StatusNotFound, "flow run not found")
		return
	case errors.Is(err, db.ErrConflict):
		writeError(w, http.StatusConflict, "flow run tree is still running or waiting; stop or resume it first")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
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
