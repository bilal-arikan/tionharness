package tools

import "context"

// ArtifactRef identifies a stored artifact returned to the model after a
// create/update so the chat UI can render a clickable card linking to it.
type ArtifactRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

// ArtifactSink persists artifacts created/updated by an agent during a turn. It
// is supplied by the interactive chat layer (carrying the origin session +
// agent); autonomous runs without a sink make the artifact tools graceful
// no-ops that return an error the model can recover from.
//
// Kept in the tools package (not agent/api) so built-in tools can reach it
// without importing those packages (which would cycle).
type ArtifactSink interface {
	CreateArtifact(ctx context.Context, title, kind, language, content string) (ArtifactRef, error)
	UpdateArtifact(ctx context.Context, id, content string) (ArtifactRef, error)
}

type artifactKey struct{}

// WithArtifacts attaches an artifact sink to ctx so the create_artifact /
// update_artifact tools can persist content mid-turn.
func WithArtifacts(ctx context.Context, sink ArtifactSink) context.Context {
	return context.WithValue(ctx, artifactKey{}, sink)
}

// artifactsFrom returns the sink attached to ctx, or nil when none is present
// (e.g. scheduler runs with no open client connection).
func artifactsFrom(ctx context.Context) ArtifactSink {
	s, _ := ctx.Value(artifactKey{}).(ArtifactSink)
	return s
}
