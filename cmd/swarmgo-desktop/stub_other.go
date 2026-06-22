//go:build !windows

// The native desktop window currently targets Windows (WebView2, CGO-free).
// On other platforms this binary is a stub; use the headless server
// (cmd/swarmgo) with a browser, or add a CGO webview backend later.
package main

import "fmt"

func main() {
	fmt.Println("swarmgo-desktop is Windows-only for now; run the headless 'swarmgo' server instead.")
}
