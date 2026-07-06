package agent

// SessionRenderDir resolves the per-session directory where render_template
// writes its filled HTML output. It is a SIBLING of the progress/ tree under the
// workspace store root, which gives /api/files a single clean whitelist boundary:
// only files under <store>/render/ may be served as text for inline html-preview.
// The path itself comes from db.RenderDir — one source of truth shared with the
// session-delete + startup sweep reclaimers, so the location cannot drift.
//
// An empty sessionID (catalog/preview builds with no session bound to the turn)
// yields an empty string. The render_template tool treats that as "no session
// bound" and fails loudly rather than writing to a shared or ambiguous location —
// creation of the directory is deferred to the tool's Call so a mkdir failure
// surfaces to the caller instead of being swallowed here.
func (r *Runtime) SessionRenderDir(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return r.db.RenderDir(sessionID)
}
