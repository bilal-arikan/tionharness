package api

import "testing"

// TestWorkspaceOptionalPath pins the bootstrap allowlist: the routes that must
// work with NO active workspace (workspace CRUD, templates, folder picker, plus
// process-global infra) are optional; every workspace-scoped route is not, so it
// gets a clean 409 instead of dereferencing a nil workspace.
func TestWorkspaceOptionalPath(t *testing.T) {
	optional := []string{
		"/api/workspaces",
		"/api/workspaces/WS1",
		"/api/workspaces/attach",
		"/api/workspace-templates",
		"/api/pick-folder",
		"/api/external-tools",
		"/api/events",
		// Process-global infra: none of these read a workspace, so a fresh
		// install must be able to reach them before one exists.
		"/health",
		"/api/version",
		"/api/version/update",
		"/api/debug/store-stats",
	}
	for _, p := range optional {
		if !workspaceOptionalPath(p) {
			t.Errorf("%s must be workspace-optional (bootstrap route)", p)
		}
	}
	scoped := []string{
		"/api/agents",
		"/api/insight/lenses",
		"/api/insight/scan",
		"/api/tasks",
		"/api/workspace-settings", // resolved from X-Workspace-Id → needs a workspace
		"/api/sessions",
	}
	for _, p := range scoped {
		if workspaceOptionalPath(p) {
			t.Errorf("%s is workspace-scoped and must NOT be optional", p)
		}
	}
}
