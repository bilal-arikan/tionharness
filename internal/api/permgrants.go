package api

import (
	"sync"

	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// permGrantStore holds per-session permission grants ("Always allow" choices) so
// they survive across turns within a session. Both the native gate (via context)
// and the CLI permission-prompt tool (via the run) consult the same per-session
// instance. In-memory: grants reset on server restart.
//
// Keyed by scopeKey(workspaceID, sessionID): session ids repeat across workspace
// stores, and a grant is an explicit "always allow THIS session" decision — it
// must never be inherited by a same-numbered session in another workspace.
type permGrantStore struct {
	mu        sync.Mutex
	bySession map[string]*tools.PermissionGrants
}

func newPermGrantStore() *permGrantStore {
	return &permGrantStore{bySession: map[string]*tools.PermissionGrants{}}
}

// forSession returns the grant set for a session, creating it on first use.
func (p *permGrantStore) forSession(wsID, sessionID string) *tools.PermissionGrants {
	key := scopeKey(wsID, sessionID)
	p.mu.Lock()
	defer p.mu.Unlock()
	g := p.bySession[key]
	if g == nil {
		g = tools.NewPermissionGrants()
		p.bySession[key] = g
	}
	return g
}
