package proc

import (
	"context"
	"os/exec"
)

// Command is a drop-in replacement for exec.Command that returns a cmd already
// configured to spawn without a visible console window (see Hide). Prefer this
// over exec.Command for any console child so the windowless desktop app
// (tionswarm-desktop, -H windowsgui) never flashes a terminal.
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	Hide(cmd)
	return cmd
}

// CommandContext is a drop-in replacement for exec.CommandContext with the same
// no-console-window behaviour as Command.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	Hide(cmd)
	return cmd
}
