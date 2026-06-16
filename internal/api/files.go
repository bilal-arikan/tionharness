package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// servableExt is the allowlist of file extensions the chat UI may load inline
// (images shown in messages). We intentionally do not serve arbitrary files —
// only media the renderer knows how to display.
var servableExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".avif": "image/avif",
}

// handleServeFile streams a local image file referenced by chat content so the
// frontend can render it inline (e.g. ![](C:\path\shot.png) or a tool that
// produced a screenshot). Read-only and restricted to image types.
//
// GET /api/files?path=<absolute or file:// path>
func (s *Server) handleServeFile(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("path")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	// Accept file:// URLs as well as bare paths.
	raw = strings.TrimPrefix(raw, "file://")
	raw = strings.TrimPrefix(raw, "/") // file:///C:/... → C:/...
	path := filepath.Clean(raw)

	ext := strings.ToLower(filepath.Ext(path))
	mime, ok := servableExt[ext]
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported file type: "+ext)
		return
	}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "max-age=3600")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}
