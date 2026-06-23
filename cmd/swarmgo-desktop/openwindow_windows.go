//go:build windows

package main

import (
	"os"
	"os/exec"
)

// spawnWindow launches a new connect-only instance of this binary, pointed at
// the running server's base URL plus the given hash route. go-webview2 is
// single-window-per-process, so a separate process is how we get a second
// native window (see _Docs/30-COKLU-PENCERE.md).
//
// NOTE: this deliberately uses plain exec.Command, NOT proc.Command. proc.Command
// sets SysProcAttr.HideWindow (STARTF_USESHOWWINDOW + SW_HIDE) to suppress a
// console flash — correct for console children, but it would start this GUI
// child's WebView2 window HIDDEN. The child is a -H windowsgui binary with no
// console, so there is nothing to suppress anyway.
func spawnWindow(baseURL, route string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	target := baseURL + "/#" + route
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), envWebviewURL+"="+target)
	_ = cmd.Start()
}
