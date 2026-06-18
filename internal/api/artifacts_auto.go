package api

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/db"
)

// fileWriteTools are the tool names that write a full file body we can capture
// as an artifact: the native built-in (write_file) and the claude CLI's own
// file tool (Write). Edits (partial diffs) are intentionally excluded.
var fileWriteTools = map[string]bool{
	"write_file": true,
	"Write":      true,
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

// artifactKindForPath infers an artifact kind (and code language) from a file's
// extension so the right renderer is used in the viewer.
func artifactKindForPath(filePath string) (kind, language string) {
	ext := strings.ToLower(path.Ext(strings.ReplaceAll(filePath, "\\", "/")))
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
		fp, content, ok := parseFileWrite(st.Tool, st.Input)
		if !ok || seen[fp] {
			continue
		}
		seen[fp] = true
		kind, lang := artifactKindForPath(fp)
		if _, err := database.SaveFileArtifact(ctx, sessionID, agentID, fp, baseName(fp), kind, lang, content, "agent"); err != nil {
			s.logger.Warn("auto artifact capture failed", "path", fp, "error", err)
		}
	}
}
