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
	"syscall"
	"time"

	"github.com/bilal/swarmgo/internal/api"
	"github.com/bilal/swarmgo/internal/config"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/workspace"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}
	logger.Info("config loaded", "data_dir", cfg.DataDir, "addr", cfg.Addr)

	if _, err := config.LoadSecret(cfg.DataDir); err != nil {
		logger.Error("secret load failed", "error", err)
		os.Exit(1)
	}

	registry := providers.NewRegistry(cfg.AnthropicAPIKey)
	logger.Info("providers",
		"anthropic_api", cfg.AnthropicAPIKey != "",
		"claude_cli", registry.ClaudeCLIAvailable())

	// Workspace manager: each workspace owns its own DB + agent runtime.
	manager, err := workspace.NewManager(cfg.DataDir, registry, logger)
	if err != nil {
		logger.Error("workspace manager init failed", "error", err)
		os.Exit(1)
	}
	defer manager.Close()
	logger.Info("workspaces ready", "count", len(manager.List()))

	server := api.NewServer(manager, registry, logger)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
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
