package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/proc"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

const (
	shellMaxOutputBytes = 64 * 1024 // cap combined stdout+stderr
	shellMaxTimeout     = 120 * time.Second
	shellDefaultTimeout = 30 * time.Second
)

// shellArgs is the shared input schema for both the Bash and PowerShell tools.
type shellArgs struct {
	Command         string `json:"command"`
	TimeoutSec      int    `json:"timeout_sec"`
	RunInBackground bool   `json:"run_in_background"`
}

// resolvePOSIXShell finds the POSIX shell to back the Bash tool: /bin/sh on Unix,
// or a bash.exe (git-bash/WSL) on Windows. Returns ok=false on Windows when no
// bash is on PATH, so the Bash tool is simply not offered there (PowerShell is).
func resolvePOSIXShell() (string, bool) {
	if runtime.GOOS == "windows" {
		return lookInterpreter("bash")
	}
	return "/bin/sh", true
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
	if _, ok := resolvePOSIXShell(); ok {
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
	mgr *ShellManager // background-shell registry (nil = run_in_background unavailable)
}

// NewShellTool binds the tool to a base working directory and resolves the POSIX
// shell. Use Available to check whether a shell was found before registering it.
func NewShellTool(sb Sandbox) ShellTool {
	exe, _ := resolvePOSIXShell()
	return ShellTool{sb: sb, exe: exe}
}

// WithManager returns a copy of the tool wired to a session's background-shell
// manager, enabling run_in_background. Without it, background execution reports it
// is unavailable (the historical foreground-only behaviour).
func (t ShellTool) WithManager(m *ShellManager) ShellTool { t.mgr = m; return t }

// Available reports whether a backing POSIX shell was found (always true on Unix;
// on Windows only when a bash.exe is on PATH).
func (t ShellTool) Available() bool { return t.exe != "" }

func (ShellTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "Bash",
		Description: "Run a command through the POSIX shell (/bin/sh on Unix, bash.exe on Windows) and " +
			"return its combined stdout+stderr (truncated to 64KB). Starts in the working directory but may " +
			"operate on any path. Bounded by a timeout (default 30s, max 120s). Use POSIX/Bash syntax. On " +
			"Windows prefer the PowerShell tool for native tasks (cmdlets, registry, $env: variables). " +
			"Set run_in_background=true for a long-running command (dev server, watcher): it returns a shell " +
			"id immediately — poll shell_output and stop it with shell_kill.",
		InputSchema: shellInputSchema,
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
		return proc.CommandContext(runCtx, t.exe, "-c", command)
	}
	if args.RunInBackground {
		return startBackgroundShell(t.mgr, t.sb, args, "Bash", build)
	}
	return runShell(ctx, t.sb, args, onChunk, build)
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

// Available reports whether a PowerShell host (pwsh/powershell.exe) was found.
func (t PowerShellTool) Available() bool { return t.exe != "" }

func (PowerShellTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "PowerShell",
		Description: "Run a command through PowerShell (pwsh 7+ if available, else Windows PowerShell 5.1) " +
			"and return its combined stdout+stderr (truncated to 64KB). Starts in the working directory but " +
			"may operate on any path. Bounded by a timeout (default 30s, max 120s). Use PowerShell syntax: " +
			"cmdlets (Get-ChildItem), $env:VAR for environment variables, 2>$null (not 2>/dev/null), and " +
			"registry PSDrives (HKLM:\\). The command runs DIRECTLY in PowerShell — do NOT wrap it in another " +
			"`powershell -Command \"...\"` (that re-parses the string and strips $variable references).",
		InputSchema: shellInputSchema,
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
		return proc.CommandContext(runCtx, t.exe, "-NoProfile", "-NonInteractive", "-Command", command)
	}
	if args.RunInBackground {
		return startBackgroundShell(t.mgr, t.sb, args, "PowerShell", build)
	}
	return runShell(ctx, t.sb, args, onChunk, build)
}

// shellInputSchema is shared by both shell tools.
var shellInputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"command":{"type":"string","description":"The command line to execute"},
		"timeout_sec":{"type":"integer","description":"Timeout in seconds (default 30, max 120). Ignored when run_in_background is true."},
		"run_in_background":{"type":"boolean","description":"Run detached and return a shell id immediately instead of waiting. Use for long-running processes (dev servers, watchers); read output with shell_output and stop with shell_kill."}
	},
	"required":["command"],
	"additionalProperties":false
}`)

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
func runShell(ctx context.Context, sb Sandbox, args shellArgs, onChunk func(string), build func(ctx context.Context, command string) *exec.Cmd) (string, error) {
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

	cmd := build(runCtx, args.Command)
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
	return result, nil
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
