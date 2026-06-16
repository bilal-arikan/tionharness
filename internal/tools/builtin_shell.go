package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/bilal/swarmgo/internal/providers"
)

const (
	shellMaxOutputBytes = 64 * 1024       // cap combined stdout+stderr
	shellMaxTimeout     = 120 * time.Second
	shellDefaultTimeout = 30 * time.Second
)

// ShellTool runs a shell command inside the workspace sandbox directory. It is a
// high-risk tool: it is only registered when explicitly enabled (see the agent
// runtime's shell gate), runs with a bounded timeout, and is confined to the
// sandbox working directory. On Windows it uses PowerShell, elsewhere /bin/sh.
type ShellTool struct{ sb Sandbox }

// NewShellTool binds the tool to a workspace sandbox.
func NewShellTool(sb Sandbox) ShellTool { return ShellTool{sb: sb} }

func (ShellTool) Def() providers.ToolDef {
	shell := "/bin/sh -c"
	if runtime.GOOS == "windows" {
		shell = "PowerShell"
	}
	return providers.ToolDef{
		Name: "shell",
		Description: fmt.Sprintf(
			"Run a shell command in the workspace directory (%s) and return its combined stdout+stderr (truncated to 64KB). Bounded by a timeout (default 30s, max 120s). Use for builds, tests, and file operations.",
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
	if !t.sb.Ready() {
		return "", fmt.Errorf("shell sandbox is not configured")
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
		cmd = exec.CommandContext(runCtx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", args.Command)
	} else {
		cmd = exec.CommandContext(runCtx, "/bin/sh", "-c", args.Command)
	}
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
