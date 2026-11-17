package agent

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// --- token-optimizer capability (rtk / sqz) ---------------------------------
//
// Mirrors the codebase-memory capability: when an OPTIONAL external token
// optimizer is wired into the workspace, inject a short context block so the
// agent knows it exists and how it changes the tool I/O it sees. Two optimizers
// are recognized, both driven by tool hooks (never invoked by the agent):
//   - rtk — a PreToolUse hook that rewrites shell commands to run through the
//     `rtk` proxy, shrinking the output of common dev commands.
//   - sqz — a hook that pipes tool I/O through the `sqz` compressor, so the tool
//     output the agent sees may be summarized/shortened.
//
// Detection is by the hook COMMAND (what actually runs), NOT by a binary merely
// on PATH: a tool on PATH does nothing until a hook wires it in, so probing PATH
// would over-claim. \b anchors avoid matching the markers inside other words.

var (
	rtkHookMarkerRe = regexp.MustCompile(`\brtk\b`)
	sqzHookMarkerRe = regexp.MustCompile(`\bsqz\b`)
)

// tokenOptimizerState reports which optimizers are wired as enabled hooks and the
// distinct matcher patterns of the hooks that carry them (the tools they cover).
type tokenOptimizerState struct {
	rtk      bool
	sqz      bool
	matchers []string
}

func (s tokenOptimizerState) present() bool { return s.rtk || s.sqz }

// detectTokenOptimizers scans this workspace's enabled Pre/PostToolUse hooks for
// the rtk and sqz markers. Errors are LOGGED (not silently swallowed) and skipped,
// so a partial read never masks a present optimizer as absent without a trace.
func (r *Runtime) detectTokenOptimizers(ctx context.Context) tokenOptimizerState {
	var st tokenOptimizerState
	seen := map[string]bool{}
	for _, ev := range []string{db.HookPreToolUse, db.HookPostToolUse} {
		hooks, err := r.db.ListEnabledHooksByEvent(ctx, ev)
		if err != nil {
			r.logger.Warn("token-optimizer probe: list hooks failed", "event", ev, "error", err)
			continue
		}
		for _, h := range hooks {
			cmd := strings.ToLower(h.Command)
			hit := false
			if rtkHookMarkerRe.MatchString(cmd) {
				st.rtk = true
				hit = true
			}
			if sqzHookMarkerRe.MatchString(cmd) {
				st.sqz = true
				hit = true
			}
			if !hit {
				continue
			}
			m := strings.TrimSpace(h.Matcher)
			if m == "" {
				m = "*" // an empty matcher fires for every tool
			}
			if !seen[m] {
				seen[m] = true
				st.matchers = append(st.matchers, m)
			}
		}
	}
	slices.Sort(st.matchers)
	return st
}

var tokenOptimizerCapability = Capability{
	ID: "token-optimizers",
	Detect: func(ctx context.Context, r *Runtime, _ db.Agent) bool {
		return r.detectTokenOptimizers(ctx).present()
	},
	Context: func(ctx context.Context, r *Runtime, cwd, provider string) string {
		return tokenOptimizerGuidance(r.detectTokenOptimizers(ctx))
	},
}

// tokenOptimizerGuidance renders the injected block for the detected optimizers.
// Returns "" when none are present (safe no-op for the prompt assembler).
func tokenOptimizerGuidance(st tokenOptimizerState) string {
	if !st.present() {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Token optimization active\n")
	b.WriteString("This workspace runs your shell calls through external token-optimizers. " +
		"They are applied FOR you — never invoke them yourself, and never pre-truncate a command to save tokens:\n")
	if st.rtk {
		b.WriteString(rtkGuidance)
	}
	if st.sqz {
		b.WriteString(sqzGuidance)
		b.WriteString(sqzOutputFormsGuidance)
	}
	// The two act at OPPOSITE ends of the same call, and their interaction produces
	// a result that looks like a malfunction if unexplained (see below).
	if st.rtk && st.sqz {
		b.WriteString(rtkSqzInteractionGuidance)
	}
	if len(st.matchers) > 0 && !tokenOptimizerCoversAll(st.matchers) {
		b.WriteString("Scope: these hooks match the " + humanJoinMatchers(st.matchers) +
			" tool(s) ONLY — commands run through other shell tools are NOT optimized. Prefer the matched tool for token-heavy commands (logs, greps, diffs, test runs) so the optimization applies.\n")
	}
	b.WriteString("Run the commands you actually need and let the optimizer trim the output. " +
		"When you need the byte-exact raw output of an rtk-rewritten command, run it as `rtk proxy <command>`.")
	return b.String()
}

// rtkGuidance describes the COMMAND-layer optimizer. The critical fact for the
// agent is that rtk is LOSSY BY DESIGN — it drops passing tests — so a short
// result is the feature working, not the command failing to run.
const rtkGuidance = "- rtk (command layer) — for test/build/lint runners and `git status`/`git log`, the command is " +
	"rewritten to run through the `rtk` proxy before it executes. You get a SUMMARY, not the raw log: " +
	"`go test ./...` comes back as \"226 passed in 1 packages\" instead of hundreds of lines. This is " +
	"deliberate loss of PASSING detail — failures are preserved in full, with file:line and the assertion " +
	"message. A short result means everything passed; it does NOT mean the command did not run.\n" +
	"  If a rewritten command FAILS and the summary does not explain why, re-run that same command as " +
	"`rtk proxy <command>` for the raw output rather than guessing at the cause.\n"

// sqzGuidance describes the OUTPUT-layer optimizer.
const sqzGuidance = "- sqz (output layer) — after a command runs, output above ~2KB is passed through the `sqz` " +
	"compressor, so what you read may be abbreviated. This is LOSSLESS: nothing is dropped, only encoded " +
	"more compactly. It is expected optimization, NOT truncation and NOT data loss.\n"

// rtkSqzInteractionGuidance is the both-active case. It exists for one concrete
// failure mode: rtk shrinks a test run so far that the result lands under sqz's
// size threshold, so sqz never runs and no abbreviation legend appears. An agent
// told "output is compressed" but shown uncompressed output can conclude the
// optimizer broke and start working around a problem that does not exist.
const rtkSqzInteractionGuidance = "  Both are active, at opposite ends of the same call: rtk shapes the " +
	"COMMAND before it runs, sqz compresses the OUTPUT after. They stack, and neither replaces the other — " +
	"rtk wins on test runners (it knows passing tests are noise), sqz wins on diffs and file dumps (it " +
	"abbreviates any repetitive text). Expect the two to appear in different places: once rtk has reduced a " +
	"test run to a few lines, that result is below sqz's size threshold, so you will see NO abbreviation " +
	"legend on it. That is the pipeline working, not sqz failing.\n"

// sqzOutputFormsGuidance names the two shapes sqz output actually arrives in.
// Both are easy to misread as a broken tool, and the recovery (`sqz expand`) is
// only obvious once you know the convention:
//
//   - the abbreviation legend, which is self-describing once you see the header;
//   - a bare `§ref:<hash>§`, which is NOT — it replaces the ENTIRE result when the
//     command reproduced output sqz already compressed this session. An agent that
//     reads it as an empty/failed result re-runs the command, gets the identical
//     ref back, and can loop.
const sqzOutputFormsGuidance = "  Compressed output arrives in two shapes. " +
	"(1) An `[Abbreviations]` header mapping `«A1»`-style placeholders to the text they replace — " +
	"substitute them back as you read; nothing is lost. " +
	"(2) A bare `§ref:<hash>§` as the WHOLE result: this command reproduced output that appeared " +
	"EARLIER IN THIS SESSION, so sqz replaced it with a pointer instead of repeating it. " +
	"To get those bytes back, run `sqz expand '§ref:<hash>§'` — it prints the original in full. " +
	"Do NOT re-run the command hoping for different output: it returns the same pointer.\n"

// tokenOptimizerCoversAll reports whether the matcher set already fires for every
// tool (a "*" or empty matcher), in which case the scope note is pointless.
func tokenOptimizerCoversAll(matchers []string) bool {
	for _, m := range matchers {
		if m == "*" || m == "" {
			return true
		}
	}
	return false
}

// humanJoinMatchers renders matcher patterns as `a`, `b` and `c` for prose.
func humanJoinMatchers(matchers []string) string {
	quoted := make([]string, len(matchers))
	for i, m := range matchers {
		quoted[i] = "`" + m + "`"
	}
	switch len(quoted) {
	case 1:
		return quoted[0]
	case 2:
		return quoted[0] + " and " + quoted[1]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
	}
}
