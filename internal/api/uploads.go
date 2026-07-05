package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

const (
	// maxUploadBytes caps a single uploaded file (25 MB).
	maxUploadBytes = 25 << 20
	// maxInlineTextBytes caps how much of a text/pasted attachment is inlined into
	// the provider message (the full file is still on disk for read_file). Kept
	// modest so a large paste does not bloat every subsequent turn's context;
	// bigger files are read on demand via the read_file tool.
	maxInlineTextBytes = 16 << 10
)

// handleUpload stores one user-supplied file under the workspace uploads
// directory and returns its Attachment descriptor. Pasted long text is uploaded
// the same way (as a text/plain blob). The file is written inside the workspace
// sandbox root so agents can later open it via their read_file tool.
//
// POST /api/uploads  (multipart/form-data: file=<binary>, sessionId=<id>)
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	if wsp == nil {
		writeError(w, http.StatusBadRequest, "no workspace")
		return
	}

	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload: "+err.Error())
		return
	}
	sessionID := strings.TrimSpace(r.FormValue("sessionId"))
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "sessionId is required")
		return
	}
	// sessionID is interpolated into the on-disk upload path, so it must be a
	// single traversal-free segment — otherwise a crafted id ("../../…") could
	// write the file outside the uploads directory.
	if !safePathSegment(sessionID) {
		writeError(w, http.StatusBadRequest, "invalid sessionId")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	name := sanitizeFileName(header.Filename)
	id := uuid.NewString()[:8]
	// Relative path under the sandbox root (DataDir/workspace). All of a session's
	// files (chat attachments, manual uploads, artifact content) are collected in
	// one per-session folder: artifacts/<sessionId>/. Forward slashes match the
	// agent's read_file path style.
	rel := "artifacts/" + sessionID + "/" + id + "-" + name
	artifactsRoot := filepath.Join(wsp.DataDir, "workspace", "artifacts")
	abs := filepath.Join(wsp.DataDir, "workspace", filepath.FromSlash(rel))
	// Defense in depth: the resolved file must stay inside the artifacts root even
	// if some component slipped past the checks above.
	if !withinDir(artifactsRoot, abs) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dst, err := os.Create(abs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	size, copyErr := io.Copy(dst, io.LimitReader(file, maxUploadBytes))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(abs)
		writeError(w, http.StatusInternalServerError, "write failed")
		return
	}

	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = mimeFromName(name)
	}
	kind := attachmentKind(mime, name)

	att := db.Attachment{
		ID:      id,
		Name:    name,
		Mime:    mime,
		Kind:    kind,
		Size:    size,
		RelPath: rel,
	}
	// Inline the content of text-like attachments (capped + valid UTF-8) so the
	// model sees it without a read_file round-trip.
	if kind == "text" || kind == "code" {
		if b, rerr := os.ReadFile(abs); rerr == nil && len(b) <= maxInlineTextBytes && utf8.Valid(b) {
			att.TextContent = string(b)
		}
	}

	writeJSON(w, http.StatusOK, att)
}

// handleDeleteUpload removes a single uploaded attachment by its
// workspace-relative path. Used when the user cancels a staged attachment (the
// tray "x") before sending, so the already-uploaded file is not orphaned.
// Restricted to the uploads/ subtree and traversal-guarded.
//
// DELETE /api/uploads?rel=uploads/<sid>/<file>
func (s *Server) handleDeleteUpload(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	if wsp == nil {
		writeError(w, http.StatusBadRequest, "no workspace")
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("rel"))
	if rel == "" {
		writeError(w, http.StatusBadRequest, "rel is required")
		return
	}
	clean := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(rel, "/")))
	// Only files inside uploads/ may be deleted (never arbitrary workspace files).
	sep := string(filepath.Separator)
	if clean != "uploads" && !strings.HasPrefix(clean, "uploads"+sep) {
		writeError(w, http.StatusBadRequest, "only uploads may be deleted")
		return
	}
	if strings.Contains(clean, ".."+sep) || strings.HasSuffix(clean, sep+"..") || clean == ".." {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	abs := filepath.Join(wsp.DataDir, "workspace", clean)
	_ = os.Remove(abs) // best-effort: missing file is not an error
	w.WriteHeader(http.StatusNoContent)
}

// safePathSegment reports whether s is a single path segment safe to embed in a
// filesystem path: non-empty, not "."/"..", and free of path separators or any
// ".." traversal sequence.
func safePathSegment(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") {
		return false
	}
	return true
}

// withinDir reports whether abs resolves to a location inside (or equal to) root
// after cleaning — used as a final guard against path traversal.
func withinDir(root, abs string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(abs))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// sanitizeFileName strips any directory components and keeps a safe basename.
func sanitizeFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}

// mimeFromName guesses a coarse MIME type from the file extension when the
// client did not supply one.
func mimeFromName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".md", ".txt", ".log", ".csv":
		return "text/plain"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

// codeExt is the set of extensions treated as source code (kind "code").
var codeExt = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".py": true, ".rs": true, ".java": true, ".c": true, ".h": true,
	".cpp": true, ".cs": true, ".rb": true, ".php": true, ".sh": true,
	".html": true, ".css": true, ".sql": true, ".yaml": true, ".yml": true,
	".toml": true, ".json": true, ".xml": true,
}

// attachmentKind maps a MIME type (with a filename fallback) to a coarse
// category used for icon selection and provider-message rendering.
func attachmentKind(mime, name string) string {
	m := strings.ToLower(mime)
	switch {
	case strings.HasPrefix(m, "image/"):
		return "image"
	case strings.HasPrefix(m, "audio/"):
		return "audio"
	case strings.HasPrefix(m, "video/"):
		return "video"
	case m == "application/pdf":
		return "pdf"
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".pdf":
		return "pdf"
	case ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx":
		return "office"
	case ".zip", ".tar", ".gz", ".rar", ".7z":
		return "archive"
	case ".txt", ".md", ".log", ".csv":
		return "text"
	}
	if codeExt[ext] {
		return "code"
	}
	if strings.HasPrefix(m, "text/") {
		return "text"
	}
	return "file"
}
