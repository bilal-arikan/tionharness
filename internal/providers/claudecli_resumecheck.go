package providers

import (
	"os"
	"path/filepath"
	"strings"
)

// ResumeVerifier is implemented by CLI providers whose --resume id points at a
// transcript the provider can check for BEFORE spending a turn on it. The
// caller (api.planClaudeResume) uses it to fall back to a cold start — with the
// full transcript — instead of shipping a delta the CLI will reject.
type ResumeVerifier interface {
	// CanResume reports whether sessionID still names a resumable conversation.
	CanResume(sessionID string) bool
}

// isMissingConversation reports whether a CLI failure detail is the "the resume
// id does not exist here" rejection, whose wording ("No conversation found with
// session ID: <uuid>") is matched on its stable prefix only.
func isMissingConversation(detail string) bool {
	return strings.Contains(detail, "No conversation found with session ID")
}

// CanResume reports whether the claude CLI can still --resume sessionID from
// the config home this provider is pointed at. Claude Code stores one transcript
// per conversation as <CLAUDE_CONFIG_DIR>/projects/<cwd-slug>/<session-id>.jsonl,
// so the id is resumable exactly when that file exists somewhere under projects/.
//
// This exists because the config home MOVES: when the app-global home replaced
// the per-workspace ones, every stored resume id still named a transcript that
// only existed in the old home. The CLI answers that with "No conversation found
// with session ID: …" and exit 1, which the turn retried verbatim — three dead
// turns and a `stuck` session per thread. Checking first turns it into a normal
// cold turn.
//
// The project subdirectory is not derived from the working directory: the slug
// encoding is the CLI's private business and a session may legitimately have been
// started from a different cwd, so every project dir is scanned instead.
//
// Unknown → resumable: with no config dir set (the CLI uses the ambient ~/.claude)
// there is nothing authoritative to check, and an unreadable projects/ directory
// must not silently downgrade a healthy warm session to a cold one.
func (c *ClaudeCLI) CanResume(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return false
	}
	if c.configDir == "" {
		return true
	}
	projects := filepath.Join(c.configDir, "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		return true
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if st, err := os.Stat(filepath.Join(projects, e.Name(), id+".jsonl")); err == nil && st.Mode().IsRegular() {
			return true
		}
	}
	return false
}
