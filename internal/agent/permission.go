package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
)

// Permission prompt option labels offered to the user in "ask" mode.
const (
	permApprove       = "Allow once"
	permApproveAlways = "Always allow"
	permDeny          = "Deny"
)

// permGate decides whether a tool call may execute under the agent's permission
// mode. It returns (allowed, denialMessage); when allowed is false the caller
// feeds denialMessage back to the model as an error tool result so the model can
// adapt instead of the turn aborting.
//
// Modes:
//   - auto / "":  everything runs.
//   - read-only:  only RiskRead tools run; write/exec are blocked.
//   - ask:        RiskRead runs; write/exec prompt the user via the ask channel.
//     "Always allow" is remembered in granted for the rest of the turn so the
//     same tool is not re-prompted. With no interactive asker (autonomous run)
//     write/exec are denied — set the agent to "auto" for unattended writes.
func permGate(ctx context.Context, mode string, call providers.ToolCall, granted map[string]bool) (bool, string) {
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
		if risk == tools.RiskRead || granted[call.Name] {
			return true, ""
		}
		ask := tools.AskerFrom(ctx)
		if ask == nil {
			return false, fmt.Sprintf("permission denied: %q (%s) requires approval but no interactive session is available", call.Name, risk)
		}
		ans, err := ask(ctx, fmt.Sprintf("Allow this agent to run %q (%s)?", call.Name, risk),
			[]string{permApprove, permApproveAlways, permDeny})
		if err != nil {
			return false, "permission denied: approval request failed: " + err.Error()
		}
		switch normalizePermAnswer(ans) {
		case permApproveAlways:
			granted[call.Name] = true
			return true, ""
		case permApprove:
			return true, ""
		default:
			return false, fmt.Sprintf("permission denied by user: %q was not approved", call.Name)
		}
	default:
		// Unknown mode behaves like auto rather than locking the agent out.
		return true, ""
	}
}

// normalizePermAnswer maps a free-text or clicked answer onto a canonical
// decision. Anything that is not clearly an approval is treated as a denial.
func normalizePermAnswer(ans string) string {
	a := strings.ToLower(strings.TrimSpace(ans))
	switch {
	case strings.Contains(a, "always"):
		return permApproveAlways
	case strings.HasPrefix(a, "allow"), strings.HasPrefix(a, "yes"), a == "y", strings.Contains(a, "approve"):
		return permApprove
	default:
		return permDeny
	}
}
