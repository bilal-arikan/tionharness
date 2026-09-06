package workspace

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/agent"
)

// Propagating a built-in system agent edit.
//
// A built-in's customisation lives in one app-global document, and each
// workspace holds its own copy of the built-in row seeded from it. The workspace
// that served the edit re-imposes its own row inline (see db's
// storeBuiltinCustomisation); this is what brings every OTHER already-open
// workspace in line, so the change is visible everywhere without a restart.

// PropagateSystemAgentEdit re-seeds the built-in system agents of every open
// workspace except `originID`, which already applied the edit itself.
//
// Best-effort per workspace: one workspace failing to re-seed must not stop the
// others, so failures are logged and the pass continues. A workspace that misses
// the update still picks it up on its next boot, since seeding reads the same
// app-global document.
func (m *Manager) PropagateSystemAgentEdit(ctx context.Context, originID string) {
	m.mu.RLock()
	targets := make([]*Workspace, 0, len(m.workspaces))
	for id, ws := range m.workspaces {
		if id == originID || ws == nil || ws.DB == nil {
			continue
		}
		targets = append(targets, ws)
	}
	m.mu.RUnlock()

	defs := agent.SystemAgentDefaults()
	for _, ws := range targets {
		if err := ws.DB.EnsureSystemAgents(ctx, defs...); err != nil {
			m.logger.Warn("system agent edit not propagated to workspace",
				"workspace", ws.ID, "error", err)
		}
	}
}
