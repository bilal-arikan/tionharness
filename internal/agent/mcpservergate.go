package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// mcpServerGate answers, for a namespaced MCP server key, whether ANY tool that
// server could expose is permitted for this agent.
//
// It exists because the CLI providers (claude-cli, codex-cli) mount external MCP
// servers into the CLI PROCESS: the CLI runs its own tool loop, so neither the
// advertised catalog nor the per-call dispatch passes through toolFilter. Found
// live (SES948): a coder agent whose allowlist was ["Read","LS","Glob","Grep",
// "Write","Edit","Bash"] drove Playwright and the codebase-memory server for a
// whole turn — the tools never appeared in its system prompt, yet every call
// succeeded, because writeCLIMCPConfig/codexMCPSpec mounted every ENABLED server
// and allowlisted it with a server-level `mcp__<key>` wildcard. The agent tools
// screen showed those tools as blocked the entire time.
//
// The gate is deliberately PREFIX-based rather than tool-exact: the CLI path has
// no tool list for a server without connecting to it (which would spawn a second
// copy of every stdio server alongside the CLI's own). A server key is decidable
// from the agent's patterns alone, because a namespaced MCP tool is always
// "<key>__<tool>":
//
//   - blocked: a pattern that covers the whole server ("playwright*",
//     "playwright__*", or the bare key) drops it.
//   - legacy allowlist (non-empty): the server survives only when some allow
//     pattern targets it — an exact name ("playwright__browser_click"), a prefix
//     ("playwright__*"), or the bare key. A "group:<category>" key never matches
//     an MCP tool (groups classify built-ins), so a built-ins-only allowlist
//     drops every server, which is exactly the native-path behaviour.
//
// Per-tool precision inside a surviving server stays with the CLI's own
// permission layer; this gate closes the "server the agent may not touch at all"
// hole, which is the one the UI already claims to enforce.
//
// exemptServer (may be "") is the infrastructure server that the ALLOWLIST does
// not gate — see allowlistExemptServer. The explicit denylist still applies to it.
//
// Returns nil when the agent constrains nothing (mount everything, as before).
//
// A malformed permission document is an ERROR and the gate returned with it
// denies EVERY server. The unmarshal error used to be discarded, which left both
// lists empty — and empty lists mean "unconstrained", so a corrupt allowlist
// mounted every MCP server into the CLI process: the exact hole this gate exists
// to close. The caller must surface the error, not fall back to the nil gate.
func mcpServerGate(agent db.Agent, exemptServer string) (func(serverKey string) bool, error) {
	denyAll := func(string) bool { return false }
	overrides, err := ParseToolOverridesErr(agent)
	if err != nil {
		return denyAll, fmt.Errorf("agent %s: %w", agent.ID, err)
	}
	blocked := blockedPatterns(overrides)
	var allowed []string
	if raw := strings.TrimSpace(agent.AllowedTools); raw != "" {
		if uerr := json.Unmarshal([]byte(raw), &allowed); uerr != nil {
			return denyAll, fmt.Errorf("agent %s: allowed_tools: %w", agent.ID, uerr)
		}
	}
	if len(blocked) == 0 && len(allowed) == 0 {
		return nil, nil
	}
	return func(serverKey string) bool {
		if serverKey == "" {
			return true
		}
		for _, p := range blocked {
			if patternCoversServer(p, serverKey) {
				return false
			}
		}
		if exemptServer != "" && serverKey == exemptServer {
			return true // infrastructure, not a persona's work tool (see allowlistExemptServer)
		}
		if len(allowed) == 0 {
			return true
		}
		for _, p := range allowed {
			if patternTargetsServer(p, serverKey) {
				return true
			}
		}
		return false
	}, nil
}

// patternCoversServer reports whether a BLOCKED pattern bans the server as a
// whole. Only server-wide shapes count: banning a single tool
// ("playwright__browser_click") must not unmount the server, since its other
// tools stay legitimate.
func patternCoversServer(pattern, serverKey string) bool {
	if tools.IsGroupKey(pattern) {
		return false // groups classify built-ins, never namespaced MCP tools
	}
	if pattern == serverKey || pattern == serverKey+"__" {
		return true
	}
	if !strings.HasSuffix(pattern, "*") {
		return false
	}
	// A prefix pattern covers the server when the prefix is a prefix OF the key
	// ("play*", "playwright*") or is exactly the key's namespace ("playwright__*").
	prefix := strings.TrimSuffix(pattern, "*")
	return prefix == serverKey+"__" || strings.HasPrefix(serverKey, prefix)
}

// patternTargetsServer reports whether an ALLOW pattern reaches at least one tool
// of the server — the mirror of patternCoversServer, but satisfied by a single
// exact tool name too.
func patternTargetsServer(pattern, serverKey string) bool {
	if tools.IsGroupKey(pattern) {
		return false
	}
	if patternCoversServer(pattern, serverKey) {
		return true
	}
	// Exact tool name inside the server, or a prefix that narrows within it.
	return strings.HasPrefix(pattern, serverKey+"__")
}

// allowlistExemptServer returns the name of the codebase-memory MCP server when
// this workspace has one enabled, or "" otherwise. Its tools are exempt from the
// per-agent ALLOWLIST on both paths (toolFilter for native, mcpServerGate for the
// CLI providers).
//
// Rationale, and the precedent it follows: toolFilter already exempts the
// coordination tools from the allowlist, because the allowlist describes which
// WORK tools a persona may use, and whether a session can drive workers is a
// property of the session, not the persona. The code graph is the same kind of
// thing one level down — it is how an agent READS the repository, the same role
// Read/Glob/Grep play, and those are in every profile's allowlist already. A
// worker denied the graph does not lose a capability, it just burns tokens
// grepping; and the static prompt tells every agent to prefer the graph over
// grep, so withholding it makes the prompt lie (see codebaseMemoryCapability).
//
// The exemption is ONLY over the allowlist. The workspace-level switch and the
// agent's EXPLICIT denylist still remove it — so an operator can still take the
// graph away from one agent, deliberately, and that decision is honoured.
//
// Scoped to this one server on purpose: it is the only MCP server TionHarness
// treats as infrastructure (it ships a capability probe and a prompt block for
// it, and nothing else). A second such server would be an explicit decision
// here, not an accident.
func (r *Runtime) allowlistExemptServer(ctx context.Context) string {
	if !r.CodebaseMemoryEnabled() {
		return ""
	}
	servers, err := r.db.ListEnabledMCPServers(ctx)
	if err != nil {
		r.logger.Warn("allowlist-exempt server: mcp server list failed", "error", err)
		return ""
	}
	return codebaseMemoryServerName(servers)
}

// isExemptTool reports whether a namespaced tool name belongs to the
// allowlist-exempt server. exemptServer == "" disables the exemption entirely.
func isExemptTool(name, exemptServer string) bool {
	if exemptServer == "" {
		return false
	}
	return strings.HasPrefix(name, exemptServer+"__")
}
