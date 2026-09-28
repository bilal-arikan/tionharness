// Package web serves the compiled frontend single-page app from inside the
// binary, so the whole product ships as one executable. The Vite build is
// emitted into dist/ (see frontend/vite.config.ts outDir) and baked in at
// compile time via go:embed.
//
// When no real build is bundled (dist holds only the .gitkeep placeholder),
// Handler reports ok=false so the server skips mounting the SPA and logs a hint
// instead of serving a blank page — keeping `go build` green for backend-only
// or dev (proxy) workflows.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// all: includes dotfiles (e.g. the .gitkeep placeholder) so this compiles even
// before the frontend has been built.
//
//go:embed all:dist
var embedded embed.FS

// Handler returns an http.Handler that serves the embedded SPA: real files by
// path, with a fallback to index.html for client-side routes. ok is false when
// no build was bundled.
func Handler() (http.Handler, bool) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, false // placeholder only — build not bundled
	}
	return staticHandler(sub, index), true
}
