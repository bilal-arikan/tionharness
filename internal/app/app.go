// Package app holds the shared TionSwarm bootstrap sequence so multiple entry
// points (the headless server in cmd/tionswarm and the native desktop window in
// cmd/tionswarm-desktop) wire up the exact same subsystems without duplicating
// the boot logic. Bootstrap is behaviour-preserving: it is the former
// cmd/tionswarm/main.go body, lifted verbatim.
package app

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/api"
	"github.com/bilal-arikan/tionswarm/internal/backup"
	"github.com/bilal-arikan/tionswarm/internal/config"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/logbuf"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/settings"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// App is a fully wired, ready-to-serve TionSwarm instance.
type App struct {
	logger   *slog.Logger
	manager  *workspace.Manager
	server   *api.Server
	httpSrv  *http.Server
	listener net.Listener
	settings *settings.Store
	backups  *backup.Manager
}

// Appearance returns the current UI appearance settings (preset id, theme
// mode, accent hex). The desktop window uses these to tint its native title
// bar so the OS chrome matches the in-app theme; values reflect live edits
// from the Settings screen.
func (a *App) Appearance() (preset, theme, accent string) {
	cur := a.settings.Get()
	return cur.ThemePreset, cur.Theme, cur.Accent
}

// SetupLogging builds the ring-buffer-backed logger used by every entry point
// and installs it as the slog default. The returned buffer feeds the in-app
// Logs screen; the logger writes to both the buffer and stdout.
func SetupLogging() (*logbuf.Buffer, *slog.Logger) {
	logs := logbuf.New(2000)
	// Mirror logs to stdout and (best-effort) an on-disk file under the data dir
	// so the in-app Logs screen can reveal/copy the full history. If the file
	// cannot be opened, fall back to stdout-only — logging must never block boot.
	var w io.Writer = os.Stdout
	if f, err := openLogFile(); err == nil {
		w = io.MultiWriter(os.Stdout, f)
	}
	logger := slog.New(logs.Handler(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})))
	slog.SetDefault(logger)
	return logs, logger
}

// openLogFile opens the append-mode log file (creating its parent dir) that the
// Logs screen reveals. The handle intentionally lives for the whole process.
func openLogFile() (*os.File, error) {
	p := config.LogFilePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

// Bootstrap wires every subsystem (secret, settings, providers, tunables,
// workspace manager, API server) and opens the TCP listener. Passing
// cfg.Addr = "127.0.0.1:0" makes the OS pick a free port; Addr() then reports
// the resolved address. It does not begin serving — call Serve for that.
func Bootstrap(cfg *config.Config, logs *logbuf.Buffer, logger *slog.Logger) (*App, error) {
	// Open the listener FIRST, before the heavy workspace init below. The port
	// then accepts connections immediately (queued in the kernel backlog); Serve
	// (called after Bootstrap returns) drains them once init is done. Otherwise a
	// dev frontend (vite proxy) hitting /api during boot gets ECONNREFUSED until
	// init finishes — the "works on the 2nd start" race.
	// cfg.Addr "127.0.0.1:0" → OS picks a free port; Addr() reports the resolved one.
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return nil, err
	}

	secret, err := config.LoadSecret(cfg.DataDir)
	if err != nil {
		ln.Close()
		return nil, err
	}

	// Application settings (theme, providers, budgets, ...). One-time migration:
	// fold an env-provided Anthropic key into the encrypted settings store so the
	// store becomes the single source of truth thereafter.
	settingsStore, err := settings.Open(cfg.DataDir, secret)
	if err != nil {
		ln.Close()
		return nil, err
	}
	if settingsStore.Get().AnthropicKeyEnc == "" && cfg.AnthropicAPIKey != "" {
		key := cfg.AnthropicAPIKey
		if _, err := settingsStore.Apply(settings.Patch{AnthropicKey: &key}); err != nil {
			logger.Warn("migrate env anthropic key failed", "error", err)
		}
	}

	registry := providers.NewRegistry(cfg.AnthropicAPIKey)
	logger.Info("providers",
		"anthropic_api", settingsStore.AnthropicKey() != "",
		"claude_cli", registry.ClaudeCLIAvailable())

	// Process-wide tunables (autonomy pause, title-model override) shared by
	// every workspace runtime and updated from the settings screen.
	tun := agent.NewTunables()
	// The gated tool capabilities (shell / self-management / agent delegation) are
	// off by default and now live in the Settings screen (persisted settings.json,
	// pushed live via applySettings). The legacy TIONSWARM_ENABLE_* env vars act as a
	// one-time boot seed: when set truthy they ENABLE the matching capability in
	// settings (they never disable), so existing dev workflows keep working while
	// the Settings toggle is the source of truth thereafter.
	envOn := func(name string) *bool {
		if v := os.Getenv(name); v == "1" || strings.EqualFold(v, "true") {
			b := true
			return &b
		}
		return nil
	}
	// TIONSWARM_ENABLE_SELFMANAGE was removed 2026-07-01 and TIONSWARM_ENABLE_DELEGATION
	// on 2026-07-02 (both are always installed now; visibility is per-tool from the
	// Tools screen). Env-seedable capabilities: the shell, and code-execution mode
	// (TIONSWARM_CODE_MODE — run_code + generated MCP python bindings, _Docs/44;
	// also requires the shell capability to take effect).
	seed := settings.Patch{
		EnableShell:    envOn("TIONSWARM_ENABLE_SHELL"),
		EnableCodeMode: envOn("TIONSWARM_CODE_MODE"),
	}
	if seed.EnableShell != nil || seed.EnableCodeMode != nil {
		if _, err := settingsStore.Apply(seed); err != nil {
			logger.Warn("seed enable-flags from env failed", "error", err)
		} else {
			logger.Warn("gated tool capabilities seeded from env into settings (Settings screen is now the source of truth)")
		}
	}

	// Process-wide event bus: autonomous runtimes publish notifications here and
	// the API streams them to the UI over SSE.
	bus := events.NewBus()

	// Workspace manager: each workspace owns its own DB + agent runtime.
	manager, err := workspace.NewManager(cfg.DataDir, registry, tun, secret, bus, logs, logger)
	if err != nil {
		ln.Close()
		return nil, err
	}
	logger.Info("workspaces ready", "count", len(manager.List()))

	server := api.NewServer(manager, registry, settingsStore, tun, logs, bus, logger)
	// Wire the application-settings bridge into every workspace runtime so the
	// get_settings / update_settings self-management tools can read and live-apply
	// settings (the server owns the apply hook; the manager owns the runtimes).
	manager.SetSettingsBridge(server.SettingsBridge())
	// Wire the cross-workspace management bridge so the list/create/rename/
	// delete_workspace self-management tools can manage workspaces through the
	// manager (which the server holds) and notify the UI on change.
	manager.SetWorkspaceBridge(server.WorkspaceBridge())

	// Periodic workspace backups: one process-wide manager that snapshots every
	// workspace's data dir on the interval from settings. Wiring it into the
	// server pushes the live config and starts/stops the loop via applySettings.
	backups := backup.New(cfg.DataDir, func() []backup.Target {
		targets := manager.BackupTargets()
		out := make([]backup.Target, 0, len(targets))
		for _, t := range targets {
			out = append(out, backup.Target{ID: t.ID, Name: t.Name, Dir: t.Dir})
		}
		return out
	}, logger)
	server.SetBackupManager(backups)

	// Advertise this server's own loopback URL so CLI agents can reach the
	// in-process Interaction MCP endpoint (ask_user/todo_write) for their turn.
	// Use the resolved listener address so a ":0" port is correct.
	server.SetBaseURL(ln.Addr().String())

	httpSrv := &http.Server{
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		// IdleTimeout reaps idle keep-alive connections. WriteTimeout is left
		// unset on purpose: it would abort long-lived SSE streams.
		IdleTimeout: 120 * time.Second,
	}

	return &App{
		logger:   logger,
		manager:  manager,
		server:   server,
		httpSrv:  httpSrv,
		listener: ln,
		settings: settingsStore,
		backups:  backups,
	}, nil
}

// Addr returns the resolved listen address (host:port), with the real port even
// when Bootstrap was given a ":0" port.
func (a *App) Addr() string { return a.listener.Addr().String() }

// URL returns the loopback base URL a local client (browser or webview) should
// open. A wildcard/empty bind host is normalised to 127.0.0.1.
func (a *App) URL() string {
	host, port, ok := strings.Cut(a.Addr(), ":")
	if !ok {
		return "http://" + a.Addr()
	}
	// net may report "[::]" for a wildcard bind; collapse to loopback.
	if host == "" || host == "0.0.0.0" || host == "[::]" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + host + ":" + port
}

// Serve blocks serving HTTP until Shutdown is called (then returns nil) or the
// server fails.
func (a *App) Serve() error {
	a.logger.Info("TionSwarm starting", "addr", a.Addr())
	if err := a.httpSrv.Serve(a.listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the HTTP server and closes the workspace manager.
func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info("shutting down")
	if a.backups != nil {
		a.backups.Stop()
	}
	err := a.httpSrv.Shutdown(ctx)
	a.manager.Close() // closes each workspace Runtime (incl. the MCP pool the gateway shares)
	return err
}
