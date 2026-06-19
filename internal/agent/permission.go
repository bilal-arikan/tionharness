package agent

import (
	"context"
	"fmt"

	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
)

// permGate decides whether a tool call may execute under the agent's permission
// mode. It returns (allowed, denialMessage); when allowed is false the caller
// feeds denialMessage back to the model as an error tool result so the model can
// adapt instead of the turn aborting.
//
// Modes:
//   - auto / "":  everything runs.
//   - read-only:  only RiskRead tools run; write/exec are blocked.
//   - ask:        RiskRead runs; write/exec prompt the user via the permission
//     prompter (a dedicated StepPermission card). "Always allow" is recorded in
//     the session grants (tools.GrantsFrom(ctx)) so the same tool is not
//     re-prompted for the rest of the session. With no prompter (autonomous run)
//     write/exec are denied — set the agent to "auto" for unattended writes.
func permGate(ctx context.Context, mode string, call providers.ToolCall) (bool, string) {
	risk := tools.Classify(call.Name)
	switch mode {
	case "", "auto":
		return true, ""
	case "read-only":
		if risk == tools.RiskRead {
			return true, ""
		}
		return false, fmt.Sprintf("permission denied: agent is in read-only mode, so %q (%s) cannot run", call.Name, risk)
	case "ask":
		if risk == tools.RiskRead {
			return true, ""
		}
		// Argument-aware grants (B2): a standing rule may cover this exact call
		// (e.g. shell(git *) approving `git status`) without re-prompting.
		arg := tools.RepresentativeArg(call.Name, call.Input)
		grants := tools.GrantsFrom(ctx)
		if grants.Matches(call.Name, arg) {
			return true, ""
		}
		prompt := tools.PermissionPrompterFrom(ctx)
		if prompt == nil {
			return false, fmt.Sprintf("permission denied: %q (%s) requires approval but no interactive session is available", call.Name, string(risk))
		}
		ans, err := prompt(ctx, call.Name, string(risk), arg, tools.PermissionOptions)
		if err != nil {
			return false, "permission denied: approval request failed: " + err.Error()
		}
		switch tools.NormalizePermission(ans) {
		case "always":
			// Scope "always" to the command family for exec tools (shell(git *)),
			// or whole-tool otherwise — see DeriveGrantRule.
			grants.GrantRule(tools.DeriveGrantRule(call.Name, arg))
			return true, ""
		case "allow":
			return true, ""
		default:
			return false, fmt.Sprintf("permission denied by user: %q was not approved", call.Name)
		}
	default:
		// Unknown mode behaves like auto rather than locking the agent out.
		return true, ""
	}
}
