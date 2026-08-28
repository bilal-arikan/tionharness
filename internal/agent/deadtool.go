package agent

import (
	"context"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// deadToolMarker is the lowercase substring every "the tool you named does not
// exist" rejection carries, on both sides of the claude-cli boundary:
//
//   - the CLI's own registry: "No such tool available: X. X exists but is not
//     enabled in this context."
//   - TionHarness's Interaction MCP backend: "No such tool available: <name>. Use
//     tool_search(...)"
//
// It is matched case-insensitively, so the constant is kept lowercase and shared
// with autotag.go's permissionDenyMarkers — one spelling of the marker, two
// consumers (classification there, repair here).
const deadToolMarker = "no such tool available"

// deadToolDebugName is the debug-journal Name stamped on a dead-tool repair, so
// the episode is greppable in debug.jsonl next to the other guardrail entries.
const deadToolDebugName = "dead_tool_activate"

// The failure this repairs (WS20/SES79): on the claude-cli path the extended
// Interaction tier starts EMPTY and grows only as the model calls activate_tools.
// When the model instead calls a deferred tool straight away — e.g.
// mcp__tionharness_extended__list_tasks — the CLI rejects it from its OWN registry,
// before the call ever reaches TionHarness's gateway. Nothing server-side observes a
// call, so nothing activates the tool, and the model reads a flat "No such tool
// available". In SES79 it re-issued the identical call for three turns and the work
// stopped.
//
// TionHarness DOES see the rejection: it arrives as an errored tool step on the CLI's
// stream-json trace. deadToolRepair reads that step, activates the named tool
// server-side (which pushes tools/list_changed so the CLI re-lists it), and appends
// an instruction telling the model to re-issue the same call by the same name.

// DeadToolActivator activates one on-demand (deferred) Interaction MCP tool for a
// LIVE turn, addressed by the turn's per-run Bearer token. bareName is the tool
// name with its mcp__<server>__ prefix already stripped.
//
// It reports whether the name is really in that run's on-demand catalog. false
// means "not ours" — an unknown or misspelled tool, whose error must reach the
// model untouched so it can correct itself. Installed by the api server (which
// owns the run registry and the activation state); nil = no repair.
type DeadToolActivator func(ctx context.Context, token, bareName string) bool

// deadToolRepair is the per-turn repair state. It carries no Runtime reference —
// activation and journalling are injected — so it stays unit-testable without a
// live workspace, mirroring mcpRepair in this package.
type deadToolRepair struct {
	// bearer is the turn's per-run Interaction token: the address activation is
	// applied to, since the activated set is per session/run, not global.
	bearer   string
	activate DeadToolActivator
	journal  func(ctx context.Context, name string)

	mu sync.Mutex
	// done records the tools already repaired THIS TURN. A second rejection of the
	// same name means activation did not help (a stale CLI registry, a name the
	// model keeps mangling); repairing again would only append the same instruction
	// forever, so the original error is passed through instead.
	done map[string]bool
}

func newDeadToolRepair(bearer string, activate DeadToolActivator, journal func(context.Context, string)) *deadToolRepair {
	return &deadToolRepair{bearer: bearer, activate: activate, journal: journal, done: map[string]bool{}}
}

// deadToolRepairFor builds the repairer for one turn, or nil when this turn cannot
// repair: no activator wired, or no Interaction endpoint token to activate against.
// The nil case also covers the native tool loop, where the deferred tiers do not
// exist and every advertised tool is directly callable.
func (r *Runtime) deadToolRepairFor(ag db.Agent, inter tools.InteractionEndpoint) *deadToolRepair {
	if r.deadToolActivate == nil || inter.Token == "" {
		return nil
	}
	agentID := ag.ID
	return newDeadToolRepair(inter.Token, r.deadToolActivate, func(ctx context.Context, name string) {
		r.emitDebug(ctx, db.DebugEvent{
			Type:    db.DebugGuardrail,
			AgentID: agentID,
			Name:    deadToolDebugName,
			Detail:  name,
		})
	})
}

// repair inspects one live trace step and, when it is a dead-tool rejection for an
// on-demand TionHarness tool, activates that tool and appends the retry instruction to
// the step's output in place. Every other step is left byte-identical.
//
// A nil repairer is a no-op so the caller needs no branch.
func (d *deadToolRepair) repair(ctx context.Context, st *TurnStep) {
	if d == nil || st.Kind != StepTool || !st.IsError {
		return
	}
	called := parseDeadToolName(st.Output)
	if called == "" {
		return
	}
	bare, ok := bareInteractionToolName(called)
	if !ok {
		return // an external MCP or CLI-native tool: not ours to activate
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.done[bare] {
		return
	}
	if !d.activate(ctx, d.bearer, bare) {
		return // not in the on-demand catalog — the error stands, unmodified
	}
	d.done[bare] = true
	st.Output += deadToolNudge(called)
	if d.journal != nil {
		d.journal(ctx, called)
	}
}

// parseDeadToolName extracts the tool name out of a "No such tool available: X"
// rejection. It returns "" when the marker is absent or carries no name, so the
// caller can tell "not this failure" from "this failure, tool X".
//
// The name is terminated by the first character that cannot be part of a tool
// identifier: the CLI appends a sentence ("... : X. X exists but is not enabled"),
// the backend appends a suggestion, and the whole thing may be wrapped in a
// <tool_use_error> element.
func parseDeadToolName(output string) string {
	i := strings.Index(strings.ToLower(output), deadToolMarker)
	if i < 0 {
		return ""
	}
	rest := output[i+len(deadToolMarker):]
	rest = strings.TrimLeft(rest, ": \t")
	end := strings.IndexFunc(rest, func(r rune) bool { return !isToolNameRune(r) })
	if end >= 0 {
		rest = rest[:end]
	}
	return rest
}

// isToolNameRune reports whether r may appear inside a tool identifier. MCP tool
// names are [A-Za-z0-9_-]; the dot is deliberately EXCLUDED so a trailing sentence
// ("... list_tasks. It exists but ...") terminates the name.
func isToolNameRune(r rune) bool {
	return r == '_' || r == '-' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// bareInteractionToolName strips the TionHarness Interaction namespace off a called
// tool name. The second return is false for any other name (external MCP servers,
// CLI-native tools, bare names) — those are outside this repair's authority.
func bareInteractionToolName(called string) (string, bool) {
	for _, p := range []string{extendedToolPrefix, interactionToolPrefix} {
		if bare := strings.TrimPrefix(called, p); bare != called && bare != "" {
			return bare, true
		}
	}
	return "", false
}

// deadToolNudge renders the model-facing (English) instruction appended to the
// failed step. It names the EXACT namespaced form to re-call — the bare name is
// rejected by the CLI the same way — and forbids a detour through activate_tools,
// which is what the model reached for in SES79 after the third identical failure.
func deadToolNudge(called string) string {
	return "\n\n[dead tool repair] `" + called + "` is in this session's on-demand tool catalog but had not been activated, " +
		"which is why the call was rejected before it reached TionHarness. It has now been activated for this session. " +
		"Re-issue the SAME call with this exact name (`" + called + "`) — it will resolve. " +
		"Do not call activate_tools for it and do not look for a different tool."
}
