package agent

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// sqzCompressTimeout bounds the optimizer subprocess so a hung sqz never blocks a
// shell tool result.
const sqzCompressTimeout = 10 * time.Second

// sqzShellFilter returns an output post-processor that pipes a shell command's
// combined output through `sqz compress` in-process, or nil when sqz is not wired
// for this workspace (no sqz hook opt-in) or the binary is not on PATH.
//
// Why in-process: sqz's PreToolUse hook only rewrites the native "Bash" tool name;
// every TionSwarm shell runs through the bridged `mcp__tionswarm_interaction__Bash`,
// which sqz does not recognize — so the hook never fires (verified: same command is
// rewritten under "Bash" but passed through under the bridged name). Invoking
// `sqz compress` here (raw output on stdin) applies the same compression to bridged
// AND native shells, Bash AND PowerShell, without depending on the shell being able
// to resolve sqz on its own PATH (which is fragile under WSL/Git-bash).
//
// Safety: sqz self-gates (small/precise output — hashes, keys — returns verbatim) and
// its primary mechanism is LOSSLESS n-gram abbreviation (an inline legend the model
// reads back), so the agent loses no information. On ANY error the ORIGINAL output is
// returned (fail open) and the failure is LOGGED — compression is an optimization,
// never a correctness step, so it must never drop a valid command result.
func (r *Runtime) sqzShellFilter(ctx context.Context) func(cmd, output string) string {
	// Per-workspace override (WSSettings.ShellOutputCompression): off disables it
	// outright; on forces it regardless of hooks; auto (default) requires the sqz
	// hook opt-in — so removing the sqz hook is itself a natural off switch.
	switch r.shellCompressMode.Load() {
	case shellCompressOff:
		return nil
	case shellCompressOn:
		// forced on — skip the hook-detection gate; the binary check below still applies.
	default:
		if !r.detectTokenOptimizers(ctx).sqz {
			return nil // no sqz hook wired and not forced on
		}
	}
	sqzPath, err := exec.LookPath("sqz")
	if err != nil {
		r.logger.Warn("sqz shell filter: binary not on PATH; shell output not compressed", "error", err)
		return nil
	}
	return func(cmd, output string) string {
		runCtx, cancel := context.WithTimeout(ctx, sqzCompressTimeout)
		defer cancel()
		c := exec.CommandContext(runCtx, sqzPath, "compress", "--cmd", cmd)
		c.Stdin = strings.NewReader(output)
		var out, errBuf bytes.Buffer
		c.Stdout = &out
		c.Stderr = &errBuf
		if runErr := c.Run(); runErr != nil {
			r.logger.Warn("sqz compress failed; returning raw shell output",
				"error", runErr, "stderr", strings.TrimSpace(errBuf.String()))
			return output
		}
		compressed := strings.TrimRight(out.String(), "\n")
		if strings.TrimSpace(compressed) == "" {
			r.logger.Warn("sqz compress returned empty; returning raw shell output")
			return output
		}
		return compressed
	}
}
