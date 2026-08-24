package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
)

// isFileWritingMCP reports whether an MCP server writes output FILES to the local
// filesystem and therefore needs the caller's session scratchpad added to its
// allowed roots. Today that is the Playwright MCP (browser_take_screenshot / PDF
// save), detected by "playwright" appearing in its command or args (e.g.
// `bunx @playwright/mcp`). The check is centralized so more file-writing servers
// can be added in one place.
func isFileWritingMCP(m db.MCPServer) bool {
	hay := strings.ToLower(m.Command + " " + m.Args)
	return strings.Contains(hay, "playwright")
}

// sessionScratchpad resolves (and lazily creates) the active session's scratchpad
// directory: <store>/sessions/<sid>/scratchpad. This is the SAME location the
// agent is told to use for durable output, so it is the root a file-writing MCP
// must be allowed to write into. It is resolved from the passed session id — never
// a cached one — so it always tracks the turn's real session.
func (r *Runtime) sessionScratchpad(sessionID string) (string, error) {
	dir, err := r.db.SessionDir(sessionID)
	if err != nil {
		return "", err
	}
	pad := filepath.Join(dir, "scratchpad")
	if err := os.MkdirAll(pad, 0o755); err != nil {
		return "", err
	}
	return pad, nil
}

// applyMCPScratchpadRoot adds the ACTIVE session's scratchpad to a file-writing
// MCP server's allowed roots so its file-saving tools (Playwright's
// browser_take_screenshot / PDF export) stop being denied with "outside allowed
// roots". The Playwright MCP confines writes to its workspace roots — and, since
// this client advertises no MCP roots, to its cwd — so we:
//
//   - set the subprocess Dir to the scratchpad (makes the scratchpad the cwd, i.e.
//     the allowed root), and
//   - inject `--output-dir=<scratchpad>` so relative filenames also land there,
//     unless the operator already configured an --output-dir.
//
// The scratchpad is re-resolved from the turn's session id on EVERY build, so a
// later session (SES4) never inherits an earlier one's path (SES1). Because Dir
// rides the connection fingerprint, a new session re-dials with its own root
// rather than reusing a stale connection; the server is also scoped per
// (session, agent) so its session-specific root cannot bleed into a shared,
// workspace-wide connection.
//
// Non-file-writing servers pass through untouched. A build with no session bound
// (catalog/preview) cannot resolve a scratchpad, so the server is left as-is and,
// when a scratchpad is expected but unresolvable, a warning is logged rather than
// silently spawning a server whose writes will be denied.
func (r *Runtime) applyMCPScratchpadRoot(ctx context.Context, cfg mcp.ServerConfig, m db.MCPServer, scopeKey string) mcp.ServerConfig {
	if !isFileWritingMCP(m) {
		return cfg
	}
	sid := SessionIDFrom(ctx)
	if sid == "" {
		// Catalog/preview build with no live session — nothing to root against. This
		// is not an error path (no real spawn happens here), so stay quiet.
		return cfg
	}
	pad, err := r.sessionScratchpad(sid)
	if err != nil {
		// A real turn with a real session but no resolvable scratchpad: do NOT hide
		// it. Warn so the operator sees why browser_take_screenshot / PDF saves will
		// be denied instead of the failure looking like a Playwright bug.
		r.logger.Warn("mcp: cannot resolve session scratchpad for file-writing server; screenshot/PDF saves will be denied (outside allowed roots)",
			"server", m.Name, "session", sid, "error", err)
		return cfg
	}
	cfg.Dir = pad
	cfg.Args = ensureOutputDirArg(cfg.Args, pad)
	// Give this server its OWN pooled connection per (session, agent) so a
	// session-specific root can't leak into another session's shared connection.
	if scopeKey != "" {
		cfg.ScopeKey = scopeKey
	}
	return cfg
}

// ensureOutputDirArg appends `--output-dir=<dir>` to a Playwright MCP arg list
// unless the operator already supplied an --output-dir (in either `--output-dir X`
// or `--output-dir=X` form), whose choice is then respected. It never mutates the
// input slice.
func ensureOutputDirArg(args []string, dir string) []string {
	for _, a := range args {
		if a == "--output-dir" || strings.HasPrefix(a, "--output-dir=") {
			return args
		}
	}
	out := make([]string, len(args), len(args)+1)
	copy(out, args)
	return append(out, "--output-dir="+dir)
}
