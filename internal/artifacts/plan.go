package artifacts

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// RecordPlan persists and announces the session's rolling approved plan.
// The chat adapter exposes AppendPlanArtifact separately so autonomous sinks
// retain their original capabilities at the plan-approval bridge.
func (s *Sink) RecordPlan(ctx context.Context, planMarkdown string) (tools.ArtifactRef, error) {
	a, err := s.db.AppendPlanArtifact(ctx, s.sessionID, s.agentID, planMarkdown)
	return s.result(a, "güncellendi", err)
}
