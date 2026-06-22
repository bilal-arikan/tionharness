// Package main is the SwarmGo entry point.
// SwarmGo is a self-hosted multi-agent AI runtime, a Go reimplementation
// of SwarmClaw with a custom UI/UX.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/app"
	"github.com/bilal-arikan/swarmgo/internal/config"
)

func main() {
	logs, logger := app.SetupLogging()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}
	logger.Info("config loaded", "data_dir", cfg.DataDir, "addr", cfg.Addr)

	// Optional profiling server (loopback-only, off by default; SWARMGO_PPROF=1).
	startPprof(logger)

	application, err := app.Bootstrap(cfg, logs, logger)
	if err != nil {
		logger.Error("bootstrap failed", "error", err)
		os.Exit(1)
	}

	go func() {
		if err := application.Serve(); err != nil {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
}
