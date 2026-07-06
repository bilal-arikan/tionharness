package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
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
var capabilities = []Capability{codebaseMemoryCapability}

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

var codebaseMemoryCapability = Capability{
	ID: "codebase-memory-mcp",
	Detect: func(ctx context.Context, r *Runtime) bool {
		servers, err := r.db.ListEnabledMCPServers(ctx)
		if err != nil {
			return false
		}
		return codebaseMemoryCommand(servers) != ""
	},
	Context: func(ctx context.Context, r *Runtime, cwd string) string {
		var b strings.Builder
		b.WriteString(codebaseMemoryGuidance)
		if r.CBMStoreDir() != "" {
			b.WriteString("\nThis workspace uses an ISOLATED index store, so results never mix with other workspaces.")
		}
		if p := projectIDForPath(cwd); p != "" {
			b.WriteString("\nYour working directory maps to project id `" + p + "` (auto-indexed on first use). " +
				"If a query reports the project is unknown, run list_projects to confirm the exact id.")
		}
		return b.String()
	},
}

const codebaseMemoryGuidance = "# Code knowledge-graph available\n" +
	"A codebase-memory MCP server is connected. For ANY code search, navigation, or " +
	"structural understanding, PREFER its tools over broad grep/file-scans: search_code " +
	"(text/symbol), search_graph + get_code_snippet (read a definition), query_graph / " +
	"trace_path (relationships), get_architecture (workspace-wide overview). It is faster " +
	"and far more token-efficient. After code changes, re-run index_repository (or rely on " +
	"the background watcher) so results stay fresh."

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

// CBMStoreDir is this workspace's isolated codebase-memory index store: a sibling
// of the workspace sandbox (like the skills/ and hook-scripts/ dirs). Empty when
// the workspace dir is unknown (bare test runtimes) — callers then fall back to
// the server's default store.
func (r *Runtime) CBMStoreDir() string {
	if r.workDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(r.workDir), "cbm-store")
}

// EnsureCodebaseIndexed fires a best-effort, background incremental index of cwd
// into this workspace's isolated store, at most once per (cwd, store) per process.
// No-op when cwd is empty or no codebase-memory server is enabled. Failures are
// LOGGED (not silently swallowed) and clear the guard so a later turn can retry.
func (r *Runtime) EnsureCodebaseIndexed(ctx context.Context, cwd string) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return
	}
	servers, err := r.db.ListEnabledMCPServers(ctx)
	if err != nil {
		return
	}
	command := codebaseMemoryCommand(servers)
	if command == "" {
		return
	}
	store := r.CBMStoreDir()
	key := cwd + "|" + store
	if _, seen := r.cbmIndexed.LoadOrStore(key, true); seen {
		return
	}
	go func() {
		arg, _ := json.Marshal(map[string]string{"repo_path": filepath.ToSlash(cwd)})
		cmd := exec.Command(command, "cli", "index_repository", string(arg))
		cmd.Env = os.Environ()
		if store != "" {
			if mkErr := os.MkdirAll(store, 0o755); mkErr != nil {
				r.logger.Warn("codebase-memory store dir create failed", "store", store, "error", mkErr)
			}
			cmd.Env = append(cmd.Env, "CBM_CACHE_DIR="+store)
		}
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			r.logger.Warn("codebase-memory auto-index failed",
				"cwd", cwd, "error", runErr, "output", strings.TrimSpace(string(out)))
			r.cbmIndexed.Delete(key) // allow a later turn to retry
		}
	}()
}
