package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/claudeauth"
	"github.com/bilal-arikan/tionswarm/internal/proc"
)

// ClaudeCLI drives the locally-installed `claude` (Claude Code) CLI in
// non-interactive print mode. It authenticates via the user's existing
// OAuth/subscription login — no API key required.
//
// This is the "CLI provider" approach: a local coding CLI driven as a provider.
type ClaudeCLI struct {
	binPath string
	model   string // optional alias/name override, e.g. "sonnet"
	// configDir, when set, is exported as CLAUDE_CONFIG_DIR into the subprocess so
	// the CLI reads its config home (skills, settings, slash commands, global
	// CLAUDE.md, login) from an isolated directory instead of the shared ~/.claude.
	// Empty → inherit the ambient ~/.claude (default behaviour). This lets TionSwarm
	// drive a "clean" CLI without touching the user's existing installation.
	configDir string
	// authKind/authToken inject a credential into the subprocess env so an isolated
	// configDir authenticates without an interactive in-dir `claude login`:
	//   "oauth"  → CLAUDE_CODE_OAUTH_TOKEN  (Max/Pro subscription token)
	//   "apikey" → ANTHROPIC_API_KEY        (API billing)
	// Empty token/kind injects nothing (rely on the config dir's own login).
	authKind  string
	authToken string

	// MCP delegation (Phase 8): when mcpConfigPath is set, the CLI is launched
	// with that MCP config and restricted to allowedTools. The CLI then runs
	// the full agentic tool loop itself and returns the final text. This is the
	// keyless tool-use path (no ANTHROPIC_API_KEY required).
	mcpConfigPath   string
	allowedTools    []string
	disallowedTools []string // CLI built-ins to suppress (e.g. AskUserQuestion, TodoWrite)
	// permissionPromptTool, when set, is handed to --permission-prompt-tool so the
	// CLI routes tools needing approval through that MCP tool (used in "ask" mode
	// instead of acceptEdits). Empty → fall back to the --permission-mode flag.
	permissionPromptTool string
	// settingsPath, when set, is handed to --settings so the CLI loads a TionSwarm-
	// generated settings.json (permission deny-list + PreToolUse/PostToolUse hooks)
	// for this turn. Lifecycle mirrors mcpConfigPath: set fresh by ConfigureMCP each
	// MCP turn (possibly "") and only emitted on the MCP path.
	settingsPath string
}

// NewClaudeCLI creates a provider that invokes the given claude binary. configDir,
// when non-empty, isolates the CLI's config home via CLAUDE_CONFIG_DIR (pass "" to
// inherit the user's ~/.claude). authKind/authToken inject a credential env var
// (pass "","" for none; see the struct fields).
func NewClaudeCLI(binPath, model, configDir, authKind, authToken string) *ClaudeCLI {
	return &ClaudeCLI{binPath: binPath, model: model, configDir: configDir, authKind: authKind, authToken: authToken}
}

// Installed reports whether the configured claude binary can be resolved to an
// executable — an absolute/relative path that exists, or a bare name found on
// PATH. It does NOT check login state; a true result only means the CLI is
// present to run. Callers use this to distinguish "CLI missing" (steer the user
// to set up a provider) from "CLI present but not authenticated" (offer login).
func (c *ClaudeCLI) Installed() bool {
	if c.binPath == "" {
		return false
	}
	_, err := exec.LookPath(c.binPath)
	return err == nil
}

func (c *ClaudeCLI) Preflight(ctx context.Context) error {
	env := cliBaseEnv("ENABLE_TOOL_SEARCH=auto")
	if c.configDir != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+c.configDir)
	}
	if c.authToken != "" {
		switch c.authKind {
		case "oauth":
			env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+c.authToken)
		case "apikey":
			env = append(env, "ANTHROPIC_API_KEY="+c.authToken)
		}
	}
	config := strings.Join([]string{c.model, c.authKind, c.authToken}, "\x00")
	return runCLIPreflight(ctx, c.Name(), c.binPath, c.configDir, config, env, "settings.json")
}

// ConfigureMCP enables MCP tool delegation for subsequent Complete calls.
// configPath points to a claude --mcp-config JSON file; allowedTools is the
// list of tool identifiers the CLI may use (e.g. "mcp__filesystem");
// disallowedTools suppresses conflicting CLI built-ins (e.g. AskUserQuestion,
// TodoWrite) so the TionSwarm Interaction MCP equivalents are used instead.
// settingsPath points to a --settings file (permission deny-list + hooks); pass
// "" for none. Its lifecycle is tied to configPath so it is reset every MCP turn.
func (c *ClaudeCLI) ConfigureMCP(configPath string, allowedTools, disallowedTools []string, permissionPromptTool, settingsPath string) {
	c.mcpConfigPath = configPath
	c.allowedTools = allowedTools
	c.disallowedTools = disallowedTools
	c.permissionPromptTool = permissionPromptTool
	c.settingsPath = settingsPath
}

// ConfigureCLIMCP implements CLIProvider by forwarding the transport-agnostic
// spec to ConfigureMCP. spec.Servers is deliberately not consumed here: on the
// claude path the caller has already rendered the servers into the
// --mcp-config file named by spec.ConfigPath, which is what the CLI reads.
func (c *ClaudeCLI) ConfigureCLIMCP(spec CLIMCPSpec) {
	c.ConfigureMCP(spec.ConfigPath, spec.AllowedTools, spec.DisallowedTools, spec.PermissionPrompt, spec.SettingsPath)
}

// SetConfigDir overrides the CLAUDE_CONFIG_DIR this provider exports into its
// subprocess, replacing the value baked in at construction. TionSwarm calls this
// per turn (Runtime.PinClaudeHome) so the CLI runs against the app-global
// <dataDir>/claude-home — one shared login/settings home — instead of the
// ambient ~/.claude. Empty is ignored so an instance that configured its own
// config home (K1) keeps it.
func (c *ClaudeCLI) SetConfigDir(dir string) {
	if dir == "" {
		return
	}
	c.configDir = dir
}

// ConfigDir returns the CLAUDE_CONFIG_DIR this provider currently exports —
// either the value baked in at construction (the instance's own configDir
// field, K1) or one set later via SetConfigDir. Callers use this to tell
// whether the instance already owns a dedicated config home before falling
// back to a workspace-derived one.
func (c *ClaudeCLI) ConfigDir() string { return c.configDir }

// Name implements Provider.
func (c *ClaudeCLI) Name() string { return "claude-cli" }

// permissionModeArgs maps TionSwarm's permission mode onto the claude CLI's
// permission flags. In headless (-p) mode the default mode cannot prompt for
// approval, so Edit/Write/Bash are refused unless an explicit mode is set:
//   - "read-only" → --permission-mode plan       (no mutations)
//   - "ask"       → --permission-mode acceptEdits (edits auto-approved)
//   - "auto"/""   → --dangerously-skip-permissions (everything auto-approved)
func permissionModeArgs(mode string) []string {
	switch mode {
	case "read-only":
		return []string{"--permission-mode", "plan"}
	case "ask":
		return []string{"--permission-mode", "acceptEdits"}
	default: // "auto", "" and any unknown value
		return []string{"--dangerously-skip-permissions"}
	}
}

// interactionSystemNote tells the CLI to use the TionSwarm Interaction MCP tools
// (which surface in the TionSwarm UI) instead of its own built-ins, which can't be
// answered in non-interactive print mode.
const interactionSystemNote = "To ask the user a clarifying question, call the ask_user tool and wait for the reply. " +
	"To create, show, or update a task checklist, ALWAYS call the todo_write tool — it persists to the session's progress file. " +
	"Do NOT use any built-in checklist or task tool (AskUserQuestion, TodoWrite, TaskCreate, TaskUpdate, TaskList, TaskGet): they do not reach TionSwarm and the progress view stays empty. " +
	"To delegate a focused sub-task to another agent, use the run_subagent tool when it is available; never use the built-in Task or Agent subagent launcher, which runs invisibly to TionSwarm."

// usesInteractionTools reports whether the TionSwarm Interaction MCP tools are in
// the allowlist for this call.
func (c *ClaudeCLI) usesInteractionTools() bool {
	for _, t := range c.allowedTools {
		// Matches both interaction tiers: mcp__tionswarm_interaction__* (core) and
		// mcp__tionswarm_extended__* (extended).
		if strings.Contains(t, "tionswarm_interaction") || strings.Contains(t, "tionswarm_extended") {
			return true
		}
	}
	return false
}

// permissionArgs maps the request's permission mode onto the claude CLI flags.
// In "ask" mode with a permission-prompt tool wired, route tools needing approval
// through it (CLI default mode + --permission-prompt-tool → real per-tool approval
// in the TionSwarm UI). Otherwise map the mode to a CLI permission flag (plan /
// acceptEdits / bypass) so headless edits aren't silently refused. Shared by the
// one-shot Complete path and the persistent-session launcher.
func (c *ClaudeCLI) permissionArgs(req Request) []string {
	if c.permissionPromptTool != "" {
		args := []string{"--permission-prompt-tool", c.permissionPromptTool}
		// read-only ALSO runs in plan mode: the CLI blocks every mutation itself, so
		// the only call that reaches the prompt tool is ExitPlanMode — where TionSwarm
		// renders the plan for approval. "ask" keeps the CLI's default mode so each
		// write/exec tool is gated individually through the prompt.
		if req.PermissionMode == "read-only" {
			args = append(args, "--permission-mode", "plan")
		}
		return args
	}
	return permissionModeArgs(req.PermissionMode)
}

// mcpArgs builds the MCP-delegation flags from the provider's ConfigureMCP state.
// The single-value --mcp-config is terminated by the boolean --strict-mcp-config;
// --disallowedTools (suppressing conflicting CLI built-ins) precedes the trailing
// --allowedTools so neither variadic flag swallows the other. Empty when no MCP
// config is set. Shared by Complete and the persistent-session launcher.
func (c *ClaudeCLI) mcpArgs() []string {
	if c.mcpConfigPath == "" {
		return nil
	}
	args := []string{"--mcp-config", c.mcpConfigPath, "--strict-mcp-config"}
	if c.settingsPath != "" {
		args = append(args, "--settings", c.settingsPath)
	}
	if len(c.disallowedTools) > 0 {
		args = append(args, "--disallowedTools")
		args = append(args, c.disallowedTools...)
	}
	if len(c.allowedTools) > 0 {
		args = append(args, "--allowedTools")
		args = append(args, c.allowedTools...)
	}
	return args
}

// buildSystemAndPrompt assembles the two halves of a claude-cli invocation, kept
// pure for testing:
//
//   - sys: the appended system prompt. ONLY the STATIC prefix (req.System) plus the
//     constant interaction note. Byte-stable across turns for a given agent, so
//     Claude Code's request-prefix prompt cache stays warm turn-to-turn (and across
//     sessions). The volatile suffix is deliberately excluded.
//   - prompt: the conversation prompt sent on stdin. The VOLATILE per-turn context
//     (req.SystemDynamic: turn-start clock, recalled memory, running summary,
//     sessions block, …) is prepended as a delimited [Context] block so it rides in
//     the uncached message tail instead of busting the cached system prefix. With
//     --resume (delta send) this still attaches to the turn actually on the wire.
//
// Before this split the static + volatile halves were merged into one appended
// system prompt; the seconds-precise clock line alone changed the cached prefix
// every turn, forcing a full cold cache write each turn (measured: turn 2 came
// back with cache_read=0). See _Docs/17.
func (c *ClaudeCLI) buildSystemAndPrompt(req Request) (sys, prompt string) {
	sys = strings.TrimSpace(req.System)
	if c.usesInteractionTools() {
		sys = strings.TrimSpace(sys + "\n\n" + interactionSystemNote)
	}
	// The rolling summary (req.Summary) stays woven into the uncached [Context]
	// tail here — the claude-cli path is already cache-optimal via --resume (its
	// warm server-side prefix is untouched), and weaving the summary each turn
	// keeps it FRESH after a mid-session fold, which the native head-message
	// placement (P2) cannot do on the warm delta path. So P2 is native-only; the
	// CLI behaviour is unchanged (summary simply moves from SystemDynamic back into
	// the effective dynamic here).
	prompt = withDynamic(serializeTranscript(req.Messages), joinNonEmpty(req.SystemDynamic, req.Summary))
	return sys, prompt
}

// --- stream-json event shapes (--output-format stream-json --verbose) ---
//
// The CLI emits one JSON object per line: system/init, assistant (content
// blocks: text/thinking/tool_use), user (tool_result blocks), and a final
// result envelope. We parse this stream to capture the CLI's own tool loop and
// thinking as an activity trace — keyless, no ANTHROPIC_API_KEY required.

type cliUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	// The CLI's stream-json reports Anthropic prompt-cache accounting too. These
	// are the savings that --resume (warm server-side cache) produces; without
	// parsing them the Usage screen shows 0 cache for every claude-cli turn and
	// the value of resume is invisible. cache_read = cheap (served from cache),
	// cache_creation = premium (written to cache this turn).
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

type cliBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"` // tool_use block identifier
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"` // tool_result → references a tool_use id
	Content   json.RawMessage `json:"content"`     // tool_result: string or block array
	IsError   bool            `json:"is_error"`
}

type cliMessage struct {
	ID      string     `json:"id"` // API message id — one id may span several assistant events
	Model   string     `json:"model"`
	Content []cliBlock `json:"content"`
	Usage   *cliUsage  `json:"usage"`
}

type cliEvent struct {
	Type           string                     `json:"type"`
	Subtype        string                     `json:"subtype"`
	Message        *cliMessage                `json:"message"`
	IsError        bool                       `json:"is_error"`
	APIErrorStatus string                     `json:"api_error_status"` // result envelope: upstream API error (e.g. rate_limit)
	Error          string                     `json:"error"`            // standalone error line (e.g. {"error":"authentication_failed"})
	Result         string                     `json:"result"`
	Usage          *cliUsage                  `json:"usage"`
	ModelUsage     map[string]json.RawMessage `json:"modelUsage"`
	NumTurns       int                        `json:"num_turns"`       // result event: internal tool-loop API round-trips this turn
	SessionID      string                     `json:"session_id"`      // emitted on system/init and result events
	RateLimit      *cliRateLimit              `json:"rate_limit_info"` // emitted on rate_limit_event
}

// cliRateLimit mirrors the rate_limit_info object the CLI emits on a
// "rate_limit_event". status "allowed"/"allowed_warning" both let the turn
// proceed (warning just means the window is filling up); anything outside the
// "allowed" family (e.g. "rejected"/"blocked") means the subscription window is
// exhausted and, when overage is disabled, the turn cannot proceed.
type cliRateLimit struct {
	Status                string `json:"status"`
	RateLimitType         string `json:"rateLimitType"`
	OverageStatus         string `json:"overageStatus"`
	OverageDisabledReason string `json:"overageDisabledReason"`
	ResetsAt              int64  `json:"resetsAt"`
}

// Complete implements Provider by shelling out to `claude -p` and parsing its
// streamed JSON event log line-by-line. When req.OnEvent is set, each activity
// step is delivered as soon as it completes (step-by-step streaming); the final
// Response carries the full text + trace regardless.
func (c *ClaudeCLI) Complete(ctx context.Context, req Request) (*Response, error) {
	// Note: do NOT use --bare here — it skips keychain reads and breaks the
	// OAuth/subscription login ("Not logged in"). stream-json needs --verbose.
	args := []string{"-p", "--output-format", "stream-json", "--verbose"}

	model := req.Model
	if model == "" {
		model = c.model
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	// Resume a prior CLI session (warm prompt cache + server-side history) when the
	// caller supplies its id. The caller then sends only the new turn(s) — the CLI
	// already holds the earlier conversation. The id rotates each resume turn, so the
	// caller must persist Response.SessionID for the next turn.
	if req.ResumeSessionID != "" {
		args = append(args, "--resume", req.ResumeSessionID)
	}
	args = append(args, c.permissionArgs(req)...)
	// Assemble the STABLE appended system prompt (static prefix only — keeps Claude
	// Code's prompt cache warm turn-to-turn and across sessions for the same agent)
	// plus the conversation prompt with the volatile dynamic context folded into the
	// message tail. See buildSystemAndPrompt / _Docs/17.
	sys, prompt := c.buildSystemAndPrompt(req)
	// The system prompt is handed to the subprocess one of two ways (req.SysPromptFile):
	//   - inline (default): --append-system-prompt <text>. Simplest, no temp file.
	//   - file: --append-system-prompt-file <path>. Windows caps a process command
	//     line at ~32 KB (ERROR_FILENAME_EXCED_RANGE / errno 206), so a very large
	//     system prompt (skills + core memory + dynamic context) can overflow it as
	//     an inline argument; the file mode rides only a short path on the command
	//     line. The conversation prompt already goes via stdin (see runAttempt). The
	//     temp file is removed once both retry attempts finish.
	if sys != "" {
		if req.SysPromptFile {
			f, ferr := os.CreateTemp("", "tionswarm-sysprompt-*.txt")
			if ferr != nil {
				return nil, fmt.Errorf("write system prompt file: %w", ferr)
			}
			sysPromptPath := f.Name()
			defer os.Remove(sysPromptPath)
			if _, werr := f.WriteString(sys); werr != nil {
				f.Close()
				return nil, fmt.Errorf("write system prompt file: %w", werr)
			}
			if cerr := f.Close(); cerr != nil {
				return nil, fmt.Errorf("write system prompt file: %w", cerr)
			}
			args = append(args, "--append-system-prompt-file", sysPromptPath)
		} else {
			args = append(args, "--append-system-prompt", sys)
		}
	}

	args = append(args, c.mcpArgs()...)

	// The claude CLI occasionally exits non-zero RIGHT AFTER init — before any
	// assistant output or tool call — with empty stderr (an intermittent crash on
	// the MCP-delegation path, seen on parallel flow nodes). That failure produced
	// no content and ran no tools, so it is SAFE to retry with a fresh process. We
	// retry only that "clean crash" once; any failure that produced output, ran a
	// tool (possible side effects), or came from context cancellation is returned
	// as-is.
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, retryable, err := c.runAttempt(ctx, args, prompt, model, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable || ctx.Err() != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

// ProbeAuth runs a minimal, tool-free `claude -p` against this provider's config
// home to verify the CLI is authenticated WITHOUT spending a real agent turn. It
// returns nil when logged in, a classified auth error (see isAuthErrorText) when
// the login is missing/expired/revoked, or the raw failure for any other startup
// problem. Intended as a pre-flight check so an auth lapse surfaces up front instead
// of failing the first real turn. It reflects the EFFECTIVE credential: an injected
// auth token (env) wins over the config dir's own login, matching turn behaviour.
func (c *ClaudeCLI) ProbeAuth(ctx context.Context) error {
	// No MCP, no system prompt, tiny prompt — the cheapest invocation that still
	// exercises the auth/login path. --strict-mcp-config + empty config keeps the
	// CLI from loading any project/user MCP servers (fast, isolated).
	args := []string{"-p", "--output-format", "stream-json", "--verbose",
		"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}
	cmd := proc.CommandContext(ctx, c.binPath, args...)
	proc.TreeKill(cmd)
	cmd.Env = cliBaseEnv("ENABLE_TOOL_SEARCH=auto")
	if c.configDir != "" {
		cmd.Env = append(cmd.Env, "CLAUDE_CONFIG_DIR="+c.configDir)
	}
	if c.authToken != "" {
		switch c.authKind {
		case "oauth":
			cmd.Env = append(cmd.Env, "CLAUDE_CODE_OAUTH_TOKEN="+c.authToken)
		case "apikey":
			cmd.Env = append(cmd.Env, "ANTHROPIC_API_KEY="+c.authToken)
		}
	}
	cmd.Stdin = strings.NewReader("ok")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, serr := cmd.StdoutPipe()
	if serr != nil {
		return serr
	}
	if serr := cmd.Start(); serr != nil {
		return serr
	}
	p := newCLIParser(c.model, nil)
	rd := bufio.NewReader(stdout)
	for {
		line, rerr := rd.ReadString('\n')
		if line != "" {
			p.feed(line)
		}
		if rerr != nil {
			break
		}
	}
	runErr := cmd.Wait()
	if p.notLoggedIn {
		msg := strings.TrimSpace(p.authMsg)
		if msg == "" {
			msg = "authentication_failed"
		}
		home := c.configDir
		if home == "" {
			home = "the CLI's default config dir (~/.claude)"
		}
		return fmt.Errorf("claude CLI not logged in (%s): run `claude /login` with CLAUDE_CONFIG_DIR=%s, or switch this agent to an API-key provider", msg, home)
	}
	if runErr != nil {
		return fmt.Errorf("claude CLI auth probe failed: %v %s", runErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// runAttempt runs the claude CLI subprocess once and parses its stream. It
// returns the response on success (including a salvaged partial), or an error.
// retryable is true only for a "clean crash": the process exited non-zero, parsed
// no final result, salvaged no content, AND executed no tool — so re-running has
// no duplicate side effects.
// cliBaseEnv returns the parent environment with the variables that make a
// nested claude-cli misbehave stripped out. When TionSwarm is itself launched from
// inside another Claude Code (agent SDK / CLI), the parent exports CLAUDECODE=1,
// CLAUDE_CODE_* and ANTHROPIC_DEFAULT_*_MODEL. Inheriting these makes the child
// claude believe it is a nested sub-agent and silently fall back to the small/fast
// (haiku) model, IGNORING --model opus. Strip them so the child always runs as a
// clean top-level CLI. TionSwarm re-adds what it actually needs (CLAUDE_CONFIG_DIR,
// auth) after this. CLAUDE_CODE_GIT_BASH_PATH is preserved — the CLI needs it to
// locate bash on Windows.
func cliBaseEnv(extra ...string) []string {
	src := os.Environ()
	out := make([]string, 0, len(src)+len(extra))
	for _, kv := range src {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if k == "CLAUDE_CODE_GIT_BASH_PATH" {
			out = append(out, kv) // keep: needed to find bash on Windows
			continue
		}
		switch {
		case k == "CLAUDECODE",
			k == "CLAUDE_EFFORT",
			k == "ANTHROPIC_MODEL",
			k == "ANTHROPIC_SMALL_FAST_MODEL",
			strings.HasPrefix(k, "CLAUDE_CODE_"),
			strings.HasPrefix(k, "CLAUDE_AGENT_SDK"),
			strings.HasPrefix(k, "ANTHROPIC_DEFAULT_"):
			continue // strip nesting / model-override leakage
		}
		out = append(out, kv)
	}
	// Harden the CLI's own subprocess environment the same way the native shell
	// tools are: a claude-cli agent running `git commit` must not hang on a GUI
	// editor (core.editor=notepad) or a credential prompt inside the stdin-less
	// child. Guards win over inherited values (appended last).
	return proc.HardenedEnv(append(out, extra...))
}

// ensureEnvDefault appends KEY=val to env only when KEY is not already present,
// so an inherited/user-set value always wins over the default.
func ensureEnvDefault(env []string, key, val string) []string {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return env
		}
	}
	return append(env, prefix+val)
}

// cliStartupTimeout bounds the time-to-first-output for a claude-cli turn. A
// subprocess that emits nothing within this window is treated as a hung MCP
// startup and killed (retryable). It guards ONLY startup — once the first line
// arrives the turn is demonstrably alive and later silence is a legitimately
// long tool call, bounded by the CLI's own MCP_TOOL_TIMEOUT and the caller ctx.
const cliStartupTimeout = 90 * time.Second

func (c *ClaudeCLI) runAttempt(ctx context.Context, args []string, prompt, model string, req Request) (resp *Response, retryable bool, err error) {
	cmd := proc.CommandContext(ctx, c.binPath, args...)
	// The CLI spawns its own children (MCP servers, and whatever a Bash tool call
	// shells out to — a build daemon outlives the build that started it). They
	// inherit this command's stdout pipe, so killing the CLI alone leaves the pipe
	// open and cmd.Wait below never returns. TreeKill reaps the whole tree on
	// cancellation and caps Wait with a WaitDelay backstop.
	proc.TreeKill(cmd)
	// Enable the CLI's threshold-based MCP tool search (claude-cli 2.1.x+): tool
	// schemas that fit within 10% of the context window are inlined and only the
	// overflow is deferred. Combined with the core interaction server's alwaysLoad
	// flag, the eager tier (Bash, ask_user, ...) is always present while the extended
	// self-management surface is lazily discovered via ToolSearch. Inherit the parent
	// environment and append the flag (cmd.Env nil would otherwise drop it).
	cmd.Env = cliBaseEnv("ENABLE_TOOL_SEARCH=auto")
	// Long sync run_subagent calls (a delegated agent reads files, runs tests, ...)
	// can exceed the CLI's default 60s MCP tool-call timeout, surfacing to the caller
	// as "The operation timed out." Give MCP tool calls and server startup generous
	// headroom so sync delegation completes in-band. Only fill defaults the parent
	// env didn't already provide, so a user override still wins. Values are ms.
	cmd.Env = ensureEnvDefault(cmd.Env, "MCP_TOOL_TIMEOUT", "600000")
	cmd.Env = ensureEnvDefault(cmd.Env, "MCP_TIMEOUT", "60000")
	// Thinking parity: "Kapalı" turns extended thinking fully off in the CLI
	// (MAX_THINKING_TOKENS=0). Also restores parallel tool batching on claude-code
	// ≥2.1.203 ("think XOR batch") — see Request.DisableThinking.
	if req.DisableThinking {
		cmd.Env = append(cmd.Env, "MAX_THINKING_TOKENS=0")
	}
	// Max effort parity: Claude Code's settings.json effortLevel enum rejects "max"
	// (it silently downgrades to high the moment the session touches /effort or
	// /model), so the --settings file can only carry up to xhigh. The env var is
	// the only channel the CLI honours for max reasoning — see Request.CLIEffortLevel.
	if strings.EqualFold(req.CLIEffortLevel, "max") {
		cmd.Env = append(cmd.Env, "CLAUDE_CODE_EFFORT_LEVEL=max")
	}
	// Isolated config home: point the CLI at a clean CLAUDE_CONFIG_DIR so its
	// skills/settings/commands/global CLAUDE.md/login come from there instead of the
	// shared ~/.claude. Appended last so it overrides any inherited value.
	if c.configDir != "" {
		cmd.Env = append(cmd.Env, "CLAUDE_CONFIG_DIR="+c.configDir)
	}
	// Inject the configured credential so an isolated config dir authenticates
	// without an interactive in-dir login. Per the CLI's auth precedence,
	// ANTHROPIC_API_KEY outranks the OAuth token, so we set exactly one. Appended
	// last → overrides any inherited value.
	if c.authToken != "" {
		switch c.authKind {
		case "oauth":
			cmd.Env = append(cmd.Env, "CLAUDE_CODE_OAUTH_TOKEN="+c.authToken)
		case "apikey":
			cmd.Env = append(cmd.Env, "ANTHROPIC_API_KEY="+c.authToken)
		}
	}
	// Run inside the workspace sandbox so relative paths (e.g. an attachment's
	// "uploads/<sid>/<file>") resolve there rather than the backend's launch
	// directory. Only set when the dir exists; otherwise inherit the default cwd.
	if req.WorkDir != "" {
		if fi, statErr := os.Stat(req.WorkDir); statErr == nil && fi.IsDir() {
			cmd.Dir = req.WorkDir
		}
	}
	cmd.Stdin = strings.NewReader(prompt) // pass prompt via stdin to avoid arg limits
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, serr := cmd.StdoutPipe()
	if serr != nil {
		return nil, false, serr
	}
	// Admit only ONE launch while an OAuth refresh is due for this claude-home.
	// Refresh tokens are single-use, so concurrent processes that all find the token
	// expired race for it: the losers get invalid_grant and the CLI CLEARS the shared
	// credential file, breaking every later turn in the workspace (observed live with
	// a 3-level coordinator tree). A no-op while the token is comfortably valid —
	// the normal case — so parallel fan-out is unaffected. Immediately before Start,
	// so the gate covers the launch it admits rather than the whole turn.
	claudeauth.SerializeRefresh(c.configDir)
	if serr := cmd.Start(); serr != nil {
		return nil, false, serr
	}

	// Parse events as they stream so OnEvent fires step-by-step. ReadString
	// handles arbitrarily long lines (tool results / the init tool list).
	p := newCLIParser(model, req.OnEvent)
	rd := bufio.NewReader(stdout)
	var tail []string // bounded ring of recent raw stdout lines (crash diagnostics)
	const tailMax = 12
	// fullOut tees the COMPLETE stdout (bounded) so a failure whose fatal cause
	// scrolled out of the 12-line tail (e.g. an early exit after SessionStart
	// hooks, where the tail is all hook noise) is still fully recoverable from the
	// dumped log file. Capped so a huge successful stream can't balloon memory.
	var fullOut bytes.Buffer
	const fullOutCap = 1 << 20 // 1 MiB
	// Startup watchdog: a claude-cli subprocess that connects to the MCP bridge can
	// deadlock during MCP `initialize`, then produce NO stdout and never exit — the
	// blocking read below would hang for the whole (often deadline-less) turn, so no
	// llm_call, no error, and the chat's typing indicator never clears (observed: a
	// spawned board-automation turn stuck 7+ min with zero output). Guard only the
	// TIME-TO-FIRST-OUTPUT via a reader goroutine + timer; once the first line
	// arrives the turn is alive and later silence is a legitimately long tool call.
	type readItem struct {
		line string
		err  error
	}
	lines := make(chan readItem, 1)
	readerDone := make(chan struct{})
	defer close(readerDone)
	go func() {
		for {
			line, rerr := rd.ReadString('\n')
			select {
			case lines <- readItem{line, rerr}:
			case <-readerDone:
				return
			}
			if rerr != nil {
				return
			}
		}
	}()

	startup := time.NewTimer(cliStartupTimeout)
	defer startup.Stop()
	sawOutput := false
	startupHang := false
readLoop:
	for {
		select {
		case it := <-lines:
			if it.line != "" {
				if !sawOutput {
					sawOutput = true
					startup.Stop() // first output → turn is alive; drop the startup guard
				}
				p.feed(it.line)
				if fullOut.Len() < fullOutCap {
					fullOut.WriteString(it.line)
				}
				if s := strings.TrimSpace(it.line); s != "" {
					tail = append(tail, s)
					if len(tail) > tailMax {
						tail = tail[len(tail)-tailMax:]
					}
				}
			}
			if it.err != nil {
				break readLoop
			}
		case <-startup.C:
			// No stdout at all within the startup budget → almost certainly an MCP
			// startup hang. Kill the subprocess so the read unblocks; cmd.Wait then
			// returns and we surface a clear, retryable failure below.
			startupHang = true
			proc.KillTree(cmd)
			break readLoop
		case <-ctx.Done():
			// Cancellation (idle watchdog, hard cap, human stop) must end the read
			// loop on its own rather than waiting for the pipe to close: a surviving
			// grandchild can keep it open indefinitely. TreeKill's cancel hook is
			// already reaping the tree; leave the loop and let Wait's WaitDelay bound
			// the rest.
			break readLoop
		}
	}
	runErr := cmd.Wait()

	out, parseErr := p.finish()
	if parseErr == nil {
		return out, false, nil
	}
	if runErr == nil {
		return nil, false, parseErr
	}
	// Resilience: the CLI can exit non-zero AFTER producing a usable answer — e.g.
	// an is_error tool result (an interactive tool reached on an autonomous turn)
	// derails an otherwise-complete turn so no final "result" event arrives. Rather
	// than fail the whole turn / flow node, salvage the assistant content captured
	// before the crash. Genuine error results (hadError) are NOT salvaged.
	if partial := p.salvage(); partial != nil {
		return partial, false, nil
	}
	// Startup watchdog fired: the subprocess emitted nothing within cliStartupTimeout
	// and was killed. There is no salvageable content — surface a clear, retryable
	// failure so self-healing retries once and the turn fails fast instead of hanging.
	if startupHang {
		return nil, true, fmt.Errorf(
			"claude CLI produced no output within %s and was killed as a likely MCP startup hang (retryable) — check the interaction MCP bridge / concurrent-spawn load (exit: %v)",
			cliStartupTimeout, runErr)
	}
	// The CLI often writes its error to stdout (a non-JSON line) and leaves stderr
	// empty — surface whatever it printed so the failure is not a bare "exit status
	// 1" with no diagnostic, plus a full-log dump for the lines the tail missed.
	detail := strings.TrimSpace(stderr.String())
	if detail == "" {
		detail = stdoutCrashTail(tail)
	}
	if logPath := dumpCLIFailure(c.binPath, args, req.WorkDir, runErr, fullOut.Bytes(), stderr.Bytes()); logPath != "" {
		detail = strings.TrimSpace(detail + " | full-log: " + logPath)
	}
	// Authentication failure: this claude-home has no valid login (never ran
	// /login, or the token/key expired or was revoked). The request was refused
	// before any answer and a retry hits the same wall in milliseconds — classify
	// it clearly, mark it NON-retryable, and point at the exact config dir to fix.
	if p.notLoggedIn {
		msg := strings.TrimSpace(p.authMsg)
		if msg == "" {
			msg = "authentication_failed"
		}
		home := c.configDir
		if home == "" {
			home = "the CLI's default config dir (~/.claude)"
		}
		return nil, false, fmt.Errorf(
			"claude CLI authentication failed (%s): this workspace's claude-home is not logged in — run `claude /login` with CLAUDE_CONFIG_DIR=%s, or switch this agent to an API-key provider (anthropic/openrouter) (exit: %v)",
			msg, home, runErr)
	}
	// Usage / rate-limit rejection: the subscription window is exhausted (overage
	// disabled), so the request was refused before any answer. Retrying immediately
	// only burns the next attempt against the same wall — classify it clearly and
	// mark it NON-retryable so the flow fails fast with an actionable reason.
	if p.rateLimited {
		msg := strings.TrimSpace(p.rateLimitMsg)
		if msg == "" {
			msg = "subscription usage window exhausted"
		}
		return nil, false, fmt.Errorf("claude CLI usage/rate limit reached: %s (exit: %v)", msg, runErr)
	}
	// Stale --resume target: the id names a conversation this config home does not
	// have (typically because the CLI config home moved). The transcript will not
	// reappear, so every retry rejects the same id — mark it NON-retryable and name
	// the id + home, so the fix (drop the stored resume id, start cold) is obvious.
	// The caller's pre-flight check (ClaudeCLI.CanResume) normally prevents this;
	// reaching here means the home changed after that check.
	if req.ResumeSessionID != "" && isMissingConversation(detail) {
		home := c.configDir
		if home == "" {
			home = "the CLI's default config dir (~/.claude)"
		}
		return nil, false, fmt.Errorf(
			"claude CLI cannot resume session %s: no such conversation under %s — the CLI config home no longer holds this transcript; the next turn must start cold (exit: %v)",
			req.ResumeSessionID, home, runErr)
	}
	// Died right after init with zero model output (only system/hook/init events).
	// This is the signature of a usage-limit rejection that emitted no rate_limit
	// event, a login lapse ("Not logged in"), or an MCP startup failure — name those
	// likely causes instead of a bare "exit status 1".
	if !p.sawModelTurn {
		retryable = !p.ranTool()
		return nil, retryable, fmt.Errorf("claude CLI exited after init with no model output (likely usage/rate limit, login, or MCP startup failure): %v %s", runErr, strings.TrimSpace(detail))
	}
	// A clean crash (no content, no executed tool) is safe to retry once.
	retryable = !p.ranTool()
	return nil, retryable, fmt.Errorf("claude CLI failed: %v %s", runErr, strings.TrimSpace(detail))
}

// dumpCLIFailure writes the full claude-cli stdout + stderr plus the invocation
// details to a timestamped log file under the OS temp dir, so a failure whose
// fatal cause scrolled out of the inline 12-line tail (e.g. an early exit right
// after SessionStart hooks) is still fully recoverable for diagnosis. Returns
// the log path, or "" on any error (best-effort — never blocks the failure path).
func dumpCLIFailure(bin string, args []string, workDir string, runErr error, stdout, stderr []byte) string {
	name := fmt.Sprintf("tionswarm-cli-fail-%d-%d.log", time.Now().UnixNano(), os.Getpid())
	path := filepath.Join(os.TempDir(), name)
	var b bytes.Buffer
	fmt.Fprintf(&b, "claude-cli failure\n")
	fmt.Fprintf(&b, "time:    %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "bin:     %s\n", bin)
	fmt.Fprintf(&b, "args:    %v\n", args)
	fmt.Fprintf(&b, "workDir: %s\n", workDir)
	fmt.Fprintf(&b, "exit:    %v\n", runErr)
	b.WriteString("\n===== STDERR =====\n")
	b.Write(stderr)
	b.WriteString("\n===== STDOUT (full, capped 1MiB) =====\n")
	b.Write(stdout)
	b.WriteString("\n")
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		return ""
	}
	return path
}

// stdoutCrashTail builds a short diagnostic string from the last raw stdout
// lines when the CLI exits non-zero with empty stderr. The fatal cause is at
// the END of the stream, so it surfaces the most informative lines — plain
// (non-JSON) panics/errors and stream-json "result"/error events that carry
// is_error and the failure reason — and, when truncating, keeps the tail end
// rather than the (noisy) startup head. Falls back to the raw tail so
// something is always reported.
func stdoutCrashTail(lines []string) string {
	informative := func(l string) bool {
		if l == "" {
			return false
		}
		if l[0] != '{' {
			return true // plain text: panic / error message
		}
		return strings.Contains(l, `"is_error":true`) ||
			strings.Contains(l, `"type":"result"`) ||
			strings.Contains(l, `"subtype":"error`) ||
			strings.Contains(l, `"error"`)
	}
	var picks []string
	for _, l := range lines {
		if informative(l) {
			picks = append(picks, l)
		}
	}
	if len(picks) == 0 {
		picks = lines // fallback: raw tail
	}
	out := strings.TrimSpace(strings.Join(picks, " | "))
	const max = 800
	if len(out) > max {
		out = "…" + out[len(out)-max:] // keep the END (the fatal part)
	}
	if out == "" {
		return "(no stderr and empty stdout — CLI exited without output)"
	}
	return "stdout-tail: " + out
}

// cliStreamParser incrementally consumes the stream-json event log, building a
// Response.Trace and (when onEvent is set) emitting each step the moment it is
// ready: thinking immediately, intermediate text on flush, a tool step once its
// result arrives. The trailing text is the final answer (not emitted as a step).
type cliStreamParser struct {
	resp         *Response
	onEvent      func(TraceStep)
	toolIdx      map[string]int // tool_use id → index in resp.Trace
	emitted      map[int]bool   // trace index → already delivered via onEvent
	pending      strings.Builder
	finalText    string
	sawResult    bool
	hadError     bool
	errText      string
	sawModelTurn bool                 // any assistant/tool/result content seen (vs. only system/init noise)
	rateLimited  bool                 // the turn was rejected by a subscription usage / rate limit
	rateLimitMsg string               // human-readable detail for the rate-limit failure
	notLoggedIn  bool                 // the turn was rejected because this claude-home is not authenticated
	authMsg      string               // human-readable detail for the auth failure ("Not logged in · ...")
	toolStart    map[string]time.Time // tool_use id → time the event was seen (for per-tool latency)
	// Parallel-batch grouping: the CLI splits ONE API assistant message (which may
	// carry several parallel tool_use blocks) into several stream events sharing the
	// same message id. Track the current message's tool trace indices so the 2nd+
	// tool_use of a message allocates a batch id and stamps the earlier ones too.
	curMsgID    string // assistant message id currently being accumulated
	curMsgTools []int  // trace indices of that message's tool steps
	batchSeq    int    // 1-based batch id allocator (unique within the turn)
}

// primaryModelUsage returns the model key that consumed the most tokens in a
// result envelope's modelUsage map, chosen deterministically (alphabetical
// tie-break). A turn may invoke several models — e.g. an auxiliary tool-search
// haiku call next to the primary answer — so the map has multiple keys; the one
// that processed the full context (largest total tokens) is the answer's model.
// Output tokens alone are misleading (the tiny auxiliary call can emit more), so
// score by input + output + cache.
func primaryModelUsage(mu map[string]json.RawMessage) string {
	best := ""
	bestScore := -1
	for k, raw := range mu {
		var u struct {
			InputTokens              int `json:"inputTokens"`
			OutputTokens             int `json:"outputTokens"`
			CacheReadInputTokens     int `json:"cacheReadInputTokens"`
			CacheCreationInputTokens int `json:"cacheCreationInputTokens"`
		}
		_ = json.Unmarshal(raw, &u)
		score := u.InputTokens + u.OutputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
		if score > bestScore || (score == bestScore && (best == "" || k < best)) {
			best, bestScore = k, score
		}
	}
	return best
}

func newCLIParser(model string, onEvent func(TraceStep)) *cliStreamParser {
	return &cliStreamParser{
		resp:      &Response{Model: model},
		onEvent:   onEvent,
		toolIdx:   map[string]int{},
		emitted:   map[int]bool{},
		toolStart: map[string]time.Time{},
	}
}

func (p *cliStreamParser) emit(i int) {
	if p.onEvent == nil || p.emitted[i] || i < 0 || i >= len(p.resp.Trace) {
		return
	}
	p.emitted[i] = true
	p.onEvent(p.resp.Trace[i])
}

func (p *cliStreamParser) flushText() {
	t := strings.TrimSpace(p.pending.String())
	p.pending.Reset()
	if t != "" {
		p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "text", Text: t})
		p.emit(len(p.resp.Trace) - 1)
	}
}

// feed processes one event line from the stream.
func (p *cliStreamParser) feed(line string) {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return
	}
	var ev cliEvent
	if json.Unmarshal([]byte(line), &ev) != nil {
		return
	}
	// Capture the CLI session id wherever it appears (system/init first, result
	// last). The result event's id is the one to resume from next turn, so letting
	// later events overwrite is correct.
	if ev.SessionID != "" {
		p.resp.SessionID = ev.SessionID
	}
	// A login lapse surfaces first as a standalone {"error":"authentication_failed"}
	// line (before the result envelope). Catch it here so the failure is classified
	// as auth regardless of which event carried the signal.
	if isAuthErrorText(ev.Error) {
		p.notLoggedIn = true
		if p.authMsg == "" {
			p.authMsg = strings.TrimSpace(ev.Error)
		}
	}

	switch ev.Type {
	case "rate_limit_event":
		// The CLI reports the subscription rate-limit window on every turn. The
		// "allowed" family lets the request proceed: "allowed" is the normal case and
		// "allowed_warning" only signals the window is filling up (observed event:
		// status=allowed_warning, utilization 0.64, isUsingOverage=false — 36% of quota
		// still free). Only a status OUTSIDE that family (e.g. "rejected"/"blocked")
		// means the window is exhausted and, with overage disabled, the request is
		// refused before any assistant output. Matching just "allowed" mis-flagged the
		// warning as a hard limit, reported a bogus "usage/rate limit reached" and
		// masked the real exit cause (letting the !sawModelTurn branch classify it,
		// which is also retryable when no tool ran).
		if rl := ev.RateLimit; rl != nil && rl.Status != "" && !strings.HasPrefix(strings.ToLower(rl.Status), "allowed") {
			p.rateLimited = true
			p.rateLimitMsg = describeRateLimit(rl)
		}
	case "assistant":
		p.sawModelTurn = true
		if ev.Message == nil {
			return
		}
		// New API message → reset the parallel-batch accumulator (see curMsgID).
		// An empty id (older CLI) degrades to per-event grouping, which is still
		// correct when one event carries all of a message's blocks.
		if ev.Message.ID != p.curMsgID {
			p.curMsgID = ev.Message.ID
			p.curMsgTools = p.curMsgTools[:0]
		}
		if ev.Message.Model != "" {
			p.resp.Model = ev.Message.Model
		}
		if ev.Message.Usage != nil {
			p.resp.Usage.OutputTokens += ev.Message.Usage.OutputTokens
			if ev.Message.Usage.InputTokens > p.resp.Usage.InputTokens {
				p.resp.Usage.InputTokens = ev.Message.Usage.InputTokens
			}
			if v := ev.Message.Usage.CacheReadInputTokens; v > p.resp.Usage.CacheReadTokens {
				p.resp.Usage.CacheReadTokens = v
			}
			if v := ev.Message.Usage.CacheCreationInputTokens; v > p.resp.Usage.CacheWriteTokens {
				p.resp.Usage.CacheWriteTokens = v
			}
		}
		for _, b := range ev.Message.Content {
			switch b.Type {
			case "text":
				p.pending.WriteString(b.Text)
			case "thinking":
				p.flushText()
				if t := strings.TrimSpace(b.Thinking); t != "" {
					p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "thinking", Text: t})
					p.emit(len(p.resp.Trace) - 1)
				}
			case "tool_use":
				p.flushText()
				p.resp.Trace = append(p.resp.Trace, TraceStep{Kind: "tool", Tool: b.Name, Input: b.Input})
				idx := len(p.resp.Trace) - 1
				if b.ID != "" {
					p.toolIdx[b.ID] = idx
					p.toolStart[b.ID] = time.Now() // start the latency clock for this tool
				}
				// Parallel-batch grouping: the 2nd tool_use of the SAME API message
				// allocates a batch id and stamps every tool of that message (incl.
				// retroactively the 1st, whose result has not arrived yet — tool steps
				// emit only on tool_result, so the stamp lands before delivery).
				p.curMsgTools = append(p.curMsgTools, idx)
				if len(p.curMsgTools) == 2 {
					p.batchSeq++
				}
				if len(p.curMsgTools) >= 2 {
					for _, ti := range p.curMsgTools {
						p.resp.Trace[ti].Batch = p.batchSeq
					}
				}
				// Not emitted yet — wait for its tool_result to fill the output.
			}
		}
	case "user":
		if ev.Message == nil {
			return
		}
		for _, b := range ev.Message.Content {
			if b.Type != "tool_result" {
				continue
			}
			if i, ok := p.toolIdx[b.ToolUseID]; ok {
				p.resp.Trace[i].Output = toolResultText(b.Content)
				p.resp.Trace[i].IsError = b.IsError
				if start, ok := p.toolStart[b.ToolUseID]; ok {
					p.resp.Trace[i].DurMs = time.Since(start).Milliseconds()
					delete(p.toolStart, b.ToolUseID)
				}
				p.emit(i)
			}
		}
	case "result":
		p.sawResult = true
		p.sawModelTurn = true
		if ev.IsError {
			p.hadError = true
			p.errText = ev.Result
			// A usage/rate-limit rejection often surfaces here as the result error
			// (api_error_status == "rate_limit" or wording in the result text) rather
			// than a separate rate_limit_event — classify it either way.
			if isRateLimitText(ev.APIErrorStatus) || isRateLimitText(ev.Result) {
				p.rateLimited = true
				if p.rateLimitMsg == "" {
					p.rateLimitMsg = strings.TrimSpace(ev.APIErrorStatus + " " + ev.Result)
				}
			}
			// A login lapse commonly surfaces here as result "Not logged in · Please
			// run /login". Classify it so the caller fails fast with an actionable
			// message instead of a bare "exit status 1" that gets retried in vain.
			if isAuthErrorText(ev.APIErrorStatus) || isAuthErrorText(ev.Result) {
				p.notLoggedIn = true
				if p.authMsg == "" {
					p.authMsg = strings.TrimSpace(ev.Result)
				}
			}
			return
		}
		p.finalText = ev.Result
		if ev.Usage != nil {
			if ev.Usage.InputTokens > 0 {
				p.resp.Usage.InputTokens = ev.Usage.InputTokens
			}
			if ev.Usage.OutputTokens > 0 {
				p.resp.Usage.OutputTokens = ev.Usage.OutputTokens
			}
			// The result envelope carries the authoritative aggregate; let it win.
			if ev.Usage.CacheReadInputTokens > 0 {
				p.resp.Usage.CacheReadTokens = ev.Usage.CacheReadInputTokens
			}
			if ev.Usage.CacheCreationInputTokens > 0 {
				p.resp.Usage.CacheWriteTokens = ev.Usage.CacheCreationInputTokens
			}
		}
		// A single turn can touch more than one model: ENABLE_TOOL_SEARCH runs an
		// auxiliary haiku call alongside the primary (opus) answer, so the result
		// envelope's modelUsage holds several keys. Iterating the map and taking
		// "any" key picked one at RANDOM (Go map order is unspecified), which made
		// resp.Model flap between the primary and the auxiliary model turn-to-turn —
		// a phantom "model downgraded to haiku" that never actually happened. Pick
		// the model that did the real work: the one with the most tokens.
		if m := primaryModelUsage(ev.ModelUsage); m != "" {
			p.resp.Model = m
		}
		// num_turns = how many internal model API round-trips the CLI made this turn.
		// The Usage above is the SUM across those round-trips (cache_read especially is
		// cumulative — verified: result cacheRead == Σ per-assistant cacheRead), so the
		// caller divides Usage by ProviderCalls to recover the per-call context size.
		if ev.NumTurns > 0 {
			p.resp.ProviderCalls = ev.NumTurns
		}
	}
}

// finish resolves the final answer and emits any tool steps whose result never
// arrived (so the UI still sees them).
func (p *cliStreamParser) finish() (*Response, error) {
	if p.hadError {
		return nil, fmt.Errorf("claude CLI error: %s", p.errText)
	}
	if !p.sawResult {
		return nil, fmt.Errorf("claude CLI: no result in stream")
	}
	for i := range p.resp.Trace {
		if p.resp.Trace[i].Kind == "tool" {
			p.emit(i)
		}
	}
	if p.finalText == "" {
		p.finalText = strings.TrimSpace(p.pending.String())
	}
	// Output guard: if the CLI echoed a harness repair reminder instead of an
	// answer (a malformed replayed turn can trigger this), don't surface it as
	// the assistant's reply. Strip the artifact; if nothing genuine remains,
	// fail the turn so the caller can retry rather than persist the reminder.
	if isRepairArtifact(p.finalText) {
		if cleaned := sanitizeTranscriptText(p.finalText); cleaned != "" {
			p.finalText = cleaned
		} else {
			return nil, fmt.Errorf("claude CLI returned only a repair reminder, not an answer")
		}
	}
	p.resp.Text = p.finalText
	return p.resp, nil
}

// ranTool reports whether any tool was invoked during the turn — used to decide
// if a crashed turn is safe to retry (a tool may have side effects, so a turn
// that reached one is NOT retried).
func (p *cliStreamParser) ranTool() bool {
	for i := range p.resp.Trace {
		if p.resp.Trace[i].Kind == "tool" {
			return true
		}
	}
	return false
}

// describeRateLimit renders a compact, human-readable summary of a rate-limit
// window for the failure message (type, status, overage state, reset time).
func describeRateLimit(rl *cliRateLimit) string {
	parts := []string{}
	if rl.RateLimitType != "" {
		parts = append(parts, rl.RateLimitType+" window")
	}
	if rl.Status != "" {
		parts = append(parts, "status="+rl.Status)
	}
	if rl.OverageStatus != "" {
		parts = append(parts, "overage="+rl.OverageStatus)
	}
	if rl.OverageDisabledReason != "" {
		parts = append(parts, rl.OverageDisabledReason)
	}
	if rl.ResetsAt > 0 {
		parts = append(parts, "resets "+time.Unix(rl.ResetsAt, 0).Format("2006-01-02 15:04"))
	}
	if len(parts) == 0 {
		return "rate limit reached"
	}
	return strings.Join(parts, ", ")
}

// isRateLimitText reports whether a result/api-error string signals a usage or
// rate-limit rejection (used to classify a result-error envelope).
func isRateLimitText(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "rate_limit") ||
		strings.Contains(s, "rate limit") ||
		strings.Contains(s, "usage limit") ||
		strings.Contains(s, "usage_limit") ||
		strings.Contains(s, "quota")
}

// isAuthErrorText reports whether a result/api-error/error string signals an
// authentication failure — this claude-home has no valid login (never ran
// /login, or the OAuth token / API key expired or was revoked). Such failures
// are NOT retryable: a second attempt hits the same wall in milliseconds.
func isAuthErrorText(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "authentication_failed") ||
		strings.Contains(s, "not logged in") ||
		strings.Contains(s, "please run /login") ||
		strings.Contains(s, "invalid api key") ||
		strings.Contains(s, "invalid x-api-key") ||
		strings.Contains(s, "oauth token has expired") ||
		strings.Contains(s, "oauth authentication is currently not supported") ||
		strings.Contains(s, "invalid bearer token")
}

// salvage recovers whatever assistant content the parser accumulated when the
// stream was cut off before a final "result" event (the CLI crashed/exited at the
// end of the turn). It lets a long research turn that died on a trailing fault
// still return its work instead of failing the whole turn / flow node. Returns
// nil when there is nothing usable, or when the stream carried a genuine error
// result (hadError) — those must propagate, not be masked as success.
func (p *cliStreamParser) salvage() *Response {
	if p.hadError {
		return nil
	}
	text := strings.TrimSpace(p.finalText)
	if text == "" {
		text = strings.TrimSpace(p.pending.String())
	}
	if text == "" {
		// Fall back to the last non-empty assistant text step in the trace.
		for i := len(p.resp.Trace) - 1; i >= 0; i-- {
			if p.resp.Trace[i].Kind == "text" && strings.TrimSpace(p.resp.Trace[i].Text) != "" {
				text = strings.TrimSpace(p.resp.Trace[i].Text)
				break
			}
		}
	}
	if text == "" {
		return nil
	}
	p.resp.Text = text
	return p.resp
}

// toolResultText extracts displayable text from a tool_result content field,
// which the CLI encodes either as a JSON string or an array of content blocks.
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []cliBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var b strings.Builder
		for _, blk := range blocks {
			if blk.Text != "" {
				b.WriteString(blk.Text)
			}
		}
		return b.String()
	}
	return string(raw)
}

// serializeTranscript turns a multi-turn history into a single prompt.
// For a single user turn it returns the text directly; otherwise it builds
// a labelled transcript so the CLI has prior context.
func serializeTranscript(msgs []Message) string {
	// Filter to user/assistant turns.
	turns := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == RoleUser || m.Role == RoleAssistant {
			turns = append(turns, m)
		}
	}
	if len(turns) == 0 {
		return ""
	}
	if len(turns) == 1 {
		return turns[0].Text
	}

	var b strings.Builder
	b.WriteString("Continue this conversation. Reply only as the assistant to the final user message.\n\n")
	for _, m := range turns {
		label := "User"
		text := m.Text
		if m.Role == RoleAssistant {
			label = "Assistant"
			// Strip any leaked tool-call / harness markup from prior assistant
			// turns so the CLI never sees a malformed message and injects its own
			// repair <system-reminder> (which the model would then echo back).
			text = sanitizeTranscriptText(text)
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(text)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}
