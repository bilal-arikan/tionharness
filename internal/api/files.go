package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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

// textServableExt is the allowlist of extensions served as text/plain in the
// as=text mode (inline html-preview + text fetches). Served as text/plain —
// NEVER text/html — so an .html render output can never execute as a same-origin
// page; the chat UI injects the returned text into a sandboxed iframe srcDoc.
var textServableExt = map[string]bool{
	".html": true, ".htm": true,
	".md": true, ".markdown": true,
	".json": true, ".txt": true, ".csv": true, ".log": true,
	".xml": true, ".yaml": true, ".yml": true, ".svg": true,
	".toml": true, ".ts": true, ".tsx": true, ".js": true,
	".go": true, ".py": true, ".sh": true, ".sql": true,
}

// handleServeFile streams a local image file referenced by chat content so the
// frontend can render it inline (e.g. ![](C:\path\shot.png) or a tool that
// produced a screenshot). Read-only and restricted to image types. The as=text
// mode (serveTextFile) additionally serves render_template output as text/plain.
//
// GET /api/files?path=<absolute or file:// path>[&as=text]
func (s *Server) handleServeFile(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("as") == "text" {
		s.serveTextFile(w, r)
		return
	}
	var path string
	// fromRel marks the sandbox-confined branch: those paths may additionally be
	// served as text/plain (file artifacts such as .md reports), which is not
	// allowed for arbitrary absolute paths.
	fromRel := false
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
		path = filepath.Join(wsp.SandboxRoot(), clean)
		fromRel = true
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
		// Text-like files stored inside the workspace sandbox (e.g. a `file`
		// artifact's .md report) are served as text/plain — never text/html — so
		// the artifacts screen can preview and download them.
		if !fromRel || !textServableExt[ext] {
			writeError(w, http.StatusUnsupportedMediaType, "unsupported file type: "+ext)
			return
		}
		mime = "text/plain; charset=utf-8"
		w.Header().Set("X-Content-Type-Options", "nosniff")
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

// serveTextFile serves a local file as text/plain (never text/html) for the
// inline html-preview renderer. It is deliberately NARROW: only ABSOLUTE paths
// under the active workspace's render root (<store>/render/) are served, and only
// for a small text-extension allowlist. text/plain + X-Content-Type-Options:nosniff
// guarantees an .html render output can never execute as a same-origin page — the
// chat UI injects the returned text into a sandboxed iframe srcDoc instead.
//
// GET /api/files?path=<absolute path>&as=text
func (s *Server) serveTextFile(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	if wsp == nil || wsp.DB == nil {
		writeError(w, http.StatusBadRequest, "no workspace")
		return
	}
	raw := r.URL.Query().Get("path")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	// Accept file:// URLs as well as bare paths (file:///C:/... → C:/...).
	raw = strings.TrimPrefix(raw, "file://")
	raw = strings.TrimPrefix(raw, "/")
	path := filepath.Clean(raw)

	// Whitelist boundary: only render_template output (under <store>/render/) may
	// be read as text. This keeps as=text from reading arbitrary text files off
	// disk even though the media path accepts any absolute path.
	renderRoot := filepath.Join(wsp.DB.Root(), "render")
	if !underDir(renderRoot, path) {
		writeError(w, http.StatusForbidden, "path is not under the render root")
		return
	}

	ext := strings.ToLower(filepath.Ext(path))
	if !textServableExt[ext] {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported text file type: "+ext)
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

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}

// underDir reports whether target is dir itself or a descendant of it, after
// cleaning both. On Windows the comparison is case-insensitive (NTFS is), so a
// path echoed back with different casing still resolves inside the boundary.
func underDir(dir, target string) bool {
	dir = filepath.Clean(dir)
	target = filepath.Clean(target)
	if runtime.GOOS == "windows" {
		dir = strings.ToLower(dir)
		target = strings.ToLower(target)
	}
	if dir == target {
		return true
	}
	return strings.HasPrefix(target, dir+string(filepath.Separator))
}
