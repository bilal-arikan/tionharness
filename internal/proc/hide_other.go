//go:build !windows

package proc

import "os/exec"

// Hide is a no-op on non-Windows platforms, where console children do not pop
// up a window.
func Hide(cmd *exec.Cmd) {}

// HideConsole is a no-op on non-Windows platforms.
func HideConsole(cmd *exec.Cmd) {}
