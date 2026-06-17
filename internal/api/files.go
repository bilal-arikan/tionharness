package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// servableExt is the allowlist of file extensions the chat UI / artifacts screen
// may load inline (images, video and audio). We intentionally do not serve
// arbitrary files — only media the renderer knows how to display. http.ServeContent
// handles HTTP range requests, so <video>/<audio> seeking works out of the box.
var servableExt = map[string]string{
	// Images.
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".avif": "image/avif",
	// Video.
	".mp4":  "video/mp4",
	".m4v":  "video/x-m4v",
	".webm": "video/webm",
	".ogv":  "video/ogg",
	".mov":  "video/quicktime",
	".mkv":  "video/x-matroska",
	// Audio.
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".m4a":  "audio/mp4",
	".oga":  "audio/ogg",
	".ogg":  "audio/ogg",
	".flac": "audio/flac",
	".aac":  "audio/aac",
}

// handleServeFile streams a local image file referenced by chat content so the
// frontend can render it inline (e.g. ![](C:\path\shot.png) or a tool that
// produced a screenshot). Read-only and restricted to image types.
//
// GET /api/files?path=<absolute or file:// path>
func (s *Server) handleServeFile(w http.ResponseWriter, r *http.Request) {
	var path string
	// `rel` resolves a workspace-relative path (e.g. an uploaded attachment under
	// "uploads/...") against the active workspace sandbox root. `path` is an
	// absolute/file:// path (e.g. a screenshot a tool produced). rel wins.
	if rel := r.URL.Query().Get("rel"); rel != "" {
		wsp := ws(r)
		if wsp == nil {
			writeError(w, http.StatusBadRequest, "no workspace")
			return
		}
		// Clean within the sandbox; reject any traversal that escapes it.
		clean := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(rel, "/")))
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			writeError(w, http.StatusBadRequest, "invalid path")
			return
		}
		path = filepath.Join(wsp.DataDir, "workspace", clean)
	} else {
		raw := r.URL.Query().Get("path")
		if raw == "" {
			writeError(w, http.StatusBadRequest, "path or rel is required")
			return
		}
		// Accept file:// URLs as well as bare paths.
		raw = strings.TrimPrefix(raw, "file://")
		raw = strings.TrimPrefix(raw, "/") // file:///C:/... → C:/...
		path = filepath.Clean(raw)
	}

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
