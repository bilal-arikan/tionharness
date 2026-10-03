package agent

import (
	"github.com/bilal-arikan/tionharness/internal/artifacts"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// artifactSink connects autonomous turns to the shared persistence core.
type artifactSink struct {
	*artifacts.Sink
}

// NewArtifactSink binds artifacts to this run's session and agent, including
// scheduler, spawn and flow turns that have no SSE chat client.
func (r *Runtime) NewArtifactSink(sessionID, agentID string) tools.ArtifactSink {
	return &artifactSink{Sink: artifacts.NewSink(r.db, sessionID, agentID, r.publish)}
}
