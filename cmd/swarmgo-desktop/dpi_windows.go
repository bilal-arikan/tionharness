//go:build windows

package main

import "syscall"

// Windows DPI awareness. Without this, a high-DPI display (scaling > 100%, e.g.
// 125%/150% on most laptops) makes Windows bitmap-stretch the window — blurring
// all text. Declaring Per-Monitor-V2 awareness makes WebView2 render crisply at
// the real pixel density. Must run before any window is created.
var (
	user32                            = syscall.NewLazyDLL("user32.dll")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
)

// dpiPerMonitorAwareV2 is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, the
// special handle value (-4) passed to SetProcessDpiAwarenessContext.
const dpiPerMonitorAwareV2 = ^uintptr(3) // -4 as an unsigned pointer

// setDPIAware opts the process into per-monitor-v2 DPI awareness on Windows 10
// 1703+, falling back to the legacy system-DPI call on older builds. Errors are
// ignored: the worst case is the pre-existing blurry scaling.
func setDPIAware() {
	if procSetProcessDpiAwarenessContext.Find() == nil {
		if ret, _, _ := procSetProcessDpiAwarenessContext.Call(dpiPerMonitorAwareV2); ret != 0 {
			return // success
		}
	}
	// Fallback for older Windows: system-DPI awareness (crisp at the primary
	// monitor's scale, may blur when moved to a differently-scaled monitor).
	if procSetProcessDPIAware.Find() == nil {
		_, _, _ = procSetProcessDPIAware.Call()
	}
}
