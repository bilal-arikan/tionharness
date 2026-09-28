package api

import "net/http"

func (s *Server) handleDeleteCatalogAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := s.workspaces.AgentCatalog().GetAgent(r.Context(), id)
	if writeDBError(w, err, "agent not found") {
		return
	}
	if agent.Locked {
		writeError(w, http.StatusConflict, "Built-in agents cannot be deleted")
		return
	}
	if !s.confirmSharedAgentEdit(w, r, id) {
		return
	}
	rows, err := s.workspaces.CatalogAgents(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	for _, row := range rows {
		if row.Agent.ID != id {
			continue
		}
		// Check every assignment before changing any of them.
		for _, link := range row.Assignments {
			target, err := s.workspaces.Get(link.WorkspaceID)
			if writeDBError(w, err, "workspace unavailable") {
				return
			}
			if busy, _ := s.agentRunning(r.Context(), target, link.AgentID); busy {
				writeError(w, http.StatusConflict, "Stop the agent in "+link.WorkspaceName+" before deleting it")
				return
			}
		}
		for _, link := range row.Assignments {
			target, err := s.workspaces.Get(link.WorkspaceID)
			if writeDBError(w, err, "workspace unavailable") {
				return
			}
			if writeAgentWriteError(w, target.DB.DetachCatalogAgent(r.Context(), id), "agent not found") {
				return
			}
		}
	}
	if writeAgentWriteError(w, s.workspaces.AgentCatalog().DeleteAgent(r.Context(), id), "agent not found") {
		return
	}
	s.publishAgentCatalogChanged()
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}
