package workspace

import (
	"context"
	"github.com/bilal-arikan/tionharness/internal/db"
)

type AgentAssignment struct {
	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName"`
	AgentID       string `json:"agentId"`
	Archived      bool   `json:"archived,omitempty"`
}

type CatalogAgent struct {
	Agent       db.Agent          `json:"agent"`
	Assignments []AgentAssignment `json:"assignments"`
}

func (m *Manager) AgentCatalog() *db.DB { return m.agentCatalog }

func (m *Manager) checkCatalogRole(id string) error {
	for _, meta := range m.List() {
		ws, err := m.Get(meta.ID)
		if err != nil {
			continue
		}
		if err := ws.DB.CheckCatalogRole(id); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) CatalogAgents(ctx context.Context) ([]CatalogAgent, error) {
	agents, err := m.agentCatalog.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	assignments := map[string][]AgentAssignment{}
	for _, meta := range m.List() {
		ws, err := m.Get(meta.ID)
		if err != nil {
			continue
		}
		rows, err := ws.DB.ListAgents(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range rows {
			assignments[a.CatalogID] = append(assignments[a.CatalogID], AgentAssignment{ws.ID, ws.Name, a.ID, a.Archived})
		}
	}
	result := make([]CatalogAgent, 0, len(agents))
	for _, a := range agents {
		links := assignments[a.ID]
		if links == nil {
			links = []AgentAssignment{}
		}
		result = append(result, CatalogAgent{a, links})
	}
	return result, nil
}

// CatalogImpact includes assignments of descendants, whose inherited values
// can also change when a parent profile is edited.
func (m *Manager) CatalogImpact(ctx context.Context, id string) ([]AgentAssignment, error) {
	rows, err := m.CatalogAgents(ctx)
	if err != nil {
		return nil, err
	}
	affected := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, row := range rows {
			if affected[row.Agent.ParentID] && !affected[row.Agent.ID] {
				affected[row.Agent.ID], changed = true, true
			}
		}
	}
	seen := map[string]bool{}
	result := []AgentAssignment{}
	for _, row := range rows {
		if !affected[row.Agent.ID] {
			continue
		}
		for _, link := range row.Assignments {
			if seen[link.WorkspaceID] {
				continue
			}
			seen[link.WorkspaceID] = true
			result = append(result, link)
		}
	}
	return result, nil
}
