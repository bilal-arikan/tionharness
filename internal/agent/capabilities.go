package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/mcp"
)

// Capability is a probe + context-block contract for an OPTIONAL external tool a
// workspace may or may not have. When Detect reports the tool is present, Context
// yields a short block injected into the agent's cached static prompt prefix so
// the agent knows the tool exists and how to use it. The set is GENERIC: a future
// tool plugs in by appending one Capability whose Detect may probe an MCP server,
// an on-PATH binary (exec.LookPath), or a settings flag — the assembler code does
// not change.
type Capability struct {
	ID      string
	Detect  func(ctx context.Context, r *Runtime) bool
	Context func(ctx context.Context, r *Runtime, cwd string) string
}

// capabilities is the ordered registry of optional-tool probes. Append here to
// teach the agent about a new external tool.
var capabilities = []Capability{codebaseMemoryCapability, tokenOptimizerCapability, shellEnvironmentCapability}

// CapabilityContext concatenates the context blocks of every capability currently
// present. cwd is the session working directory (may be ""), used by capabilities
// that resolve a per-directory target (e.g. a repo project id). Returns "" when no
// capability is present — a safe no-op for the prompt assembler. Cheap enough to
// call per turn; the result is stable so it never disturbs the prompt cache.
func (r *Runtime) CapabilityContext(ctx context.Context, cwd string) string {
	var b strings.Builder
	for _, c := range capabilities {
		if !c.Detect(ctx, r) {
			continue
		}
		if blk := strings.TrimSpace(c.Context(ctx, r, cwd)); blk != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(blk)
		}
	}
	return b.String()
}

// --- codebase-memory-mcp capability -----------------------------------------

// codebaseMemoryCommandMarker identifies the codebase-memory-mcp executable in a
// stored MCP server's Command path (the server has no slug field).
const codebaseMemoryCommandMarker = "codebase-memory-mcp"

// codebaseMemoryCommand returns the configured codebase-memory-mcp executable path
// when an enabled stdio MCP server points at it, or "" when absent. One scan backs
// both presence detection and the auto-index helper.
func codebaseMemoryCommand(servers []db.MCPServer) string {
	for _, m := range servers {
		if m.Transport != db.MCPTransportStdio && m.Transport != "" {
			continue
		}
		if strings.Contains(strings.ToLower(m.Command), codebaseMemoryCommandMarker) {
			return m.Command
		}
	}
	return ""
}

// codebaseMemoryServerName returns the configured server NAME for the
// codebase-memory-mcp stdio server ("" when absent). The prompt guidance needs the
// exact namespaced tool names (`<server>__<tool>`), and the namespace prefix is the
// server's stored name — so the hint must be built from the live row, never
// hardcoded.
func codebaseMemoryServerName(servers []db.MCPServer) string {
	for _, m := range servers {
		if m.Transport != db.MCPTransportStdio && m.Transport != "" {
			continue
		}
		if strings.Contains(strings.ToLower(m.Command), codebaseMemoryCommandMarker) {
			return m.Name
		}
	}
	return ""
}

// codebaseMemoryCmd returns the enabled codebase-memory executable path for this
// workspace, or "" when no such server is enabled. Backs the codebase_workspace_search
// tool registration (fan-out search across the workspace store).
func (r *Runtime) codebaseMemoryCmd(ctx context.Context) string {
	// Single gate for the whole feature: when the workspace toggle is off, report
	// "no server" so the hint block, auto-index, and workspace-search tool all
	// disappear together.
	if !r.CodebaseMemoryEnabled() {
		return ""
	}
	servers, err := r.db.ListEnabledMCPServers(ctx)
	if err != nil {
		return ""
	}
	return codebaseMemoryCommand(servers)
}

var codebaseMemoryCapability = Capability{
	ID: "codebase-memory-mcp",
	Detect: func(ctx context.Context, r *Runtime) bool {
		return r.codebaseMemoryCmd(ctx) != ""
	},
	Context: func(ctx context.Context, r *Runtime, cwd string) string {
		var b strings.Builder
		servers, err := r.db.ListEnabledMCPServers(ctx)
		if err != nil {
			r.logger.Warn("codebase-memory capability: server list failed", "error", err)
			return ""
		}
		b.WriteString(codebaseMemoryGuidance(servers))
		if p := projectIDForPath(cwd); p != "" {
			b.WriteString("\nYour working directory maps to project id `" + p + "` (auto-indexed on first use). " +
				"If a query reports the project is unknown, run list_projects to confirm the exact id.")
		}
		return b.String()
	},
}

// codebaseMemoryGuidance builds the prompt hint with the EXACT namespaced tool
// names (`<server>__<tool>`) taken from the live MCP server row. Bare tool names
// (search_code, …) were the root cause of models guessing a wrong namespace
// (e.g. codebase_memory__search_code) and getting "no server" errors — the hint now
// spells the full callable name so no guessing is possible. Returns "" when no
// codebase-memory server row is present: a hint without a real namespace would
// point the model at non-existent tools, which is worse than no hint at all.
func codebaseMemoryGuidance(servers []db.MCPServer) string {
	server := codebaseMemoryServerName(servers)
	if server == "" {
		return ""
	}
	ns := func(tool string) string {
		return mcp.NamespaceTool(server, tool)
	}
	return "# Code knowledge-graph available\n" +
		"A codebase-memory MCP server is connected. For ANY code search, navigation, or " +
		"structural understanding, use its tools FIRST: " + ns("search_code") + " (text/symbol), " +
		ns("search_graph") + " + " + ns("get_code_snippet") + " (read a definition), " +
		ns("query_graph") + " / " + ns("trace_path") + " (relationships), " +
		ns("get_architecture") + " (workspace-wide overview). It is faster and far " +
		"more token-efficient. Do NOT reach for raw shell greps (PowerShell Select-String, " +
		"Get-Content -Recurse, grep, findstr) as your first move — they are the LAST resort, " +
		"only when the index genuinely has no answer for a query. After code changes, re-run " +
		ns("index_repository") + " (or rely on the background watcher) so results stay fresh."
}

// projectIDForPath mirrors codebase-memory-mcp's path->project-id rule: path
// separators (and the drive colon) collapse to '-', and any character outside
// [A-Za-z0-9-] is dropped. Empty in -> empty out. Kept in sync with the server's
// naming so the agent can address the project without a discovery round-trip; the
// context block still points the agent at list_projects if the guess ever misses.
func projectIDForPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	b := make([]rune, 0, len(p))
	for _, ch := range p {
		switch {
		case ch == '/' || ch == '\\' || ch == ':':
			if len(b) > 0 && b[len(b)-1] != '-' {
				b = append(b, '-')
			}
		case (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-':
			b = append(b, ch)
		}
	}
	return strings.Trim(string(b), "-")
}

// sessionCwd returns the session's EXPLICIT working directory (the repo the agent
// operates on) from ctx, or "" when unset. Unlike effectiveWorkDir it does NOT
// fall back to the workspace default, so the capability project-id hint and the
// auto-index only ever target a real repo the user chose — never the sandbox root.
func (r *Runtime) sessionCwd(ctx context.Context) string {
	sid := SessionIDFrom(ctx)
	if sid == "" {
		return ""
	}
	s, err := r.db.GetSession(ctx, sid)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(s.WorkingDir)
}

// EnsureCodebaseIndexed fires a best-effort, background incremental index of cwd
// into the server's own store, at most once per cwd per process. No-op when cwd
// is empty or no codebase-memory server is enabled. Failures are LOGGED (not
// silently swallowed) and clear the guard so a later turn can retry.
func (r *Runtime) EnsureCodebaseIndexed(ctx context.Context, cwd string) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return
	}
	command := r.codebaseMemoryCmd(ctx) // "" when disabled or no server
	if command == "" {
		return
	}
	key := cwd
	if _, seen := r.cbmIndexed.LoadOrStore(key, true); seen {
		return
	}
	go func() {
		// Flag form: codebase-memory-mcp 0.10 deprecated raw-JSON CLI args.
		cmd := exec.Command(command, "cli", "index_repository", "--repo-path", filepath.ToSlash(cwd))
		cmd.Env = os.Environ()
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			r.logger.Warn("codebase-memory auto-index failed",
				"cwd", cwd, "error", runErr, "output", strings.TrimSpace(string(out)))
			r.cbmIndexed.Delete(key) // allow a later turn to retry
		}
	}()
}
