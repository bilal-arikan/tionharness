package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// Default tool-loop guardrail thresholds (external-context-agent tool_guardrails parity).
// Warnings never prevent execution; block/halt fire only when the hard stop is
// explicitly enabled, so interactive sessions get a gentle nudge by default.
const (
	DefaultGuardExactWarnAfter    = 2 // identical failing call → warn
	DefaultGuardExactBlockAfter   = 5 // identical failing call → block (hard stop)
	DefaultGuardSameToolWarnAfter = 3 // same tool consecutive failures → warn
	DefaultGuardSameToolHaltAfter = 8 // same tool consecutive failures → halt turn (hard stop)
	DefaultGuardNoProgressWarn    = 2 // identical successful idempotent repeats → warn
	DefaultGuardNoProgressBlock   = 5 // identical successful idempotent repeats → block (hard stop)
)

// toolGuardConfig is the per-turn guardrail policy, resolved from tunables.
type toolGuardConfig struct {
	warnings bool // append recovery guidance to failing results (default on)
	hardStop bool // block/halt on the upper thresholds (opt-in circuit breaker)
}

// guardVerdict is what the pre-execution check tells the loop to do with a call.
type guardVerdict int

const (
	guardAllow guardVerdict = iota
	guardBlock              // skip execution, feed a synthetic error result
	guardHalt               // end the whole turn after this batch (controlled)
)

// toolGuard tracks per-turn tool-call observations and decides when the model
// is looping. Side-effect free by design (the hermes tool_guardrails pattern):
// it only counts and answers questions; the loop owns turning verdicts into
// synthetic results, warning hints, or a controlled halt.
type toolGuard struct {
	cfg        toolGuardConfig
	exactFail  map[string]int // name+input hash → consecutive identical failures
	sameFail   map[string]int // name → consecutive failures (any args)
	noProgress map[string]int // name+input hash → identical successful idempotent repeats
}

func newToolGuard(cfg toolGuardConfig) *toolGuard {
	return &toolGuard{
		cfg:        cfg,
		exactFail:  map[string]int{},
		sameFail:   map[string]int{},
		noProgress: map[string]int{},
	}
}

// callKey collapses a tool call to a stable identity: name + input digest.
func callKey(call providers.ToolCall) string {
	sum := sha256.Sum256(call.Input)
	return call.Name + ":" + hex.EncodeToString(sum[:8])
}

// check runs BEFORE a call executes. With the hard stop disabled it always
// allows (warnings ride the result text via observe). With it enabled, a call
// that already crossed a block threshold is refused without executing, and a
// tool past the halt threshold ends the turn after this batch.
func (g *toolGuard) check(call providers.ToolCall) (verdict guardVerdict, reason string) {
	if !g.cfg.hardStop {
		return guardAllow, ""
	}
	if g.sameFail[call.Name] >= DefaultGuardSameToolHaltAfter {
		return guardHalt, fmt.Sprintf("%s failed %d times in a row this turn", call.Name, g.sameFail[call.Name])
	}
	key := callKey(call)
	if g.exactFail[key] >= DefaultGuardExactBlockAfter {
		return guardBlock, fmt.Sprintf("this exact %s call already failed %d times this turn", call.Name, g.exactFail[key])
	}
	if tools.Classify(call.Name) == tools.RiskRead && g.noProgress[key] >= DefaultGuardNoProgressBlock {
		return guardBlock, fmt.Sprintf("this identical %s call already succeeded %d times this turn (no new information)", call.Name, g.noProgress[key])
	}
	return guardAllow, ""
}

// observe runs AFTER a call executed (or was blocked) and updates the counters.
// It returns a non-empty recovery hint when warnings are enabled and the call
// just crossed a warning threshold — the loop appends it to the tool result so
// the model reads the guidance exactly where the failure happened.
func (g *toolGuard) observe(call providers.ToolCall, res providers.ToolResult) (hint string) {
	key := callKey(call)
	if res.IsError {
		g.exactFail[key]++
		g.sameFail[call.Name]++
		delete(g.noProgress, key)
		if !g.cfg.warnings {
			return ""
		}
		if g.exactFail[key] >= DefaultGuardExactWarnAfter {
			return exactFailureHint(call.Name, g.exactFail[key])
		}
		if g.sameFail[call.Name] >= DefaultGuardSameToolWarnAfter {
			return sameToolFailureHint(call.Name, g.sameFail[call.Name])
		}
		return ""
	}
	// Success: the failure streaks end here.
	delete(g.exactFail, key)
	g.sameFail[call.Name] = 0
	// Identical successful repeats of an idempotent (read-class) tool return the
	// same data — re-reading is a loop, not progress.
	if tools.Classify(call.Name) == tools.RiskRead {
		g.noProgress[key]++
		if g.cfg.warnings && g.noProgress[key] > DefaultGuardNoProgressWarn {
			return noProgressHint(call.Name, g.noProgress[key])
		}
	}
	return ""
}

// blockedResultMsg is the synthetic error result for a call the guardrail
// refused to execute (hard stop enabled, block threshold crossed).
func blockedResultMsg(reason string) string {
	return "blocked by loop guardrail: " + reason + ". Do not repeat this call. " +
		"Change your approach: different arguments, a different tool, or report the blocker."
}

// Recovery hints are model-facing (English) and action-oriented: diagnose
// before retrying, vary the approach, never fall back to text-only replies.

func exactFailureHint(tool string, count int) string {
	return fmt.Sprintf(
		"\n\n[loop guardrail] This exact %s call has failed %d times this turn. "+
			"Do not repeat it unchanged. Inspect the error above, verify your assumptions "+
			"(paths, arguments, preconditions), then try different arguments or a different tool. "+
			"Keep using tools — do not switch to a text-only reply.", tool, count)
}

func sameToolFailureHint(tool string, count int) string {
	base := fmt.Sprintf(
		"\n\n[loop guardrail] %s has failed %d times this turn. This looks like a loop. "+
			"Diagnose before retrying: inspect the latest error and verify your assumptions first. ",
		tool, count)
	switch tool {
	case "Bash", "PowerShell", "shell":
		return base + "Run a small diagnostic (pwd / ls) in the same tool, then try an absolute path, " +
			"a simpler command, a different working directory, or a file tool (Read/Write/Edit) instead."
	default:
		return base + "Try different arguments, a narrower query/path, an absolute path when relevant, " +
			"or a different tool that can make progress. If the blocker is external, report it " +
			"after one diagnostic attempt instead of repeating the same failing call."
	}
}

func noProgressHint(tool string, count int) string {
	return fmt.Sprintf(
		"\n\n[loop guardrail] This identical %s call has now succeeded %d times this turn with the "+
			"same result. Re-reading the same data is not progress — act on what you already have, "+
			"or query something new.", tool, count)
}
