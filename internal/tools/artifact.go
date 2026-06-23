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
	CreateArtifact(ctx context.Context, spec CreateArtifactSpec) (ArtifactRef, error)
	UpdateArtifact(ctx context.Context, id, content string) (ArtifactRef, error)
}

// CreateArtifactSpec describes a new artifact. For text kinds (markdown/code/
// html/text/svg/mermaid) Content holds the body. For media/file kinds (image/
// video/audio/file) SourcePath points at the file on disk — absolute or
// workspace-relative — and Content is an optional caption; the bytes are never
// carried through the model context.
type CreateArtifactSpec struct {
	Title      string
	Kind       string
	Language   string
	Content    string
	SourcePath string
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

// HasArtifactSink reports whether an artifact sink is attached to ctx, so a
// caller can install a fallback only when one is missing (avoids overriding the
// chat layer's session-bound sink).
func HasArtifactSink(ctx context.Context) bool { return artifactsFrom(ctx) != nil }
