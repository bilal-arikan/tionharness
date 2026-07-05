package api

import (
	"sync"

	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// permGrantStore holds per-session permission grants ("Always allow" choices) so
// they survive across turns within a session. Both the native gate (via context)
// and the CLI permission-prompt tool (via the run) consult the same per-session
// instance. In-memory: grants reset on server restart.
type permGrantStore struct {
	mu        sync.Mutex
	bySession map[string]*tools.PermissionGrants
}

func newPermGrantStore() *permGrantStore {
	return &permGrantStore{bySession: map[string]*tools.PermissionGrants{}}
}

// forSession returns the grant set for a session, creating it on first use.
func (p *permGrantStore) forSession(sessionID string) *tools.PermissionGrants {
	p.mu.Lock()
	defer p.mu.Unlock()
	g := p.bySession[sessionID]
	if g == nil {
		g = tools.NewPermissionGrants()
		p.bySession[sessionID] = g
	}
	return g
}
