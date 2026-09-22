package agent

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
)

// --- zvec-grep capability ----------------------------------------------------
//
// zvec-grep is codebase-memory's sibling in the search layer. The code graph
// answers STRUCTURAL questions about symbols it parsed (callers, call chains,
// architecture); zvec-grep answers INTENT questions whose wording or location is
// unknown, over code, docs and config alike, from a hybrid full-text + vector
// index. The capability keeps codebaseMemoryCapability's contract: presence is an
// enabled stdio server row, the block names the exact namespaced tool, the block
// is graded by the pool's connection state, and it is only written for an agent
// that may call the tool.
//
// One difference shapes the block. TionHarness does not forward an MCP server's
// initialize `instructions` to the model, so zvec-grep's own routing rules
// (search vs. exact grep, absolute root, never index on your own) would never
// reach the agent. The ones it needs to use the tool correctly are restated here.

// zvecGrepSearchTool is the one tool zvec-grep's default ("agent") MCP toolset
// registers. The "full" toolset adds managed rg and index administration, but
// that is a switch on the daemon shared by every client, so the block names only
// the tool that is always there.
const zvecGrepSearchTool = "zvec_grep_search"

// zvecGrepPackageMarker identifies a zvec-grep server launched through its npm
// package instead of the `zg` shim (node …/@zvec/zvec-grep/dist/cli/index.js,
// npx @zvec/zvec-grep).
const zvecGrepPackageMarker = "zvec-grep"

// zvecGrepShimExts are the launcher extensions the `zg` shim can carry: npm
// installs zg, zg.cmd and zg.ps1 side by side on Windows.
var zvecGrepShimExts = []string{".cmd", ".exe", ".ps1", ".bat"}

// isZvecGrepShim reports whether command is the `zg` shim, by base name. A
// substring match would be wrong: "zg" occurs inside unrelated names (zgrep),
// while the base name is unambiguous. Both separators are split on, so a Windows
// path stored in the database resolves the same on any host.
func isZvecGrepShim(command string) bool {
	base := strings.ToLower(path.Base(strings.ReplaceAll(strings.TrimSpace(command), `\`, "/")))
	for _, ext := range zvecGrepShimExts {
		base = strings.TrimSuffix(base, ext)
	}
	return base == "zg"
}

// isZvecGrepServer reports whether a stored MCP server row launches zvec-grep:
// the `zg` shim itself, or a launcher (node, npx) whose command or arguments name
// the package. Only stdio rows qualify, as for codebaseMemoryCommand — the
// auto-index runs the same CLI, which an http row does not identify.
func isZvecGrepServer(m db.MCPServer) bool {
	if m.Transport != db.MCPTransportStdio && m.Transport != "" {
		return false
	}
	if isZvecGrepShim(m.Command) || strings.Contains(strings.ToLower(m.Command), zvecGrepPackageMarker) {
		return true
	}
	var args []string
	if json.Unmarshal([]byte(m.Args), &args) != nil {
		return false
	}
	for _, a := range args {
		if strings.Contains(strings.ToLower(a), zvecGrepPackageMarker) {
			return true
		}
	}
	return false
}

// zvecGrepServerRow returns the first zvec-grep row in servers.
func zvecGrepServerRow(servers []db.MCPServer) (db.MCPServer, bool) {
	for _, m := range servers {
		if isZvecGrepServer(m) {
			return m, true
		}
	}
	return db.MCPServer{}, false
}

// SetZvecGrep toggles the zvec-grep capability system for this workspace (prompt
// block, auto-index, allowlist exemption).
func (r *Runtime) SetZvecGrep(enabled bool) { r.zvecGrepEnabled.Store(enabled) }

// ZvecGrepEnabled reports whether the zvec-grep capability system is on for this
// workspace.
func (r *Runtime) ZvecGrepEnabled() bool { return r.zvecGrepEnabled.Load() }

// zvecGrepServer returns this workspace's enabled zvec-grep server row, or false
// when the workspace switch is off or no such server is enabled. It gates the
// prompt block and the auto-index together, the way codebaseMemoryCmd gates
// codebase-memory.
func (r *Runtime) zvecGrepServer(ctx context.Context) (db.MCPServer, bool) {
	if !r.ZvecGrepEnabled() {
		return db.MCPServer{}, false
	}
	servers, err := r.db.ListEnabledMCPServers(ctx)
	if err != nil {
		r.logger.Warn("zvec-grep capability: mcp server list failed", "error", err)
		return db.MCPServer{}, false
	}
	return zvecGrepServerRow(servers)
}

// zvecGrepState reports what the pool knows about the zvec-grep server's
// connection, with codebaseMemoryState's rule: no pool means unknown, never dead.
func (r *Runtime) zvecGrepState(server string) mcp.ServerState {
	if r.mcpPool == nil {
		return mcp.ServerUnknown
	}
	return r.mcpPool.ServerState(server)
}

var zvecGrepCapability = Capability{
	ID: "zvec-grep",
	Detect: func(ctx context.Context, r *Runtime, agent db.Agent) bool {
		server, ok := r.zvecGrepServer(ctx)
		if !ok || r.zvecGrepState(server.Name) == mcp.ServerDead {
			return false
		}
		// Reachability, not just presence (see codebaseMemoryCapability): the block
		// tells the agent to search with this tool before grepping, so it may only
		// be written for an agent whose explicit denylist leaves the tool callable.
		if filter := r.toolFilter(ctx, agent); filter != nil && !filter(mcp.NamespaceTool(server.Name, zvecGrepSearchTool)) {
			return false
		}
		return true
	},
	Context: func(ctx context.Context, r *Runtime, cwd, provider string) string {
		if !r.ZvecGrepEnabled() {
			return ""
		}
		servers, err := r.db.ListEnabledMCPServers(ctx)
		if err != nil {
			r.logger.Warn("zvec-grep capability: mcp server list failed", "error", err)
			return ""
		}
		server, ok := zvecGrepServerRow(servers)
		if !ok {
			return ""
		}
		state := r.zvecGrepState(server.Name)
		if state == mcp.ServerDead {
			return ""
		}
		cbm := ""
		if r.CodebaseMemoryEnabled() {
			cbm = codebaseMemoryServerName(servers)
		}
		return zvecGrepGuidance(server.Name, state, cbm, cwd, provider)
	},
}

// zvecGrepGuidance builds the prompt block with the EXACT namespaced tool name
// taken from the live server row — the rule codebaseMemoryGuidance learned the
// hard way: a bare name makes models guess a namespace and hit "no server".
// Returns "" without a server row.
//
// cbmServer (may be "") is the codebase-memory server name when that capability
// is on as well. Both blocks then sit side by side, so this one states the split
// between them; the graph tools are named conditionally because this block cannot
// see whether the agent's denylist leaves them callable.
//
// provider selects the callable spelling of the search_index tool the block
// sends the agent to when an index is missing — see searchIndexToolFor.
func zvecGrepGuidance(server string, state mcp.ServerState, cbmServer, cwd, provider string) string {
	if server == "" {
		return ""
	}
	search := mcp.NamespaceTool(server, zvecGrepSearchTool)
	var b strings.Builder
	b.WriteString("# Semantic workspace search available\n")
	if state == mcp.ServerAlive {
		b.WriteString("A zvec-grep MCP server is connected. ")
	} else {
		b.WriteString("A zvec-grep MCP server is configured for this workspace; whether it is connected right now is not verified here. " +
			"If " + search + " is missing from your tool list, or a call reports no such server, fall back to Glob/Grep for that query instead of retrying. ")
	}
	b.WriteString(search + " searches a hybrid full-text + vector index of the workspace — code, docs and config alike. " +
		"Use it FIRST when the wording or location is unknown, or when the answer needs concepts, relationships, data or control flow, design rationale, or synthesis across files. " +
		"Use Glob/Grep instead when an exact literal, identifier, filename, config key, error message or regex is enough. " +
		"For a mixed task, pass the intent as `query` and the exact anchors as `fts`, then verify with Read/Grep.\n")
	if cbmServer != "" {
		b.WriteString("When the code-graph tools " + mcp.NamespaceTool(cbmServer, "search_graph") + " and " +
			mcp.NamespaceTool(cbmServer, "trace_path") + " are in your tool list, they own symbol-level structure " +
			"(callers, call chains, impact); " + search + " owns discovery by intent, which a symbol graph cannot answer.\n")
	}
	b.WriteString("Every call needs `root`, an absolute path; a subdirectory resolves to the index of the repository that contains it. " +
		"Results carry bounded source snippets: treat a sufficient snippet as already read, read `freshness` from the response instead of checking index status, and stop once the evidence is sufficient.\n")
	b.WriteString("A result that starts with [INDEX_MISSING] means that path has no index: use Glob/Grep for that query. " +
		searchIndexDirective(provider, "zg index") + "\n")
	if cwd = strings.TrimSpace(cwd); cwd != "" {
		if isEphemeralWorkdir(cwd) {
			b.WriteString("Your working directory `" + cwd + "` is a temporary working copy that is never indexed; use Glob/Grep there.")
		} else {
			b.WriteString("Your working directory is `" + cwd + "`: pass it as `root`. If it has no index yet, one is built in the background on first use — until then searches report [INDEX_MISSING].")
		}
	}
	return b.String()
}
