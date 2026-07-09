package agent

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
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
	sort.Strings(st.matchers)
	return st
}

var tokenOptimizerCapability = Capability{
	ID: "token-optimizers",
	Detect: func(ctx context.Context, r *Runtime) bool {
		return r.detectTokenOptimizers(ctx).present()
	},
	Context: func(ctx context.Context, r *Runtime, cwd string) string {
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
	b.WriteString("This workspace wires external token-optimizers as tool hooks; they run automatically — you never invoke them:\n")
	if st.rtk {
		b.WriteString("- rtk — a PreToolUse hook routes shell commands through the `rtk` proxy, shrinking the output of common dev commands (git status/log/diff, tests, builds, greps).\n")
	}
	if st.sqz {
		b.WriteString("- sqz — a hook runs tool I/O through the `sqz` compressor, so tool output you see may be summarized/shortened. That is expected optimization, NOT truncation or data loss.\n")
	}
	if len(st.matchers) > 0 && !tokenOptimizerCoversAll(st.matchers) {
		b.WriteString("Scope: these hooks match the " + humanJoinMatchers(st.matchers) +
			" tool(s) ONLY — commands run through other shell tools are NOT optimized. Prefer the matched tool for token-heavy commands (logs, greps, diffs, test runs) so the optimization applies.\n")
	}
	b.WriteString("Run the commands you actually need and let the optimizer trim the output — do not avoid useful commands or pre-truncate to save tokens.")
	return b.String()
}

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
