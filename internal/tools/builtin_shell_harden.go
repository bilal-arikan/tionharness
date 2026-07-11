package tools

import (
	"os/exec"

	"github.com/bilal-arikan/tionswarm/internal/proc"
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
//
// When confined (an autonomous/spawned turn with no human in the loop), it also
// forces commit/tag signing off: a signed git operation would otherwise block
// forever on a GPG pinentry passphrase prompt nobody can answer. Interactive
// turns keep signing enabled.
func hardenShellCmd(cmd *exec.Cmd, confined bool) *exec.Cmd {
	cmd.Env = proc.HardenedEnv(cmd.Env) // cmd.Env nil → os.Environ()+guards
	if confined {
		cmd.Env = append(cmd.Env, proc.DisableGitSigningEnv()...)
	}
	proc.TreeKill(cmd)
	return cmd
}
