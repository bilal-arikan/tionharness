package providers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// CodexCLI drives the locally-installed `codex` CLI in non-interactive exec
// mode (`codex exec --json`). Like ClaudeCLI it is a CLI TRANSPORT: the CLI runs
// its own agentic loop and TionHarness's tools reach it over the MCP bridge, so
// the native tool loop is not involved. Authentication is keyless — the user's
// existing ChatGPT/Codex login inside CODEX_HOME.
type CodexCLI struct {
	binPath string
	model   string // optional model slug override, e.g. "gpt-5.4-mini"
	// configDir is exported as CODEX_HOME into the subprocess: the CLI's config
	// home (auth.json, config.toml, thread state). It must be an EXISTING
	// directory or codex refuses to start. Empty → inherit the ambient ~/.codex.
	configDir string

	// MCP delegation: one turn's server set, rendered into
	// <configDir>/config.toml before launch. Set by ConfigureCLIMCP.
	mcpServers map[string]CLIMCPServer

	// mcpProbe overrides the remote-MCP reachability probe. Nil means the real
	// network probe; tests set it to stay offline.
	mcpProbe codexMCPProbe
}

// NewCodexCLI creates a provider that invokes the given codex binary. configDir,
// when non-empty, isolates the CLI's config home via CODEX_HOME (pass "" to
// inherit the user's ~/.codex). Unlike the claude transport there is no
// credential injection: codex reads its login from <CODEX_HOME>/auth.json only.
func NewCodexCLI(binPath, model, configDir string) *CodexCLI {
	return &CodexCLI{binPath: binPath, model: model, configDir: configDir}
}

// Name implements Provider.
func (c *CodexCLI) Name() string { return "codex-cli" }

// Installed reports whether the configured codex binary resolves to an
// executable — an absolute/relative path that exists, or a bare name found on
// PATH. It does NOT check login state.
func (c *CodexCLI) Installed() bool {
	if c.binPath == "" {
		return false
	}
	_, err := exec.LookPath(c.binPath)
	return err == nil
}

func (c *CodexCLI) Preflight(ctx context.Context) error {
	env := codexBaseEnv()
	if c.configDir != "" {
		env = append(env, "CODEX_HOME="+c.configDir)
	}
	return runCLIPreflight(ctx, c.Name(), c.binPath, c.configDir, c.model, env, "config.toml")
}

// SetConfigDir overrides the CODEX_HOME this provider exports into its
// subprocess. TionHarness calls this per turn so each workspace drives the CLI
// against its OWN config home (<workspace>/codex-home). Empty is ignored so the
// value baked in at construction survives when no workspace is derivable.
func (c *CodexCLI) SetConfigDir(dir string) {
	if dir == "" {
		return
	}
	c.configDir = dir
}

// ConfigDir returns the CODEX_HOME this provider currently exports — either
// the value baked in at construction (the instance's own configDir field,
// K1) or one set later via SetConfigDir. Callers use this to tell whether
// the instance already owns a dedicated config home before falling back to
// a workspace-derived one.
func (c *CodexCLI) ConfigDir() string { return c.configDir }

// ConfigureCLIMCP implements CLIProvider. Only spec.Servers is consumed: codex
// has no --mcp-config equivalent, so the servers are rendered into the
// CODEX_HOME's config.toml at launch. The claude-only knobs (AllowedTools,
// DisallowedTools, PermissionPrompt, SettingsPath, ConfigPath) have no codex
// counterpart and are deliberately dropped — per-tool gating on this transport
// is expressed by which servers are configured at all.
func (c *CodexCLI) ConfigureCLIMCP(spec CLIMCPSpec) {
	c.mcpServers = spec.Servers
}

// codexSandboxArgs maps TionHarness's permission mode onto codex's sandbox flags.
//
// WARNING — "ask" is NOT per-tool approval here. `codex exec` REJECTS every
// approval request ("approval is not supported in exec mode"), so the closest
// honest mapping is "writes confined to the workspace". The UI must not present
// it as real approval.
func codexSandboxArgs(mode string) []string {
	switch mode {
	case "read-only":
		return []string{"-s", "read-only"}
	case "ask":
		return []string{"-s", "workspace-write"}
	default: // "auto", "" and any unknown value
		return []string{"--dangerously-bypass-approvals-and-sandbox"}
	}
}

// buildArgs assembles the codex exec invocation, kept pure for testing.
//
// FLAG ORDER IS LOAD-BEARING: `-s`, `-c`, `--json` and friends are options of
// the `exec` subcommand and MUST precede the `resume` SUB-subcommand. Putting
// them after it fails with "unexpected argument '-s' found". Only --model and
// the two --dangerously-* flags are true globals; they are emitted in the same
// leading block anyway so a single rule ("everything before resume") holds.
//
// The prompt is NOT an argument — it goes on stdin, because Windows caps a
// process command line at ~32 KB and a full turn prompt overflows that.
func (c *CodexCLI) buildArgs(req Request, model string) []string {
	args := []string{"exec", "--json", "--ephemeral",
		// The workspace sandbox is not necessarily a git repo; without this codex
		// refuses to run outside one.
		"--skip-git-repo-check",
		// DO NOT add --ignore-user-config: isolation is already achieved via
		// CODEX_HOME (the subprocess reads its own home, not the invoking user's
		// ~/.codex). --ignore-user-config skips the "user" config LAYER, and that
		// layer is CODEX_HOME/config.toml itself — the exact file writeCodexConfig
		// renders MCP servers and developer_instructions into (codex-rs
		// config/src/loader/mod.rs, load_user_instance around line 516: `if
		// ignore_user_config { return None }` before that file is even read). With
		// the flag set the CLI never sees our MCP servers, so every tool call fails
		// with "not in tool registry" — verified live A/B: same config.toml, same
		// prompt; flag present → tool lookup fails, flag absent → mcp_tool_call
		// completes.
		//
		// Fail loudly on an unknown config key instead of silently dropping an
		// override we believed we had applied. Confirmed independent of
		// ignore_user_config in the same loader (strict_config only affects unknown
		// field handling, not which layers are read), and the config we render is
		// schema-valid, so this does not reject it.
		"--strict-config",
	}
	if model != "" {
		args = append(args, "-m", model)
	}
	args = append(args, codexSandboxArgs(req.PermissionMode)...)
	if req.WorkDir != "" {
		if fi, err := os.Stat(req.WorkDir); err == nil && fi.IsDir() {
			args = append(args, "-C", req.WorkDir)
		}
	}
	// Resume keeps the CLI's server-side thread (and its prompt cache) warm, so
	// only the new turn needs to be sent. Unlike claude-cli's rotating session id
	// the codex thread id is STABLE, so the caller can store it once.
	if req.ResumeSessionID != "" {
		args = append(args, "resume", req.ResumeSessionID)
	}
	return args
}

// buildConfig assembles the config.toml for this turn. The system prompt has no
// command-line equivalent on codex (there is no --append-system-prompt), so
// both halves ride in developer_instructions; the volatile per-turn context is
// kept in the stdin prompt instead, exactly as on the claude path, so it does
// not churn the cached prefix.
func (c *CodexCLI) buildConfig(req Request) codexConfig {
	return codexConfig{
		DeveloperInstructions: strings.TrimSpace(joinNonEmpty(req.System, interactionSystemNote)),
		ReasoningEffort:       codexReasoningEffort(req),
		// TionHarness supplies its own equivalents through the MCP bridge, and the
		// codex built-ins are invisible in the TionHarness UI: update_plan would
		// shadow todo_write, and experimental_request_user_input would block the
		// turn on a prompt no one can answer in exec mode.
		DisableUpdatePlan:       true,
		DisableRequestUserInput: true,
		// Codex's native multi-agent/collab tools duplicate spawn_worker and are
		// unusable under --ephemeral (no thread store), so a model that reaches
		// for them loses the turn to "collab spawn failed: no thread with id".
		DisableNativeMultiAgent: true,
		Servers:                 c.mcpServers,
	}
}

// codexReasoningEffort maps the request's reasoning knobs onto codex's
// model_reasoning_effort. DisableThinking is the explicit "off" switch and wins;
// otherwise the resolved CLI effort level passes through when codex accepts it.
func codexReasoningEffort(req Request) string {
	if req.DisableThinking {
		return "none"
	}
	switch strings.ToLower(strings.TrimSpace(req.CLIEffortLevel)) {
	case "minimal":
		return "minimal"
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh":
		return "xhigh"
	case "max":
		return "max"
	}
	return "" // let codex use its own default
}

// buildPrompt renders the conversation for stdin. The volatile per-turn context
// (clock, recalled memory, rolling summary) is prepended as a delimited
// [Context] block so it stays out of developer_instructions — see the claude
// path's buildSystemAndPrompt for the caching rationale, which applies here too
// (codex caches the developer/system prefix the same way).
func (c *CodexCLI) buildPrompt(req Request) string {
	return withDynamic(serializeTranscript(req.Messages), joinNonEmpty(req.SystemDynamic, req.Summary))
}

// Complete implements Provider by shelling out to `codex exec --json` and
// parsing its JSONL stream line-by-line. When req.OnEvent is set, each activity
// step is delivered as soon as it is final; the returned Response carries the
// full text + trace regardless.
func (c *CodexCLI) Complete(ctx context.Context, req Request) (*Response, error) {
	if c.binPath == "" {
		return nil, fmt.Errorf("codex CLI: no binary configured")
	}
	model := req.Model
	if model == "" {
		model = c.model
	}
	return c.completeWithArgs(ctx, c.buildArgs(req, model), c.buildPrompt(req), model, req)
}

// completeWithArgs writes the turn's config.toml and runs the codex subprocess,
// retrying once on a clean crash and once more after disabling an MCP server
// that refused to start. args and prompt are parameters rather than derived
// here so tests can drive the whole recovery path with a fake codex binary.
func (c *CodexCLI) completeWithArgs(ctx context.Context, args []string, prompt, model string, req Request) (*Response, error) {
	home, cleanupHome, err := prepareShadowHome(c.configDir)
	if err != nil {
		return nil, fmt.Errorf("codex CLI: prepare shadow CODEX_HOME: %w", err)
	}
	defer cleanupHome()

	// Dropping a server costs the turn that server's tools, so it must not be
	// silent. These notes ride the same channel the parser uses for
	// "[codex error]" lines — text trace steps, streamed live when the caller
	// listens and carried in the response trace either way.
	var mcpNotes []TraceStep
	note := func(text string) {
		step := TraceStep{Kind: "text", Text: text}
		mcpNotes = append(mcpNotes, step)
		if req.OnEvent != nil {
			req.OnEvent(step)
		}
	}

	// The config.toml is the ONLY channel for the system prompt and the MCP
	// servers, so a write failure must fail the turn rather than silently run a
	// tool-less, persona-less agent.
	cfg := c.buildConfig(req)
	writeConfig := func() ([]string, error) {
		_, dropped, cleanup, err := writeCodexConfig(ctx, home, cfg, c.mcpProbe)
		if err != nil {
			return nil, err
		}
		if cleanup != nil {
			defer cleanup()
		}
		return dropped, nil
	}
	if home != "" {
		dropped, err := writeConfig()
		if err != nil {
			return nil, err
		}
		if len(dropped) > 0 {
			note("[codex] unreachable MCP server(s) omitted from this turn: " +
				strings.Join(dropped, ", ") + " — their tools are unavailable until the server is back up.")
		}
	} else if len(c.mcpServers) > 0 {
		return nil, fmt.Errorf("codex CLI: MCP servers configured but no CODEX_HOME set — cannot write config.toml")
	}

	// Mirror the claude path's single retry: a "clean crash" (died before any
	// turn/item event, ran no tool) has no side effects and is safe to re-run.
	// Everything classified — auth, quota, model — is terminal and returns at once.
	// The extra attempt is the MCP fallback below, which only runs once.
	var lastErr error
	mcpFallbackUsed := false
	for attempt := 0; attempt < 3; attempt++ {
		resp, retryable, err := c.runAttempt(ctx, args, prompt, model, req, home)
		if err == nil {
			if len(mcpNotes) > 0 {
				resp.Trace = append(append([]TraceStep{}, mcpNotes...), resp.Trace...)
			}
			return resp, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, err
		}
		// Codex treats every configured MCP server as required: one broken
		// handshake aborts the whole session, even when the turn never needed
		// that server's tools. The preflight probe cannot see this — the port
		// answers, the protocol handshake is what fails — so recover here by
		// dropping the offending server(s) and running the turn once more.
		if !mcpFallbackUsed && home != "" && codexMCPStartupFailure(err.Error()) {
			drop := codexNamedMCPServers(err.Error(), cfg.Servers)
			if len(drop) == 0 {
				drop = codexRemoteServerKeys(cfg.Servers)
			}
			if len(drop) > 0 {
				mcpFallbackUsed = true
				cfg.Servers = codexServersWithout(cfg.Servers, drop)
				if _, werr := writeConfig(); werr != nil {
					return nil, werr
				}
				note("[codex] MCP server(s) failed to start and were disabled for this turn: " +
					strings.Join(drop, ", ") + " — retrying without their tools.")
				continue
			}
		}
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

// ProbeAuth implements AuthProber for the codex-cli transport: the cheapest
// real turn that still exercises the login path, so an expired or revoked
// credential is reported instead of being hidden behind a present-but-dead
// auth.json. The reply is discarded; only the error matters. A 401 is
// classified mid-stream and kills the subprocess early, so this returns in
// seconds rather than waiting out codex's ten internal retries.
func (c *CodexCLI) ProbeAuth(ctx context.Context) error {
	_, err := c.Complete(ctx, Request{
		System:   "You are a connectivity probe. Reply with exactly: OK",
		Messages: []Message{{Role: RoleUser, Text: "ping"}},
	})
	return err
}

// codexStartupTimeout bounds the time-to-first-output for a codex turn. A
// subprocess that emits nothing within this window is treated as a hung MCP
// startup and killed. It guards ONLY startup: once the first line arrives the
// turn is demonstrably alive and later silence is a legitimately long tool call,
// bounded by tool_timeout_sec and the caller's ctx.
const codexStartupTimeout = 90 * time.Second

// codexIdleOutputTimeout generously bounds the gap between output lines. A
// surviving grandchild can hold the stdout pipe open after codex itself exits;
// 15 minutes still permits a legitimately slow, silent 10-minute tool call.
const codexIdleOutputTimeout = 15 * time.Minute

var codexIdleOutputTimeoutDuration = codexIdleOutputTimeout

// runAttempt runs the codex subprocess once and parses its stream. retryable is
// true only when re-running is free of duplicate side effects: the process
// produced no terminal-classified failure, no salvageable content, and ran no
// tool.
func (c *CodexCLI) runAttempt(ctx context.Context, args []string, prompt, model string, req Request, home string) (resp *Response, retryable bool, err error) {
	cmd := proc.CommandContextNested(ctx, c.binPath, args...)
	// codex spawns its own children (MCP servers, and whatever the turn shells
	// out to — a Gradle daemon outlives the build that started it). They inherit
	// this command's stdout pipe, so killing codex alone leaves the pipe open and
	// cmd.Wait below never returns: the turn wedges forever even though codex
	// already finished. TreeKill reaps the whole tree on cancellation and caps
	// Wait with a WaitDelay backstop.
	proc.TreeKill(cmd)
	cmd.Env = codexBaseEnv()
	if home != "" {
		// Appended last so it overrides any inherited CODEX_HOME. Codex errors out
		// when this points at a missing directory, so surface that here rather than
		// as an opaque subprocess exit.
		if fi, statErr := os.Stat(home); statErr != nil || !fi.IsDir() {
			return nil, false, fmt.Errorf("codex CLI: CODEX_HOME %q is not an existing directory: %v", home, statErr)
		}
		cmd.Env = append(cmd.Env, "CODEX_HOME="+home)
	}
	// -C already tells codex which directory to work in; setting the process cwd
	// too keeps relative paths in attachments resolving inside the sandbox.
	if req.WorkDir != "" {
		if fi, statErr := os.Stat(req.WorkDir); statErr == nil && fi.IsDir() {
			cmd.Dir = req.WorkDir
		}
	}
	// prompt via stdin — see buildArgs. codex rejects the whole turn with
	// "input is not valid UTF-8 (invalid byte at offset N)" if a single byte is
	// malformed, which a rune-splitting truncation upstream can produce; replace
	// those bytes rather than lose the turn.
	cmd.Stdin = strings.NewReader(strings.ToValidUTF8(prompt, "�"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, serr := cmd.StdoutPipe()
	if serr != nil {
		return nil, false, serr
	}
	if serr := cmd.Start(); serr != nil {
		return nil, false, serr
	}

	p := newCodexParser(model, req.OnEvent)
	rd := bufio.NewReader(stdout)
	var tail []string
	const tailMax = 12

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

	startup := time.NewTimer(codexStartupTimeout)
	defer startup.Stop()
	idle := time.NewTimer(codexIdleOutputTimeoutDuration)
	if !idle.Stop() {
		<-idle.C
	}
	defer idle.Stop()
	sawOutput := false
	startupHang := false
	idleHang := false
	// killedEarly records that we tore the process down ourselves on a terminal
	// error, so the resulting non-zero exit is expected rather than diagnostic.
	killedEarly := false
	failClass := codexFailureNone
readLoop:
	for {
		select {
		case it := <-lines:
			if it.line != "" {
				if !sawOutput {
					sawOutput = true
					startup.Stop()
				}
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(codexIdleOutputTimeoutDuration)
				p.feed(it.line)
				if s := strings.TrimSpace(it.line); s != "" {
					tail = append(tail, s)
					if len(tail) > tailMax {
						tail = tail[len(tail)-tailMax:]
					}
				}
				// Early kill on a terminal failure. On a 401 codex retries
				// internally 5× over websocket then 5× over https — ~35 s of dead
				// stream — before exiting. The first error event already tells us the
				// turn cannot succeed, so stop paying for the rest of it.
				if p.hadError && failClass == codexFailureNone {
					if cls := classifyCodexError(p.errText); cls != codexFailureNone {
						failClass = cls
						killedEarly = true
						proc.KillTree(cmd)
						break readLoop
					}
				}
			}
			if it.err != nil {
				break readLoop
			}
		case <-startup.C:
			startupHang = true
			proc.KillTree(cmd)
			break readLoop
		case <-idle.C:
			idleHang = true
			proc.KillTree(cmd)
			break readLoop
		case <-ctx.Done():
			// Cancellation (idle watchdog, hard cap, human stop) must end the read
			// loop on its own rather than waiting for the pipe to close: a surviving
			// grandchild can keep it open indefinitely. TreeKill's cancel hook is
			// already reaping the tree; leave the loop and let Wait's WaitDelay bound
			// the rest.
			killedEarly = true
			break readLoop
		}
	}
	runErr := cmd.Wait()

	// A terminal failure detected mid-stream short-circuits everything below: the
	// classification, not the exit code, is the real diagnosis.
	if failClass != codexFailureNone {
		return nil, false, newCodexFailureError(failClass, strings.TrimSpace(p.errText), home)
	}

	out, parseErr := p.finish()
	if parseErr == nil {
		return out, false, nil
	}
	// The stream reported a failure the mid-stream check did not classify (it can
	// arrive on the very last line, after which the loop exits on EOF). Classify
	// it here too so a late 401 is still non-retryable and actionable.
	if p.hadError {
		if cls := classifyCodexError(p.errText); cls != codexFailureNone {
			return nil, false, newCodexFailureError(cls, strings.TrimSpace(p.errText), home)
		}
		// An unclassified reported error: real and terminal as far as we can tell,
		// but the turn may already have run tools, so never retry it blindly.
		return nil, false, parseErr
	}
	// Preserve any content parsed before either a normal exit or watchdog kill.
	if partial := p.salvage(); partial != nil {
		return partial, false, nil
	}
	if startupHang {
		return nil, true, fmt.Errorf(
			"codex CLI produced no output within %s and was killed as a likely MCP startup hang (retryable) — check the interaction MCP bridge (exit: %v)",
			codexStartupTimeout, runErr)
	}
	if idleHang {
		return nil, false, fmt.Errorf(
			"codex CLI produced no output for %s and was killed after the idle output timeout (non-retryable) (exit: %v) %s",
			codexIdleOutputTimeout, runErr, stdoutCrashTail(tail))
	}
	if runErr == nil {
		// The process exited cleanly yet produced no turn.completed — a truncated
		// or empty stream.
		return nil, false, parseErr
	}
	detail := strings.TrimSpace(stderr.String())
	if detail == "" {
		detail = stdoutCrashTail(tail)
	}
	if killedEarly {
		// Unreachable in practice (failClass returns above); kept so a future edit
		// that kills for another reason cannot report our own kill as a CLI crash.
		return nil, false, fmt.Errorf("codex CLI was terminated after a terminal error: %s", detail)
	}
	// Never saw a single turn/item event: the process died during startup. Nothing
	// ran, so it is safe to retry once.
	if !p.sawTurn {
		return nil, true, fmt.Errorf("codex CLI exited before producing any turn output (likely login, config, or MCP startup failure): %v %s", runErr, detail)
	}
	retryable = !p.ranTool()
	return nil, retryable, fmt.Errorf("codex CLI failed: %v %s", runErr, detail)
}

// codexBaseEnv returns the parent environment hardened the same way the native
// shell tools are, so a codex agent running `git commit` cannot hang on a GUI
// editor or a credential prompt inside the stdin-less child. There is no codex
// counterpart to the claude path's nesting/model filter (see claudecli.go):
// codex defines no nesting env var that would silently downgrade the child's
// model, so nothing of that kind is stripped here.
//
// Credentials ARE filtered, with an exemption for codex's own auth namespace —
// see codexCLIEnvExempt.
func codexBaseEnv() []string {
	return proc.HardenedEnv(proc.CredentialSafeEnvExcept(os.Environ(), codexCLIEnvExempt))
}
