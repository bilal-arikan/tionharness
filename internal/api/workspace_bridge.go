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
func (s *Server) WorkspaceBridge() tools.WorkspaceBridge {
	tools.SetChangeNotifier(toolChangeNotifier{server: s})
	return workspaceBridge{srv: s}
}

func toWorkspaceInfo(m workspace.Meta) tools.WorkspaceInfo {
	return tools.WorkspaceInfo{
		ID:             m.ID,
		Name:           m.Name,
		Path:           m.Path,
		CreatedByAgent: m.CreatedBy != "",
		CreatedAt:      m.CreatedAt,
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

func (b workspaceBridge) CreateWorkspace(name string, options tools.CreateWorkspaceOptions) (tools.WorkspaceInfo, error) {
	wsNew, err := b.srv.workspaces.Create(name, options.ParentPath, options.CreatedBy)
	if err != nil {
		return tools.WorkspaceInfo{}, err
	}
	patch := workspace.WSSettingsPatch{}
	if options.Icon != "" {
		patch.Icon = &options.Icon
	}
	if options.Color != "" {
		patch.Color = &options.Color
	}
	if options.WorkingDir != "" {
		patch.DefaultWorkingDir = &options.WorkingDir
	}
	if patch.Icon != nil || patch.Color != nil || patch.DefaultWorkingDir != nil {
		if _, err := b.srv.workspaces.UpdateSettings(wsNew.ID, patch); err != nil {
			return tools.WorkspaceInfo{}, fmt.Errorf("apply workspace settings: %w", err)
		}
	}
	templateID := options.Template
	if templateID == "" {
		templateID = blankTemplateID
	}
	b.srv.seedWorkspaceFromTemplate(context.Background(), wsNew, templateID)
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
