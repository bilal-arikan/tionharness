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
	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
	"github.com/bilal-arikan/tionswarm/internal/providers"
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

func (s *Server) handleListFlowRuns(w http.ResponseWriter, r *http.Request) {
	flowID := r.URL.Query().Get("flowId")
	runs, err := ws(r).DB.ListFlowRuns(r.Context(), flowID)
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

type sessionFlowReq struct {
	FlowID      string          `json:"flowId"`
	Input       string          `json:"input"`
	Attachments []db.Attachment `json:"attachments"`
}

// handleSessionRunFlow runs a flow and records the result as a turn in the given
// chat session: a user message (the input) plus an assistant message whose body
// is the rendered run transcript. Powers triggering flows from the chat composer
// ("/" command). The flow_run row is still created so the trace lives in history.
func (s *Server) handleSessionRunFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()

	session, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}

	req, ok := bindJSON[sessionFlowReq](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.FlowID) == "" {
		writeError(w, http.StatusBadRequest, "flowId is required")
		return
	}

	flow, err := wsp.DB.GetFlow(ctx, req.FlowID)
	if writeDBError(w, err, "flow not found") {
		return
	}

	// Fold any attachments into the flow input (same block format chat uses), so
	// the flow's agent nodes see attached text/files via {{input}}.
	flowInput := conversation.InlineAttachments(req.Input, req.Attachments)

	// Detach the flow execution from the request lifecycle: a client disconnect
	// (tab close / navigation / network blip) must NOT cancel in-flight flow nodes,
	// otherwise a still-running parallel child fails with "context canceled" while
	// its siblings succeed. Mirrors the chat turn's context.WithoutCancel durability.
	ctx = context.WithoutCancel(ctx)

	// Manual (user-initiated) run: not budget-gated. Setup errors (bad graph) come
	// back as runErr; execution failures land in run.Status.
	run, runErr := wsp.Runtime.RunFlow(ctx, req.FlowID, flowInput, false, nil)

	// User message: what the user typed after the command (the flow input). The
	// raw text + attachment chips are stored; the folded text only seeds the flow.
	userText := strings.TrimSpace(req.Input)
	if userText == "" {
		userText = "🔀 " + flow.Name
	}
	userMsg, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID:   session.ID,
		Role:        providers.RoleUser,
		Text:        userText,
		Attachments: req.Attachments,
	})
	if writeDBError(w, err, "session not found") {
		return
	}

	agentID := finalAgentID(flow, run)
	if agentID == "" {
		agentID = session.AgentID
	}
	msg, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		AgentID:   agentID,
		Text:      flowRunMarkdown(flow, run, runErr),
		Steps:     "[]",
	})
	if writeDBError(w, err, "session not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"userMessage": userMsg, "replyMessage": msg})
}

// flowRunMarkdown renders a finished flow run as a chat-ready markdown transcript:
// a header, one section per executed node (title + output), and a failure note if
// the run errored.
func flowRunMarkdown(flow db.Flow, run db.FlowRun, runErr error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔀 **%s** akışı çalıştı\n\n", flow.Name)
	if runErr != nil {
		fmt.Fprintf(&b, "⚠️ Akış başlatılamadı: %s", runErr.Error())
		return b.String()
	}
	var st orchestration.State
	_ = json.Unmarshal([]byte(run.State), &st)
	for i, t := range st.Trace {
		title := t.Title
		if title == "" {
			title = t.NodeID
		}
		fmt.Fprintf(&b, "#### %d. %s\n\n%s\n\n", i+1, title, t.Output)
	}
	if run.Status == db.FlowFailure {
		fmt.Fprintf(&b, "---\n\n⚠️ **Durum: hata** — %s", run.Error)
	}
	return strings.TrimSpace(b.String())
}

// finalAgentID returns the agent of the last agent node that executed in the run,
// so the resulting chat message is attributed to the agent that produced the
// final output. Empty if it can't be resolved.
func finalAgentID(flow db.Flow, run db.FlowRun) string {
	g, err := orchestration.ParseGraph(flow.Graph)
	if err != nil {
		return ""
	}
	var st orchestration.State
	if json.Unmarshal([]byte(run.State), &st) != nil {
		return ""
	}
	for i := len(st.Trace) - 1; i >= 0; i-- {
		for _, n := range g.Nodes {
			if n.ID == st.Trace[i].NodeID && n.Type == orchestration.NodeAgent {
				return n.AgentID
			}
		}
	}
	return ""
}

// handleSessionRunFlowStream is the SSE variant of handleSessionRunFlow: it runs
// the flow with a per-node observer so the client sees each node start/finish
// live, then persists + returns the same user/assistant turn. Events:
//
//	meta  → { userMessage }           (once)
//	node  → orchestration.NodeEvent   (per node start/done)
//	reply → { replyMessage }          (terminal, success)
//	error → { error }                 (terminal, setup failure)
func (s *Server) handleSessionRunFlowStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()

	session, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}
	req, ok := bindJSON[sessionFlowReq](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.FlowID) == "" {
		writeError(w, http.StatusBadRequest, "flowId is required")
		return
	}
	flow, err := wsp.DB.GetFlow(ctx, req.FlowID)
	if writeDBError(w, err, "flow not found") {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	// Persist the user message (raw text + attachment chips) before streaming.
	userText := strings.TrimSpace(req.Input)
	if userText == "" {
		userText = "🔀 " + flow.Name
	}
	userMsg, err := wsp.DB.AddMessage(ctx, db.Message{
		SessionID:   session.ID,
		Role:        providers.RoleUser,
		Text:        userText,
		Attachments: req.Attachments,
	})
	if writeDBError(w, err, "session not found") {
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// The observer may fire concurrently (parallel children), so guard the writer.
	var mu sync.Mutex
	sse := func(event string, data any) {
		mu.Lock()
		defer mu.Unlock()
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	sse("meta", map[string]any{"userMessage": userMsg})

	flowInput := conversation.InlineAttachments(req.Input, req.Attachments)
	obs := func(ev orchestration.NodeEvent) { sse("node", ev) }
	// Detach flow execution from the request: if the client disconnects mid-run,
	// the flow (and its in-flight parallel nodes) must finish and persist rather
	// than dying with "context canceled". SSE writes to a gone client simply no-op;
	// the run still completes and lands in history. Mirrors chat-turn durability.
	runCtx := context.WithoutCancel(ctx)
	run, runErr := wsp.Runtime.RunFlow(runCtx, req.FlowID, flowInput, false, obs)

	agentID := finalAgentID(flow, run)
	if agentID == "" {
		agentID = session.AgentID
	}
	// Persist with the detached context so the result lands in history even if the
	// client already disconnected (the run completed regardless).
	msg, aerr := wsp.DB.AddMessage(runCtx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		AgentID:   agentID,
		Text:      flowRunMarkdown(flow, run, runErr),
		Steps:     "[]",
	})
	if aerr != nil {
		sse("error", map[string]any{"error": aerr.Error()})
		return
	}
	sse("reply", map[string]any{"replyMessage": msg})
}
