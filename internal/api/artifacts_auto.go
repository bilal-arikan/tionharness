package api

import (
	"context"
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
)

// fileWriteTools are the tool names that write a full file body we can capture
// as an artifact. The native built-in and the claude CLI share the name "Write";
// create_file covers an alternate CLI shape. Edits (partial diffs) are excluded.
var fileWriteTools = map[string]bool{
	"Write":       true,
	"create_file": true,
}

// parseFileWrite extracts the path + full content from a file-writing tool call.
// It accepts both the native shape ({path,content}) and the claude CLI shape
// ({file_path,content}). ok is false when the step is not a capturable write.
func parseFileWrite(tool string, input json.RawMessage) (filePath, content string, ok bool) {
	if !fileWriteTools[tool] || len(input) == 0 {
		return "", "", false
	}
	var in struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", "", false
	}
	filePath = in.Path
	if filePath == "" {
		filePath = in.FilePath
	}
	content = in.Content
	if strings.TrimSpace(filePath) == "" || content == "" {
		return "", "", false
	}
	return filePath, content, true
}

// codeLangByExt maps a lowercase file extension to a syntax-highlight language
// for kind=code artifacts.
var codeLangByExt = map[string]string{
	".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript",
	".tsx": "tsx", ".jsx": "jsx", ".json": "json", ".yaml": "yaml", ".yml": "yaml",
	".toml": "toml", ".sh": "bash", ".sql": "sql", ".rs": "rust", ".rb": "ruby",
	".java": "java", ".c": "c", ".h": "c", ".cpp": "cpp", ".cs": "csharp",
	".php": "php", ".css": "css", ".xml": "xml", ".kt": "kotlin", ".swift": "swift",
}

// artifactMediaKindByExt maps a file extension to a media/file artifact kind.
// These render from SourcePath (the file on disk) rather than from inline text —
// so a screenshot/PDF/binary becomes a viewable artifact without its bytes ever
// entering the model context.
var artifactMediaKindByExt = map[string]string{
	".png": db.ArtifactImage, ".jpg": db.ArtifactImage, ".jpeg": db.ArtifactImage,
	".gif": db.ArtifactImage, ".webp": db.ArtifactImage, ".bmp": db.ArtifactImage,
	".ico": db.ArtifactImage, ".avif": db.ArtifactImage,
	".mp4": db.ArtifactVideo, ".webm": db.ArtifactVideo, ".mov": db.ArtifactVideo,
	".mp3": db.ArtifactAudio, ".wav": db.ArtifactAudio, ".ogg": db.ArtifactAudio, ".m4a": db.ArtifactAudio,
	".pdf": db.ArtifactFile, ".zip": db.ArtifactFile, ".xlsx": db.ArtifactFile,
	".docx": db.ArtifactFile, ".pptx": db.ArtifactFile,
}

// mediaKindForExt returns the media/file artifact kind for an extension, or ""
// when the extension is not a recognised media/file type.
func mediaKindForExt(ext string) string {
	return artifactMediaKindByExt[strings.ToLower(ext)]
}

// artifactKindForPath infers an artifact kind (and code language) from a file's
// extension so the right renderer is used in the viewer. Media/file extensions
// resolve to their media kind (rendered from SourcePath).
func artifactKindForPath(filePath string) (kind, language string) {
	ext := strings.ToLower(path.Ext(strings.ReplaceAll(filePath, "\\", "/")))
	if mk := mediaKindForExt(ext); mk != "" {
		return mk, ""
	}
	switch ext {
	case ".md", ".markdown":
		return db.ArtifactMarkdown, ""
	case ".html", ".htm":
		return db.ArtifactHTML, ""
	case ".svg":
		return db.ArtifactSVG, ""
	case ".mmd", ".mermaid":
		return db.ArtifactMermaid, ""
	case ".txt", ".csv", ".tsv", ".log", "":
		return db.ArtifactText, ""
	}
	if lang, ok := codeLangByExt[ext]; ok {
		return db.ArtifactCode, lang
	}
	return db.ArtifactText, ""
}

// producedPathRe matches an absolute file path (Windows drive-letter or POSIX)
// ending in a media/file extension, used to find files a tool reported saving in
// its output text/JSON (e.g. a screenshot tool returning {"path":"C:\\...png"}).
var producedPathRe = regexp.MustCompile(
	`(?i)(?:[a-z]:\\[^"'\r\n<>|]*?|/[^"'\r\n<>|]*?)\.(?:png|jpe?g|gif|webp|bmp|ico|avif|mp4|webm|mov|mp3|wav|ogg|m4a|pdf|zip|xlsx|docx|pptx)`)

// extractProducedMediaPaths scans a tool's output for absolute paths of media/
// file artifacts the tool saved. Best-effort and heuristic: candidates are later
// validated by stat (a path to a non-existent file is silently skipped), so a
// false match never creates a bogus artifact. JSON-escaped backslashes (\\) in
// the output are unescaped first so Windows paths match.
func extractProducedMediaPaths(output string) []string {
	if output == "" {
		return nil
	}
	unescaped := strings.ReplaceAll(output, `\\`, `\`)
	matches := producedPathRe.FindAllString(unescaped, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

// baseName returns the final path element (handling both / and \ separators) for
// use as an artifact title.
func baseName(filePath string) string {
	return path.Base(strings.ReplaceAll(strings.TrimRight(filePath, "/\\"), "\\", "/"))
}

// captureFileArtifacts scans a finished turn's trace for files the agent wrote
// and upserts each as an artifact (deduped by source path within the session),
// so file deliverables land in the Artifacts screen automatically — without the
// agent having to call create_artifact. Best-effort: failures are logged, never
// surfaced to the user.
func (s *Server) captureFileArtifacts(ctx context.Context, database *db.DB, sessionID, agentID string, steps []agent.TurnStep) {
	if sessionID == "" {
		return
	}
	seen := map[string]bool{}
	for _, st := range steps {
		if st.Kind != agent.StepTool || st.IsError {
			continue
		}
		// A file-writing tool (Write/create_file) carries the path + full body.
		if fp, content, ok := parseFileWrite(st.Tool, st.Input); ok && !seen[fp] {
			seen[fp] = true
			s.captureWrittenFile(ctx, database, sessionID, agentID, fp, content)
		}
		// Any tool may report saving a media/file on disk in its output (e.g. a
		// screenshot tool returning a saved path) — capture those as media
		// artifacts referencing the file, never embedding the bytes.
		for _, fp := range extractProducedMediaPaths(st.Output) {
			if seen[fp] {
				continue
			}
			seen[fp] = true
			s.captureMediaFile(ctx, database, sessionID, agentID, fp)
		}
	}
}

// captureWrittenFile records a file produced by a Write/create_file call. Media
// extensions are captured by path (no binary-as-text); text/code by content.
func (s *Server) captureWrittenFile(ctx context.Context, database *db.DB, sessionID, agentID, fp, content string) {
	if mediaKindForExt(path.Ext(fp)) != "" {
		s.captureMediaFile(ctx, database, sessionID, agentID, fp)
		return
	}
	kind, lang := artifactKindForPath(fp)
	if _, err := database.SaveFileArtifact(ctx, sessionID, agentID, fp, baseName(fp), kind, lang, content, "agent"); err != nil {
		s.logger.Warn("auto artifact capture failed", "path", fp, "error", err)
	}
}

// captureMediaFile records a media/file artifact for a file on disk, importing
// it under the workspace (copying when it lives outside) so the viewer can
// stream it. A path to a non-existent/unreadable file is skipped quietly.
func (s *Server) captureMediaFile(ctx context.Context, database *db.DB, sessionID, agentID, fp string) {
	rel, err := database.ImportMediaSource(sessionID, fp)
	if err != nil {
		return // file gone / outside reach — not a capturable artifact
	}
	kind := mediaKindForExt(path.Ext(fp))
	if kind == "" {
		kind = db.ArtifactFile
	}
	if _, err := database.SaveFileArtifact(ctx, sessionID, agentID, rel, baseName(fp), kind, "", "", "agent"); err != nil {
		s.logger.Warn("auto media artifact capture failed", "path", fp, "error", err)
	}
}
