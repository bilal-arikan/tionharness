package tools

import "fmt"

// Self-correcting tool errors: when a built-in tool rejects its input, the
// message should tell the agent what to do next in a few words, not just what
// went wrong. These helpers keep that guidance consistent across tools.

// argErr wraps a JSON argument-decode failure with a short, actionable hint.
// The underlying Go error already names the offending field/type; the suffix
// nudges the agent to re-read the schema instead of blindly retrying.
func argErr(err error) error {
	return fmt.Errorf("invalid arguments: %w — fix: match this tool's input schema (required fields, exact types); see its examples", err)
}

// cronHint describes the accepted cron format (robfig/cron v3, standard
// 5-field, no seconds) with concrete examples, so a rejected cronExpr can be
// fixed without guessing.
const cronHint = `Fix cronExpr — 5 fields "min hour dom mon dow" (no seconds), e.g. "0 * * * *"=hourly, "*/15 * * * *"=every 15m, "30 9 * * 1-5"=09:30 on weekdays; or a descriptor like "@hourly"/"@daily".`

// argErrFor is argErr for tools that name themselves in the message (the
// meta/interaction tools use "invalid <tool> input: %w"). It keeps the tool
// name and adds the same actionable schema hint.
func argErrFor(tool string, err error) error {
	return fmt.Errorf("invalid %s input: %w — fix: match this tool's input schema (required fields, exact types); see its examples", tool, err)
}

// enumErr reports that a field got an unsupported value and lists the allowed
// ones, so the agent can correct it without guessing. field is the argument
// name, got is what was supplied, allowed are the valid values.
func enumErr(field, got string, allowed ...string) error {
	return fmt.Errorf("invalid %s %q — fix: use one of %v", field, got, allowed)
}
