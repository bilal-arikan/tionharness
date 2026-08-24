package agent

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestRtkRewriteWorthIt locks the ALLOWLIST. `rtk rewrite` will happily rewrite
// commands where the result is measurably worse, so trusting it blindly would
// regress two families we have numbers for.
func TestRtkRewriteWorthIt(t *testing.T) {
	allowed := []string{
		"go test ./...",
		"go build ./...",
		"cargo test",
		"npm run build",
		"pytest -q",
		"GOLANGCI-LINT run", // program match is case-insensitive
		"go.exe test ./...", // and .exe-suffix tolerant on Windows
		"git status",
		"git log -30",
		// The `cd <dir> &&` prefix is how agents overwhelmingly write commands —
		// 10 of 11 shell calls in WS10/SES63. Rejecting the line on its `&&` meant
		// rtk never fired for such an agent. rtk itself handles the shape, wrapping
		// only the tail.
		"cd /c/proj && go test ./...",
		"cd /c/proj && cargo test",
		`cd "/c/My Proj" && npm run build`,
		"cd /c/proj && git status",
		// Agents append `2>&1` reflexively — a live turn on 2026-07-31 lost the
		// rewrite to it. It merges streams rather than writing anywhere, and rtk
		// preserves the token, so it is the one redirection form allowed through.
		"go test ./... 2>&1",
		"cd /c/proj && go test ./... 2>&1",
		"cargo test 2>&1",
	}
	for _, cmd := range allowed {
		if !rtkRewriteWorthIt(cmd) {
			t.Errorf("expected %q to be eligible for rtk rewrite", cmd)
		}
	}

	skipped := map[string]string{
		"":                         "empty command",
		"git diff":                 "measured: rtk 12556 tokens vs sqz 7485 — sqz wins on diffs",
		"git diff --stat":          "same family as git diff",
		"cat big.txt":              "measured: rtk read returns MORE bytes than cat (adds line numbers)",
		"ls -la":                   "not measured; unmeasured families are left alone",
		"echo hello":               "nothing to optimize",
		"grep -r foo .":            "not measured",
		"go test ./... | tee":      "shell operators — the leading token no longer describes what runs",
		"go test $(pkg)":           "command substitution",
		"cd /tmp && go test > out": "redirect in the tail: rtk's SUMMARY would be written into the file",
		"cd /tmp && go test | tee": "pipe in the tail",
		"cd a && cd b && go test":  "two cd levels leave an operator in the remainder",
		"cd /tmp && git diff":      "tail is eligible-shaped but git diff is excluded on merit",
		"cd && go test":            "bare cd is not the shape being matched",
		"cd /tmp /x && go test":    "cd with extra words is not the shape being matched",
		"pushd /tmp && go test":    "only `cd` is unwrapped",
		// Only the exact `2>&1` word is exempt; every other redirection still writes
		// somewhere, where rtk's SUMMARY would land instead of the real output.
		"go test ./... 2>err.txt":       "stderr to a FILE, not a stream merge",
		"go test ./... > out 2>&1":      "the stdout redirect is still a file write",
		"go test ./... 2>&1x":           "not the standalone token",
		"cd /c/p && go test 2>&1 > out": "same, behind a cd prefix",
	}
	for cmd, why := range skipped {
		if rtkRewriteWorthIt(cmd) {
			t.Errorf("expected %q to be skipped (%s)", cmd, why)
		}
	}
}

// TestRtkWrapperProgramsExcluded locks the removals that MEASUREMENT forced, not
// taste. Each of these looks like an obvious allowlist candidate — which is how
// they got added — and each was demonstrated to make things worse or wrong.
func TestRtkWrapperProgramsExcluded(t *testing.T) {
	wrappers := map[string]string{
		"npx eslint src":       "rtk drops npx; `rtk lint` missed the local eslint and reported 2 errors for a 90-error run",
		"npx vitest run":       "same npx drop",
		"lint src":             "rtk lint mis-resolved the linter and reported a WRONG count",
		"prettier --check .":   "same universal-wrapper family as lint",
		"format .":             "same",
		"uv pip list":          "rtk drops uv → `rtk pip list` queries the SYSTEM python, a different answer",
		"uv run pytest":        "same first token; rtk handles the uv-run form itself when its own hook is used",
		"tsc --noEmit":         "measured neutral: rtk 4534 vs 4500 raw; sqz alone already gets 1832",
		"pip list":             "measured: 2352 → 2344, no benefit",
		"pip install requests": "same family",
		// From rtk-ai/rtk#950's own list of Windows .cmd wrappers rtk cannot spawn.
		"tsserver --version": "upstream #950: Windows .cmd wrapper",
		"corepack enable":    "upstream #950: Windows .cmd wrapper",
		// pnpm was allowlisted only for being "npm-shaped" and was never measured
		// here (not installed), so upstream's evidence is the only evidence there is.
		"pnpm run build": "upstream #950 + never measured — nothing holds it up",
		"pnpm test":      "same",
	}
	for cmd, why := range wrappers {
		if rtkRewriteWorthIt(cmd) {
			t.Errorf("expected %q to stay OUT of the allowlist (%s)", cmd, why)
		}
	}

	// The exclusion must be by PROGRAM, so it cannot be sidestepped by an argument
	// that happens to name an allowlisted runner.
	if rtkRewriteWorthIt("npx jest --ci") {
		t.Error("npx must be excluded even when its argument is an allowlisted runner")
	}
	// ...and it must not leak onto the direct invocation of that same runner.
	if !rtkRewriteWorthIt("jest --ci") {
		t.Error("a DIRECT runner invocation must still be eligible")
	}

	// npm is the ONE member of #950's list that survives, because measurement
	// contradicts the issue here: `rtk npm run build` really builds, and the
	// 824 → 404 saving reproduces. Locking it prevents a future "apply the
	// upstream list literally" pass from silently dropping a verified win.
	if !rtkRewriteWorthIt("npm run build") {
		t.Error("npm must stay eligible: measured working here, with a reproduced saving")
	}
}

// TestRtkCommandFilter_Gate mirrors the sqz gate: opt-in by hook, per-workspace
// override, and never a broken filter when the binary is missing.
func TestRtkCommandFilter_Gate(t *testing.T) {
	r := lifecycleRuntime(t)
	ctx := context.Background()

	if f := r.rtkCommandFilter(ctx); f != nil {
		t.Fatal("rtkCommandFilter must be nil when no rtk hook is wired")
	}

	// Opt in the way the Hooks panel does: a PreToolUse hook that prefixes `rtk `.
	seedHook(t, r, db.HookPreToolUse, "Bash",
		`$j=[Console]::In.ReadToEnd()|ConvertFrom-Json; $c=$j.tool_input.command; if($c -and -not ($c -like 'rtk *')){ $j.tool_input.command='rtk '+$c }`)
	f := r.rtkCommandFilter(ctx)

	if _, err := exec.LookPath("rtk"); err != nil {
		if f != nil {
			t.Fatal("with rtk opted-in but not on PATH, the filter must be nil (never broken)")
		}
		t.Skip("rtk not on PATH; live-rewrite path not exercised")
	}
	if f == nil {
		t.Fatal("with the rtk hook wired and rtk on PATH, the filter must be non-nil")
	}

	// Live: an allowlisted runner is rewritten to an rtk invocation...
	rewritten, opt := f("go test ./...")
	if !strings.HasPrefix(rewritten, "rtk ") {
		t.Fatalf("expected an rtk invocation, got %q", rewritten)
	}
	if opt == nil || opt.Kind != "rtk" || opt.Command != rewritten {
		t.Fatalf("expected the rewrite to be reported for the UI chip, got %+v", opt)
	}
	// ...and a command outside the allowlist is left exactly as typed, even though
	// `rtk rewrite` itself would offer a replacement for it.
	if got, gotOpt := f("git diff"); got != "" || gotOpt != nil {
		t.Fatalf("git diff must be left alone (sqz compresses diffs better), got %q / %+v", got, gotOpt)
	}

	// The `cd <dir> &&` form must survive END TO END against the real binary: rtk
	// keeps the prefix and wraps only the tail, and our prefix validation has to
	// accept exactly that. This is the shape agents actually emit (10 of 11 calls
	// in WS10/SES63), so a mismatch here silently disables rtk in practice while
	// every unit test still passes.
	cdCmd := "cd /c/proj && go test ./..."
	cdOut, cdOpt := f(cdCmd)
	if !strings.HasPrefix(cdOut, "cd /c/proj && rtk ") {
		t.Fatalf("expected the cd prefix preserved and only the tail wrapped, got %q", cdOut)
	}
	if cdOpt == nil || cdOpt.Command != cdOut {
		t.Fatalf("the rewritten compound command must be reported for the chip, got %+v", cdOpt)
	}
	// A redirect in the tail must still be refused even behind a cd prefix: rtk's
	// summary would land in the file instead of the command's real output.
	if got, _ := f("cd /c/proj && go test ./... > out.txt"); got != "" {
		t.Fatalf("a redirect in the tail must not be rewritten, got %q", got)
	}

	// Per-workspace override, same tri-state as the sqz filter.
	r.SetShellCommandRewrite("off")
	if r.rtkCommandFilter(ctx) != nil {
		t.Fatal("shellCommandRewrite=off must disable the filter despite the rtk hook")
	}
	r.SetShellCommandRewrite("on")
	if r.rtkCommandFilter(ctx) == nil {
		t.Fatal("shellCommandRewrite=on must enable the filter when rtk is on PATH")
	}
	r.SetShellCommandRewrite("") // restore auto
}

// TestTokenOptimizerGuidance_PerCombination covers the three shapes the prompt
// block can take. The both-active case must explain the interaction, because the
// symptom it produces (a test result with no abbreviation legend) otherwise reads
// as a broken optimizer.
func TestTokenOptimizerGuidance_PerCombination(t *testing.T) {
	rtkOnly := tokenOptimizerGuidance(tokenOptimizerState{rtk: true, matchers: []string{"*"}})
	sqzOnly := tokenOptimizerGuidance(tokenOptimizerState{sqz: true, matchers: []string{"*"}})
	both := tokenOptimizerGuidance(tokenOptimizerState{rtk: true, sqz: true, matchers: []string{"*"}})

	if strings.Contains(rtkOnly, "sqz") {
		t.Errorf("rtk-only block must not describe sqz:\n%s", rtkOnly)
	}
	// The lossy-summary warning is the whole point of the rtk block.
	for _, want := range []string{"rtk", "SUMMARY", "no_compress"} {
		if !strings.Contains(rtkOnly, want) {
			t.Errorf("rtk-only block missing %q:\n%s", want, rtkOnly)
		}
	}

	if strings.Contains(sqzOnly, "rtk") {
		t.Errorf("sqz-only block must not describe rtk:\n%s", sqzOnly)
	}
	// The §ref form is the one that looks like a broken tool, and it has an EXACT
	// recovery (`sqz expand`) — telling the agent to re-run instead would just
	// return the same pointer.
	for _, want := range []string{"sqz", "LOSSLESS", "§ref:", "sqz expand"} {
		if !strings.Contains(sqzOnly, want) {
			t.Errorf("sqz-only block missing %q:\n%s", want, sqzOnly)
		}
	}

	if !strings.Contains(both, "Both are active") {
		t.Errorf("both-active block must explain the interaction:\n%s", both)
	}
	if !strings.Contains(both, "below sqz's size threshold") {
		t.Errorf("both-active block must explain why a short rtk result carries no sqz legend:\n%s", both)
	}
	// Neither single-optimizer block should claim an interaction that isn't there.
	for name, block := range map[string]string{"rtk-only": rtkOnly, "sqz-only": sqzOnly} {
		if strings.Contains(block, "Both are active") {
			t.Errorf("%s block must not claim both optimizers are active:\n%s", name, block)
		}
	}
}
