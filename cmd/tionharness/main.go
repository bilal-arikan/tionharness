// Package main is the TionHarness entry point.
// TionHarness is a self-hosted multi-agent AI runtime written in Go with a
// custom UI/UX.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bilal-arikan/tionharness/internal/app"
	"github.com/bilal-arikan/tionharness/internal/config"
	"github.com/bilal-arikan/tionharness/internal/proc"
)

func main() {
	logs, logger := app.SetupLogging()

	// macOS/Linux: a Finder/launchd/desktop-launcher start inherits a minimal PATH
	// that hides the claude/codex CLIs, node, rg and Homebrew tools.
	if added := proc.AugmentPATH(); len(added) > 0 {
		logger.Info("PATH augmented for child processes", "added", added)
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "error", err)
		os.Exit(1)
	}
	logger.Info("config loaded", "data_dir", cfg.DataDir, "addr", cfg.Addr)

	// Optional profiling server (loopback-only, off by default; TIONHARNESS_PPROF=1).
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
	// SIGHUP: closing the terminal on macOS/Linux must still run Shutdown, or
	// children in their own process groups are orphaned.
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
}
