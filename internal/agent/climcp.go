package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/mcp"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// cliMCPConfig is the on-disk shape claude --mcp-config expects.
type cliMCPConfig struct {
	MCPServers map[string]cliMCPServer `json:"mcpServers"`
}

type cliMCPServer struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type,omitempty"`    // sse | http
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"` // http transport (e.g. Authorization)
	// AlwaysLoad exempts a server's tools from the CLI's tool-search deferral
	// (claude-cli 2.1.x+): they are inlined at turn start instead of discovered via
	// ToolSearch. Used for the eager interaction tier so Bash/ask_user/use_skill are
	// always available without a discovery round-trip.
	AlwaysLoad bool `json:"alwaysLoad,omitempty"`
}

// interactionCoreKey / interactionExtendedKey are the two mcp-config keys for the
// in-process Interaction MCP server. Core keeps the historical "swarmgo_interaction"
// key so existing namespaced references (use_skill, core_memory, trace stripping)
// stay valid; Extended is a separate key whose tools the CLI defers via ToolSearch
// (claude-cli 2.1.x+). The CLI namespaces tools as mcp__<key>__<tool>.
const (
	interactionCoreKey     = "swarmgo_interaction"
	interactionExtendedKey = "swarmgo_extended"
)

// permissionPromptToolID is the namespaced Interaction MCP tool the claude CLI is
// pointed at via --permission-prompt-tool (only in "ask" mode) so risky tools are
// gated through SwarmGo's approval UI instead of auto-approved. It lives on the core
// (always-loaded) server so the permission round-trip never waits on tool search.
const permissionPromptToolID = "mcp__" + interactionCoreKey + "__permission_prompt"

// promptToolForMode returns the permission-prompt tool id to hand the CLI, but
// only in "ask" mode with a live Interaction endpoint. Empty otherwise:
// read-only uses CLI plan mode and auto uses bypass — neither needs a per-tool
// prompt.
func promptToolForMode(mode string, inter tools.InteractionEndpoint) string {
	if mode == "ask" && inter.URL != "" {
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
//     CLI can reach SwarmGo's human-in-the-loop tools (ask_user/todo_write), and
//     the conflicting CLI built-ins (AskUserQuestion/TodoWrite) are disallowed.
//
// Returns an empty path when there is nothing to wire.
func (r *Runtime) writeCLIMCPConfig(ctx context.Context, mcpEnabled bool, inter tools.InteractionEndpoint) (string, []string, []string, func(), error) {
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
		// Allowlist derives from the names the backend advertises for this turn
		// (InteractionEndpoint.Core/ExtendedToolNames) — a single source shared with
		// Tools(), so a tool added to the backend is allowlisted automatically. Each
		// name is namespaced under its tier's server key.
		for _, t := range inter.CoreToolNames {
			allowed = append(allowed, "mcp__"+interactionCoreKey+"__"+t)
		}
		for _, t := range inter.ExtendedToolNames {
			allowed = append(allowed, "mcp__"+interactionExtendedKey+"__"+t)
		}
		// Suppress the CLI's own equivalents, which can't be answered/honored in
		// one-shot -p mode: AskUserQuestion has no live client, and ScheduleWakeup
		// schedules a wake the CLI subprocess never lives to fire — SwarmGo's own
		// schedule_wake (above) replaces it with a real timer.
		//
		// The checklist family is the subtle one: newer Claude Code CLIs renamed the
		// old TodoWrite into a TaskCreate/TaskUpdate/TaskList/TaskGet family. Whichever
		// the CLI version exposes, it SHADOWS SwarmGo's bridged todo_write — the model
		// reaches for the native tool, so nothing reaches the progress sink and the
		// progress card stays empty. Suppress BOTH names (disallowing a tool the CLI
		// doesn't have is harmless) so todo_write is the only checklist path.
		disallowed = append(disallowed,
			"AskUserQuestion",
			"TodoWrite", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet",
			"ScheduleWakeup")
		// Skill: the CLI's native skill tool only sees its own .claude/skills dirs,
		// never SwarmGo's workspace skills — so a weak model reaching for it fails
		// with "Unknown skill". The bridged use_skill (above) is the correct path,
		// so suppress the native one to force it.
		disallowed = append(disallowed, "Skill")
		// Subagent launcher: the CLI's native delegation tool (older CLIs call it
		// `Task`, newer ones `Agent`) spawns a child entirely inside the CLI process —
		// invisible to SwarmGo, so it bypasses the bridged run_subagent (no `subagent`
		// trace, no SwarmGo agent/profile target, no budget accounting). When
		// delegation is enabled run_subagent is the gated replacement; when it is
		// disabled the agent should not delegate at all. Either way the native launcher
		// must be suppressed — same shadowing class as TodoWrite/Skill above.
		disallowed = append(disallowed, "Task", "Agent")
		// Bash: only suppress the CLI's native POSIX Bash when SwarmGo's own shell is
		// bridged (shell enabled) as its replacement — otherwise the agent would lose
		// shell entirely (SwarmGo's shell is not bridged when disabled). With the
		// bridge present, all commands route through SwarmGo's PowerShell shell.
		if r.tun.ShellEnabled() {
			disallowed = append(disallowed, "Bash")
		}
	}

	if len(cfg.MCPServers) == 0 {
		return "", nil, nil, func() {}, nil
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", nil, nil, nil, err
	}
	f, err := os.CreateTemp("", "swarmgo-mcp-*.json")
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

// cliSettings is the subset of the claude CLI's settings.json SwarmGo generates
// per turn: a permission deny-list (defense-in-depth alongside --disallowedTools,
// with pattern support) plus the workspace's PreToolUse/PostToolUse hooks so the
// CLI's own tool loop fires the same hooks the native loop does (CLI-path hooks).
type cliSettings struct {
	Permissions *cliPermissions          `json:"permissions,omitempty"`
	Hooks       map[string][]cliHookRule `json:"hooks,omitempty"`
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
// from SwarmGo's execHook (PowerShell on Windows). A hook authored for SwarmGo's
// shell may need adjusting to run identically here.
func (r *Runtime) writeCLISettings(ctx context.Context, deny []string) (string, func(), error) {
	set := cliSettings{}
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
				Matcher: h.Matcher,
				Hooks:   []cliHookSpec{{Type: "command", Command: h.Command, Timeout: h.TimeoutSec}},
			})
		}
	}
	if len(hooks) > 0 {
		set.Hooks = hooks
	}

	if set.Permissions == nil && set.Hooks == nil {
		return "", func() {}, nil
	}

	data, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return "", func() {}, err
	}
	f, err := os.CreateTemp("", "swarmgo-settings-*.json")
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
		"deny", len(deny), "hookEvents", len(hooks))
	return path, func() { _ = os.Remove(path) }, nil
}
