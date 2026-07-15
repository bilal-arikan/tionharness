package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/mcp"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// cliMCPConfig is the on-disk shape claude --mcp-config expects.
type cliMCPConfig struct {
	MCPServers map[string]cliMCPServer `json:"mcpServers"`
}

type cliMCPServer struct {
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

// interactionCoreKey / interactionExtendedKey are the two mcp-config keys for the
// in-process Interaction MCP server. Core keeps the historical "tionswarm_interaction"
// key so existing namespaced references (use_skill, trace stripping)
// stay valid; Extended is a separate key whose tools the CLI defers via ToolSearch
// (claude-cli 2.1.x+). The CLI namespaces tools as mcp__<key>__<tool>.
const (
	interactionCoreKey     = "tionswarm_interaction"
	interactionExtendedKey = "tionswarm_extended"
)

// permissionPromptToolID is the namespaced Interaction MCP tool the claude CLI is
// pointed at via --permission-prompt-tool (only in "ask" mode) so risky tools are
// gated through TionSwarm's approval UI instead of auto-approved. It lives on the core
// (always-loaded) server so the permission round-trip never waits on tool search.
const permissionPromptToolID = "mcp__" + interactionCoreKey + "__permission_prompt"

// promptToolForMode returns the permission-prompt tool id to hand the CLI for a
// turn, or "" when no per-tool prompt is needed. It is wired in two modes (both
// need a live Interaction endpoint to answer the prompt):
//   - "ask":       every write/exec tool is gated through the prompt for approval.
//   - "read-only": the CLI also runs in --permission-mode plan (mutations blocked
//     outright); the ONLY call that reaches the prompt is ExitPlanMode, which
//     TionSwarm renders as a plan-approval card.
//
// "auto" uses bypass and needs no prompt.
func promptToolForMode(mode string, inter tools.InteractionEndpoint) string {
	if (mode == "ask" || mode == "read-only") && inter.URL != "" {
		return permissionPromptToolID
	}
	return ""
}

// writeCLIMCPConfig renders the claude --mcp-config file for one turn and returns
// the path, the allowlist of tool identifiers, the list of CLI built-ins to
// disallow, and a cleanup func.
//
//   - When mcpEnabled, every enabled external MCP server is included.
//   - When inter.URL is set, the in-process Interaction MCP server is added so the
//     CLI can reach TionSwarm's human-in-the-loop tools (ask_user/todo_write), and
//     the conflicting CLI built-ins (AskUserQuestion/TodoWrite) are disallowed.
//
// Returns an empty path when there is nothing to wire.
func (r *Runtime) writeCLIMCPConfig(ctx context.Context, mcpEnabled bool, inter tools.InteractionEndpoint, mode string) (string, []string, []string, func(), error) {
	cfg := cliMCPConfig{MCPServers: map[string]cliMCPServer{}}
	var allowed, disallowed []string

	if mcpEnabled {
		servers, err := r.db.ListEnabledMCPServers(ctx)
		if err != nil {
			return "", nil, nil, nil, err
		}
		for _, m := range servers {
			sc := toServerConfig(m)
			key, _, _ := mcp.SplitNamespaced(mcp.NamespaceTool(sc.Name, "x"))
			entry := cliMCPServer{}
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

	if inter.URL != "" {
		authHeader := map[string]string{"Authorization": "Bearer " + inter.Token}
		// Two server entries point at the SAME in-process endpoint via distinct path
		// suffixes (/core, /extended) so the handler returns each tier's subset:
		//   - Core: alwaysLoad → never deferred (eager: Bash, ask_user, use_skill, ...).
		//   - Extended: deferred by the CLI's ToolSearch (self-management + NameOnly).
		// ENABLE_TOOL_SEARCH=auto (set on the CLI process env) inlines the extended
		// set when it fits in 10% of context and defers only the overflow.
		base := strings.TrimRight(inter.URL, "/")
		cfg.MCPServers[interactionCoreKey] = cliMCPServer{
			Type:       "http",
			URL:        base + "/core",
			Headers:    authHeader,
			AlwaysLoad: true,
		}
		cfg.MCPServers[interactionExtendedKey] = cliMCPServer{
			Type:    "http",
			URL:     base + "/extended",
			Headers: authHeader,
		}
		// Core tier stays per-tool (eager, alwaysLoad — a small, stable set): each name
		// is namespaced under the core server key.
		for _, t := range inter.CoreToolNames {
			allowed = append(allowed, "mcp__"+interactionCoreKey+"__"+t)
		}
		// Extended tier uses a SERVER-LEVEL wildcard (`mcp__tionswarm_extended`, no tool
		// suffix) instead of enumerating each tool — the same shape external MCP servers
		// already use above (`mcp__`+key). Two reasons (Doc 52 §3-D / §11-decision 4):
		//   - Gateway pattern: a tool added mid-session via tools/list_changed is already
		//     permitted without touching --allowedTools (verified: a server-level wildcard
		//     covers a late-registered tool — spike Q2).
		//   - Persistent-session warmth: a per-tool list changes whenever the extended set
		//     changes, churning the launch fingerprint and cold-restarting the process
		//     every turn. A constant wildcard keeps the allowlist — and the fingerprint —
		//     stable across turns.
		// The real per-tool gate remains: (a) the backend only advertises the tools it
		// wants callable in tools/list, and (b) the permission-prompt layer (ask mode).
		// Unconditional: the extended server entry is always wired above, so its wildcard
		// is always present — forward-compatible with the gateway's empty-start surface
		// (extended advertises nothing until tools/list_changed grows it, yet each grown
		// tool is already permitted).
		allowed = append(allowed, "mcp__"+interactionExtendedKey)
		// Suppress the CLI's own equivalents, which can't be answered/honored in
		// one-shot -p mode: AskUserQuestion has no live client, and ScheduleWakeup
		// schedules a wake the CLI subprocess never lives to fire — TionSwarm's own
		// schedule_wake (above) replaces it with a real timer.
		//
		// The checklist family is the subtle one: newer Claude Code CLIs renamed the
		// old TodoWrite into a TaskCreate/TaskUpdate/TaskList/TaskGet family. Whichever
		// the CLI version exposes, it SHADOWS TionSwarm's bridged todo_write — the model
		// reaches for the native tool, so nothing reaches the progress sink and the
		// progress card stays empty. Suppress BOTH names (disallowing a tool the CLI
		// doesn't have is harmless) so todo_write is the only checklist path.
		disallowed = append(disallowed,
			"AskUserQuestion",
			"TodoWrite", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet",
			"ScheduleWakeup")
		// Skill: the CLI's native skill tool only sees its own <CLAUDE_CONFIG_DIR>/skills
		// dir, never TionSwarm's workspace tier (<workspace>/skills) or global tier
		// (~/.tionswarm/skills) — so a weak model reaching for it fails with "Unknown
		// skill". The bridged use_skill (above) is the single correct path (it serves
		// both tiers), so suppress the native one to force it.
		disallowed = append(disallowed, "Skill")
		// Subagent launcher: the CLI's native delegation tool (older CLIs call it
		// `Task`, newer ones `Agent`) spawns a child entirely inside the CLI process —
		// invisible to TionSwarm, so it bypasses the bridged run_subagent (no `subagent`
		// trace, no TionSwarm agent/profile target, no budget accounting). When
		// delegation is enabled run_subagent is the gated replacement; when it is
		// disabled the agent should not delegate at all. Either way the native launcher
		// must be suppressed — same shadowing class as TodoWrite/Skill above.
		// AgentOutputTool is the reader alias newer CLIs expose for a launched
		// subagent's output; with the launcher gone it has nothing to read, but
		// suppressing it too keeps the whole native delegation family off the menu.
		disallowed = append(disallowed, "Task", "Agent", "AgentOutputTool")
		// Peer messaging: claude-cli 2.x ships a native `SendMessage` tool (sibling of
		// Task/Agent) that talks to the CLI's OWN in-process subagents — it knows
		// nothing about TionSwarm agents, so it fails with "agent not found" even for a
		// valid TionSwarm id. It SHADOWS the bridged send_message (DeliverAgentMessage);
		// a model that discovers the native one via ToolSearch reaches for it and every
		// delivery fails. Suppress it so bridged send_message is the only peer-DM path.
		disallowed = append(disallowed, "SendMessage")
		// Bash: only suppress the CLI's native POSIX Bash when TionSwarm's own shell is
		// bridged (shell enabled) as its replacement — otherwise the agent would lose
		// shell entirely (TionSwarm's shell is not bridged when disabled). With the
		// bridge present, all commands route through TionSwarm's own sandboxed shells
		// (bridged Bash-preferred, plus PowerShell for Windows-native tasks).
		if r.tun.ShellEnabled() {
			// Also suppress the native background-shell siblings (BashOutput/KillShell,
			// renamed TaskOutput/TaskStop in newer CLIs): they only operate on shells the
			// native Bash spawned, which is now gone, so they are inert — but suppressing
			// them keeps the whole native shell family off the menu so a model never
			// reaches for them instead of TionSwarm's bridged run_in_background +
			// shell_manage. Both old and new names are listed because a CLI upgrade could
			// swap the exposed name; disallowing an absent tool is a no-op, so covering
			// both is safe across CLI versions. These stay INSIDE the ShellEnabled guard
			// on purpose: when shell is disabled the native Bash lives, and TaskOutput/
			// TaskStop are then the only way to manage the background tasks it spawns.
			disallowed = append(disallowed,
				"Bash", "BashOutput", "KillShell", "TaskOutput", "TaskStop")
		}
		// Plan mode: claude-cli's EnterPlanMode/ExitPlanMode only complete when their
		// exit approval can be answered. TionSwarm answers it via the permission-prompt
		// tool, which is wired only in "ask" and "read-only" modes (see
		// promptToolForMode). In "auto" (bypass) there is no approver, so a voluntary
		// plan-mode entry would hang on the headless "Exit plan mode?" prompt and the
		// tool result comes back is_error — suppress both so the agent just executes.
		if mode != "ask" && mode != "read-only" {
			disallowed = append(disallowed, "EnterPlanMode", "ExitPlanMode")
		}
	}

	if len(cfg.MCPServers) == 0 {
		return "", nil, nil, func() {}, nil
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", nil, nil, nil, err
	}
	f, err := os.CreateTemp("", "tionswarm-mcp-*.json")
	if err != nil {
		return "", nil, nil, nil, err
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", nil, nil, nil, err
	}
	_ = f.Close()

	cleanup := func() { _ = os.Remove(path) }
	r.logger.Info("cli mcp config written", "path", filepath.Base(path),
		"servers", len(cfg.MCPServers), "interaction", inter.URL != "")
	return path, allowed, disallowed, cleanup, nil
}

// cliSettings is the subset of the claude CLI's settings.json TionSwarm generates
// per turn: a permission deny-list (defense-in-depth alongside --disallowedTools,
// with pattern support), the workspace's PreToolUse/PostToolUse hooks so the
// CLI's own tool loop fires the same hooks the native loop does (CLI-path hooks),
// and an explicit effortLevel (parallel-tool-call batching guard, see
// cliEffortLevel).
type cliSettings struct {
	Permissions *cliPermissions          `json:"permissions,omitempty"`
	Hooks       map[string][]cliHookRule `json:"hooks,omitempty"`
	EffortLevel string                   `json:"effortLevel,omitempty"`
}

type cliPermissions struct {
	Deny []string `json:"deny,omitempty"`
}

// cliHookRule mirrors Claude Code's settings hook shape: a matcher plus a list of
// command hooks to run for tools matching it.
type cliHookRule struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []cliHookSpec `json:"hooks"`
}

type cliHookSpec struct {
	Type    string `json:"type"`              // always "command"
	Command string `json:"command"`           // shell command
	Timeout int    `json:"timeout,omitempty"` // seconds
}

// writeCLISettings renders a per-turn claude --settings file carrying (a) a
// permission deny-list mirroring the native-tool suppression (so a CLI version
// that honours permissions.deny in bypass mode blocks them even if a flag is
// ignored), and (b) the workspace's enabled PreToolUse/PostToolUse hooks, so a
// claude-cli agent's own tool loop triggers the same hooks the native loop runs.
// Returns ("", noop, nil) when there is nothing to write (no deny + no hooks), so
// the caller passes --settings only when it carries something.
//
// Caveat: CLI hooks run under the CLI's own hook runner/shell, which may differ
// from TionSwarm's execHook (PowerShell on Windows). A hook authored for TionSwarm's
// shell may need adjusting to run identically here.
//
// effort pins the CLI's effortLevel for this turn (see cliEffortLevel); it is
// always non-empty, so the settings file is now written on every MCP-delegated
// turn (previously only when a deny-list or hooks existed).
func (r *Runtime) writeCLISettings(ctx context.Context, deny []string, effort string) (string, func(), error) {
	set := cliSettings{EffortLevel: effort}
	if len(deny) > 0 {
		set.Permissions = &cliPermissions{Deny: append([]string(nil), deny...)}
	}

	hooks := map[string][]cliHookRule{}
	// CLI-path hook passthrough is opt-out (on by default): skip the hooks block
	// entirely when disabled, leaving only the permission deny-list.
	for _, event := range []string{db.HookPreToolUse, db.HookPostToolUse} {
		if !r.tun.CLIHooksEnabled() {
			break
		}
		list, err := r.db.ListEnabledHooksByEvent(ctx, event)
		if err != nil {
			r.logger.Warn("cli settings: list hooks failed", "event", event, "error", err)
			continue
		}
		for _, h := range list {
			if h.Type != "" && h.Type != "command" {
				continue // only command hooks map to the CLI contract
			}
			hooks[event] = append(hooks[event], cliHookRule{
				// Translate TionSwarm's comma-glob matcher to Claude Code regex — a
				// verbatim comma list never matches in the CLI (see cliMatcherRegex).
				Matcher: cliMatcherRegex(h.Matcher),
				Hooks:   []cliHookSpec{{Type: "command", Command: h.Command, Timeout: h.TimeoutSec}},
			})
		}
	}
	if len(hooks) > 0 {
		set.Hooks = hooks
	}

	if set.Permissions == nil && set.Hooks == nil && set.EffortLevel == "" {
		return "", func() {}, nil
	}

	data, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return "", func() {}, err
	}
	f, err := os.CreateTemp("", "tionswarm-settings-*.json")
	if err != nil {
		return "", func() {}, err
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", func() {}, err
	}
	_ = f.Close()
	r.logger.Info("cli settings written", "path", filepath.Base(path),
		"deny", len(deny), "hookEvents", len(hooks), "effort", effort)
	return path, func() { _ = os.Remove(path) }, nil
}

// cliEffortLevel maps an agent's ThinkingLevel onto Claude Code's effortLevel
// setting, so the level has a CLI-side meaning. Claude Code ≥2.1.203 serialises
// parallel tool calls when effortLevel is unset (default adaptive effort) on
// SIMPLE tasks — an explicit effort restores their batching; complex (thinking)
// tasks additionally need thinking off (Request.DisableThinking, "think XOR
// batch") — see _Docs/05 2026-07-09/10. Empty ("Kapalı") and "off" map to
// "high": thinking is already disabled for those levels, and high effort keeps
// simple-task batching (a low effort risks re-serialising it).
func cliEffortLevel(thinkingLevel string) string {
	switch thinkingLevel {
	case "low":
		return "low"
	case "medium":
		return "medium"
	default: // "", "off", "high", "xhigh", "max", unknown
		return "high"
	}
}
