package tools

import (
	"context"
	"strings"
)

// Permission decision option labels shown to the user (and accepted as
// free-text). User-facing → Turkish; NormalizePermission also accepts the
// English equivalents so typed answers work either way.
const (
	PermAllowOnce   = "İzin ver"
	PermAllowAlways = "Her zaman izin ver"
	PermDeny        = "Reddet"
)

// PermissionOptions is the clickable answer set offered for a permission prompt,
// shared by the native gate and the CLI permission-prompt tool.
var PermissionOptions = []string{PermAllowOnce, PermAllowAlways, PermDeny}

// Plan-approval option labels shown when a claude-cli agent calls ExitPlanMode to
// present its plan (user-facing → Turkish).
const (
	PlanApprove = "Planı onayla"
	PlanReject  = "Reddet"
)

// PlanOptions is the clickable answer set offered for a plan-approval prompt
// (ExitPlanMode), surfaced through the same answer channel as the permission gate.
var PlanOptions = []string{PlanApprove, PlanReject}

// PlanApproved reports whether a clicked or typed answer approves the plan.
// Anything not clearly an approval is treated as a rejection.
func PlanApproved(ans string) bool {
	a := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(ans, "İ", "i")))
	switch {
	case strings.Contains(a, "onayla") || strings.Contains(a, "approve") ||
		strings.Contains(a, "proceed") || strings.HasPrefix(a, "evet") ||
		strings.HasPrefix(a, "yes") || a == "y":
		return true
	default:
		return false
	}
}

// NormalizePermission maps a clicked or typed answer onto a canonical decision:
// "always" | "allow" | "deny". Anything not clearly an approval is a denial.
// The Turkish dotted capital "İ" is normalised to "i" first because strings.
// ToLower maps it to "i̇" (i + combining dot), which would break the "izin"
// match. "always"/"her zaman" is checked before "izin" since "Her zaman izin
// ver" contains both.
func NormalizePermission(ans string) string {
	a := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(ans, "İ", "i")))
	switch {
	case strings.Contains(a, "always") || strings.Contains(a, "her zaman"):
		return "always"
	case strings.Contains(a, "allow") || strings.Contains(a, "izin") ||
		strings.HasPrefix(a, "yes") || strings.HasPrefix(a, "evet") || a == "y":
		return "allow"
	default:
		return "deny"
	}
}

// PermissionFunc prompts the user to approve a risky tool call and blocks until
// an answer (or ctx is done). tool is the tool name, risk its tier
// ("write"/"exec"), and arg the representative argument (e.g. a shell command;
// "" when not applicable) so the prompt can show what is being approved.
// Supplied by the interactive chat layer; autonomous runs leave it unset.
type PermissionFunc func(ctx context.Context, tool, risk, arg string, options []string) (string, error)

// permKey keys the PermissionFunc on a request context.
type permKey struct{}

// WithPermissionPrompter attaches an approval prompter to ctx so the permission
// gate can ask the user before running write/exec tools. Mirrors WithAsker.
func WithPermissionPrompter(ctx context.Context, fn PermissionFunc) context.Context {
	return context.WithValue(ctx, permKey{}, fn)
}

// PermissionPrompterFrom returns the prompter attached to ctx, or nil when none
// is present (autonomous runs with no open client connection).
func PermissionPrompterFrom(ctx context.Context) PermissionFunc {
	fn, _ := ctx.Value(permKey{}).(PermissionFunc)
	return fn
}
