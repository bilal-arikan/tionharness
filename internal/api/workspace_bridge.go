package api

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionswarm/internal/tools"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// workspaceBridge adapts the workspace manager to the tools.WorkspaceBridge
// interface, so the list/create/rename/delete_workspace agent tools manage
// workspaces the same way the switcher does (create seeds a blank template;
// every change notifies open UIs over SSE so the switcher refreshes live).
// Provenance and the "can't delete current/last" guards live in the tools layer
// and the manager respectively — this adapter is the thin manager↔tools seam.
type workspaceBridge struct{ srv *Server }

// WorkspaceBridge returns this server's workspace bridge for injection into the
// workspace runtimes (via Manager.SetWorkspaceBridge).
func (s *Server) WorkspaceBridge() tools.WorkspaceBridge { return workspaceBridge{srv: s} }

func toWorkspaceInfo(m workspace.Meta) tools.WorkspaceInfo {
	return tools.WorkspaceInfo{
		ID:             m.ID,
		Name:           m.Name,
		Path:           m.Path,
		CreatedByAgent: m.CreatedBy != "",
	}
}

func (b workspaceBridge) ListWorkspaces() []tools.WorkspaceInfo {
	metas := b.srv.workspaces.List()
	out := make([]tools.WorkspaceInfo, 0, len(metas))
	for _, m := range metas {
		out = append(out, toWorkspaceInfo(m))
	}
	return out
}

func (b workspaceBridge) CreateWorkspace(name, parentPath, createdBy string) (tools.WorkspaceInfo, error) {
	wsNew, err := b.srv.workspaces.Create(name, parentPath, createdBy)
	if err != nil {
		return tools.WorkspaceInfo{}, err
	}
	// Seed the blank template (default agent + config tree) so the new workspace
	// is usable immediately, exactly like a UI-created one.
	b.srv.seedWorkspaceFromTemplate(context.Background(), wsNew, blankTemplateID)
	b.srv.publishWorkspacesChanged(fmt.Sprintf("Bir ajan yeni bir workspace oluşturdu: %s", wsNew.Name))
	return toWorkspaceInfo(wsNew.Meta), nil
}

func (b workspaceBridge) RenameWorkspace(id, name string) (tools.WorkspaceInfo, error) {
	if err := b.srv.workspaces.Rename(id, name); err != nil {
		return tools.WorkspaceInfo{}, err
	}
	ws, err := b.srv.workspaces.Get(id)
	if err != nil {
		return tools.WorkspaceInfo{}, err
	}
	b.srv.publishWorkspacesChanged(fmt.Sprintf("Bir ajan bir workspace'i yeniden adlandırdı: %s", ws.Name))
	return toWorkspaceInfo(ws.Meta), nil
}

func (b workspaceBridge) DeleteWorkspace(id string) error {
	if err := b.srv.workspaces.Delete(id); err != nil {
		return err
	}
	b.srv.publishWorkspacesChanged("Bir ajan bir workspace'i sildi.")
	return nil
}
