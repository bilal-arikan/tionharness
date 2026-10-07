package agent

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// RecordEntityRead stamps the workspace read ledger when a successful tool call
// read one map node (tools.ReadRefFor decides which calls count). Both provider
// paths call it: the native tool loop after every call, the CLI interaction
// bridge after every bridged call. Best-effort: a failed write is logged, never
// surfaced to the turn.
func (r *Runtime) RecordEntityRead(ctx context.Context, sessionID, agentID, toolName string, input json.RawMessage) {
	if r == nil || r.db == nil {
		return
	}
	var resolve func(string) (string, bool)
	if store := r.Notes(); store != nil {
		resolve = func(ref string) (string, bool) {
			n, ok := store.Resolve(ref)
			return n.ID, ok
		}
	}
	ref, ok := tools.ReadRefFor(toolName, input, resolve)
	if !ok {
		return
	}
	if sessionID == "" {
		sessionID = tools.CurrentSessionID(ctx)
	}
	if err := r.db.RecordViewRead(ref, agentID, sessionID); err != nil {
		slog.Warn("record view read failed", "component", "agent", "ref", ref, "error", err)
	}
}
