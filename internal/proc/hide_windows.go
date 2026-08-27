//go:build windows

// Package proc holds OS-specific helpers for launching child processes. On
// Windows, console child processes (git, powershell, the claude CLI, MCP stdio
// servers) would each pop up a console window when the parent is a windowless
// GUI app (tionharness-desktop, built with -H windowsgui). Hide suppresses that
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

// HideNested hides cmd's own console window WITHOUT CREATE_NO_WINDOW, for a
// child that spawns further console children of its own (an agentic CLI
// launching MCP stdio servers, or shelling out to a build tool). CREATE_NO_WINDOW
// gives cmd no console at all — and when a console-less process then spawns a
// child without an explicit console flag, Windows allocates that grandchild a
// BRAND NEW, VISIBLE console, because there is no console to inherit. That is
// the flashing terminal users see on every MCP tool call from a hidden
// claude-cli/codex-cli turn (see internal/providers/claudecli.go,
// claudecli_session.go, codexcli.go). HideWindow alone still allocates cmd a
// console but hides it immediately, so grandchildren inherit that existing
// hidden console instead of creating their own.
func HideNested(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}
