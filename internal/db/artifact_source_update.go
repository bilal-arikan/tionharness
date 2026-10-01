package db

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// UpdateArtifactFromSource shares file-backed updates across interactive and
// autonomous tool sinks. Relative paths follow the session's project directory.
func (d *DB) UpdateArtifactFromSource(ctx context.Context, id, sessionID, path string) (Artifact, error) {
	a, err := d.GetArtifact(ctx, id)
	if err != nil {
		return Artifact{}, err
	}
	if a.SessionID != sessionID {
		return Artifact{}, fmt.Errorf("artifact does not belong to this session")
	}
	if !filepath.IsAbs(path) {
		if wd := d.sessionWorkingDir(sessionID); wd != "" {
			path = filepath.Join(wd, filepath.FromSlash(path))
		} else {
			path = filepath.Join(d.workspaceDir(), filepath.FromSlash(path))
		}
	}
	if isTextArtifact(a.Kind) {
		f, err := os.Open(path)
		if err != nil {
			return Artifact{}, err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return Artifact{}, err
		}
		if !info.Mode().IsRegular() {
			return Artifact{}, fmt.Errorf("sourcePath must be a regular file")
		}
		const maxTextBytes = 16 * 1024 * 1024
		body, err := io.ReadAll(io.LimitReader(f, maxTextBytes+1))
		if err != nil {
			return Artifact{}, err
		}
		if len(body) > maxTextBytes {
			return Artifact{}, fmt.Errorf("text artifact source exceeds 16 MiB; use a file artifact")
		}
		return d.UpdateArtifactContent(ctx, id, string(body))
	}
	rel, err := d.ImportMediaSource(sessionID, path)
	if err != nil {
		return Artifact{}, err
	}
	a, _, err = d.UpdateArtifactSource(ctx, id, rel)
	return a, err
}
