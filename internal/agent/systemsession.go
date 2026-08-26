package agent

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// isSystemAgentSession reports whether session belongs to an agent marked as a
// system agent. Agent.System is the source of truth; sessions do not duplicate
// that classification.
func (r *Runtime) isSystemAgentSession(ctx context.Context, session db.Session) (bool, error) {
	if session.AgentID == "" {
		return false, fmt.Errorf("session %s has no agent", session.ID)
	}
	agent, err := r.db.GetAgent(ctx, session.AgentID)
	if err != nil {
		return false, fmt.Errorf("resolve agent %s for session %s: %w", session.AgentID, session.ID, err)
	}
	return agent.System, nil
}
