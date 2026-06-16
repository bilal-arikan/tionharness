package providers

import (
	"regexp"
	"strings"
)

// Harness / tool-call markup that must never be replayed to the claude CLI as
// plain transcript text. When a prior assistant turn's stored text contains
// leaked tool-call XML (e.g. </parameter></function_results>) or a harness
// repair reminder, re-serialising it verbatim makes the CLI's input parser treat
// the turn as malformed and inject its own <system-reminder>; the model then
// echoes that reminder as a visible reply instead of answering. Stripping these
// fragments before replay removes the trigger at the root.
var (
	reSystemReminder = regexp.MustCompile(`(?is)<system-reminder>.*?</system-reminder>`)
	reHarnessBlock   = regexp.MustCompile(`(?is)<(function_calls|function_results)\b[^>]*>.*?</(function_calls|function_results)>`)
	reHarnessTag     = regexp.MustCompile(`(?i)</?(function_calls|function_results|antml:invoke|antml:parameter|invoke|parameter)\b[^>]*>`)
)

// sanitizeTranscriptText strips leaked harness/tool-call markup from a stored
// turn before it is replayed to the claude CLI as flattened transcript text.
// It is intentionally conservative: it only removes the specific harness tag
// names, leaving ordinary user/assistant prose (including most code) intact.
func sanitizeTranscriptText(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	s = reSystemReminder.ReplaceAllString(s, "")
	s = reHarnessBlock.ReplaceAllString(s, "")
	s = reHarnessTag.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// isRepairArtifact reports whether a model output is actually a harness repair
// reminder (or contains stray harness tags) rather than a genuine answer.
func isRepairArtifact(s string) bool {
	return strings.Contains(s, "<system-reminder>") ||
		strings.Contains(s, "</function_results>") ||
		strings.Contains(s, "</function_calls>")
}
