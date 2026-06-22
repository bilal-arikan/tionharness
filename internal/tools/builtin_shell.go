package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/bilal/swarmgo/internal/proc"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/bilal/swarmgo/internal/providers"
)

const (
	shellMaxOutputBytes = 64 * 1024 // cap combined stdout+stderr
	shellMaxTimeout     = 120 * time.Second
	shellDefaultTimeout = 30 * time.Second
)

// ShellTool runs a shell command. It is a high-risk tool: it is only registered
// when explicitly enabled (see the agent runtime's shell gate) and runs with a
// bounded timeout. It starts in the sandbox base directory but is NOT confined
// to it — commands may operate on any path. On Windows it uses PowerShell,
// elsewhere /bin/sh.
type ShellTool struct{ sb Sandbox }

// NewShellTool binds the tool to a base working directory (Sandbox.Root).
func NewShellTool(sb Sandbox) ShellTool { return ShellTool{sb: sb} }

func (ShellTool) Def() providers.ToolDef {
	shell := "/bin/sh -c"
	if runtime.GOOS == "windows" {
		shell = "PowerShell"
	}
	return providers.ToolDef{
		Name: "Bash",
		Description: fmt.Sprintf(
			"Run a shell command (%s) and return its combined stdout+stderr (truncated to 64KB). Starts in the working directory but may cd to and operate on any path. Bounded by a timeout (default 30s, max 120s). Use for builds, tests, and file operations. "+
				"The command runs DIRECTLY in the shell above — on Windows do NOT wrap it in another `powershell -Command \"...\"` / `powershell.exe -Command \"...\"`; pass the PowerShell statements as-is (e.g. `$x = Invoke-RestMethod ...; $x.foo`). Wrapping it re-parses the string and strips `$variable` references.",
			shell,
		),
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"command":{"type":"string","description":"The command line to execute"},
				"timeout_sec":{"type":"integer","description":"Timeout in seconds (default 30, max 120)"}
			},
			"required":["command"],
			"additionalProperties":false
		}`),
	}
}

// Call runs the command and returns its full output (non-streaming).
func (t ShellTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	return t.CallStream(ctx, input, nil)
}

// CallStream runs the command, forwarding each output chunk to onChunk (when
// non-nil) as the process writes it, while still returning the full (capped)
// output. Implements StreamingTool so the agent loop can surface live tool
// output as tool_delta steps.
func (t ShellTool) CallStream(ctx context.Context, input json.RawMessage, onChunk func(string)) (string, error) {
	var args struct {
		Command    string `json:"command"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return "", fmt.Errorf("command is required")
	}
	// Autonomous brake: when the sandbox is confined (autonomous turn + the
	// AutonomousConfine guard), block network-mutating git operations. A scheduled
	// or spawned agent must not push to a remote without a human in the loop;
	// interactive chat (unconfined) is unaffected. Best-effort substring guard.
	if t.sb.Confined && isNetworkMutatingGit(args.Command) {
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

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Defensive unwrap: this tool already runs inside powershell.exe. Agents
		// often redundantly wrap their command in `powershell -Command "..."`,
		// which makes the OUTER shell expand (and strip) any $variable before the
		// inner shell ever sees it — breaking scripts like `$x = ...; $x | ...`.
		// Strip a single redundant wrapper so the statements run directly.
		args.Command = unwrapRedundantPowershell(args.Command)
		cmd = exec.CommandContext(runCtx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", args.Command)
	} else {
		cmd = exec.CommandContext(runCtx, "/bin/sh", "-c", args.Command)
	}
	proc.Hide(cmd) // no console flash under the windowless desktop app
	cmd.Dir = t.sb.Root

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
var powershellWrapperRe = regexp.MustCompile(`(?is)^\s*powershell(?:\.exe)?\s+(?:-\S+\s+)*-c(?:ommand)?\s+"(.*)"\s*$`)

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
