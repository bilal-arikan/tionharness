//go:build windows

package proc

import (
	"os/exec"
	"strconv"
	"time"
)

// reapDelay bounds how long cmd.Wait blocks after the context is cancelled before
// the direct child is force-killed — a backstop when a grandchild keeps an output
// pipe open past the tree kill.
const reapDelay = 5 * time.Second

// TreeKill makes a context-bound command tear down its WHOLE child tree when its
// context is cancelled or times out, instead of orphaning grandchildren. On
// Windows a plain Process.Kill (what exec.CommandContext does by default) kills
// only the immediate shell (bash.exe/powershell.exe); any git.exe — and the
// editor git spawned — survives as a zombie and leaves a .git/index.lock that
// wedges later commits. taskkill /T walks and kills the entire tree.
func TreeKill(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.WaitDelay = reapDelay
	cmd.Cancel = func() error {
		KillTree(cmd)
		return nil // WaitDelay force-kills the direct child if it lingers
	}
}

// KillTree force-kills a running command and its whole descendant tree right
// now, without going through context cancellation. Callers that tear a process
// down themselves (a CLI provider aborting on a terminal error or a startup
// hang) must use this rather than cmd.Process.Kill: killing only the direct
// child leaves grandchildren — a Gradle daemon, an MCP server — alive holding
// the inherited stdout pipe, and cmd.Wait then blocks forever on a pipe that
// never reaches EOF.
func KillTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	kill := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid))
	Hide(kill)
	_ = kill.Run() // best-effort: the process may already be gone
}
