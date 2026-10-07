//go:build !windows

// The native desktop window currently targets Windows (WebView2, CGO-free).
// On other platforms this binary is a stub; use the headless server
// (cmd/tionharness) with a browser, or add a CGO webview backend later.
package main

import (
	"fmt"
	"os"
)

// main exits non-zero so a launcher or .app wrapper reports the failure instead
// of "succeeding" with nothing on screen.
func main() {
	fmt.Fprintln(os.Stderr, "tionharness-desktop is Windows-only for now; run the headless 'tionharness' server instead (scripts/serve.sh --ui).")
	os.Exit(1)
}
