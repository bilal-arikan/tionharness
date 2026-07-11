package proc

import "os"

// nonInteractiveGuards are environment overrides that keep child processes — git
// above all — from blocking on an interactive prompt inside TionSwarm's
// windowless, stdin-less subprocesses. Without them an agent-run `git commit`
// can launch the configured GUI editor (e.g. core.editor=notepad) or a
// credential/pager prompt and hang forever, burning a whole turn and orphaning
// the process plus a stale .git/index.lock. Each var is behaviour-neutral except
// when git would otherwise stop to ask a human.
func nonInteractiveGuards() []string {
	return []string{
		"GIT_TERMINAL_PROMPT=0",    // never prompt for credentials on the terminal
		"GIT_EDITOR=true",          // override core.editor (notepad/vim) → no editor hang
		"GIT_SEQUENCE_EDITOR=true", // same for an interactive-rebase todo list
		"GIT_PAGER=cat",            // never page (would wait for the pager to close)
		"PAGER=cat",
		"GIT_OPTIONAL_LOCKS=0",  // skip opportunistic locks that can wedge on a crash
		"GCM_INTERACTIVE=never", // Git Credential Manager: no popup
	}
}

// DisableGitSigningEnv returns git's env-based config injection that turns commit
// AND tag signing off (commit.gpgsign=false, tag.gpgsign=false) via
// GIT_CONFIG_COUNT/KEY/VALUE. It is meant for autonomous/confined turns only: an
// unattended `git commit` on a repo configured with commit.gpgsign=true would
// block forever on a GPG pinentry passphrase prompt that no human can answer.
// Interactive turns keep signing (a person can enter the passphrase), so callers
// gate this on the confined/autonomous flag.
//
// Assumes the ambient environment carries no GIT_CONFIG_* injection of its own
// (true for TionSwarm's process); appended last, these win via os/exec dedup.
func DisableGitSigningEnv() []string {
	return []string{
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=commit.gpgsign", "GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=tag.gpgsign", "GIT_CONFIG_VALUE_1=false",
	}
}

// HardenedEnv returns base (or the current process environment when base is nil)
// with the non-interactive guards appended. Appending last is deliberate: os/exec
// deduplicates the environment keeping the LAST value for each key (case-
// insensitively on Windows), so the guards always win over any inherited value —
// a child git can never fall back to an interactive editor or credential prompt.
// Callers that curate their own environment (e.g. the claude CLI, which strips
// nesting vars first) pass it as base so their edits are preserved.
func HardenedEnv(base []string) []string {
	if base == nil {
		base = os.Environ()
	}
	out := make([]string, 0, len(base)+len(nonInteractiveGuards()))
	out = append(out, base...)
	return append(out, nonInteractiveGuards()...)
}
