//go:build windows

// Command swarmgo-desktop runs SwarmGo as a native desktop application: it
// boots the same in-process server as cmd/swarmgo on a free loopback port, then
// shows the embedded UI in a WebView2 window instead of a browser tab. It is
// Windows-only and uses github.com/jchv/go-webview2 (pure Go, no CGO; relies on
// the WebView2 runtime, which ships with Windows 11).
package main

import (
	"context"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/jchv/go-webview2"

	"github.com/bilal/swarmgo/internal/app"
	"github.com/bilal/swarmgo/internal/config"
)

func main() {
	// The webview event loop must own the main OS thread.
	runtime.LockOSThread()

	logs, logger := app.SetupLogging()

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

	url := application.URL()
	if !waitForHealth(url, 8*time.Second) {
		logger.Warn("server did not become healthy in time; opening window anyway", "url", url)
	}

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug: false,
		WindowOptions: webview2.WindowOptions{
			Title:  "SwarmGo",
			Width:  1280,
			Height: 800,
			Center: true,
		},
	})
	if w == nil {
		// WebView2 runtime missing (rare on Win11). Fall back to the default
		// browser so the app is still usable, then keep serving until interrupt.
		logger.Warn("WebView2 runtime unavailable; falling back to the default browser", "url", url)
		openBrowser(url)
		select {} // block forever; user closes via taskbar / Ctrl-C
	}
	defer w.Destroy()

	// Tint the native title bar (caption, min/max/close buttons, border) to match
	// the in-app theme, and keep it in sync if the user switches themes.
	hwnd := uintptr(w.Window())
	preset, theme, _ := application.Appearance()
	applyTitleBar(hwnd, preset, theme)
	go watchTitleBar(w, hwnd, application, preset, theme)

	w.Navigate(url)
	w.Run() // blocks until the window is closed

	// Window closed → graceful shutdown so no orphan server goroutine lingers.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
}

// watchTitleBar re-applies the native title-bar tint whenever the in-app theme
// changes (the user can switch presets from the Settings screen at runtime).
// DWM calls are marshalled onto the UI thread via Dispatch.
func watchTitleBar(w webview2.WebView, hwnd uintptr, application *app.App, preset, theme string) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		p, t, _ := application.Appearance()
		if p == preset && t == theme {
			continue
		}
		preset, theme = p, t
		w.Dispatch(func() { applyTitleBar(hwnd, p, t) })
	}
}

// waitForHealth polls GET <url>/health until it returns 200 or the timeout
// elapses, so the window opens onto a ready server rather than a connection
// error.
func waitForHealth(url string, timeout time.Duration) bool {
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url + "/health")
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
func openBrowser(url string) {
	// rundll32 url.dll avoids a transient console window that `cmd /c start` flashes.
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
