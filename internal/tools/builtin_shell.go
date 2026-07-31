package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/proc"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

const shellMaxOutputBytes = 64 * 1024 // cap combined stdout+stderr

// Shell timeouts are process-global and settings-driven (ShellDefaultTimeoutSec /
// ShellMaxTimeoutSec) via SetShellTimeouts, pushed from applySettings. A per-call
// timeout_sec arg still overrides shellDefaultTimeout, clamped to shellMaxTimeout.
var (
	shellMaxTimeout     = 120 * time.Second
	shellDefaultTimeout = 30 * time.Second
)

// SetShellTimeouts overrides the default and hard-max shell command timeouts (in
// seconds). A value <= 0 leaves the corresponding timeout unchanged, so a partial
// settings push never zeroes a live timeout.
func SetShellTimeouts(defaultSec, maxSec int) {
	if defaultSec > 0 {
		shellDefaultTimeout = time.Duration(defaultSec) * time.Second
	}
	if maxSec > 0 {
		shellMaxTimeout = time.Duration(maxSec) * time.Second
	}
}

// shellArgs is the shared input schema for both the Bash and PowerShell tools.
type shellArgs struct {
	Command         string   `json:"command"`
	TimeoutSec      int      `json:"timeout_sec"`
	RunInBackground flexBool `json:"run_in_background"`
	// NoCompress skips the token-optimizer output filter for THIS call, returning the
	// byte-exact raw output. Advertised only when a filter is active (see Def). Use it
	// when you need the output verbatim (a value you will parse/compare exactly).
	NoCompress flexBool `json:"no_compress"`
}

// resolvePowerShell finds a PowerShell host for the PowerShell tool, preferring
// PowerShell 7+ (pwsh, cross-platform, modern syntax) over Windows PowerShell 5.1
// (powershell.exe). Returns ok=false when neither is present (typical on Unix
// without pwsh installed), so the tool is not offered there.
func resolvePowerShell() (string, bool) {
	return lookInterpreter("pwsh", "powershell")
}

// ShellToolNames reports which shell tools buildRegistry would register on this
// host, by name: "Bash" when a POSIX shell backs it, "PowerShell" when a
// PowerShell host is present. It shares the SAME resolvers as the tools
// themselves (resolvePOSIXShell / resolvePowerShell), so the names advertised in
// the prompt can never drift from what is actually registered. No Sandbox is
// needed — availability depends only on PATH, not on the base directory. Returns
// an empty slice when neither host is found (should not happen on a supported OS).
func ShellToolNames() []string {
	var names []string
	if _, _, ok := resolvePOSIXShell(); ok {
		names = append(names, "Bash")
	}
	if _, ok := resolvePowerShell(); ok {
		names = append(names, "PowerShell")
	}
	return names
}

// ShellTool runs a command through the POSIX shell (the "Bash" tool): /bin/sh on
// Unix, bash.exe on Windows. High-risk (RiskExec), gated behind the shell switch.
// It starts in the sandbox base directory but is NOT confined to it. Its
// PowerShell sibling (PowerShellTool) handles Windows-native shells.
type ShellTool struct {
	sb  Sandbox
	exe string
	// preArgs precede "-c <command>" in the argv. Empty for a direct bash; on a
	// WSL-only Windows host it carries the `-e bash` that turns exe (wsl.exe) into
	// a POSIX shell (see resolvePOSIXShell).
	preArgs []string
	mgr     *ShellManager // background-shell registry (nil = run_in_background unavailable)
	// outFilter optionally post-processes the combined output before it is returned
	// to the model (e.g. an external token-optimizer like sqz). nil = passthrough. It
	// runs only on foreground runs and only above shellCompressMinBytes; the live UI
	// stream (onChunk) is never filtered. Injected by the agent layer. The second
	// return value reports what the optimizer actually did (nil = nothing measurable),
	// so the UI can show a chip instead of the compression being invisible.
	outFilter ShellOutputFilter
	// cmdFilter optionally rewrites the command before it runs (rtk). nil = as-typed.
	cmdFilter ShellCommandFilter
	// advertiseOptimizer forces no_compress into the SCHEMA even though no filter is
	// wired into this instance. See AdvertiseOptimizerFlag.
	advertiseOptimizer bool
}

// ShellOutputFilter post-processes a shell command's combined output before it is
// returned to the model, and reports the optimization it applied (nil when the
// output was passed through unchanged or the optimizer produced no measurement).
// It MUST fail open — on any internal error it returns the original output.
type ShellOutputFilter func(cmd, output string) (string, *ShellOptimization)

// ShellCommandFilter rewrites a command BEFORE it runs so an external optimizer
// (rtk) can make it produce less output in the first place — e.g. a test runner
// that reports only failures. Returns "" to leave the command untouched, which is
// also the required behaviour on any internal error (fail open: never block a
// command because an optimization could not be applied).
//
// This is the counterpart of ShellOutputFilter at the other end of the call: one
// shapes the command, the other compresses the result, and they compose.
type ShellCommandFilter func(cmd string) (string, *ShellOptimization)

// NewShellTool binds the tool to a base working directory and resolves the POSIX
// shell. Use Available to check whether a shell was found before registering it.
func NewShellTool(sb Sandbox) ShellTool {
	exe, preArgs, _ := resolvePOSIXShell()
	return ShellTool{sb: sb, exe: exe, preArgs: preArgs}
}

// WithManager returns a copy of the tool wired to a session's background-shell
// manager, enabling run_in_background. Without it, background execution reports it
// is unavailable (the historical foreground-only behaviour).
func (t ShellTool) WithManager(m *ShellManager) ShellTool { t.mgr = m; return t }

// WithOutputFilter returns a copy whose combined output is post-processed by f
// before being returned to the model (nil = passthrough). Used to route shell
// output through an external token-optimizer (sqz) in-process, since the CLI hook
// path cannot reach TionSwarm's bridged shell tool name.
func (t ShellTool) WithOutputFilter(f ShellOutputFilter) ShellTool {
	t.outFilter = f
	return t
}

// WithCommandFilter returns a copy whose command is rewritten by f before it runs
// (nil = as-typed). Used to route shell commands through an external command-layer
// optimizer (rtk) in-process, for the same reason WithOutputFilter exists: the CLI
// hook path cannot reach TionSwarm's bridged shell tool name.
func (t ShellTool) WithCommandFilter(f ShellCommandFilter) ShellTool {
	t.cmdFilter = f
	return t
}

// AdvertiseOptimizerFlag puts no_compress in the SCHEMA even when this instance
// carries no filter. It exists for the Interaction MCP bridge, which builds the
// tool definition from a bare, sandbox-less tool while the ACTUAL filter is
// installed per turn by Runtime.NewShellRunner.
//
// Without it the bridged schema omitted no_compress and set
// "additionalProperties": false, so the flag was not merely undocumented — it was
// forbidden. That broke a promise made elsewhere: when a rewritten command fails,
// the optimizer note tells the agent to "re-run with no_compress: true", and on
// 2026-07-31 an agent that obeyed had the call rejected, then reasoned from the
// schema that the option did not exist and fell back to `> file 2>&1`. The escape
// hatch has to be reachable on the path that recommends it.
func (t ShellTool) AdvertiseOptimizerFlag() ShellTool {
	t.advertiseOptimizer = true
	return t
}

// Available reports whether a backing POSIX shell was found (always true on Unix;
// on Windows only when a bash.exe is on PATH).
func (t ShellTool) Available() bool { return t.exe != "" }

func (t ShellTool) Def() providers.ToolDef {
	bg := t.mgr != nil
	desc := "Run a command through the POSIX shell (/bin/sh on Unix, bash.exe on Windows) and " +
		"return its combined stdout+stderr (truncated to 64KB). Starts in the working directory but may " +
		"operate on any path. Bounded by a timeout (default 30s, max 120s). Use POSIX/Bash syntax. This is " +
		"the PREFERRED shell — reach for it first, including on Windows. Only switch to the PowerShell tool " +
		"for Windows-native tasks Bash cannot do (cmdlets, registry, $env: variables)."
	if bg {
		desc += bgHint
	}
	return providers.ToolDef{
		Name:        "Bash",
		Description: desc,
		InputSchema: shellInputSchema(bg, t.outFilter != nil || t.advertiseOptimizer),
	}
}

func (t ShellTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	return t.CallStream(ctx, input, nil)
}

func (t ShellTool) CallStream(ctx context.Context, input json.RawMessage, onChunk func(string)) (string, error) {
	args, err := parseShellArgs(input)
	if err != nil {
		return "", err
	}
	build := func(runCtx context.Context, command string) *exec.Cmd {
		argv := append(append([]string{}, t.preArgs...), "-c", command)
		return hardenShellCmd(proc.CommandContext(runCtx, t.exe, argv...), t.sb.Confined)
	}
	if args.RunInBackground {
		return startBackgroundShell(t.mgr, t.sb, args, "Bash", build)
	}
	return runShell(ctx, t.sb, args, onChunk, build, t.outFilter, t.cmdFilter)
}

// PowerShellTool runs a command through PowerShell (pwsh preferred, else
// powershell.exe) — the Windows-native sibling of the Bash tool. Same execution
// core, risk tier and gating; only the command wrapping and syntax differ. Giving
// it an explicit name lets the model emit correct PowerShell syntax (cmdlets,
// $env:VAR, no &&, 2>$null) instead of guessing from a "Bash"-named tool.
type PowerShellTool struct {
	sb  Sandbox
	exe string
	mgr *ShellManager // background-shell registry (nil = run_in_background unavailable)
	// outFilter — see ShellTool.outFilter. Same contract for the PowerShell sibling.
	outFilter ShellOutputFilter
	// cmdFilter — see ShellTool.cmdFilter.
	cmdFilter ShellCommandFilter
	// advertiseOptimizer — see ShellTool.AdvertiseOptimizerFlag.
	advertiseOptimizer bool
}

// NewPowerShellTool binds the tool to a base working directory and resolves a
// PowerShell host. Use Available to check one was found before registering it.
func NewPowerShellTool(sb Sandbox) PowerShellTool {
	exe, _ := resolvePowerShell()
	return PowerShellTool{sb: sb, exe: exe}
}

// WithManager returns a copy wired to a session's background-shell manager,
// enabling run_in_background (see ShellTool.WithManager).
func (t PowerShellTool) WithManager(m *ShellManager) PowerShellTool { t.mgr = m; return t }

// WithOutputFilter returns a copy whose combined output is post-processed by f
// before return (nil = passthrough). See ShellTool.WithOutputFilter.
func (t PowerShellTool) WithOutputFilter(f ShellOutputFilter) PowerShellTool {
	t.outFilter = f
	return t
}

// WithCommandFilter returns a copy whose command is rewritten before it runs.
// See ShellTool.WithCommandFilter.
func (t PowerShellTool) WithCommandFilter(f ShellCommandFilter) PowerShellTool {
	t.cmdFilter = f
	return t
}

// AdvertiseOptimizerFlag — see ShellTool.AdvertiseOptimizerFlag.
func (t PowerShellTool) AdvertiseOptimizerFlag() PowerShellTool {
	t.advertiseOptimizer = true
	return t
}

// Available reports whether a PowerShell host (pwsh/powershell.exe) was found.
func (t PowerShellTool) Available() bool { return t.exe != "" }

func (t PowerShellTool) Def() providers.ToolDef {
	bg := t.mgr != nil
	desc := "Run a command through PowerShell (pwsh 7+ if available, else Windows PowerShell 5.1) " +
		"and return its combined stdout+stderr (truncated to 64KB). Use ONLY when the Bash tool cannot do " +
		"the job — i.e. for Windows-native tasks (cmdlets, registry, $env: variables); prefer Bash for " +
		"everything else. Starts in the working directory but " +
		"may operate on any path. Bounded by a timeout (default 30s, max 120s). Use PowerShell syntax: " +
		"cmdlets (Get-ChildItem), $env:VAR for environment variables, 2>$null (not 2>/dev/null), and " +
		"registry PSDrives (HKLM:\\). The command runs DIRECTLY in PowerShell — do NOT wrap it in another " +
		"`powershell -Command \"...\"` (that re-parses the string and strips $variable references)."
	if bg {
		desc += bgHint
	}
	return providers.ToolDef{
		Name:        "PowerShell",
		Description: desc,
		InputSchema: shellInputSchema(bg, t.outFilter != nil || t.advertiseOptimizer),
	}
}

func (t PowerShellTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	return t.CallStream(ctx, input, nil)
}

func (t PowerShellTool) CallStream(ctx context.Context, input json.RawMessage, onChunk func(string)) (string, error) {
	args, err := parseShellArgs(input)
	if err != nil {
		return "", err
	}
	build := func(runCtx context.Context, command string) *exec.Cmd {
		// Defensive unwrap: this tool already runs inside PowerShell. Agents often
		// redundantly wrap their command in `powershell -Command "..."`, which makes
		// the OUTER shell expand (and strip) any $variable before the inner shell sees
		// it — breaking scripts like `$x = ...; $x | ...`. Strip one redundant wrapper.
		command = unwrapRedundantPowershell(command)
		// Force UTF-8 I/O on the legacy Windows PowerShell 5.1 host so non-ASCII
		// (e.g. Turkish) file content is not mangled on the round-trip to Go. No-op
		// for pwsh 7+, which is UTF-8 by default. See builtin_shell_encoding.go.
		command = applyWinPSUTF8(t.exe, command)
		return hardenShellCmd(proc.CommandContext(runCtx, t.exe, "-NoProfile", "-NonInteractive", "-Command", command), t.sb.Confined)
	}
	if args.RunInBackground {
		return startBackgroundShell(t.mgr, t.sb, args, "PowerShell", build)
	}
	return runShell(ctx, t.sb, args, onChunk, build, t.outFilter, t.cmdFilter)
}

// shellInputSchema builds the shared shell-tool schema. run_in_background is only
// advertised when the tool has a background-shell manager wired (withBackground):
// without one, the runtime would reject the field at call time — so it must not
// appear in the schema in the first place (don't offer what you'll refuse).
func shellInputSchema(withBackground, withCompress bool) json.RawMessage {
	props := `"command":{"type":"string","description":"The command line to execute"},
		"timeout_sec":{"type":"integer","description":"Timeout in seconds (default 30, max 120)."}`
	if withBackground {
		props = `"command":{"type":"string","description":"The command line to execute"},
		"timeout_sec":{"type":"integer","description":"Timeout in seconds (default 30, max 120). Ignored when run_in_background is true."},
		"run_in_background":{"type":"boolean","description":"Run detached and return a shell id immediately instead of waiting. Use for long-running processes (dev servers, watchers); read output with shell_output and stop with shell_kill."}`
	}
	// no_compress is advertised only when an output token-optimizer is active for this
	// tool — otherwise the flag would be a no-op the model shouldn't see.
	if withCompress {
		props += `,
		"no_compress":{"type":"boolean","description":"Return the byte-exact raw output, skipping the token-optimizer compression applied to large output. Use only when you must parse/compare the output verbatim."}`
	}
	return json.RawMessage(`{"type":"object","properties":{` + props + `},"required":["command"],"additionalProperties":false}`)
}

// bgHint is the trailing run_in_background sentence appended to a shell tool's
// description only when background execution is actually available.
const bgHint = " Set run_in_background=true for a long-running command (dev server, watcher): it returns a shell id immediately — poll shell_output and stop it with shell_kill."

func parseShellArgs(input json.RawMessage) (shellArgs, error) {
	var args shellArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return args, argErr(err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return args, fmt.Errorf("command is required")
	}
	return args, nil
}

// startBackgroundShell launches args.Command detached via the session's shell
// manager and returns the assigned shell id with a usage hint. It applies the same
// confined-mode git brake as runShell, then hands off to the manager (which owns
// the process lifecycle). A nil manager reports background execution is unavailable.
func startBackgroundShell(mgr *ShellManager, sb Sandbox, args shellArgs, label string, build func(ctx context.Context, command string) *exec.Cmd) (string, error) {
	if mgr == nil {
		return "", fmt.Errorf("run_in_background is not available here — run the command in the foreground instead")
	}
	if sb.Confined && isNetworkMutatingGit(args.Command) {
		return "", fmt.Errorf("blocked in confined (autonomous) mode: this command pushes to a git remote — run it from an interactive chat session instead")
	}
	id, err := mgr.Start(sb, args.Command, label, build)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Started background %s shell %q. Read its output with shell_output (shell_id=%q) and stop it with shell_kill.", label, id, id), nil
}

// runShell is the shared execution core for the Bash and PowerShell tools: the
// confined-mode git guard, timeout, working directory, capped streaming capture
// and uniform output formatting. build constructs the *exec.Cmd for the resolved
// command — the only part that differs between shells.
// shellCompressMinBytes is the output size below which the outFilter is skipped:
// small outputs are not worth an optimizer subprocess (and precise short values —
// hashes, keys — should never be touched anyway).
const shellCompressMinBytes = 2048

// rtkDegradedNote is appended when a REWRITTEN command fails. rtk replaces the
// command's real output with its own summary, and its summarizers can lose the
// actual error entirely. Measured 2026-07-28 with a go.mod carrying a stray BOM,
// where the raw command reports `go.mod:1: unexpected input character U+FEFF`:
// rtk 0.42.4 answered "Go test: No tests found", and 0.44.1 returns NOTHING at
// all. The failure mode is not going away, it just changes shape between
// releases — which is precisely why this note is unconditional rather than
// pattern-matched against any particular wording.
// Re-running the command ourselves to recover the raw text
// would double the cost of every failing build and can report a DIFFERENT result
// for a flaky test — so we do not guess. We say plainly that this is a summary of
// a failure and point at the escape hatch, and let the agent decide.
const rtkDegradedNote = "\n\n[optimizer note: this command FAILED and the text above is a token-optimized " +
	"SUMMARY produced by rtk, not the command's raw output. rtk preserves test and compile failures, but a " +
	"setup/tooling error (bad go.mod, missing toolchain) can be reduced to something uninformative like " +
	"\"No tests found\". If the summary does not explain the failure, re-run the SAME command with " +
	"no_compress: true to get the byte-exact output.]"

func runShell(ctx context.Context, sb Sandbox, args shellArgs, onChunk func(string), build func(ctx context.Context, command string) *exec.Cmd, outFilter ShellOutputFilter, cmdFilter ShellCommandFilter) (string, error) {
	// Autonomous brake: when the sandbox is confined (autonomous turn + the
	// AutonomousConfine guard), block network-mutating git operations. A scheduled
	// or spawned agent must not push to a remote without a human in the loop;
	// interactive chat (unconfined) is unaffected. Best-effort substring guard.
	if sb.Confined && isNetworkMutatingGit(args.Command) {
		return "", fmt.Errorf("blocked in confined (autonomous) mode: this command pushes to a git remote — run it from an interactive chat session instead")
	}

	timeout := shellDefaultTimeout
	if args.TimeoutSec > 0 {
		timeout = time.Duration(args.TimeoutSec) * time.Second
		if timeout > shellMaxTimeout {
			timeout = shellMaxTimeout
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Command-layer optimizer (rtk): rewrite BEFORE running so the command emits
	// less in the first place. Honours the same no_compress opt-out as the output
	// filter — one flag, one concept: "give me this command untouched".
	command := args.Command
	var rewrite *ShellOptimization
	if cmdFilter != nil && !args.NoCompress {
		if rewritten, opt := cmdFilter(args.Command); rewritten != "" && rewritten != args.Command {
			command = rewritten
			rewrite = opt
		}
	}

	cmd := build(runCtx, command)
	cmd.Dir = sb.Root

	// Same writer for stdout+stderr: exec serialises writes when they are equal,
	// so onChunk is never called concurrently.
	w := &shellStreamWriter{onChunk: onChunk, max: shellMaxOutputBytes}
	cmd.Stdout = w
	cmd.Stderr = w
	runErr := cmd.Run()

	out := w.buf.Bytes()
	truncated := w.truncated

	var b strings.Builder
	if runCtx.Err() == context.DeadlineExceeded {
		fmt.Fprintf(&b, "[command timed out after %s]\n", timeout)
	}
	b.Write(out)
	if truncated {
		b.WriteString("\n[output truncated at 64KB]")
	}
	if runErr != nil && runCtx.Err() != context.DeadlineExceeded {
		fmt.Fprintf(&b, "\n[exit error: %v]", runErr)
	}
	result := strings.TrimSpace(b.String())
	if result == "" {
		result = "(no output)"
	}
	// Both optimizers can act on one call (rtk shapes the command, sqz compresses
	// what it printed), so their findings are merged into ONE record rather than
	// recorded twice — a second record would overwrite the first and silently drop
	// whichever half it did not carry (notably the Degraded warning).
	rec := rewrite
	if rec == nil && isRTKWrapped(args.Command) {
		// The agent typed `rtk …` itself rather than us rewriting it.
		rec = &ShellOptimization{Kind: "rtk"}
	}
	// A rewritten command that FAILED is flagged here; the note itself is appended
	// AFTER the output filter below, so the recovery instruction reaches the agent
	// verbatim instead of arriving abbreviated by sqz.
	degraded := rewrite != nil && runErr != nil
	if degraded {
		rec.Degraded = true
	}
	// Optional token-optimizer post-processing (e.g. sqz), applied only to the value
	// RETURNED to the model — the live UI stream (onChunk) already showed the raw
	// output. Skipped for small outputs. The filter must fail open (return the
	// original on any error); it is an optimization, not a correctness step.
	if outFilter != nil && !args.NoCompress && len(result) >= shellCompressMinBytes {
		filtered, opt := outFilter(args.Command, result)
		result = filtered
		if opt != nil {
			// sqz measured real tokens, so its kind/counts win the label; the
			// command-layer facts (what actually ran, and whether it degraded)
			// belong to rtk and are carried over.
			merged := *opt
			if rec != nil {
				merged.Command = rec.Command
				merged.Degraded = rec.Degraded
			}
			rec = &merged
		}
	}
	if rec != nil {
		recordOptimization(ctx, *rec)
	}
	if degraded {
		result += rtkDegradedNote
	}
	return result, nil
}

// isRTKWrapped reports whether the command runs through the `rtk` token-proxy —
// i.e. rtk is the program being invoked, not a word appearing later in the line.
// Only the leading token counts, so `grep rtk log.txt` is correctly not a match.
func isRTKWrapped(cmd string) bool {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	switch strings.ToLower(fields[0]) {
	case "rtk", "rtk.exe":
		return true
	}
	return false
}

// isNetworkMutatingGit reports whether cmd looks like a git operation that
// mutates a remote (push). It is a best-effort substring check used only as the
// autonomous brake — it normalises whitespace and lowercases so "git   push" and
// "git push --force" are caught. Not a security boundary; the real boundary is
// running autonomous turns confined.
func isNetworkMutatingGit(cmd string) bool {
	norm := strings.ToLower(strings.Join(strings.Fields(cmd), " "))
	return strings.Contains(norm, "git push") || strings.Contains(norm, "git remote add") || strings.Contains(norm, "git remote set-url")
}

// powershellWrapperRe matches a command that is ENTIRELY a redundant invocation
// of `powershell[.exe] [flags] -Command "<inner>"` (or `-c "<inner>"`). Only the
// quoted single-argument form is unwrapped — anything more complex is left as-is.
var powershellWrapperRe = regexp.MustCompile(`(?is)^\s*(?:pwsh|powershell)(?:\.exe)?\s+(?:-\S+\s+)*-c(?:ommand)?\s+"(.*)"\s*$`)

// unwrapRedundantPowershell strips one redundant outer `powershell -Command "..."`
// wrapper (see the call site for why). It only unwraps when the whole command is
// the wrapper and the inner script carries no escaped quotes (`\"` or `""`), which
// would make naive unquoting wrong — in that case the original is returned
// untouched so behaviour never silently changes.
func unwrapRedundantPowershell(cmd string) string {
	m := powershellWrapperRe.FindStringSubmatch(cmd)
	if m == nil {
		return cmd
	}
	inner := m[1]
	if strings.Contains(inner, `\"`) || strings.Contains(inner, `""`) {
		return cmd
	}
	return strings.TrimSpace(inner)
}

// shellStreamWriter buffers process output up to max bytes (for the final
// result) while forwarding every chunk to onChunk for live streaming. exec
// serialises calls when stdout and stderr share one writer, so no locking is
// needed.
type shellStreamWriter struct {
	buf       bytes.Buffer
	onChunk   func(string)
	max       int
	truncated bool
}

func (w *shellStreamWriter) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		if len(p) <= room {
			w.buf.Write(p)
		} else {
			w.buf.Write(p[:room])
			w.truncated = true
		}
	} else {
		w.truncated = true
	}
	if w.onChunk != nil {
		w.onChunk(string(p))
	}
	return len(p), nil
}
