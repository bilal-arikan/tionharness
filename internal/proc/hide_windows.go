//go:build windows

// Package proc holds OS-specific helpers for launching child processes. On
// Windows, console child processes (git, powershell, the claude CLI, MCP stdio
// servers) would each pop up a console window when the parent is a windowless
// GUI app (tionswarm-desktop, built with -H windowsgui). Hide suppresses that
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
//
// NOTE: Hide also sets HideWindow (STARTF_USESHOWWINDOW + SW_HIDE), which hides
// the child's FIRST GUI window too. Use it only for pure console children
// (git, claude CLI, /bin/sh). For a console program that opens a dialog (e.g.
// the PowerShell folder picker) use HideConsole instead, or the dialog is hidden.
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

// HideConsole suppresses the child's own console window (CREATE_NO_WINDOW)
// WITHOUT hiding any GUI window the child opens. For a console program that
// shows a dialog — e.g. the PowerShell FolderBrowserDialog — so the console
// never flashes but the dialog still appears.
func HideConsole(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
