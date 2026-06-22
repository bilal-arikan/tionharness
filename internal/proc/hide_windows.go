//go:build windows

// Package proc holds OS-specific helpers for launching child processes. On
// Windows, console child processes (git, powershell, the claude CLI, MCP stdio
// servers) would each pop up a console window when the parent is a windowless
// GUI app (swarmgo-desktop, built with -H windowsgui). Hide suppresses that
// flashing window.
package proc

import (
	"os/exec"
	"syscall"
)

// createNoWindow (CREATE_NO_WINDOW) runs a console child without allocating a
// console window for it.
const createNoWindow = 0x08000000

// Hide configures cmd so it spawns without a visible console window. Safe to
// call before Start/Run/Output; merges into any existing SysProcAttr.
func Hide(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
