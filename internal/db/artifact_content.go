package db

import (
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

// removeArtifactContent deletes a text artifact's content file (best-effort).
func (d *DB) removeArtifactContent(a Artifact) {
	if a.ContentFile == "" {
		return
	}
	abs := filepath.Join(d.workspaceDir(), filepath.FromSlash(a.ContentFile))
	_ = os.Remove(abs)
}
