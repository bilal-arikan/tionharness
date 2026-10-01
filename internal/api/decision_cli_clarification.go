package api

import (
	"context"
	"encoding/json"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
)

func (b *interactionBackend) reviewCLIClarification(ctx context.Context, run *chatRun, raw json.RawMessage) (string, bool) {
	if run == nil || run.autonomous || b.apiSrv == nil || b.apiSrv.workspaces == nil {
		return "", false
	}
	ws, err := b.apiSrv.workspaces.Get(run.workspaceID)
	if err != nil || ws == nil || ws.Runtime == nil {
		return "", false
	}
	ctx = agent.WithSessionID(ctx, run.sessionID)
	caller := db.Agent{}
	if session, e := ws.DB.GetSession(ctx, run.sessionID); e == nil {
		caller, _ = ws.DB.GetAgent(ctx, session.OwnerAgentID())
	}
	return ws.Runtime.ReviewClarification(ctx, caller, raw)
}
