package api

import (
	"context"
	"path/filepath"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

func (s artifactSink) sourcePath(ctx context.Context, path string) string {
	if !filepath.IsAbs(path) {
		if session, err := s.db.GetSession(ctx, s.sessionID); err == nil && session.WorkingDir != "" {
			return filepath.Join(session.WorkingDir, path)
		}
	}
	return path
}

func (s artifactSink) UpdateArtifactSource(ctx context.Context, id, path string) (tools.ArtifactRef, error) {
	a, err := s.db.UpdateArtifactFromSource(ctx, id, s.sessionID, path)
	if err != nil {
		return tools.ArtifactRef{}, err
	}
	s.notifyArtifact(a, "updated")
	return toArtifactRef(a), nil
}
