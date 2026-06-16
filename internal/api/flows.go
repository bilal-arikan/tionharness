package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/orchestration"
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
// (with its trace). Manual runs are user-initiated, so not budget-gated.
func (s *Server) handleRunFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req runFlowReq
	_ = decodeJSON(r, &req)

	run, err := ws(r).Runtime.RunFlow(r.Context(), id, req.Input, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
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
