//go:build windows

// Command swarmgo-desktop runs SwarmGo as a native desktop application: it
// boots the same in-process server as cmd/swarmgo on a free loopback port, then
// shows the embedded UI in a WebView2 window instead of a browser tab. It is
// Windows-only and uses github.com/jchv/go-webview2 (pure Go, no CGO; relies on
// the WebView2 runtime, which ships with Windows 11).
//
// Multi-window: go-webview2 is single-window-per-process, so each extra window
// is a separate "connect-only" instance of this binary pointed at the already-
// running server via SWARMGO_WEBVIEW_URL. The primary process owns the server
// and exposes a JS bridge (swarmgoOpenWindow) that spawns those instances.
// See _Docs/30-COKLU-PENCERE.md.
package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/jchv/go-webview2"

	"github.com/bilal-arikan/swarmgo/internal/app"
	"github.com/bilal-arikan/swarmgo/internal/config"
	"github.com/bilal-arikan/swarmgo/internal/logbuf"
)

// envWebviewURL, when set, makes this a connect-only secondary window: it boots
// no server and just renders the given loopback URL in a native window.
const envWebviewURL = "SWARMGO_WEBVIEW_URL"

func main() {
	// Opt into per-monitor DPI awareness before any window exists, so high-DPI
	// displays render crisp text instead of a bitmap-stretched (blurry) window.
	setDPIAware()
	// The webview event loop must own the main OS thread.
	runtime.LockOSThread()

	logs, logger := app.SetupLogging()

	if target := os.Getenv(envWebviewURL); target != "" {
		runSecondary(logger, target)
		return
	}
	runPrimary(logs, logger)
}

// runPrimary boots the in-process server and shows the first window. It also
// binds the swarmgoOpenWindow JS function so the UI can spawn more windows.
func runPrimary(logs *logbuf.Buffer, logger *slog.Logger) {
	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		return
	}
	// Let the OS pick a free loopback port so launching the desktop app never
	// collides with a separately running headless server.
	cfg.Addr = "127.0.0.1:0"

	application, err := app.Bootstrap(cfg, logs, logger)
	if err != nil {
		logger.Error("bootstrap failed", "error", err)
		return
	}

	go func() {
		if err := application.Serve(); err != nil {
			logger.Error("server failed", "error", err)
		}
	}()

	base := application.URL()
	if !waitForHealth(base, 8*time.Second) {
		logger.Warn("server did not become healthy in time; opening window anyway", "url", base)
	}

	w := newWindow()
	if w == nil {
		logger.Warn("WebView2 runtime unavailable; falling back to the default browser", "url", base)
		openBrowser(base)
		select {}
	}
	defer w.Destroy()

	// JS bridge: window.swarmgoOpenWindow(route) spawns a new native window
	// (a connect-only secondary process) scoped to that hash route.
	if err := w.Bind("swarmgoOpenWindow", func(route string) {
		spawnWindow(base, route)
	}); err != nil {
		logger.Warn("bind swarmgoOpenWindow failed", "error", err)
	}

	hwnd := uintptr(w.Window())
	get := func() (string, string) { p, t, _ := application.Appearance(); return p, t }
	p, t := get()
	applyTitleBar(hwnd, p, t)
	maximizeWindow(hwnd) // open maximized by default
	go watchTitleBar(w, hwnd, get, p, t)

	w.Navigate(base)
	w.Run() // blocks until the window is closed

	// Primary window closed → graceful shutdown so no orphan server lingers.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
}

// runSecondary renders a connect-only window pointed at an already-running
// server (the primary process owns it). target must be a loopback URL.
func runSecondary(logger *slog.Logger, target string) {
	u, err := url.Parse(target)
	if err != nil || !isLoopback(u) {
		logger.Error("refusing non-loopback SWARMGO_WEBVIEW_URL", "url", target)
		return
	}
	base := u.Scheme + "://" + u.Host
	waitForHealth(base, 8*time.Second) // primary may still be starting

	w := newWindow()
	if w == nil {
		openBrowser(target)
		select {}
	}
	defer w.Destroy()

	hwnd := uintptr(w.Window())
	get := func() (string, string) { return fetchAppearance(base) }
	p, t := get()
	applyTitleBar(hwnd, p, t)
	maximizeWindow(hwnd) // open maximized by default
	go watchTitleBar(w, hwnd, get, p, t)

	w.Navigate(target)
	w.Run()
}

// newWindow creates the standard SwarmGo WebView2 window (nil if the runtime is
// unavailable).
func newWindow() webview2.WebView {
	return webview2.NewWithOptions(webview2.WebViewOptions{
		Debug: false,
		WindowOptions: webview2.WindowOptions{
			Title:  "SwarmGo",
			Width:  1600,
			Height: 1000,
			Center: true,
		},
	})
}

// watchTitleBar re-applies the native title-bar tint whenever the appearance
// returned by get changes (the user can switch themes from Settings at runtime).
// DWM calls are marshalled onto the UI thread via Dispatch.
func watchTitleBar(w webview2.WebView, hwnd uintptr, get func() (string, string), preset, theme string) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		p, t := get()
		if p == preset && t == theme {
			continue
		}
		preset, theme = p, t
		w.Dispatch(func() { applyTitleBar(hwnd, p, t) })
	}
}

// fetchAppearance reads the live theme (preset, theme mode) from the running
// server so a connect-only window can tint its title bar. Falls back to a dark
// frame when unreachable.
func fetchAppearance(base string) (preset, theme string) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(base + "/api/settings")
	if err != nil {
		return "", "dark"
	}
	defer resp.Body.Close()
	var dto struct {
		Theme       string `json:"theme"`
		ThemePreset string `json:"themePreset"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return "", "dark"
	}
	return dto.ThemePreset, dto.Theme
}

// isLoopback reports whether u points at the local machine, so connect-only
// windows can only ever render the local SwarmGo server.
func isLoopback(u *url.URL) bool {
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// waitForHealth polls GET <base>/health until it returns 200 or the timeout
// elapses, so the window opens onto a ready server rather than a connection
// error.
func waitForHealth(base string, timeout time.Duration) bool {
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// openBrowser opens the default browser at url (Windows fallback path).
func openBrowser(target string) {
	// rundll32 url.dll avoids a transient console window that `cmd /c start` flashes.
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
}
