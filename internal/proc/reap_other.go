//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
	"time"
)

// reapDelay bounds how long cmd.Wait blocks after the context is cancelled before
// the process group is force-killed — a backstop when a child keeps an output
// pipe open past the group signal.
const reapDelay = 5 * time.Second

// TreeKill makes a context-bound command tear down its WHOLE child tree when its
// context is cancelled or times out. The shell is placed in its own process group
// (Setpgid) so a single kill to the negative pid reaps the shell and every
// subprocess it spawned (git and any editor), instead of the default
// exec.CommandContext behaviour of killing only the direct child and orphaning
// the rest.
func TreeKill(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.WaitDelay = reapDelay
	cmd.Cancel = func() error {
		KillTree(cmd)
		return nil
	}
}

// KillTree force-kills a running command and its whole descendant tree right
// now, without going through context cancellation. Callers that tear a process
// down themselves (a CLI provider aborting on a terminal error or a startup
// hang) must use this rather than cmd.Process.Kill: killing only the direct
// child leaves grandchildren alive holding the inherited stdout pipe, and
// cmd.Wait then blocks forever on a pipe that never reaches EOF.
//
// The process group is only established when TreeKill was applied to the
// command; without it the negative-pid kill would target an unrelated group, so
// fall back to killing the direct child.
func KillTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // whole group; best-effort
		return
	}
	_ = cmd.Process.Kill()
}
