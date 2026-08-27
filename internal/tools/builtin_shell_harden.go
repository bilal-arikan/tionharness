package tools

import (
	"os/exec"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// hardenShellCmd applies the two non-interactive safeguards shared by every shell
// subprocess (Bash and PowerShell, foreground and background — they all flow
// through the same build closure):
//
//  1. A non-interactive environment: an agent-run `git commit` must never launch
//     a GUI editor (core.editor=notepad) or a credential/pager prompt and hang —
//     GIT_EDITOR=true and friends make git fail fast instead of blocking forever.
//  2. Process-tree reaping on timeout: when a command hits its deadline, kill the
//     whole child tree, not just the shell — otherwise git.exe and the editor it
//     spawned survive as zombies and leave a .git/index.lock that wedges the next
//     commit (the failure mode that stalled the coordinator for a full turn).
//  3. Credential stripping: the shell is agent-driven, so its environment must not
//     carry ANTHROPIC_API_KEY, CREDENTIAL_SECRET or the user's personal tokens — a
//     single `env` would otherwise write all of them into the transcript, the UI
//     and the next LLM request. What was withheld is listed in
//     proc.StrippedEnvVar so a command that breaks for lack of a variable says so
//     instead of failing mysteriously.
//
// When confined (an autonomous/spawned turn with no human in the loop), it also
// forces commit/tag signing off: a signed git operation would otherwise block
// forever on a GPG pinentry passphrase prompt nobody can answer. Interactive
// turns keep signing enabled.
func hardenShellCmd(cmd *exec.Cmd, confined bool) *exec.Cmd {
	// CredentialSafeEnv resolves nil to os.Environ() minus the credentials, so the
	// guards are appended to an already-filtered base (never to a raw os.Environ()).
	cmd.Env = proc.HardenedEnv(proc.CredentialSafeEnv(cmd.Env))
	if confined {
		// This is the ONLY thing `confined` does to a shell subprocess — it is not a
		// path restriction. See the note above shellMaxOutputBytes in builtin_shell.go.
		cmd.Env = append(cmd.Env, proc.DisableGitSigningEnv()...)
	}
	proc.TreeKill(cmd)
	return cmd
}
