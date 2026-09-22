package api

import (
	"context"
	"encoding/json"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// cliToolRiskFlagged runs the decider's tool-risk check (agent/decide_toolrisk.go)
// for a claude-cli permission request that a standing grant would otherwise
// auto-allow. Autonomous runs are never flagged: nobody could answer the prompt,
// and the site must never block an unattended run.
func (b *interactionBackend) cliToolRiskFlagged(ctx context.Context, run *chatRun, toolName string, input json.RawMessage) (bool, string) {
	if run == nil || run.autonomous || b.apiSrv == nil {
		return false, ""
	}
	ws, err := b.apiSrv.workspaces.Get(run.workspaceID)
	if err != nil || ws == nil || ws.Runtime == nil {
		return false, ""
	}
	// The CLI's grants live on the run rather than the context; put them there so
	// an exact "always" approval is honoured the way the native loop honours it.
	ctx = tools.WithGrants(agent.WithSessionID(ctx, run.sessionID), run.grantStore())
	// The permission prompt does not say which agent asked, so the decision is
	// logged in the decider ledger but billed to no agent budget.
	return ws.Runtime.ToolRiskFlagged(ctx, db.Agent{}, toolName, input, true)
}
