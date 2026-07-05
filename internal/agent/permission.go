package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// permLogKey carries an optional logger into permGate so approvals and standing
// "always allow" grants leave an audit trail in the in-app Logs screen. It is
// passed via context (not a parameter) so permGate keeps its pure two-value
// signature — tests calling it with context.Background() simply log nothing.
type permLogKey struct{}

// withPermLogger attaches the audit logger for permGate. nil → no-op.
func withPermLogger(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, permLogKey{}, l)
}

// logPerm emits a permission audit line via the ctx logger, if any. Nil-safe.
func logPerm(ctx context.Context, level slog.Level, msg string, args ...any) {
	if l, _ := ctx.Value(permLogKey{}).(*slog.Logger); l != nil {
		l.Log(ctx, level, msg, args...)
	}
}

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
			logPerm(ctx, slog.LevelDebug, "permission auto-allowed by standing rule", "tool", call.Name, "arg", arg)
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
			rule := tools.DeriveGrantRule(call.Name, arg)
			grants.GrantRule(rule)
			logPerm(ctx, slog.LevelInfo, "permission granted (always) — standing rule recorded", "tool", call.Name, "rule", rule.String())
			return true, ""
		case "allow":
			logPerm(ctx, slog.LevelInfo, "permission granted (once)", "tool", call.Name, "arg", arg)
			return true, ""
		default:
			logPerm(ctx, slog.LevelInfo, "permission denied by user", "tool", call.Name)
			return false, fmt.Sprintf("permission denied by user: %q was not approved", call.Name)
		}
	default:
		// Unknown mode behaves like auto rather than locking the agent out.
		return true, ""
	}
}
