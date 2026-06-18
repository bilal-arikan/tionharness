// Package main is the SwarmGo entry point.
// SwarmGo is a self-hosted multi-agent AI runtime, a Go reimplementation
// of SwarmClaw with a custom UI/UX.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/api"
	"github.com/bilal/swarmgo/internal/config"
	"github.com/bilal/swarmgo/internal/events"
	"github.com/bilal/swarmgo/internal/logbuf"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/settings"
	"github.com/bilal/swarmgo/internal/workspace"
)

func main() {
	// Capture every log record into a ring buffer (for the in-app Logs screen)
	// while still writing to stdout.
	logs := logbuf.New(2000)
	logger := slog.New(logs.Handler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	// Make this the process default so library code that logs via slog.Default()
	// (e.g. the provider transport retry warnings) is captured in the ring buffer
	// and surfaced on the in-app Logs screen.
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}
	logger.Info("config loaded", "data_dir", cfg.DataDir, "addr", cfg.Addr)

	secret, err := config.LoadSecret(cfg.DataDir)
	if err != nil {
		logger.Error("secret load failed", "error", err)
		os.Exit(1)
	}

	// Application settings (theme, providers, budgets, ...). One-time migration:
	// fold an env-provided Anthropic key into the encrypted settings store so the
	// store becomes the single source of truth thereafter.
	settingsStore, err := settings.Open(cfg.DataDir, secret)
	if err != nil {
		logger.Error("settings load failed", "error", err)
		os.Exit(1)
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
	// pushed live via applySettings). The legacy SWARMGO_ENABLE_* env vars act as a
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
	seed := settings.Patch{
		EnableShell:      envOn("SWARMGO_ENABLE_SHELL"),
		EnableSelfManage: envOn("SWARMGO_ENABLE_SELFMANAGE"),
		EnableDelegation: envOn("SWARMGO_ENABLE_DELEGATION"),
	}
	if seed.EnableShell != nil || seed.EnableSelfManage != nil || seed.EnableDelegation != nil {
		if _, err := settingsStore.Apply(seed); err != nil {
			logger.Warn("seed enable-flags from env failed", "error", err)
		} else {
			logger.Warn("gated tool capabilities seeded from SWARMGO_ENABLE_* env into settings (Settings screen is now the source of truth)")
		}
	}

	// Process-wide event bus: autonomous runtimes publish notifications here and
	// the API streams them to the UI over SSE.
	bus := events.NewBus()

	// Workspace manager: each workspace owns its own DB + agent runtime.
	manager, err := workspace.NewManager(cfg.DataDir, registry, tun, secret, bus, logs, logger)
	if err != nil {
		logger.Error("workspace manager init failed", "error", err)
		os.Exit(1)
	}
	defer manager.Close()
	logger.Info("workspaces ready", "count", len(manager.List()))

	server := api.NewServer(manager, registry, settingsStore, tun, logs, bus, logger)
	// Advertise this server's own loopback URL so CLI agents can reach the
	// in-process Interaction MCP endpoint (ask_user/todo_write) for their turn.
	server.SetBaseURL(cfg.Addr)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		// IdleTimeout reaps idle keep-alive connections. WriteTimeout is left
		// unset on purpose: it would abort long-lived SSE streams.
		IdleTimeout: 120 * time.Second,
	}

	go func() {
		logger.Info("SwarmGo starting", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
}
