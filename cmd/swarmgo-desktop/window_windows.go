//go:build windows

package main

// swMaximize is SW_MAXIMIZE for ShowWindow — opens the window maximized.
const swMaximize = 3

// procShowWindow reuses the package-level user32 lazy DLL declared in
// dpi_windows.go.
var procShowWindow = user32.NewProc("ShowWindow")

// maximizeWindow maximizes the given top-level window. go-webview2 has no
// "start maximized" option, so we set it on the HWND after creation.
func maximizeWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	_, _, _ = procShowWindow.Call(hwnd, swMaximize)
}
