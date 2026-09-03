package climcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Settings is the subset of the claude CLI's settings.json TionHarness generates
// per turn: a permission deny-list (defense-in-depth alongside --disallowedTools,
// with pattern support), the workspace's PreToolUse/PostToolUse hooks so the
// CLI's own tool loop fires the same hooks the native loop does (CLI-path hooks),
// and an explicit effortLevel (parallel-tool-call batching guard, see
// EffortLevel).
type Settings struct {
	Permissions *Permissions          `json:"permissions,omitempty"`
	Hooks       map[string][]HookRule `json:"hooks,omitempty"`
	EffortLevel string                `json:"effortLevel,omitempty"`
}

// Permissions is the settings.json permissions block (deny-list only).
type Permissions struct {
	Deny []string `json:"deny,omitempty"`
}

// HookRule mirrors Claude Code's settings hook shape: a matcher plus a list of
// command hooks to run for tools matching it.
type HookRule struct {
	Matcher string     `json:"matcher,omitempty"`
	Hooks   []HookSpec `json:"hooks"`
}

// HookSpec is one command hook.
type HookSpec struct {
	Type    string `json:"type"`              // always "command"
	Command string `json:"command"`           // shell command
	Timeout int    `json:"timeout,omitempty"` // seconds
}

// WriteSettings renders a per-turn claude --settings file carrying (a) a
// permission deny-list mirroring the native-tool suppression (so a CLI version
// that honours permissions.deny in bypass mode blocks them even if a flag is
// ignored), and (b) the workspace's enabled PreToolUse/PostToolUse hooks, so a
// claude-cli agent's own tool loop triggers the same hooks the native loop runs.
// Returns ("", noop, nil) when there is nothing to write (no deny + no hooks), so
// the caller passes --settings only when it carries something.
//
// CLI hooks run under the CLI's own hook runner, which uses a POSIX shell on every
// platform — unlike TionHarness's execHook, which uses PowerShell on Windows. The
// command is therefore translated by HookCommand so a hook authored in the
// workspace's native dialect runs identically on both paths.
//
// effort pins the CLI's effortLevel for this turn (see EffortLevel); it is
// always non-empty, so the settings file is written on every MCP-delegated turn.
func WriteSettings(ctx context.Context, h Host, deny []string, effort string) (string, func(), error) {
	noop := func() {}
	// Claude Code's settings.json effortLevel enum rejects "max" (silently
	// downgrading to high), so the file can carry at most xhigh. A max turn writes
	// xhigh as the floor here and the provider lifts it to max via the env var
	// (CLAUDE_CODE_EFFORT_LEVEL) — see providers.Request.CLIEffortLevel.
	fileEffort := effort
	if strings.EqualFold(fileEffort, "max") {
		fileEffort = "xhigh"
	}
	set := Settings{EffortLevel: fileEffort}
	if len(deny) > 0 {
		set.Permissions = &Permissions{Deny: append([]string(nil), deny...)}
	}
	logger := h.Logger()

	hooks := map[string][]HookRule{}
	// CLI-path hook passthrough is opt-out (on by default): skip the hooks block
	// entirely when disabled, leaving only the permission deny-list.
	if h.CLIHooksEnabled() {
		for _, event := range []string{db.HookPreToolUse, db.HookPostToolUse} {
			list, err := h.EnabledHooks(ctx, event)
			if err != nil {
				if logger != nil {
					logger.Warn("cli settings: list hooks failed", "event", event, "error", err)
				}
				continue
			}
			for _, hk := range list {
				if hk.Type != "" && hk.Type != "command" {
					continue // only command hooks map to the CLI contract
				}
				hooks[event] = append(hooks[event], HookRule{
					// Translate TionHarness's comma-glob matcher to Claude Code regex — a
					// verbatim comma list never matches in the CLI (see MatcherRegex).
					Matcher: MatcherRegex(hk.Matcher),
					// Translate the command to the interpreter the CLI actually spawns
					// (POSIX bash, on Windows too) — a PowerShell-authored hook passed
					// verbatim dies with a bash syntax error (see HookCommand).
					Hooks: []HookSpec{{Type: "command", Command: HookCommand(hk.Command), Timeout: hk.TimeoutSec}},
				})
			}
		}
	}
	if len(hooks) > 0 {
		set.Hooks = hooks
	}

	if set.Permissions == nil && set.Hooks == nil && set.EffortLevel == "" {
		return "", noop, nil
	}

	data, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return "", noop, err
	}
	path, cleanup, err := writeTemp("tionharness-settings-*.json", data)
	if err != nil {
		return "", noop, err
	}
	if logger != nil {
		logger.Info("cli settings written", "path", filepath.Base(path),
			"deny", len(deny), "hookEvents", len(hooks), "effort", fileEffort)
	}
	return path, cleanup, nil
}

// EffortLevel maps an agent's ThinkingLevel onto Claude Code's effortLevel
// setting, so the level has a CLI-side meaning. Claude Code ≥2.1.203 serialises
// parallel tool calls when effortLevel is unset (default adaptive effort) on
// SIMPLE tasks — an explicit effort restores their batching; complex (thinking)
// tasks additionally need thinking off (providers.Request.DisableThinking,
// "think XOR batch") — see _Docs/05 2026-07-09/10. Empty ("Kapalı") and "off"
// map to "high": thinking is already disabled for those levels, and high effort
// keeps simple-task batching (a low effort risks re-serialising it).
//
// xhigh/max/ultra pass through so the deep-reasoning tiers actually reach the CLI
// (the deliberate deep-work path); the caller accepts that thinking-on serialises
// tool calls at those tiers ("think XOR batch"). "max" cannot ride the --settings
// file (Claude Code's enum rejects it) — WriteSettings clamps it to xhigh there and
// the provider lifts it via CLAUDE_CODE_EFFORT_LEVEL.
func EffortLevel(thinkingLevel string) string {
	switch thinkingLevel {
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "xhigh":
		return "xhigh"
	case "max":
		return "max"
	case "ultra":
		return "ultra"
	case "", "off", "high":
		return "high"
	default:
		// Preserve invalid values. The turn prepare step validates stored agent
		// levels before any CLI call; retaining the token here also prevents a
		// lower-level caller from silently turning an invalid value into high effort.
		return thinkingLevel
	}
}
