package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// Workspace self-management tools let an agent list, create, rename and delete
// the WORKSPACES of the application â€” the fully-isolated stores (each its own
// agents/sessions/flows/secrets) that the switcher hops between. Unlike the
// other self-management tools (which act inside the current workspace's DB),
// these reach across workspace boundaries, so they go through a bridge wired in
// from the workspace manager (analogous to the settings bridge).
//
// Safety boundaries mirror the rest of the suite:
//   - list/create/rename are allowed on any workspace (non-destructive),
//   - delete is provenance-guarded: only workspaces an agent CREATED
//     (CreatedBy != "") may be removed â€” never a user-made workspace,
//   - an agent can never delete the workspace it is currently running in,
//   - the manager additionally forbids deleting the last remaining workspace.

// WorkspaceInfo is the bridge-facing view of one workspace. CreatedByAgent is
// true when the workspace was created by an agent (and is therefore deletable
// by an agent); Path is the on-disk data directory ("" = default location).
type WorkspaceInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path,omitempty"`
	CreatedByAgent bool   `json:"createdByAgent"`
}

// WorkspaceBridge is the seam between the agent tools and the application's
// workspace manager. It is implemented in the api layer (where the manager and
// the template-seeding + UI-notify hooks live) and injected into every runtime.
type WorkspaceBridge interface {
	// ListWorkspaces returns every workspace in creation order.
	ListWorkspaces() []WorkspaceInfo
	// CreateWorkspace makes a new isolated workspace tagged with createdBy
	// (the acting agent's id) and returns its info. parentPath, when non-empty,
	// is a user-chosen folder under which the workspace's own data dir is made;
	// empty uses the default location.
	CreateWorkspace(name, parentPath, createdBy string) (WorkspaceInfo, error)
	// RenameWorkspace changes a workspace's display name and returns its info.
	RenameWorkspace(id, name string) (WorkspaceInfo, error)
	// DeleteWorkspace removes a workspace and all its data. The manager forbids
	// deleting the last remaining workspace.
	DeleteWorkspace(id string) error
}

// wsDeps carries what the workspace tools need: the cross-workspace bridge, the
// acting agent's id (the provenance stamp on created workspaces) and the id of
// the workspace this agent is running in (so it never deletes itself out).
type wsDeps struct {
	bridge      WorkspaceBridge
	actorID     string
	currentWsID string
}

// ---- list_workspaces ----

// ListWorkspacesTool returns the application's workspaces as a compact list.
type ListWorkspacesTool struct{ d wsDeps }

// NewListWorkspacesTool constructs list_workspaces.
func NewListWorkspacesTool(b WorkspaceBridge, actorID, currentWsID string) ListWorkspacesTool {
	return ListWorkspacesTool{d: wsDeps{bridge: b, actorID: actorID, currentWsID: currentWsID}}
}

func (ListWorkspacesTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "list_workspaces",
		Description: "List the application's workspaces (fully isolated stores, each with its own agents, sessions, flows and secrets). Returns id, name, on-disk path, whether you created it (and may therefore delete it), and whether it is the one you are currently running in. Use this before rename_workspace / delete_workspace to get ids.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ListWorkspacesTool) Call(_ context.Context, _ json.RawMessage) (string, error) {
	if t.d.bridge == nil {
		return "", fmt.Errorf("workspace bridge not configured")
	}
	type row struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Path           string `json:"path,omitempty"`
		CreatedByAgent bool   `json:"createdByAgent"`
		IsCurrent      bool   `json:"isCurrent"`
	}
	list := t.d.bridge.ListWorkspaces()
	out := make([]row, 0, len(list))
	for _, w := range list {
		out = append(out, row{
			ID: w.ID, Name: w.Name, Path: w.Path,
			CreatedByAgent: w.CreatedByAgent,
			IsCurrent:      w.ID == t.d.currentWsID,
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ---- create_workspace ----

// CreateWorkspaceTool makes a new isolated workspace.
type CreateWorkspaceTool struct{ d wsDeps }

// NewCreateWorkspaceTool constructs create_workspace.
func NewCreateWorkspaceTool(b WorkspaceBridge, actorID, currentWsID string) CreateWorkspaceTool {
	return CreateWorkspaceTool{d: wsDeps{bridge: b, actorID: actorID, currentWsID: currentWsID}}
}

func (CreateWorkspaceTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "create_workspace",
		Description: "Create a new, fully-isolated workspace (its own agents, sessions, flows, secrets and store). It is tagged as created by you, so you can later rename or delete it. Optionally pass a path: a parent folder on disk under which the workspace's own data directory is created (empty = default location). Returns the new workspace id. Note: the new workspace starts empty â€” switch to it (in the UI) or create agents in it to use it.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"name":{"type":"string","description":"Display name for the workspace"},
				"path":{"type":"string","description":"Optional parent folder for the workspace's data directory (absolute path). Empty uses the default location."}
			},
			"required":["name"],
			"additionalProperties":false
		}`),
	}
}

func (t CreateWorkspaceTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.d.bridge == nil {
		return "", fmt.Errorf("workspace bridge not configured")
	}
	var in struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	created, err := t.d.bridge.CreateWorkspace(in.Name, strings.TrimSpace(in.Path), t.d.actorID)
	if err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": created.ID, "name": created.Name, "action": "created"})
	return string(b), nil
}

// ---- rename_workspace ----

// RenameWorkspaceTool changes a workspace's display name (allowed on any).
type RenameWorkspaceTool struct{ d wsDeps }

// NewRenameWorkspaceTool constructs rename_workspace.
func NewRenameWorkspaceTool(b WorkspaceBridge, actorID, currentWsID string) RenameWorkspaceTool {
	return RenameWorkspaceTool{d: wsDeps{bridge: b, actorID: actorID, currentWsID: currentWsID}}
}

func (RenameWorkspaceTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "rename_workspace",
		Description: "Rename a workspace (its display name in the switcher). Allowed on any workspace. Pass the workspace id (see list_workspaces) and the new name.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"id":{"type":"string","description":"The workspace id to rename (see list_workspaces)"},
				"name":{"type":"string","description":"The new display name"}
			},
			"required":["id","name"],
			"additionalProperties":false
		}`),
	}
}

func (t RenameWorkspaceTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.d.bridge == nil {
		return "", fmt.Errorf("workspace bridge not configured")
	}
	var in struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	in.Name = strings.TrimSpace(in.Name)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if in.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	updated, err := t.d.bridge.RenameWorkspace(in.ID, in.Name)
	if err != nil {
		return "", fmt.Errorf("rename workspace: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": updated.ID, "name": updated.Name, "action": "renamed"})
	return string(b), nil
}

// ---- delete_workspace ----

// DeleteWorkspaceTool removes an agent-created workspace (provenance-enforced).
type DeleteWorkspaceTool struct{ d wsDeps }

// NewDeleteWorkspaceTool constructs delete_workspace.
func NewDeleteWorkspaceTool(b WorkspaceBridge, actorID, currentWsID string) DeleteWorkspaceTool {
	return DeleteWorkspaceTool{d: wsDeps{bridge: b, actorID: actorID, currentWsID: currentWsID}}
}

func (DeleteWorkspaceTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_workspace",
		Description: "Delete a workspace that was created by an agent (not by the user) and ALL of its data â€” agents, sessions, flows, secrets and files. This is irreversible. You cannot delete the workspace you are currently running in, nor the last remaining workspace. Pass the workspace id (see list_workspaces).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"id":{"type":"string","description":"The workspace id to delete (see list_workspaces)"}},
			"required":["id"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteWorkspaceTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.d.bridge == nil {
		return "", fmt.Errorf("workspace bridge not configured")
	}
	var in struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return "", fmt.Errorf("id is required")
	}
	if in.ID == t.d.currentWsID {
		return "", fmt.Errorf("an agent cannot delete the workspace it is currently running in")
	}
	// Provenance guard: only agent-created workspaces may be deleted by an agent.
	var target *WorkspaceInfo
	for _, w := range t.d.bridge.ListWorkspaces() {
		if w.ID == in.ID {
			ws := w
			target = &ws
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("no workspace with id %q (use list_workspaces)", in.ID)
	}
	if !target.CreatedByAgent {
		return "", fmt.Errorf("workspace %q was created by the user and cannot be deleted by an agent", target.Name)
	}
	if err := t.d.bridge.DeleteWorkspace(in.ID); err != nil {
		return "", fmt.Errorf("delete workspace: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"id": in.ID, "action": "deleted"})
	return string(b), nil
}
