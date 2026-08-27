package proc

import (
	"context"
	"os/exec"
)

// Command is a drop-in replacement for exec.Command that returns a cmd already
// configured to spawn without a visible console window (see Hide). Prefer this
// over exec.Command for any console child so the windowless desktop app
// (tionharness-desktop, -H windowsgui) never flashes a terminal.
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

// CommandContextNested is like CommandContext but for a child that spawns its
// own grandchildren (an agentic CLI launching MCP servers, or shelling out to a
// build tool). See HideNested for why Hide alone is not enough here.
func CommandContextNested(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	HideNested(cmd)
	return cmd
}
