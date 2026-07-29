package agent

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// sqzCompressTimeout bounds the optimizer subprocess so a hung sqz never blocks a
// shell tool result.
const sqzCompressTimeout = 10 * time.Second

// sqzStatsRe extracts the exact token counts sqz reports on stderr after a
// successful compression:
//
//	[sqz] 57/841 tokens (93% reduction) [cargo test]
//
// The first number is the compressed (OUT) count, the second the original (IN)
// count. Parsing this is why the chip shows a real measurement rather than a
// character-length guess — sqz already tokenizes, we just read its answer.
var sqzStatsRe = regexp.MustCompile(`\[sqz\]\s+(\d+)/(\d+)\s+tokens`)

// sqzDedupRe matches sqz's OTHER success mode. When the output is byte-identical
// to something it compressed earlier, sqz emits no token pair — it reports
//
//	[sqz] dedup hit: §ref:a8856afd1df33ecc§ (L2)
//
// and stdout becomes just that reference, replacing the entire result. Without
// recognising this line the biggest saving sqz can make would show NO chip at
// all, on precisely the card most likely to look broken to a reader.
var sqzDedupRe = regexp.MustCompile(`\[sqz\]\s+dedup hit`)

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
func (r *Runtime) sqzShellFilter(ctx context.Context) tools.ShellOutputFilter {
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
	return func(cmd, output string) (string, *tools.ShellOptimization) {
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
			return output, nil
		}
		compressed := strings.TrimRight(out.String(), "\n")
		if strings.TrimSpace(compressed) == "" {
			r.logger.Warn("sqz compress returned empty; returning raw shell output")
			return output, nil
		}
		return compressed, parseSqzStats(errBuf.String())
	}
}

// parseSqzStats reads the token counts sqz reports on stderr. Returns nil when
// the line is absent or reports no actual saving — sqz self-gates and passes
// precise/short output through verbatim, and a chip claiming "−0%" on an
// untouched result would be a lie about what the agent read.
func parseSqzStats(stderr string) *tools.ShellOptimization {
	m := sqzStatsRe.FindStringSubmatch(stderr)
	if m == nil {
		if sqzDedupRe.MatchString(stderr) {
			return &tools.ShellOptimization{Kind: "sqz", Dedup: true}
		}
		return nil
	}
	// Both groups are \d+, so the only ParseInt failure left is overflow; treat
	// that as "no measurement" rather than reporting a garbage count.
	outTok, errOut := strconv.Atoi(m[1])
	inTok, errIn := strconv.Atoi(m[2])
	if errOut != nil || errIn != nil {
		return nil
	}
	opt := tools.ShellOptimization{Kind: "sqz", InTokens: inTok, OutTokens: outTok}
	if !opt.Measured() {
		return nil
	}
	return &opt
}
