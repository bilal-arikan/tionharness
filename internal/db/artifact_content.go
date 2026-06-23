package db

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Artifact bodies for text-like kinds are stored as real files under
// <workspace>/artifacts/<id><ext> rather than embedded in the artifact JSON, so
// the content is a first-class file on disk (browsable, agent-readable). The JSON
// references the file via Artifact.ContentFile. Media kinds keep their existing
// SourcePath (the binary file) and are untouched here.

// isTextArtifact reports whether an artifact's body is text we externalise to a
// content file (vs a media/file kind whose bytes already live on disk).
func isTextArtifact(kind string) bool {
	switch kind {
	case ArtifactMarkdown, ArtifactCode, ArtifactHTML, ArtifactText, ArtifactSVG, ArtifactMermaid:
		return true
	default:
		return false
	}
}

// codeExtByLang maps a code artifact's language to a file extension for its
// content file. Unknown languages fall back to ".txt".
var codeExtByLang = map[string]string{
	"go": ".go", "python": ".py", "javascript": ".js", "typescript": ".ts",
	"tsx": ".tsx", "jsx": ".jsx", "json": ".json", "yaml": ".yaml", "toml": ".toml",
	"bash": ".sh", "sh": ".sh", "sql": ".sql", "rust": ".rs", "ruby": ".rb",
	"java": ".java", "c": ".c", "cpp": ".cpp", "csharp": ".cs", "php": ".php",
	"css": ".css", "xml": ".xml", "kotlin": ".kt", "swift": ".swift", "html": ".html",
}

// artifactExt returns the on-disk file extension (with dot) for a text artifact.
func artifactExt(a Artifact) string {
	switch a.Kind {
	case ArtifactMarkdown:
		return ".md"
	case ArtifactHTML:
		return ".html"
	case ArtifactSVG:
		return ".svg"
	case ArtifactMermaid:
		return ".mmd"
	case ArtifactText:
		return ".txt"
	case ArtifactCode:
		if ext := codeExtByLang[strings.ToLower(a.Language)]; ext != "" {
			return ext
		}
		return ".txt"
	default:
		return ".txt"
	}
}

// workspaceDir is the workspace sandbox root (sibling of the store root), where
// all artifact files live under artifacts/<sessionDir>/.
func (d *DB) workspaceDir() string {
	return filepath.Join(filepath.Dir(d.root), "workspace")
}

// artifactSessionDir is the per-session folder segment that collects an
// artifact's files. Session-scoped artifacts group under their session id;
// sessionless ones (e.g. manual uploads) share a "_shared" bucket.
func artifactSessionDir(sessionID string) string {
	if sessionID == "" {
		return "_shared"
	}
	return sessionID
}

// ArtifactsDir returns the absolute folder that holds a session's artifact files
// (content + uploads). Used by cleanup on session delete.
func (d *DB) ArtifactsDir(sessionID string) string {
	return filepath.Join(d.workspaceDir(), "artifacts", artifactSessionDir(sessionID))
}

// writeArtifactContent writes a text artifact's body to its content file under
// <workspace>/artifacts/<sessionDir>/ and records the relative path in ContentFile.
func (d *DB) writeArtifactContent(a *Artifact) error {
	rel := "artifacts/" + artifactSessionDir(a.SessionID) + "/" + a.ID + artifactExt(*a)
	abs := filepath.Join(d.workspaceDir(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(abs, []byte(a.Content), 0o644); err != nil {
		return err
	}
	a.ContentFile = rel
	return nil
}

// readArtifactContent loads a text artifact's body from its content file into
// Content. Best-effort: a missing file leaves Content empty.
func (d *DB) readArtifactContent(a *Artifact) {
	if a.ContentFile == "" {
		return
	}
	abs := filepath.Join(d.workspaceDir(), filepath.FromSlash(a.ContentFile))
	if b, err := os.ReadFile(abs); err == nil {
		a.Content = string(b)
	}
}

// ImportMediaSource resolves a media/file artifact's source into a
// workspace-relative path the file server can stream. src may be absolute or
// already workspace-relative:
//   - a file already under <workspace>/ is returned as a clean relative path
//     (no copy — it is served in place);
//   - any other path is copied into artifacts/<sessionDir>/ and the new relative
//     path is returned, so the artifact owns a self-contained, servable copy
//     even if the original (e.g. a screenshot in Downloads) is later moved.
//
// The source must exist and be a regular file. The returned path uses forward
// slashes (matching Artifact.SourcePath / ContentFile convention).
func (d *DB) ImportMediaSource(sessionID, src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", fmt.Errorf("sourcePath is required for media artifacts")
	}
	wsDir := d.workspaceDir()
	abs := src
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(wsDir, filepath.FromSlash(src))
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("source file not found: %s", src)
	}
	// Already inside the workspace → store a clean relative path, no copy.
	if rel, err := filepath.Rel(wsDir, abs); err == nil &&
		rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel), nil
	}
	// Outside the workspace → copy in under artifacts/<sessionDir>/.
	rel := "artifacts/" + artifactSessionDir(sessionID) + "/" +
		fmt.Sprintf("media-%d%s", now(), filepath.Ext(abs))
	dst := filepath.Join(wsDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := copyFileContents(abs, dst); err != nil {
		return "", err
	}
	return rel, nil
}

// copyFileContents copies src to dst (truncating dst), streaming so large media
// files never load fully into memory.
func copyFileContents(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// removeArtifactContent deletes a text artifact's content file (best-effort).
func (d *DB) removeArtifactContent(a Artifact) {
	if a.ContentFile == "" {
		return
	}
	abs := filepath.Join(d.workspaceDir(), filepath.FromSlash(a.ContentFile))
	_ = os.Remove(abs)
}
