package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/workspace"
)

func (s *Server) registerAgentCatalogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agent-catalog", s.handleAgentCatalog)
	mux.HandleFunc("PUT /api/agent-catalog/{id}/workspaces/{workspaceID}", s.handleAssignCatalogAgent)
	mux.HandleFunc("DELETE /api/agent-catalog/{id}/workspaces/{workspaceID}", s.handleDetachCatalogAgent)
	mux.HandleFunc("POST /api/agent-catalog", s.catalogHandler(s.handleCreateAgent, false))
	mux.HandleFunc("PUT /api/agent-catalog/{id}", s.catalogHandler(s.handleUpdateAgent, false))
	mux.HandleFunc("DELETE /api/agent-catalog/{id}", s.catalogHandler(s.handleDeleteCatalogAgent, false))
	mux.HandleFunc("POST /api/agent-catalog/{id}/restore-default", s.catalogHandler(s.handleRestoreAgentDefaults, false))
	mux.HandleFunc("POST /api/agent-catalog/{id}/derive", s.catalogHandler(s.handleDeriveAgent, false))
	mux.HandleFunc("POST /api/agent-catalog/{id}/duplicate", s.catalogHandler(s.handleDuplicateAgent, false))
	mux.HandleFunc("GET /api/agent-catalog/{id}/builtin-prompt", s.catalogHandler(s.handleAgentBuiltinPrompt, false))
	mux.HandleFunc("GET /api/agent-catalog/{id}/tools", s.catalogHandler(s.handleAgentTools, true))
	mux.HandleFunc("POST /api/agent-catalog/{id}/tools", s.catalogHandler(s.handleSetAgentTools, false))
	mux.HandleFunc("GET /api/agent-catalog/{id}/context", s.catalogHandler(s.handleAgentContext, true))
}

// Reuse the validated agent handlers with the central store. Runtime-dependent
// previews use the explicitly selected workspace; they never switch app state.
func (s *Server) catalogHandler(next http.HandlerFunc, needsRuntime bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		virtual := &workspace.Workspace{DB: s.workspaces.AgentCatalog()}
		virtual.Name = "Agent library"
		if current := ws(r); current != nil {
			virtual.Runtime, virtual.DataDir = current.Runtime, current.DataDir
		}
		if needsRuntime && virtual.Runtime == nil {
			writeError(w, http.StatusConflict, "Select a workspace to preview its available tools and context")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), workspaceCtxKey, virtual)))
	}
}

func (s *Server) handleAgentCatalog(w http.ResponseWriter, r *http.Request) {
	rows, err := s.workspaces.CatalogAgents(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	workspaces := []workspaceListItem{}
	for _, entry := range s.workspaces.ListWithDegraded() {
		workspaces = append(workspaces, workspaceListItem{ID: entry.ID, Name: entry.Name, Degraded: entry.Degraded, DegradedReason: entry.Reason})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": rows, "workspaces": workspaces})
}

func (s *Server) handleAssignCatalogAgent(w http.ResponseWriter, r *http.Request) {
	target, err := s.workspaces.Get(r.PathValue("workspaceID"))
	if writeDBError(w, err, "workspace unavailable") {
		return
	}
	a, err := target.DB.AssignCatalogAgent(r.Context(), r.PathValue("id"))
	if writeAgentWriteError(w, err, "agent not found") {
		return
	}
	writeJSON(w, http.StatusOK, a)
	s.publishAgentCatalogChanged()
}

func (s *Server) handleDetachCatalogAgent(w http.ResponseWriter, r *http.Request) {
	target, err := s.workspaces.Get(r.PathValue("workspaceID"))
	if writeDBError(w, err, "workspace unavailable") {
		return
	}
	rows, err := target.DB.ListAgents(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	for _, a := range rows {
		if a.CatalogID != r.PathValue("id") {
			continue
		}
		if busy, _ := s.agentRunning(r.Context(), target, a.ID); busy {
			writeError(w, http.StatusConflict, "Stop the running agent before removing its workspace assignment")
			return
		}
	}
	if writeAgentWriteError(w, target.DB.DetachCatalogAgent(r.Context(), r.PathValue("id")), "agent not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"detached": true})
	s.publishAgentCatalogChanged()
}

// Every interactive profile, tools and reset write passes this guard, including
// edits from a workspace's existing agent screen and bulk operations.
func (s *Server) confirmSharedAgentEdit(w http.ResponseWriter, r *http.Request, id string) bool {
	if s.workspaces == nil || s.workspaces.AgentCatalog() == nil {
		return true
	}
	a, err := ws(r).DB.GetAgent(r.Context(), id)
	if err != nil {
		return true
	}
	catalogID := a.CatalogID
	if strings.HasPrefix(r.URL.Path, "/api/agent-catalog/") {
		catalogID = a.ID
	}
	if catalogID == "" {
		return true
	}
	impact, err := s.workspaces.CatalogImpact(r.Context(), catalogID)
	if writeDBError(w, err, "") {
		return false
	}
	if len(impact) <= 1 || r.Header.Get("X-Confirm-Shared-Agent") == "true" {
		return true
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":                "This shared agent affects multiple workspaces. Confirm before saving.",
		"confirmationRequired": true, "agentName": a.Name, "workspaces": impact,
	})
	return false
}
