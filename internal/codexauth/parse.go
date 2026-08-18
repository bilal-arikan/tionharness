// Package codexauth drives the `codex login --device-auth` subprocess so
// TionSwarm can offer an in-app login for the codex-cli provider, mirroring
// what internal/claudeauth does for claude-cli. Unlike claudeauth, this
// package does NOT reimplement OpenAI's OAuth: codex already performs the
// device-code dance itself and writes <CODEX_HOME>/auth.json on success. This
// package only starts that subprocess, parses its prompt, and reports status.
package codexauth

import (
	"regexp"
	"strings"
)

// ansiEscape matches a CSI (ESC '[' ... final byte) or a bare OSC/other ESC
// sequence the codex CLI emits for colour ("\x1b[94m") and reset ("\x1b[0m").
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes ANSI escape sequences from s, leaving plain text.
func StripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

// linkMarker and codeMarker anchor on the CLI's own prompt wording (observed
// verbatim in the scratchpad capture) rather than on the URL/code shape,
// since OpenAI can rotate the verification host or code format without
// changing this wording.
const (
	linkMarker = "open this link"
	codeMarker = "enter this one-time code"
)

// urlPattern is a loose fallback for locating a URL on the marker's next
// non-empty line — it does not assume any particular host.
var urlPattern = regexp.MustCompile(`https?://\S+`)

// codePattern is a loose fallback for the one-time code: short groups of
// letters/digits joined by hyphens. It intentionally does not pin an exact
// length so a future CLI format change still matches.
var codePattern = regexp.MustCompile(`^[A-Za-z0-9]{3,8}(-[A-Za-z0-9]{3,8})+$`)

// ParseDevicePrompt extracts the verification URL and one-time code from the
// codex CLI's device-auth stdout. out may still contain ANSI escapes; it is
// stripped internally. ok is false when either marker/value cannot be found
// confidently — callers must treat that as "not ready yet" or "unrecognised
// output", never guess a value.
func ParseDevicePrompt(out string) (verifyURL, code string, ok bool) {
	plain := StripANSI(out)
	lines := strings.Split(plain, "\n")

	verifyURL = firstValueAfterMarker(lines, linkMarker, urlPattern)
	code = firstValueAfterMarker(lines, codeMarker, codePattern)
	if verifyURL == "" || code == "" {
		return "", "", false
	}
	return verifyURL, code, true
}

// firstValueAfterMarker finds the first line containing marker (case
// insensitive) and returns the first regexp match found on that same line or
// one of the next few non-empty lines — the CLI wraps the value onto its own
// line, but tolerating the same-line case keeps this robust to reflow.
func firstValueAfterMarker(lines []string, marker string, pattern *regexp.Regexp) string {
	markerIdx := -1
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), marker) {
			markerIdx = i
			break
		}
	}
	if markerIdx == -1 {
		return ""
	}
	// Search the marker line itself, then up to 3 following lines for the value.
	for i := markerIdx; i < len(lines) && i < markerIdx+4; i++ {
		candidate := strings.TrimSpace(lines[i])
		if candidate == "" {
			continue
		}
		if m := pattern.FindString(candidate); m != "" {
			return m
		}
	}
	return ""
}
