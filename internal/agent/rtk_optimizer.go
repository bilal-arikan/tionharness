package agent

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/bilal-arikan/tionharness/internal/proc"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// rtkRewriteTimeout bounds the `rtk rewrite` probe. It is a pure string
// transformation (no command execution), so it should return in milliseconds;
// the timeout only guards against a wedged binary blocking every shell call.
const rtkRewriteTimeout = 5 * time.Second

// rtkCommandFilter returns a command rewriter that routes supported dev commands
// through the `rtk` proxy, or nil when rtk is not wired for this workspace.
//
// # Why in-process, like sqz
//
// rtk ships a PreToolUse hook that prefixes `rtk ` onto shell commands, but that
// hook matches the tool NAME "Bash". Every TionHarness shell runs through the
// bridged `mcp__tionharness_interaction__Bash`, which the hook does not recognize —
// the same blind spot that made sqz's hook never fire. Measured across two real
// worker sessions (SES14, SES15): 0 of 81 shell calls went through rtk. Doing the
// rewrite here covers bridged AND native shells, Bash AND PowerShell.
//
// # Why rtk instead of leaving it to sqz
//
// They shrink different things and neither subsumes the other. sqz abbreviates
// TEXT losslessly; rtk changes the COMMAND so less text exists. Measured on this
// repo (token counts from sqz's own tokenizer):
//
//	go test -v ./internal/tools   5993 → sqz 2943 → rtk 110
//	git log -30                   6595 → sqz 2027 → rtk 2157 → rtk+sqz 1167
//
// rtk wins on test runners because it makes a judgement sqz cannot: passing tests
// do not need to be reported. Failures are preserved verbatim, with file:line.
func (r *Runtime) rtkCommandFilter(ctx context.Context) tools.ShellCommandFilter {
	// Per-workspace override (WSSettings.ShellCommandRewrite), mirroring the sqz
	// gate: off disables it outright; on forces it regardless of hooks; auto
	// (default) requires the rtk hook opt-in — so removing the hook turns it off.
	switch r.shellRewriteMode.Load() {
	case shellCompressOff:
		return nil
	case shellCompressOn:
		// forced on — skip the hook-detection gate; the binary check below still applies.
	default:
		if !r.detectTokenOptimizers(ctx).rtk {
			return nil // no rtk hook wired and not forced on
		}
	}
	rtkPath, err := exec.LookPath("rtk")
	if err != nil {
		r.logger.Warn("rtk command filter: binary not on PATH; shell commands not rewritten", "error", err)
		return nil
	}
	return func(cmd string) (string, *tools.ShellOptimization) {
		if !rtkRewriteWorthIt(cmd) {
			return "", nil
		}
		runCtx, cancel := context.WithTimeout(ctx, rtkRewriteTimeout)
		defer cancel()
		// `rtk rewrite` is rtk's own entry point for hooks ("single source of
		// truth"), so the supported command set stays rtk's business rather than a
		// list we have to chase.
		//
		// The EXIT CODE is deliberately ignored. `rtk rewrite --help` documents
		// "Exits 0 and prints the rewritten command if supported", but rtk actually
		// exits 3 on success while printing a perfectly good rewrite (verified on
		// 0.42.4 and again on 0.44.1: `rtk rewrite "go test ./..."` → stdout
		// `rtk go test ./...`, exit 3). Gating on the documented code silently
		// disabled the whole feature. The OUTPUT is validated instead — it must be a
		// non-empty, changed, rtk-prefixed command — which is the property we
		// actually depend on and which cannot drift with rtk's exit-code conventions.
		c := proc.CommandContext(runCtx, rtkPath, "rewrite", cmd)
		var out, errBuf bytes.Buffer
		c.Stdout = &out
		c.Stderr = &errBuf
		_ = c.Run()
		rewritten := strings.TrimSpace(out.String())
		// Defence in depth: a rewrite must still be an rtk invocation and must
		// actually differ. Anything else means rtk returned something we did not
		// expect, and running an unrecognised command on the user's machine on the
		// strength of a parse is not a trade worth making.
		//
		// For a `cd <dir> && …` command rtk preserves the prefix and wraps only the
		// tail (verified: `cd /tmp && grep -rn foo .` → `cd /tmp && rtk grep -rn foo .`),
		// so the expected prefix is the SAME cd clause followed by "rtk ". Requiring
		// the literal cd text back also means a rewrite that altered the directory
		// would be rejected rather than run.
		cdPrefix, _ := splitCdPrefix(cmd)
		want := "rtk "
		if cdPrefix != "" {
			want = cdPrefix + " rtk "
		}
		if rewritten == "" || rewritten == strings.TrimSpace(cmd) || !strings.HasPrefix(rewritten, want) {
			if rewritten != "" {
				r.logger.Warn("rtk rewrite returned an unexpected command; running the original",
					"command", cmd, "rewritten", rewritten, "want prefix", want)
			}
			return "", nil
		}
		return rewritten, &tools.ShellOptimization{Kind: "rtk", Command: rewritten}
	}
}

// rtkRewriteWorthIt reports whether we let rtk rewrite this command.
//
// This is an ALLOWLIST, not a denylist, because `rtk rewrite` happily rewrites
// commands where the result is worse. Measured on this repo (2026-07-28):
//
//	git diff _Docs   7227 tokens raw → sqz 3602 → rtk 6429   (rtk loses to sqz)
//	cat big.txt      172200 chars    → rtk read 176201       (rtk is BIGGER: it adds line numbers)
//
// So the set is limited to families where rtk demonstrably wins — test/build/lint
// runners, whose verbose success output is exactly what it collapses — plus the
// two git subcommands measured as wins. A family absent from this list is not a
// judgement that rtk would fail on it; it means nobody has measured it yet, and
// the honest default for an unmeasured rewrite is to leave the command alone.
//
// To extend it: run the command three ways (raw / sqz / rtk), compare token counts,
// and add the family only if rtk (or rtk+sqz) wins. See _Docs/17.
func rtkRewriteWorthIt(cmd string) bool {
	_, cmd = splitCdPrefix(cmd)
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	// A command line with shell operators may run several programs; the leading
	// token no longer describes what happens, and rtk would wrap the whole string.
	// Skip those rather than reason about them.
	//
	// This is checked AFTER the `cd … &&` prefix is stripped, because that prefix is
	// how agents overwhelmingly write commands: in WS10/SES63, 10 of 11 shell calls
	// began with `cd <dir> &&`. Rejecting the whole line on its `&&` meant rtk could
	// never fire for such an agent — the measured 99% saving on `go test` simply
	// never materialised. A redirect or pipe in the REMAINDER is still refused: `cd x
	// && cmd > file` would otherwise write rtk's SUMMARY into the file.
	//
	// `2>&1` is exempt, and only it. It MERGES streams rather than writing anywhere,
	// so wrapping is harmless, and rtk preserves the token verbatim (verified:
	// `go test ./x 2>&1` → `rtk go test ./x 2>&1`). Agents append it reflexively —
	// a live turn on 2026-07-31 lost the rewrite for exactly this reason — so
	// refusing it would keep rtk off a large share of real commands for no safety
	// gained. Every other redirection form stays refused.
	if strings.ContainsAny(stripMergeStderr(cmd), "|&;><`") || strings.Contains(cmd, "$(") {
		return false
	}
	program := strings.ToLower(strings.TrimSuffix(fields[0], ".exe"))
	if rtkWrapperPrograms[program] {
		return false
	}
	if program == "git" {
		return len(fields) > 1 && rtkGitSubcommands[strings.ToLower(fields[1])]
	}
	return rtkRunnerPrograms[program]
}

// stripMergeStderr removes standalone `2>&1` tokens so the operator scan does not
// trip over them. Only the exact token is dropped, as a whole word: `2>&1x`,
// `2>file` and `>&1` keep their operator characters and are still refused.
func stripMergeStderr(cmd string) string {
	fields := strings.Fields(cmd)
	kept := fields[:0]
	for _, f := range fields {
		if f != "2>&1" {
			kept = append(kept, f)
		}
	}
	return strings.Join(kept, " ")
}

// splitCdPrefix separates a leading `cd <dir> &&` from the command it guards,
// returning the prefix (including the "&&" and its trailing space) and the rest.
// When there is no such prefix it returns ("", cmd).
//
// This one shape is worth special-casing because it is how agents actually write
// commands — they re-establish the directory on every call rather than trusting
// the tool's cwd. Only ONE level is unwrapped and only for `cd`: anything else
// (`a && b`, `cd x && cd y && z`) leaves an operator in the remainder and is
// rejected by the caller.
func splitCdPrefix(cmd string) (prefix, rest string) {
	i := strings.Index(cmd, "&&")
	if i < 0 {
		return "", cmd
	}
	head := strings.TrimSpace(cmd[:i])
	rest, ok := strings.CutPrefix(strings.ToLower(head), "cd ")
	if !ok {
		return "", cmd
	}
	// Exactly `cd <one-directory>`. The argument must be a SINGLE shell word, but a
	// Windows path routinely contains spaces, so a fully-quoted argument counts as
	// one word — `cd "/c/My Proj" && npm run build` is an ordinary command here, and
	// splitting it on whitespace would have rejected it. Anything else (bare `cd`,
	// `cd a b`) is not the shape being matched and must not be reinterpreted.
	dir := strings.TrimSpace(head[len(head)-len(rest):])
	quoted := len(dir) >= 2 && (dir[0] == '"' && dir[len(dir)-1] == '"' || dir[0] == '\'' && dir[len(dir)-1] == '\'')
	if dir == "" || (!quoted && strings.ContainsAny(dir, " \t")) {
		return "", cmd
	}
	return cmd[:i+2], strings.TrimSpace(cmd[i+2:])
}

// rtkRunnerPrograms are the test/build runners whose verbose output rtk collapses
// to a failures-only summary. They share one shape: a long, mostly uninteresting
// success log, and a short interesting failure.
//
// Measured wins (2026-07-28, rtk 0.44.1, tiktoken o200k_base — see scripts/rtk_eval.py):
//
//	go test -v ./internal/tools   6107 → rtk 12     (%99)
//	cargo test -p sample-rules   929 → rtk 16     (%98)
//	pytest -v                     3638 → rtk 137    (%59 vs sqz's best)
//	npm run build                  932 → rtk+sqz 404 (%54)
//
// Every entry below is either measured above or a DIRECT tool invocation of the
// same shape. What is deliberately absent is the class that broke — see
// rtkWrapperPrograms.
//
// WATCH — npm: rtk-ai/rtk#950 (OPEN) reports that on Windows rtk cannot spawn the
// .cmd/shell wrappers Node ships, naming npm among them. Applying that list
// literally would have removed npm too; it stays on two pieces of evidence the
// issue does not override here:
//
//  1. It did not reproduce. `rtk npm run build` really built — exit 0, 4497
//     modules, full vite output — and the 824 → 404 saving reproduced with the
//     dedup cache cleared before every measurement.
//  2. Its failure mode is LOUD. rtk's npm filter passes output through rather
//     than summarising it (rtk alone: 924 tokens vs 842 raw — it does not shrink
//     anything), so it has no counter that could invent a number the way the lint
//     filter reported "2 errors" for a 90-error run. If #950 bites, the command
//     simply fails to spawn: non-zero exit, Degraded note, visible.
//
// The saving is also NOT something we can capture without rtk. The obvious theory
// — that rtk strips ANSI and that is what helps sqz — was tested and refuted:
// stripping ANSI from the raw output ourselves changed nothing (824 → 821), while
// rtk's LARGER output still compressed to 404. Whatever sqz finds in rtk's
// formatting, it is not reproducible by us.
//
// Re-check with scripts/rtk_eval.py after an rtk or Node upgrade.
var rtkRunnerPrograms = map[string]bool{
	"go": true, "cargo": true, "dotnet": true, "mvn": true, "gradlew": true,
	"npm": true, "jest": true, "vitest": true,
	"playwright": true, "next": true,
	"pytest": true, "mypy": true, "ruff": true,
	"rspec": true, "rake": true, "rubocop": true,
	"golangci-lint": true,
}

// rtkWrapperPrograms documents the commands REMOVED from the allowlist after
// measurement, so nobody re-adds them on the "surely a lint runner qualifies"
// intuition that put them there in the first place.
//
// They share a failure mode: each is a DISPATCHER that must resolve which
// underlying tool to run, and rtk resolves it differently than the shell would.
//
//   - npx / lint / prettier / format — rtk drops the wrapper. `npx eslint src`
//     became `rtk lint src`, which could not find the project's local eslint and
//     reported "Lint: 2 errors, 0 warnings" for a run with 90 REAL errors. It is
//     wrong, not merely lossy: an agent reads that as an almost-clean codebase.
//     (Exit code was non-zero, so the Degraded guard fires — but a plausible wrong
//     number is exactly what the allowlist exists to keep out.) The live upstream
//     cause is rtk-ai/rtk#950 (OPEN): Windows cannot spawn the .cmd wrappers Node
//     ships. Our failure printed "npm error could not determine executable to run",
//     which is that issue's signature. (#1080, npx of an unregistered package
//     becoming `npm run` → ENOENT, is CLOSED since 2026-04-26 and is NOT why npx is
//     excluded — cited here only so a future reader does not re-derive it.)
//   - uv — rtk drops the interpreter: `uv pip list` became `rtk pip list`, i.e. the
//     SYSTEM python instead of uv's environment. A different answer, not a shorter
//     one. Known upstream: rtk-ai/rtk#1205, #294.
//   - tsc — direct, not a dispatcher, but it simply did not earn a place: 4500 raw →
//     sqz 1832 → rtk 4534 (worse than raw alone) → rtk+sqz 1700. A 7% gain over sqz
//     is inside the noise, and sqz already handles it.
//
// Also evaluated and not added: `pip list` (2352 → rtk 2344, no benefit),
// `git checkout` (mutating, trivial output), php/sbt (not installable here, and an
// unmeasured family is left alone by policy).
//
// tsserver, corepack and pnpm are here on UPSTREAM's word rather than our own
// measurement: rtk-ai/rtk#950 names them (with npx and tsc) as Windows .cmd
// wrappers rtk cannot spawn.
//
// pnpm is the interesting one, and the rule it illustrates is worth stating: it
// was in the allowlist purely because it is "npm-shaped". It has never been
// measured here — it is not even installed — so the only evidence about it is
// upstream's, and that evidence says broken. An unmeasured entry with contrary
// evidence has nothing left holding it up. npm survives the same issue ONLY
// because direct measurement contradicts it (see rtkRunnerPrograms).
var rtkWrapperPrograms = map[string]bool{
	"npx": true, "lint": true, "prettier": true, "format": true, "uv": true,
	"tsc": true, "pip": true, "tsserver": true, "corepack": true, "pnpm": true,
}

// rtkGitSubcommands are the git subcommands measured as rtk wins. `diff` is
// deliberately absent — sqz compresses diffs almost twice as well.
var rtkGitSubcommands = map[string]bool{
	"status": true,
	"log":    true,
}
