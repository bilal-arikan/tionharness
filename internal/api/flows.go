package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/bilal-arikan/swarmgo/internal/conversation"
	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/orchestration"
	"github.com/bilal-arikan/swarmgo/internal/providers"
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
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Graph       *orchestration.Graph  `json:"graph"`
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
	var req flowReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
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
		Name:        req.Name,
		Description: req.Description,
		Graph:       graph,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, flow)
}

func (s *Server) handleUpdateFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req flowReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	graph, err := marshalGraph(req.Graph)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid graph: "+err.Error())
		return
	}
	if err := ws(r).DB.UpdateFlow(r.Context(), db.Flow{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Graph:       graph,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	flow, _ := ws(r).DB.GetFlow(r.Context(), id)
	writeJSON(w, http.StatusOK, flow)
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

	run, sessionID, err := ws(r).Runtime.RunFlowRecorded(r.Context(), id, req.Input, false, nil)
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
	run, sessionID, err := wsp.Runtime.RunFlowRecorded(ctx, id, req.Input, false, obs)
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

	var req sessionFlowReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
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
	var req sessionFlowReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
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
	run, runErr := wsp.Runtime.RunFlow(ctx, req.FlowID, flowInput, false, obs)

	agentID := finalAgentID(flow, run)
	if agentID == "" {
		agentID = session.AgentID
	}
	msg, aerr := wsp.DB.AddMessage(ctx, db.Message{
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
