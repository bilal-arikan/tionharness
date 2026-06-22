//go:build windows

package main

import (
	"os"

	"github.com/bilal-arikan/swarmgo/internal/proc"
)

// spawnWindow launches a new connect-only instance of this binary, pointed at
// the running server's base URL plus the given hash route. go-webview2 is
// single-window-per-process, so a separate process is how we get a second
// native window (see _Docs/18-COKLU-PENCERE.md). proc.Command avoids a console
// flash from the windowless GUI parent.
func spawnWindow(baseURL, route string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	target := baseURL + "/#" + route
	cmd := proc.Command(exe)
	cmd.Env = append(os.Environ(), envWebviewURL+"="+target)
	_ = cmd.Start()
}
