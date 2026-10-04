package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// BriefPreview composes what a FRESH session of agent would be briefed with,
// without touching any session's frozen brief. Used by the agent context
// preview (api.buildAgentDynamicPrompt).
func (r *Runtime) BriefPreview(ctx context.Context, agent db.Agent) string {
	if r == nil || r.aware == nil {
		return ""
	}
	in := r.awarenessInput(ctx, db.Session{AgentID: agent.ID, Kind: "chat"}, agent, true)
	return r.aware.Preview(ctx, in).Text
}
