// Package climcp renders the per-turn configuration TionHarness hands to the
// claude CLI: the --mcp-config file (external MCP servers + the two-tier
// Interaction MCP bridge), the --allowedTools / --disallowedTools lists, the
// --tools built-in allowlist, the --settings file (permission deny-list + hook
// passthrough + effort level) and the hook matcher/command dialect bridges.
//
// It is pure with respect to the agent runtime: everything it needs from the
// workspace (enabled servers, the agent's server gate, hooks, tunables, logging)
// arrives through the Host interface, so the package can be tested with a fake
// host and never imports internal/agent. The agent package keeps thin adapters
// (Runtime.writeCLIMCPConfig / writeCLISettings) that implement Host on top of
// the runtime — extracted from internal/agent on 2026-09-03 (_Docs/81, step 1).
package climcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// Host is what the renderer needs from the surrounding workspace.
type Host interface {
	// EnabledServers lists the external MCP servers enabled in the workspace.
	EnabledServers(ctx context.Context) ([]mcp.ServerConfig, error)
	// ServerGate returns the agent's per-server admission predicate (nil = every
	// server admitted) or an error when the agent's tool restriction document is
	// malformed — the caller then fails CLOSED and mounts nothing.
	ServerGate(ctx context.Context, ag db.Agent) (func(serverKey string) bool, error)
	// EnabledHooks lists the workspace's enabled hooks for one event.
	EnabledHooks(ctx context.Context, event string) ([]db.Hook, error)
	// ShellEnabled reports whether TionHarness's own shell tool is bridged.
	ShellEnabled() bool
	// CLIHooksEnabled reports whether workspace hooks are passed to the CLI.
	CLIHooksEnabled() bool
	Logger() *slog.Logger
	// EmitDebug records a diagnostic event on the turn's session journal.
	EmitDebug(ctx context.Context, ev db.DebugEvent)
}

// Config is the on-disk shape claude --mcp-config expects.
type Config struct {
	MCPServers map[string]Server `json:"mcpServers"`
}

// Server is one --mcp-config entry.
type Server struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type,omitempty"` // sse | http
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"` // http transport (e.g. Authorization)
	// AlwaysLoad exempts a server's tools from the CLI's tool-search deferral
	// (claude-cli 2.1.x+): they are inlined at turn start instead of discovered via
	// ToolSearch. Used for the eager interaction tier so Bash/ask_user/use_skill are
	// always available without a discovery round-trip.
	AlwaysLoad bool `json:"alwaysLoad,omitempty"`
}

// InteractionCoreKey / InteractionExtendedKey are the two mcp-config keys for the
// in-process Interaction MCP server. Core keeps the historical
// "tionharness_interaction" key so existing namespaced references (use_skill,
// trace stripping) stay valid; Extended is a separate key whose tools the CLI
// defers via ToolSearch (claude-cli 2.1.x+). The CLI namespaces tools as
// mcp__<key>__<tool>.
const (
	InteractionCoreKey     = "tionharness_interaction"
	InteractionExtendedKey = "tionharness_extended"
	// InteractionToolPrefix is the namespace every core bridged tool carries on
	// the CLI wire (mcp__tionharness_interaction__<tool>).
	InteractionToolPrefix = "mcp__" + InteractionCoreKey + "__"
)

// PermissionPromptToolID is the namespaced Interaction MCP tool the claude CLI is
// pointed at via --permission-prompt-tool (only in "ask" mode) so risky tools are
// gated through TionHarness's approval UI instead of auto-approved. It lives on
// the core (always-loaded) server so the permission round-trip never waits on
// tool search.
const PermissionPromptToolID = InteractionToolPrefix + "permission_prompt"

// PromptToolForMode returns the permission-prompt tool id to hand the CLI for a
// turn, or "" when no per-tool prompt is needed. It is wired in two modes (both
// need a live Interaction endpoint to answer the prompt):
//   - "ask":       every write/exec tool is gated through the prompt for approval.
//   - "read-only": the CLI also runs in --permission-mode plan (mutations blocked
//     outright); the ONLY call that reaches the prompt is ExitPlanMode, which
//     TionHarness renders as a plan-approval card.
//
// "auto" uses bypass and needs no prompt.
func PromptToolForMode(mode string, inter tools.InteractionEndpoint) string {
	if (mode == "ask" || mode == "read-only") && inter.URL != "" {
		return PermissionPromptToolID
	}
	return ""
}

// Result is what WriteConfig produced for one turn.
type Result struct {
	// Path of the --mcp-config file, "" when nothing was wired.
	Path string
	// Allowed / Disallowed are the --allowedTools / --disallowedTools lists.
	Allowed    []string
	Disallowed []string
	// Cleanup removes the temp file; always non-nil.
	Cleanup func()
}

// WriteConfig renders the claude --mcp-config file for one turn.
//
//   - When mcpEnabled, every enabled external MCP server the AGENT may use is
//     included. ag supplies the per-agent tool restriction: the CLI runs its own
//     tool loop, so a server mounted here is reachable regardless of what
//     TionHarness advertises — Host.ServerGate is the only place the agent's
//     blocked/allowed patterns can still keep a whole server out.
//   - When inter.URL is set, the in-process Interaction MCP server is added so the
//     CLI can reach TionHarness's human-in-the-loop tools (ask_user/todo_write), and
//     the conflicting CLI built-ins (AskUserQuestion/TodoWrite) are disallowed.
//
// Returns an empty Path when there is nothing to wire.
func WriteConfig(ctx context.Context, h Host, mcpEnabled bool, ag db.Agent, inter tools.InteractionEndpoint, mode string) (Result, error) {
	cfg := Config{MCPServers: map[string]Server{}}
	var allowed, disallowed []string
	logger := h.Logger()
	if logger == nil {
		logger = slog.Default()
	}

	if mcpEnabled {
		servers, err := h.EnabledServers(ctx)
		if err != nil {
			return Result{}, err
		}
		gate, gateErr := h.ServerGate(ctx, ag)
		if gateErr != nil {
			// Fail CLOSED: the agent's permission document is unreadable, so no MCP
			// server may be mounted into the CLI process — and the turn stops here
			// rather than starting with an unenforced tool surface.
			logger.Error("cli mcp config: agent tool restriction is malformed; refusing to mount MCP servers",
				"agent", ag.ID, "error", gateErr)
			h.EmitDebug(ctx, db.DebugEvent{
				Type:    db.DebugError,
				AgentID: ag.ID,
				Name:    "mcp_server_gate_malformed",
				Detail:  "agent tool restriction is malformed; no MCP server mounted",
				Error:   gateErr.Error(),
				Err:     true,
			})
			return Result{}, gateErr
		}
		for _, sc := range servers {
			key, _, _ := mcp.SplitNamespaced(mcp.NamespaceTool(sc.Name, "x"))
			if gate != nil && !gate(key) {
				logger.Debug("cli mcp config: server withheld by agent tool restriction",
					"server", key, "agent", ag.ID)
				continue
			}
			entry := Server{}
			switch sc.Transport {
			case db.MCPTransportSSE, db.MCPTransportHTTP:
				entry.Type = sc.Transport
				entry.URL = sc.URL
				if len(sc.Headers) > 0 {
					entry.Headers = sc.Headers
				}
			default:
				entry.Command = sc.Command
				entry.Args = sc.Args
				entry.Env = sc.Env
			}
			cfg.MCPServers[key] = entry
			allowed = append(allowed, "mcp__"+key)
		}
	}

	// Native web search is an AGENT-level toggle, so its suppression must not sit
	// inside the interaction-endpoint branch below: an agent without an endpoint
	// (no bridged tools at all) would silently keep the CLI's own WebSearch/
	// WebFetch even with the toggle off. The toggle is ON by default
	// (NativeWebSearchEnabled: nil = enabled), so this branch only fires for an
	// agent that explicitly opted out — but when it does, the caller must still
	// apply the spec on the strength of `disallowed` alone.
	if !ag.NativeWebSearchEnabled() {
		disallowed = append(disallowed, "WebSearch", "WebFetch")
	}

	if inter.URL != "" {
		authHeader := map[string]string{"Authorization": "Bearer " + inter.Token}
		// Two server entries point at the SAME in-process endpoint via distinct path
		// suffixes (/core, /extended) so the handler returns each tier's subset:
		//   - Core: alwaysLoad → never deferred (eager: Bash, ask_user, use_skill, ...).
		//   - Extended: deferred by the CLI's ToolSearch (self-management + NameOnly).
		// ENABLE_TOOL_SEARCH=auto (set on the CLI process env) inlines the extended
		// set when it fits in 10% of context and defers only the overflow.
		base := strings.TrimRight(inter.URL, "/")
		cfg.MCPServers[InteractionCoreKey] = Server{
			Type:       "http",
			URL:        base + "/core",
			Headers:    authHeader,
			AlwaysLoad: true,
		}
		cfg.MCPServers[InteractionExtendedKey] = Server{
			Type:    "http",
			URL:     base + "/extended",
			Headers: authHeader,
		}
		// Core tier stays per-tool (eager, alwaysLoad — a small, stable set): each name
		// is namespaced under the core server key.
		for _, t := range inter.CoreToolNames {
			allowed = append(allowed, InteractionToolPrefix+t)
		}
		// Extended tier uses a SERVER-LEVEL wildcard (`mcp__tionharness_extended`, no tool
		// suffix) instead of enumerating each tool — the same shape external MCP servers
		// already use above (`mcp__`+key). Two reasons (Doc 52 §3-D / §11-decision 4):
		//   - Gateway pattern: a tool added mid-session via tools/list_changed is already
		//     permitted without touching --allowedTools.
		//   - Persistent-session warmth: a per-tool list changes whenever the extended set
		//     changes, churning the launch fingerprint and cold-restarting the process
		//     every turn. A constant wildcard keeps the allowlist — and the fingerprint —
		//     stable across turns.
		// The real per-tool gate remains: (a) the backend only advertises the tools it
		// wants callable in tools/list, and (b) the permission-prompt layer (ask mode).
		allowed = append(allowed, "mcp__"+InteractionExtendedKey)
		// WS17 invariant — never leave the model with a suppressed native tool AND no
		// advertised bridge. Some native suppressions below have a REQUIRED bridged
		// replacement the system prompt / skill catalog actively mandates (the prompt
		// says "ALWAYS call todo_write"; the skills block tells the model to call
		// use_skill). If that bridge was filtered out of THIS turn's advertised set
		// (workspace DisabledTools or the agent denylist dropped it from
		// CoreToolNames/ExtendedToolNames), suppressing the native counterpart too
		// leaves the model with no working tool — the "No such tool available" dead end.
		// So those families are suppressed ONLY when their bridge is actually advertised;
		// otherwise the native fallback is kept and the gap is logged (never silently
		// swallowed). The advertised set is CoreToolNames ∪ ExtendedToolNames — an
		// extended tool starts un-advertised on the wire but is reachable via
		// activate_tools, so it still counts as an available bridge.
		advertised := advertisedSet(inter)
		suppressIfBridged := func(bridge string, natives ...string) {
			if advertised[bridge] {
				disallowed = append(disallowed, natives...)
				return
			}
			logger.Warn("cli mcp config: required bridge tool not advertised; keeping native fallback",
				"bridge", bridge, "natives", natives)
		}
		// AskUserQuestion / ScheduleWakeup have NO valid native fallback in one-shot -p
		// mode (AskUserQuestion has no live client; a native wake the subprocess never
		// lives to fire), so they are suppressed unconditionally — keeping them would
		// only mislead, not help.
		disallowed = append(disallowed, "AskUserQuestion", "ScheduleWakeup")
		// The checklist family is the subtle one: newer Claude Code CLIs renamed the
		// old TodoWrite into a TaskCreate/TaskUpdate/TaskList/TaskGet family. Whichever
		// the CLI version exposes, it SHADOWS TionHarness's bridged todo_write — the model
		// reaches for the native tool, so nothing reaches the progress sink and the
		// progress card stays empty. Suppress the whole family (disallowing a tool the
		// CLI doesn't have is harmless) so todo_write is the only checklist path — but
		// only while todo_write is actually advertised (see the WS17 invariant above).
		suppressIfBridged("todo_write", todoFamily...)
		// Skill: the CLI's native skill tool only sees its own <CLAUDE_CONFIG_DIR>/skills
		// dir, never TionHarness's workspace tier (<workspace>/skills) or global tier
		// (~/.tionharness/skills) — so a weak model reaching for it fails with "Unknown
		// skill". The bridged use_skill (above) is the single correct path (it serves
		// both tiers), so suppress the native one to force it — but only while use_skill
		// is advertised, else the native Skill stays as the (CLI-native-only) fallback.
		suppressIfBridged("use_skill", "Skill")
		// Subagent launcher: the CLI's native delegation tool (older CLIs call it
		// `Task`, newer ones `Agent`) spawns a child entirely inside the CLI process —
		// invisible to TionHarness, so it bypasses the bridged run_subagent (no `subagent`
		// trace, no TionHarness agent/profile target, no budget accounting). When
		// delegation is enabled run_subagent is the gated replacement; when it is
		// disabled the agent should not delegate at all. Either way the native launcher
		// must be suppressed — same shadowing class as TodoWrite/Skill above.
		// AgentOutputTool is the reader alias newer CLIs expose for a launched
		// subagent's output; with the launcher gone it has nothing to read, but
		// suppressing it too keeps the whole native delegation family off the menu.
		disallowed = append(disallowed, "Task", "Agent", "AgentOutputTool")
		// Peer messaging: claude-cli 2.x ships a native `SendMessage` tool (sibling of
		// Task/Agent) that talks to the CLI's OWN in-process subagents — it knows
		// nothing about TionHarness agents, so it fails with "agent not found" even for a
		// valid TionHarness id. It SHADOWS the bridged send_message (DeliverAgentMessage);
		// a model that discovers the native one via ToolSearch reaches for it and every
		// delivery fails. Suppress it so bridged send_message is the only peer-DM path.
		disallowed = append(disallowed, "SendMessage")
		// Bash: only suppress the CLI's native POSIX Bash when TionHarness's own shell is
		// bridged (shell enabled) as its replacement — otherwise the agent would lose
		// shell entirely (TionHarness's shell is not bridged when disabled). With the
		// bridge present, all commands route through TionHarness's own sandboxed shells
		// (bridged Bash-preferred, plus PowerShell for Windows-native tasks).
		if h.ShellEnabled() {
			// Also suppress the native background-shell siblings (BashOutput/KillShell,
			// renamed TaskOutput/TaskStop in newer CLIs): they only operate on shells the
			// native Bash spawned, which is now gone, so they are inert — but suppressing
			// them keeps the whole native shell family off the menu so a model never
			// reaches for them instead of TionHarness's bridged run_in_background +
			// shell_manage. Both old and new names are listed because a CLI upgrade could
			// swap the exposed name; disallowing an absent tool is a no-op, so covering
			// both is safe across CLI versions. These stay INSIDE the ShellEnabled guard
			// on purpose: when shell is disabled the native Bash lives, and TaskOutput/
			// TaskStop are then the only way to manage the background tasks it spawns.
			disallowed = append(disallowed, shellFamily...)
		}
		// Plan mode: claude-cli's EnterPlanMode/ExitPlanMode only complete when their
		// exit approval can be answered. TionHarness answers it via the permission-prompt
		// tool, which is wired only in "ask" and "read-only" modes (see
		// PromptToolForMode). In "auto" (bypass) there is no approver, so a voluntary
		// plan-mode entry would hang on the headless "Exit plan mode?" prompt and the
		// tool result comes back is_error — suppress both so the agent just executes.
		if mode != "ask" && mode != "read-only" {
			disallowed = append(disallowed, planFamily...)
		}
	}

	// No servers to wire → no --mcp-config file. The disallow list still travels:
	// it is agent-level (native web search) and applies to a plain CLI turn too.
	if len(cfg.MCPServers) == 0 {
		return Result{Disallowed: disallowed, Cleanup: func() {}}, nil
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Result{}, err
	}
	path, cleanup, err := writeTemp("tionharness-mcp-*.json", data)
	if err != nil {
		return Result{}, err
	}
	logger.Info("cli mcp config written", "path", filepath.Base(path),
		"servers", len(cfg.MCPServers), "interaction", inter.URL != "")
	return Result{Path: path, Allowed: allowed, Disallowed: disallowed, Cleanup: cleanup}, nil
}

// Native tool families the bridge shadows; shared by WriteConfig (suppression)
// and NativeToolAllowlist (the positive mirror) so the two can never disagree.
var (
	todoFamily  = []string{"TodoWrite", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet"}
	shellFamily = []string{"Bash", "BashOutput", "KillShell", "TaskOutput", "TaskStop"}
	planFamily  = []string{"EnterPlanMode", "ExitPlanMode"}
)

// advertisedSet is CoreToolNames ∪ ExtendedToolNames as a lookup set.
func advertisedSet(inter tools.InteractionEndpoint) map[string]bool {
	advertised := make(map[string]bool, len(inter.CoreToolNames)+len(inter.ExtendedToolNames))
	for _, t := range inter.CoreToolNames {
		advertised[t] = true
	}
	for _, t := range inter.ExtendedToolNames {
		advertised[t] = true
	}
	return advertised
}

// writeTemp writes data to a fresh temp file and returns its path plus a remover.
func writeTemp(pattern string, data []byte) (string, func(), error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", nil, err
	}
	_ = f.Close()
	return path, func() { _ = os.Remove(path) }, nil
}
