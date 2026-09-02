package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// Canvas actions (Rota F5): the same graph edits the agent's trajectory tool
// performs, driven from the Rota screen with the revision the screen rendered
// (409 on a stale revision), plus "fork here" (spawn a worker under a session)
// and the flow-node → trajectory lookup RunView uses.
//
//	POST /api/trajectories/{id}/plan   {phases:[{id,label,profile,optional,gate:{kind,value}}], expectedRev}
//	POST /api/trajectories/{id}/phase  {id, state, reason, force, expectedRev}
//	POST /api/trajectories/{id}/finish {status, reason, expectedRev}
//	POST /api/sessions/{id}/workers    {agent, task, coordinator, workflow, cwd}
//	GET  /api/trajectories/by-node?run=RUN&node=n1

type planReq struct {
	Phases      []tools.TrajectoryPhaseInput `json:"phases"`
	ExpectedRev uint64                       `json:"expectedRev"`
}

func (s *Server) handlePlanTrajectory(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[planReq](w, r)
	if !ok {
		return
	}
	plan := make([]agent.TrajectoryPlanPhase, 0, len(req.Phases))
	for _, p := range req.Phases {
		pp := agent.TrajectoryPlanPhase{ID: p.ID, Label: p.Label, Profile: p.Profile, Optional: p.Optional}
		if p.Gate != nil {
			pp.GateKind, pp.GateVal = p.Gate.Kind, p.Gate.Value
		}
		plan = append(plan, pp)
	}
	t, err := ws(r).Runtime.PlanTrajectory(r.Context(), r.PathValue("id"), plan, req.ExpectedRev)
	if writeTrajectoryError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, t)
}

type phaseReq struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	Reason      string `json:"reason"`
	Force       bool   `json:"force"`
	ExpectedRev uint64 `json:"expectedRev"`
}

func (s *Server) handleSetTrajectoryPhase(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[phaseReq](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.State) == "" {
		writeError(w, http.StatusBadRequest, "id and state are required")
		return
	}
	t, err := ws(r).Runtime.SetTrajectoryPhase(r.Context(), r.PathValue("id"), req.ID, strings.ToLower(strings.TrimSpace(req.State)), req.Reason, req.ExpectedRev, req.Force)
	if errors.Is(err, agent.ErrGatePending) {
		// Not an error for the canvas: the card is open, the phase stays active.
		writeJSON(w, http.StatusAccepted, map[string]any{"pending": true, "message": err.Error(), "trajectory": t})
		return
	}
	if writeTrajectoryError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, t)
}

type finishReq struct {
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	ExpectedRev uint64 `json:"expectedRev"`
}

func (s *Server) handleFinishTrajectory(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[finishReq](w, r)
	if !ok {
		return
	}
	t, err := ws(r).Runtime.FinishTrajectory(r.Context(), r.PathValue("id"), req.Status, req.Reason, req.ExpectedRev)
	if writeTrajectoryError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// writeTrajectoryError maps the graph edit errors: 404 unknown, 409 stale
// revision, 422 a gate that did not pass, 400 anything else it refused.
func writeTrajectoryError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, db.ErrNotFound):
		writeError(w, http.StatusNotFound, "trajectory not found")
	case errors.Is(err, db.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, agent.ErrGateBlocked):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
	return true
}

type spawnWorkerReq struct {
	Agent       string `json:"agent"`
	Task        string `json:"task"`
	Coordinator bool   `json:"coordinator"`
	Workflow    string `json:"workflow"`
	Cwd         string `json:"cwd"`
}

// handleSpawnWorker is "buradan çatalla": spawn a worker under a coordinator
// session from the canvas, exactly as spawn_worker would from inside its turn.
func (s *Server) handleSpawnWorker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[spawnWorkerReq](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Agent) == "" || strings.TrimSpace(req.Task) == "" {
		writeError(w, http.StatusBadRequest, "agent and task are required")
		return
	}
	wsp := ws(r)
	sess, err := wsp.DB.GetSession(r.Context(), id)
	if writeDBError(w, err, "session not found") {
		return
	}
	if !sess.IsCoordinator() {
		writeError(w, http.StatusBadRequest, "session is not a coordinator; turn coordinator mode on first")
		return
	}
	res, err := wsp.Runtime.SpawnWorker(r.Context(), id, strings.TrimSpace(req.Agent), strings.TrimSpace(req.Task), sess.AgentID, agent.WorkerSpec{
		Coordinator: req.Coordinator, Workflow: strings.TrimSpace(req.Workflow), WorkingDir: strings.TrimSpace(req.Cwd),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId": res.SessionID, "agentName": res.AgentName, "queued": res.Queued, "queuePosition": res.QueuePosition,
	})
}

// handleTrajectoryByNode resolves the trajectory of a flow's coordinator node:
// the session whose origin names the run + node, then its root's trajectory.
func (s *Server) handleTrajectoryByNode(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	runID, nodeID := q.Get("run"), q.Get("node")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run is required")
		return
	}
	wsp := ws(r)
	sessions, err := wsp.DB.ListSessions(r.Context(), "")
	if writeDBError(w, err, "") {
		return
	}
	for _, sess := range sessions {
		o := sess.Lineage()
		if o.RunID != runID || (nodeID != "" && o.NodeID != nodeID) || !sess.IsCoordinator() {
			continue
		}
		rows := wsp.DB.ListTrajectories(r.Context(), db.TrajectoryFilter{RootSessionID: sess.RootSession()})
		if len(rows) == 0 {
			continue
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessionId": sess.ID, "trajectoryId": rows[0].ID})
		return
	}
	writeError(w, http.StatusNotFound, "no trajectory for that flow node")
}
