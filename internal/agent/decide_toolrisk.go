package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// Decision authority "tool-risk": a second look at command-execution calls
// that would run WITHOUT a human decision — every exec call in auto mode, and
// exec calls a standing "always allow" grant covers in ask mode. A family grant
// such as Bash(git *) also covers `git push --force`; auto mode covers
// everything.
//
// The check only ever tightens, and only toward a human: in on mode a command
// the decision model thinks needs approval becomes an approval prompt — but only
// when someone is there to answer it. An unattended run (no prompter) is never
// delayed or blocked; its commands are just measured, as in shadow mode.

// RiskFlagged is the risk label a decider-flagged prompt carries, so the
// approval card can say why a command that normally runs unasked is asking.
const RiskFlagged = "exec:decider"

const (
	riskApprovalKey = "needs_approval"
	riskLevelKey    = "risk"
)

// toolRiskCheck is installed on the tool-loop context by withToolRiskCheck and
// consulted by permGate. flagged=true asks the user; why is for the audit log.
type toolRiskCheck func(ctx context.Context, call providers.ToolCall) (flagged bool, why string)

type toolRiskCheckKey struct{}

// withToolRiskCheck installs the tool-risk check for calls made by agent. When
// the authority is off it installs nothing, so permGate pays nothing.
func (r *Runtime) withToolRiskCheck(ctx context.Context, agent db.Agent) context.Context {
	mode := r.deciderMode(authToolRisk)
	if mode == decider.ModeOff {
		return ctx
	}
	check := toolRiskCheck(func(ctx context.Context, call providers.ToolCall) (bool, string) {
		return r.checkToolRisk(ctx, agent, call, mode, tools.PermissionPrompterFrom(ctx) != nil)
	})
	return context.WithValue(ctx, toolRiskCheckKey{}, check)
}

// toolRiskFlagged runs the installed check for call, unless the user already
// approved this exact command with "Always allow".
func toolRiskFlagged(ctx context.Context, call providers.ToolCall) (bool, string) {
	check, _ := ctx.Value(toolRiskCheckKey{}).(toolRiskCheck)
	if check == nil {
		return false, ""
	}
	if arg := tools.RepresentativeArg(call.Name, call.Input); tools.GrantsFrom(ctx).MatchesExact(call.Name, arg) {
		return false, ""
	}
	return check(ctx, call)
}

// ToolRiskFlagged is the tool-risk check for callers outside the native tool
// loop — the claude-cli permission-prompt tool, whose ask-mode calls a standing
// grant would otherwise auto-allow. interactive says whether a user can answer
// an approval prompt. agent may be zero-valued when the caller cannot tell which
// agent is asking; the decision is then logged but billed to no agent budget.
func (r *Runtime) ToolRiskFlagged(ctx context.Context, agent db.Agent, toolName string, input json.RawMessage, interactive bool) (bool, string) {
	mode := r.deciderMode(authToolRisk)
	if mode == decider.ModeOff {
		return false, ""
	}
	call := providers.ToolCall{Name: toolName, Input: input}
	if arg := tools.RepresentativeArg(toolName, input); tools.GrantsFrom(ctx).MatchesExact(toolName, arg) {
		return false, ""
	}
	return r.checkToolRisk(ctx, agent, call, mode, interactive)
}

// checkToolRisk asks the decision model about one exec call. Synchronous only
// in on mode with someone to ask; otherwise it measures in the background.
func (r *Runtime) checkToolRisk(ctx context.Context, agent db.Agent, call providers.ToolCall, mode decider.Mode, interactive bool) (bool, string) {
	if tools.Classify(call.Name) != tools.RiskExec {
		return false, ""
	}
	cmd := execCommandText(call)
	if cmd == "" || readOnlyCommand(cmd) {
		return false, ""
	}
	req := toolRiskRequest(call.Name, cmd)
	threshold := r.deciderThreshold(authToolRisk)
	outcome := func(resp *decider.Response) (string, float64) {
		a := resp.Answers[riskApprovalKey]
		if a.Yes(threshold) {
			return "ask", a.Probability
		}
		return "run", a.Probability
	}
	ref := SessionIDFrom(ctx)
	if mode != decider.ModeOn || !interactive {
		r.backgroundDecision(ctx, authToolRisk, mode, agent, req, "run", ref, outcome)
		return false, ""
	}
	resp, err := r.decide(ctx, authToolRisk, agent, req, decider.WithOutcome(outcome))
	rec := decider.NewRecord(authToolRisk, decider.ModeOn, resp, err)
	rec.Baseline, rec.Ref = "run", ref
	if err != nil {
		if !decisionOff(err) {
			r.logger.Info("tool risk decision unavailable; running as before", "agent", agent.ID, "tool", call.Name, "error", err)
			r.logDecision(rec)
		}
		return false, ""
	}
	verdict, p := outcome(resp)
	rec.Outcome, rec.Strength, rec.Applied = verdict, p, verdict == "ask"
	r.logDecision(rec)
	if verdict != "ask" {
		return false, ""
	}
	level := ""
	if a, ok := resp.Answers[riskLevelKey]; ok {
		level = fmt.Sprintf(", risk level %d/2", a.Level())
	}
	return true, fmt.Sprintf("decision model: %.0f%% likely to need approval%s", p*100, level)
}

// toolRiskRequest builds the decision request for one command.
func toolRiskRequest(toolName, cmd string) decider.Request {
	return decider.Request{
		State: map[string]any{
			"tool":    toolName,
			"command": truncateRunes(cmd, 4000),
		},
		Questions: map[string]decider.Question{
			riskApprovalKey: decider.Noul(
				"A coding agent is about to run this command in the user's project without asking. Should a human approve it first?",
				"It deletes or overwrites data beyond build output, rewrites or force-pushes version-control history, pushes, publishes or deploys, changes system, security or global settings, installs software globally, or sends data to an external service.",
				"It only reads, lists, builds, tests, lints or formats, or makes a local change that version control or simply re-running can undo.",
			),
			riskLevelKey: decider.Score("How risky is running this command?",
				"Read-only", "Local, easily reversible change", "Destructive, irreversible or outward-facing"),
		},
	}
}

// execCommandText extracts the command/script text of an exec call.
func execCommandText(call providers.ToolCall) string {
	if len(call.Input) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(call.Input, &obj); err != nil {
		return ""
	}
	for _, k := range []string{"command", "cmd", "script", "code"} {
		if v, ok := obj[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
