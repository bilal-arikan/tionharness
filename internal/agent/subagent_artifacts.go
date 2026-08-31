package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// collectChildArtifacts lists the artifacts a finished subagent run produced, as
// references for the delegating caller.
//
// Only references travel back. Delegation exists so a sub-task's output does not
// flood the caller's context, and inlining a produced document would undo that in
// the one case where the output is largest; the caller reads a body by id when it
// needs one.
//
// A listing failure is NOT propagated: the subagent's actual work is already done
// and persisted, and failing the whole call over a missing index line would throw
// away a completed run. The caller simply sees no artifact references, and the
// artifacts remain reachable on the child session.
func (r *Runtime) collectChildArtifacts(ctx context.Context, childSessionID string) []tools.SubagentArtifact {
	rows, err := r.db.ListArtifacts(ctx, childSessionID)
	if err != nil {
		r.logger.Warn("subagent artifact listing failed", "session", childSessionID, "error", err)
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	out := make([]tools.SubagentArtifact, 0, len(rows))
	for _, a := range rows {
		out = append(out, tools.SubagentArtifact{ID: a.ID, Title: a.Title, Kind: a.Kind})
	}
	return out
}
