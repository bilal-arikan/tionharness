package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/flow"
	"github.com/bilal-arikan/tionharness/internal/workspace"
	"github.com/google/uuid"
)

// Evolving flows (_Docs/93): one main flow per agent, versioned, with runs,
// proposals and prompt versions. The canvas saves whole graphs; agents and the
// observer apply ops; both go through the runtime so every path validates the
// same way.

func (s *Server) registerFlowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/flows", s.handleListFlows)
	mux.HandleFunc("GET /api/flows/{id}", s.handleGetFlow)
	mux.HandleFunc("PUT /api/flows/{id}", s.handleSaveFlow)
	mux.HandleFunc("PUT /api/flows/{id}/meta", s.handleUpdateFlowMeta)
	mux.HandleFunc("POST /api/flows/{id}/validate", s.handleValidateFlow)
	mux.HandleFunc("GET /api/flows/{id}/versions", s.handleListFlowVersions)
	mux.HandleFunc("GET /api/flows/{id}/versions/{n}", s.handleGetFlowVersion)
	mux.HandleFunc("POST /api/flows/{id}/revert", s.handleRevertFlow)
	mux.HandleFunc("GET /api/flows/{id}/runs", s.handleListFlowRuns)
	mux.HandleFunc("GET /api/flow-runs/{id}", s.handleGetFlowRun)
	mux.HandleFunc("DELETE /api/flow-runs/{id}", s.handleDeleteFlowRun)
	mux.HandleFunc("POST /api/flows/{id}/optimize", s.handleOptimizeFlow)
	mux.HandleFunc("GET /api/flows/{id}/proposals", s.handleListFlowProposals)
	mux.HandleFunc("POST /api/flow-proposals/{id}/apply", s.handleApplyFlowProposal)
	mux.HandleFunc("POST /api/flow-proposals/{id}/reject", s.handleRejectFlowProposal)
	mux.HandleFunc("POST /api/flows/{id}/test", s.handleTestFlow)
	mux.HandleFunc("GET /api/agents/{id}/flow", s.handleAgentFlow)
	mux.HandleFunc("GET /api/agents/{id}/prompt-versions", s.handleListPromptVersions)
	mux.HandleFunc("POST /api/agents/{id}/prompt-versions/{n}/restore", s.handleRestorePromptVersion)
}

// flowListItem is one row of the flows screen: the flow plus what the list
// needs without a second fetch.
type flowListItem struct {
	db.Flow
	AgentName        string `json:"agentName"`
	AgentAvatar      string `json:"agentAvatar,omitempty"`
	AgentColor       string `json:"agentColor,omitempty"`
	AgentArchived    bool   `json:"agentArchived,omitempty"`
	Shape            string `json:"shape"`
	Trivial          bool   `json:"trivial"`
	NodeCount        int    `json:"nodeCount"`
	PendingProposals int    `json:"pendingProposals"`
}

func (s *Server) flowItem(ctx context.Context, wsp *workspace.Workspace, f db.Flow, pending map[string]int) flowListItem {
	item := flowListItem{Flow: f, PendingProposals: pending[f.ID]}
	if a, err := wsp.DB.GetAgent(ctx, f.AgentID); err == nil {
		item.AgentName, item.AgentAvatar, item.AgentColor, item.AgentArchived = a.Name, a.Avatar, a.Color, a.Archived
	}
	if g, err := flow.Parse(f.Graph); err == nil {
		item.Shape, item.Trivial, item.NodeCount = g.Summary(), g.IsTrivial(), len(g.Nodes)
	}
	return item
}

func (s *Server) pendingProposalCounts(ctx context.Context, wsp *workspace.Workspace) map[string]int {
	out := map[string]int{}
	props, _ := wsp.DB.ListFlowProposals(ctx, "")
	for _, p := range props {
		if p.Status == db.ProposalPending {
			out[p.FlowID]++
		}
	}
	return out
}

// handleListFlows lists one flow per live, non-system agent, creating the
// default flow for agents that have none yet.
func (s *Server) handleListFlows(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsp := ws(r)
	agents, err := wsp.DB.ListAgents(ctx)
	if writeDBError(w, err, "") {
		return
	}
	for _, a := range agents {
		if a.System || a.Deleted {
			continue
		}
		if _, err := wsp.DB.EnsureAgentFlow(ctx, a.ID); err != nil {
			s.logger.Warn("ensure agent flow failed", "agent", a.ID, "error", err)
		}
	}
	flows, err := wsp.DB.ListFlows(ctx)
	if writeDBError(w, err, "") {
		return
	}
	pending := s.pendingProposalCounts(ctx, wsp)
	out := make([]flowListItem, 0, len(flows))
	for _, f := range flows {
		if a, err := wsp.DB.GetAgent(ctx, f.AgentID); err != nil || a.System {
			continue
		}
		out = append(out, s.flowItem(ctx, wsp, f, pending))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsp := ws(r)
	f, err := wsp.DB.GetFlow(ctx, r.PathValue("id"))
	if writeDBError(w, err, "flow not found") {
		return
	}
	writeJSON(w, http.StatusOK, s.flowItem(ctx, wsp, f, s.pendingProposalCounts(ctx, wsp)))
}

func (s *Server) handleAgentFlow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wsp := ws(r)
	f, err := wsp.DB.EnsureAgentFlow(ctx, r.PathValue("id"))
	if writeDBError(w, err, "agent not found") {
		return
	}
	writeJSON(w, http.StatusOK, s.flowItem(ctx, wsp, f, s.pendingProposalCounts(ctx, wsp)))
}

type saveFlowReq struct {
	Graph  json.RawMessage `json:"graph"`
	Reason string          `json:"reason"`
}

// handleSaveFlow commits the canvas graph as a new version (author: user).
func (s *Server) handleSaveFlow(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[saveFlowReq](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	wsp := ws(r)
	g, err := flow.Parse(string(req.Graph))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "canvas edit"
	}
	v, err := wsp.Runtime.SaveFlowGraph(ctx, r.PathValue("id"), g, db.FlowAuthor{Kind: db.FlowAuthorUser}, reason)
	if err != nil {
		if err.Error() == "no change" {
			f, _ := wsp.DB.GetFlow(ctx, r.PathValue("id"))
			writeJSON(w, http.StatusOK, map[string]any{"flow": s.flowItem(ctx, wsp, f, nil), "changed": false})
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	f, _ := wsp.DB.GetFlow(ctx, r.PathValue("id"))
	publishEntityChange(wsp, "flow", "Akış kaydedildi: "+f.Name, "v"+strconv.Itoa(v.Version), map[string]string{"view": "flows", "flowId": f.ID})
	writeJSON(w, http.StatusOK, map[string]any{"flow": s.flowItem(ctx, wsp, f, nil), "version": v, "changed": true})
}

type flowMetaReq struct {
	Name   *string        `json:"name"`
	Note   *string        `json:"note"`
	Policy *db.FlowPolicy `json:"policy"`
}

func (s *Server) handleUpdateFlowMeta(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[flowMetaReq](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	wsp := ws(r)
	f, err := wsp.DB.UpdateFlowMeta(ctx, r.PathValue("id"), req.Name, req.Note, req.Policy)
	if writeDBError(w, err, "flow not found") {
		return
	}
	writeJSON(w, http.StatusOK, s.flowItem(ctx, wsp, f, s.pendingProposalCounts(ctx, wsp)))
}

// handleValidateFlow dry-runs a graph through the same validation a save uses.
func (s *Server) handleValidateFlow(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[saveFlowReq](w, r)
	if !ok {
		return
	}
	g, err := flow.Parse(string(req.Graph))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := g.Validate(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "shape": g.Summary()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "shape": g.Summary(), "trivial": g.IsTrivial(), "nodeCount": len(g.Nodes)})
}

func (s *Server) handleListFlowVersions(w http.ResponseWriter, r *http.Request) {
	vs, err := ws(r).DB.ListFlowVersions(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "") {
		return
	}
	if vs == nil {
		vs = []db.FlowVersion{}
	}
	writeJSON(w, http.StatusOK, vs)
}

func (s *Server) handleGetFlowVersion(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n <= 0 {
		writeError(w, http.StatusBadRequest, "invalid version")
		return
	}
	v, err := ws(r).DB.GetFlowVersion(r.Context(), r.PathValue("id"), n)
	if writeDBError(w, err, "version not found") {
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type revertFlowReq struct {
	Version int    `json:"version"`
	Reason  string `json:"reason"`
}

func (s *Server) handleRevertFlow(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[revertFlowReq](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	wsp := ws(r)
	v, err := wsp.Runtime.RevertFlow(ctx, r.PathValue("id"), req.Version, db.FlowAuthor{Kind: db.FlowAuthorUser}, req.Reason)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	f, _ := wsp.DB.GetFlow(ctx, r.PathValue("id"))
	publishEntityChange(wsp, "flow", "Akış geri alındı: "+f.Name, "v"+strconv.Itoa(v.Version), map[string]string{"view": "flows", "flowId": f.ID})
	writeJSON(w, http.StatusOK, map[string]any{"flow": s.flowItem(ctx, wsp, f, nil), "version": v})
}

func (s *Server) handleListFlowRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	runs, err := ws(r).DB.ListFlowRuns(r.Context(), r.PathValue("id"), limit)
	if writeDBError(w, err, "") {
		return
	}
	// The list omits the step traces (fetched per run) to keep the feed light.
	out := make([]db.FlowRun, 0, len(runs))
	for _, run := range runs {
		run.Steps = nil
		out = append(out, run)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetFlowRun(w http.ResponseWriter, r *http.Request) {
	run, err := ws(r).DB.GetFlowRun(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "run not found") {
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleDeleteFlowRun(w http.ResponseWriter, r *http.Request) {
	if err := ws(r).DB.DeleteFlowRun(r.Context(), r.PathValue("id")); writeDBError(w, err, "run not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": "deleted"})
}

func (s *Server) handleOptimizeFlow(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	if _, err := wsp.DB.GetFlow(r.Context(), r.PathValue("id")); writeDBError(w, err, "flow not found") {
		return
	}
	res, err := wsp.Runtime.OptimizeFlow(r.Context(), r.PathValue("id"), "manual")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleListFlowProposals(w http.ResponseWriter, r *http.Request) {
	props, err := ws(r).DB.ListFlowProposals(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "") {
		return
	}
	if props == nil {
		props = []db.FlowProposal{}
	}
	writeJSON(w, http.StatusOK, props)
}

func (s *Server) handleApplyFlowProposal(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	p, err := wsp.Runtime.ApplyFlowProposal(r.Context(), r.PathValue("id"), db.FlowAuthor{Kind: db.FlowAuthorUser})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	publishEntityChange(wsp, "flow", "Akış önerisi uygulandı", p.Reason, map[string]string{"view": "flows", "flowId": p.FlowID})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleRejectFlowProposal(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	p, err := wsp.Runtime.RejectFlowProposal(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	publishEntityChange(wsp, "flow", "Akış önerisi reddedildi", p.Reason, map[string]string{"view": "flows", "flowId": p.FlowID})
	writeJSON(w, http.StatusOK, p)
}

type testFlowReq struct {
	Input string `json:"input"`
}

// handleTestFlow runs the flow once on a test input: it opens a chat session
// for the flow's agent, tagged flow-test, and queues the input as its first
// turn. The run streams over the normal session/flow events.
func (s *Server) handleTestFlow(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[testFlowReq](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Input) == "" {
		writeError(w, http.StatusBadRequest, "input is required")
		return
	}
	ctx := r.Context()
	wsp := ws(r)
	f, err := wsp.DB.GetFlow(ctx, r.PathValue("id"))
	if writeDBError(w, err, "flow not found") {
		return
	}
	a, err := wsp.DB.GetAgent(ctx, f.AgentID)
	if writeDBError(w, err, "agent not found") {
		return
	}
	if writeDBError(w, a.RunnableErr(), "") {
		return
	}
	title := "🧪 Akış testi: " + strings.TrimSpace(f.Name)
	session, err := wsp.DB.CreateSession(ctx, db.Session{
		AgentID:    a.ID,
		Title:      title,
		Tags:       []string{"flow-test"},
		WorkingDir: strings.TrimSpace(wsp.Settings().DefaultWorkingDir),
	})
	if writeDBError(w, err, "") {
		return
	}
	clientMsgID := uuid.NewString()
	s.enqueueMessage(wsp.ID, chatReq{SessionID: session.ID, Message: req.Input, ClientMsgID: clientMsgID}, clientMsgID)
	writeJSON(w, http.StatusOK, map[string]any{"sessionId": session.ID, "flowId": f.ID, "agentId": a.ID})
}

func (s *Server) handleListPromptVersions(w http.ResponseWriter, r *http.Request) {
	vs, err := ws(r).DB.ListAgentPromptVersions(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "") {
		return
	}
	if vs == nil {
		vs = []db.AgentPromptVersion{}
	}
	writeJSON(w, http.StatusOK, vs)
}

func (s *Server) handleRestorePromptVersion(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n <= 0 {
		writeError(w, http.StatusBadRequest, "invalid version")
		return
	}
	wsp := ws(r)
	if err := wsp.Runtime.RestoreAgentPromptVersion(r.Context(), r.PathValue("id"), n, db.FlowAuthor{Kind: db.FlowAuthorUser}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	emitSessionChange(wsp, "", "agent")
	publishEntityChange(wsp, "agent", "Ajan promptu geri yüklendi", "v"+strconv.Itoa(n), map[string]string{"view": "agents", "agentId": r.PathValue("id")})
	writeJSON(w, http.StatusOK, map[string]any{"restored": n})
}
