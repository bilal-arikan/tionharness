package artifacts

import (
	"context"
	"path/filepath"
)

func (s *Sink) sourcePath(ctx context.Context, path string) string {
	if !filepath.IsAbs(path) {
		if session, err := s.db.GetSession(ctx, s.sessionID); err == nil && session.WorkingDir != "" {
			return filepath.Join(session.WorkingDir, path)
		}
	}
	return path
}
