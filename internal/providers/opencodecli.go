package providers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// OpenCodeCLI drives the locally-installed `opencode` CLI (anomalyco/opencode)
// in non-interactive mode: one `opencode run --format json` subprocess per
// turn. Like ClaudeCLI and CodexCLI it is a CLI TRANSPORT — opencode runs its
// own agentic loop and TionHarness's tools reach it over the MCP bridge, so the
// native tool loop is not involved. Authentication lives inside the instance's
// opencode home (`opencode auth login`, stored under <home>/data).
//
// Per-turn isolation is lighter than codex's: the home holds ONE shared
// session database and auth store (XDG_DATA_HOME / XDG_CONFIG_HOME both point
// under it), and each turn gets only its own opencode.json + system prompt in
// <home>/turns/turn-XXXX, handed over through OPENCODE_CONFIG.
type OpenCodeCLI struct {
	binPath string
	model   string // optional "provider/model" override
	// configDir is the opencode home. Must exist before a turn runs; "" inherits
	// the user's ambient ~/.local/share/opencode + ~/.config/opencode.
	configDir string
	// effortVariant passes the agent's thinking level as `--variant` (only for
	// vendors whose variant names are known; see opencodeVariant).
	effortVariant bool

	mcpServers         map[string]CLIMCPServer
	disableNativeShell bool
}

// NewOpenCodeCLI creates a provider that invokes the given opencode binary.
func NewOpenCodeCLI(binPath, model, configDir string) *OpenCodeCLI {
	return &OpenCodeCLI{binPath: binPath, model: model, configDir: configDir}
}

// Name implements Provider.
func (c *OpenCodeCLI) Name() string { return "opencode-cli" }

// Installed reports whether the configured binary resolves to an executable.
func (c *OpenCodeCLI) Installed() bool {
	if c.binPath == "" {
		return false
	}
	_, err := exec.LookPath(c.binPath)
	return err == nil
}

// Preflight verifies the binary starts (`--version`) with this home's
// environment; successful checks are cached per configuration.
func (c *OpenCodeCLI) Preflight(ctx context.Context) error {
	return runCLIPreflight(ctx, c.Name(), c.binPath, c.configDir, c.model, c.env(""))
}

// SetConfigDir points the provider at an opencode home. Empty is ignored so the
// value baked in at construction survives when no home is derivable.
func (c *OpenCodeCLI) SetConfigDir(dir string) {
	if dir == "" {
		return
	}
	c.configDir = dir
}

// ConfigDir returns the opencode home this provider currently uses.
func (c *OpenCodeCLI) ConfigDir() string { return c.configDir }

// ConfigureCLIMCP installs the current turn's MCP server set and native shell
// policy (rendered into opencode.json by buildConfig).
func (c *OpenCodeCLI) ConfigureCLIMCP(spec CLIMCPSpec) {
	c.mcpServers = spec.Servers
	c.disableNativeShell = spec.DisableNativeShell
}

// opencodeVariant maps the product's reasoning tier onto opencode's `--variant`
// for the vendors whose variant vocabulary is known. "" means "do not pass the
// flag": an unknown variant fails the whole turn inside opencode, so nothing is
// guessed for other vendors or the empty (home-default) model.
func opencodeVariant(model, effort string, disableThinking bool) string {
	if disableThinking {
		return ""
	}
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" {
		return ""
	}
	vendor, _, _ := strings.Cut(model, "/")
	switch vendor {
	case "openai":
		switch effort {
		case "minimal", "low":
			return "low"
		case "medium":
			return "medium"
		case "high":
			return "high"
		case "xhigh", "max", "ultra":
			return "xhigh"
		}
	case "anthropic":
		switch effort {
		case "minimal", "low":
			return "low"
		case "medium":
			return "medium"
		case "high":
			return "high"
		case "xhigh", "max", "ultra":
			return "max"
		}
	}
	return ""
}

// buildArgs assembles the `opencode run` invocation, kept pure for testing. The
// prompt is NOT an argument — it goes on stdin (verified: a piped message is
// read when no positional message is given), because Windows caps a process
// command line at ~32 KB and a full turn prompt overflows that.
func (c *OpenCodeCLI) buildArgs(req Request, model string) []string {
	args := []string{"run", "--format", "json", "--agent", opencodeAgentName}
	if model != "" {
		args = append(args, "-m", model)
	}
	if c.effortVariant {
		if v := opencodeVariant(model, req.CLIEffortLevel, req.DisableThinking); v != "" {
			args = append(args, "--variant", v)
		}
	}
	if req.WorkDir != "" {
		if fi, err := os.Stat(req.WorkDir); err == nil && fi.IsDir() {
			args = append(args, "--dir", req.WorkDir)
		}
	}
	if req.ResumeSessionID != "" && req.CLIResumeScope != "" && safeOpenCodeSessionID(req.ResumeSessionID) {
		args = append(args, "-s", req.ResumeSessionID)
	}
	return args
}

// buildConfig assembles the turn's opencode.json content. The static system
// prompt rides in the agent prompt file; the volatile per-turn context stays in
// the stdin prompt, exactly as on the claude/codex paths, so it does not churn
// the cached prefix.
func (c *OpenCodeCLI) buildConfig(req Request) opencodeConfig {
	aux := req.CLIRestrictNativeTools && len(req.CLINativeTools) == 0
	return opencodeConfig{
		SystemPrompt:          strings.TrimSpace(joinNonEmpty(req.System, interactionSystemNote)),
		PermissionMode:        req.PermissionMode,
		DisableNativeShell:    c.disableNativeShell,
		DisableNativeWeb:      !req.NativeWebSearch,
		DisableAllNativeTools: aux,
		Servers:               c.mcpServers,
	}
}

func (c *OpenCodeCLI) buildPrompt(req Request) string {
	return withDynamic(serializeTranscript(req.Messages), joinNonEmpty(req.SystemDynamic, req.Summary))
}

// env returns the hardened, credential-filtered child environment with the
// home's XDG directories and (when set) the turn's OPENCODE_CONFIG appended last
// so they override anything inherited. Only opencode's own namespace survives
// the credential filter: every provider key is expected to live in the home's
// auth store, not in TionHarness's environment.
func (c *OpenCodeCLI) env(configPath string) []string {
	env := proc.HardenedEnv(proc.CredentialSafeEnvExcept(os.Environ(), opencodeCLIEnvExempt))
	if c.configDir != "" {
		env = append(env,
			"XDG_DATA_HOME="+filepath.Join(c.configDir, "data"),
			"XDG_CONFIG_HOME="+filepath.Join(c.configDir, "config"),
			"XDG_CACHE_HOME="+filepath.Join(c.configDir, "cache"),
			"XDG_STATE_HOME="+filepath.Join(c.configDir, "state"),
		)
	}
	// Never let an inherited inline config or permission override leak into
	// the turn: the rendered file is the whole configuration.
	env = append(env, "OPENCODE_CONFIG_CONTENT=", "OPENCODE_PERMISSION=", "OPENCODE_DISABLE_AUTOUPDATE=1")
	if configPath != "" {
		env = append(env, "OPENCODE_CONFIG="+configPath)
	}
	return env
}

// opencodeCLIEnvExempt keeps opencode's own namespace (server password, config
// pointers set by the user's shell) through the credential filter.
func opencodeCLIEnvExempt(name string) bool {
	return strings.HasPrefix(strings.ToUpper(name), "OPENCODE_")
}

// Complete implements Provider by shelling out to `opencode run --format json`
// and parsing its JSONL stream line-by-line. When req.OnEvent is set, each
// activity step is delivered as soon as it is final.
func (c *OpenCodeCLI) Complete(ctx context.Context, req Request) (*Response, error) {
	if c.binPath == "" {
		return nil, fmt.Errorf("opencode CLI: no binary configured")
	}
	model := req.Model
	if model == "" {
		model = c.model
	}
	return c.completeWithArgs(ctx, c.buildArgs(req, model), c.buildPrompt(req), model, req)
}

// completeWithArgs writes the turn's config and runs the subprocess, retrying
// once on a clean crash (died before any step event, ran no tool). args and
// prompt are parameters so tests can drive the path with a fake binary.
func (c *OpenCodeCLI) completeWithArgs(ctx context.Context, args []string, prompt, model string, req Request) (*Response, error) {
	if c.configDir == "" && len(c.mcpServers) > 0 {
		return nil, fmt.Errorf("opencode CLI: MCP servers configured but no opencode home set — cannot write opencode.json")
	}
	turnDir, cleanup, err := prepareOpenCodeTurnDir(c.configDir)
	if err != nil {
		return nil, fmt.Errorf("opencode CLI: prepare turn dir: %w", err)
	}
	defer cleanup()
	configPath := ""
	if turnDir != "" {
		configPath, err = writeOpenCodeConfig(turnDir, c.buildConfig(req))
		if err != nil {
			return nil, fmt.Errorf("opencode CLI: %w", err)
		}
	}

	var failed []error
	for range 2 {
		resp, retryable, err := c.runAttempt(ctx, args, prompt, model, req, configPath)
		if err == nil {
			foldFailedAttempts(resp, failed)
			if req.CLIResumeScope != "" && resp.SessionID != "" {
				c.recordScopeSession(req.CLIResumeScope, resp.SessionID)
			}
			return resp, nil
		}
		failed = append(failed, err)
		if partial, _ := InterruptedResponse(err); partial != nil {
			foldFailedAttempts(partial, failed[:len(failed)-1])
			return nil, WithUsage(err, partial.Model, partial.Usage, partial.ProviderCalls)
		}
		if ctx.Err() != nil || !retryable {
			return nil, err
		}
	}
	return nil, failed[len(failed)-1]
}

// prepareOpenCodeTurnDir creates the turn-local directory the config is written
// to. An empty home keeps the ambient opencode configuration (no file written).
func prepareOpenCodeTurnDir(home string) (string, func(), error) {
	if home == "" {
		return "", func() {}, nil
	}
	root := filepath.Join(home, "turns")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", func() {}, err
	}
	sweepOpenCodeTurnDirs(root, time.Now())
	dir, err := os.MkdirTemp(root, "turn-")
	if err != nil {
		return "", func() {}, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

// sweepOpenCodeTurnDirs removes turn directories older than an hour — leftovers
// of a crashed process — so the home does not fill with stale configs.
func sweepOpenCodeTurnDirs(root string, now time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "turn-") {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > time.Hour {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}

// ProbeAuth implements AuthProber: the cheapest real turn that still exercises
// the configured default model's login path.
func (c *OpenCodeCLI) ProbeAuth(ctx context.Context) error {
	_, err := c.Complete(ctx, Request{
		System:                 "You are a connectivity probe. Reply with exactly: OK",
		Messages:               []Message{{Role: RoleUser, Text: "ping"}},
		CLIRestrictNativeTools: true,
	})
	return err
}

// opencodeStartupTimeout bounds the time-to-first-output: opencode boots a
// local server, loads MCP servers and may fetch its model catalog on a fresh
// home before the first event appears.
const opencodeStartupTimeout = 120 * time.Second

// opencodeIdleOutputTimeout is the compiled-in gap allowed between output lines
// once the stream has started; the effective window comes from the shared
// stdout-idle setting (resolveIdleOutputWindow).
const opencodeIdleOutputTimeout = 90 * time.Second

var (
	opencodeIdleMu                    sync.RWMutex
	opencodeIdleOutputTimeoutDuration = opencodeIdleOutputTimeout
)

// SetOpenCodeIdleOutputTimeout configures the stdout-silence watchdog for every
// subsequent opencode turn. d <= 0 disables it.
func SetOpenCodeIdleOutputTimeout(d time.Duration) {
	opencodeIdleMu.Lock()
	opencodeIdleOutputTimeoutDuration = d
	opencodeIdleMu.Unlock()
}

func opencodeIdleOutputWindow() time.Duration {
	opencodeIdleMu.RLock()
	defer opencodeIdleMu.RUnlock()
	return opencodeIdleOutputTimeoutDuration
}

// runAttempt runs the subprocess once and parses its stream. retryable is true
// only when re-running is free of duplicate side effects: no terminal
// classified failure, no salvageable content, no tool ran.
func (c *OpenCodeCLI) runAttempt(ctx context.Context, args []string, prompt, model string, req Request, configPath string) (resp *Response, retryable bool, err error) {
	ctx, stopCLI, watch := watchCLI(ctx, "opencode-cli", c.binPath, args)
	defer stopCLI()
	cmd := proc.CommandContextNested(ctx, c.binPath, args...)
	// opencode spawns MCP servers and whatever the turn shells out to; they
	// inherit the stdout pipe, so reap the whole tree on cancellation.
	proc.TreeKill(cmd)
	if c.configDir != "" {
		if fi, statErr := os.Stat(c.configDir); statErr != nil || !fi.IsDir() {
			return nil, false, fmt.Errorf("opencode CLI: home %q is not an existing directory: %v", c.configDir, statErr)
		}
	}
	cmd.Env = c.env(configPath)
	if req.WorkDir != "" {
		if fi, statErr := os.Stat(req.WorkDir); statErr == nil && fi.IsDir() {
			cmd.Dir = req.WorkDir
		}
	}
	cmd.Stdin = strings.NewReader(strings.ToValidUTF8(prompt, "�"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, serr := cmd.StdoutPipe()
	if serr != nil {
		return nil, false, serr
	}
	if serr := cmd.Start(); serr != nil {
		watch.Finish(serr)
		return nil, false, serr
	}
	watch.Started(cmd)

	p := newOpenCodeParser(model, c.serverKeys(), req.OnEvent)
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

	startup := time.NewTimer(opencodeStartupTimeout)
	defer startup.Stop()
	idleWindow := resolveIdleOutputWindow(ctx, opencodeIdleOutputWindow())
	idle := time.NewTimer(idleWindow)
	if !idle.Stop() {
		<-idle.C
	}
	defer idle.Stop()
	sawOutput, startupHang, idleHang, killedEarly := false, false, false, false
	failClass := opencodeFailureNone
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
				if idleWindow > 0 {
					idle.Reset(idleWindow)
				}
				p.feed(it.line)
				if s := strings.TrimSpace(it.line); s != "" {
					tail = append(tail, s)
					if len(tail) > tailMax {
						tail = tail[len(tail)-tailMax:]
					}
				}
				if p.hadError && failClass == opencodeFailureNone {
					if cls := classifyOpenCodeError(p.errName, p.errText); cls != opencodeFailureNone {
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
			reportWatchdogKill(req, WatchdogKill{Provider: "opencode-cli", Model: model, Reason: WatchdogReasonStartup, Window: opencodeStartupTimeout, Detail: stdoutCrashTail(tail)})
			break readLoop
		case <-idle.C:
			// A tool is running (a bridged run_subagent waiting on its child, a
			// slow command): silence is expected. The turn's semantic watchdog, the
			// hard cap and a human stop still end it through ctx.
			if p.toolInFlight() {
				if idleWindow > 0 {
					idle.Reset(idleWindow)
				}
				continue
			}
			idleHang = true
			proc.KillTree(cmd)
			reportWatchdogKill(req, WatchdogKill{Provider: "opencode-cli", Model: model, Reason: WatchdogReasonIdle, Window: idleWindow, Detail: stdoutCrashTail(tail)})
			break readLoop
		case <-ctx.Done():
			killedEarly = true
			break readLoop
		}
	}
	runErr := cmd.Wait()
	watch.Finish(runErr)

	if failClass != opencodeFailureNone {
		return nil, false, p.usageError(newOpenCodeFailureError(failClass, p.errText, c.configDir))
	}
	out, parseErr := p.finish()
	if parseErr == nil {
		return out, false, nil
	}
	if p.hadError {
		if cls := classifyOpenCodeError(p.errName, p.errText); cls != opencodeFailureNone {
			return nil, false, p.usageError(newOpenCodeFailureError(cls, p.errText, c.configDir))
		}
		return nil, false, p.usageError(parseErr)
	}
	if partial := p.salvage(); partial != nil {
		reason, window := "cli_stream_incomplete", time.Duration(0)
		cause := parseErr
		switch {
		case ctx.Err() != nil:
			reason, cause = "cli_cancelled", context.Cause(ctx)
		case idleHang:
			reason, window = "cli_idle_timeout", idleWindow
		case startupHang:
			reason, window = "cli_startup_timeout", opencodeStartupTimeout
		case runErr != nil:
			reason, cause = "cli_process_exit", runErr
		}
		partial.StopReason = StopInterrupted
		return nil, false, p.usageError(&CLIInterruption{Partial: partial, Reason: reason, Window: window, Cause: cause})
	}
	if startupHang {
		return nil, true, p.usageError(fmt.Errorf("opencode CLI produced no output within %s and was killed as a likely startup hang (retryable) — check the interaction MCP bridge and the opencode home (exit: %v)", opencodeStartupTimeout, runErr))
	}
	if idleHang {
		return nil, !p.ranTool(), p.usageError(fmt.Errorf("opencode CLI produced no output for %s and was killed after the idle output timeout (exit: %v) %s", idleWindow, runErr, stdoutCrashTail(tail)))
	}
	if runErr == nil {
		return nil, false, p.usageError(parseErr)
	}
	detail := strings.TrimSpace(stderr.String())
	if detail == "" {
		detail = stdoutCrashTail(tail)
	}
	if killedEarly {
		return nil, false, p.usageError(fmt.Errorf("opencode CLI was terminated after a terminal error: %s", detail))
	}
	if !p.sawStep {
		return nil, true, p.usageError(fmt.Errorf("opencode CLI exited before producing any step output (likely login, config, model or MCP startup failure): %v %s", runErr, detail))
	}
	return nil, !p.ranTool(), p.usageError(fmt.Errorf("opencode CLI failed: %v %s", runErr, detail))
}

// serverKeys returns the configured MCP server keys in sorted order, for the
// parser's tool-name mapping.
func (c *OpenCodeCLI) serverKeys() []string {
	keys := make([]string, 0, len(c.mcpServers))
	for k := range c.mcpServers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
