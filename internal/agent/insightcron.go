package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/bilal-arikan/tionswarm/internal/insight"
)

// InsightCron drives automatic retrospective scans (_Docs/60, Faz 3) on a cron
// schedule read from the workspace's insight settings (AutoScanCron). It is a
// dedicated, self-contained timer — separate from the prompt/flow Scheduler —
// because an insight scan is a direct runtime call (RunInsightScan), not an
// agent prompt: keeping it deterministic avoids routing a scan through a model.
// One InsightCron per workspace, so timers never cross workspace boundaries.
type InsightCron struct {
	rt     *Runtime
	logger *slog.Logger

	mu   sync.Mutex
	cron *cron.Cron
}

// NewInsightCron binds an auto-scan timer to a workspace runtime.
func NewInsightCron(rt *Runtime, logger *slog.Logger) *InsightCron {
	return &InsightCron{rt: rt, logger: logger}
}

// Start reads the insight settings and arms the auto-scan timer (no-op when
// AutoScanCron is empty). Safe to call once at workspace boot.
func (c *InsightCron) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rebuildLocked()
}

// Reload re-reads the settings and re-arms the timer. Call after the settings
// endpoint changes AutoScanCron so the new cadence takes effect immediately.
func (c *InsightCron) Reload(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rebuildLocked()
}

// rebuildLocked stops any running timer and re-arms it from current settings.
// An invalid cron expression is logged and leaves the timer disarmed rather than
// crashing the workspace — the scan can still be run manually.
func (c *InsightCron) rebuildLocked() error {
	if c.cron != nil {
		c.cron.Stop()
		c.cron = nil
	}
	settings, err := insight.LoadSettings(c.rt.db.Root())
	if err != nil {
		return err
	}
	if settings.AutoScanCron == "" {
		c.logger.Info("insight auto-scan disabled (no cron set)")
		return nil
	}
	cr := cron.New()
	agentID := settings.AutoScanAgentID
	if _, err := cr.AddFunc(settings.AutoScanCron, func() { c.fire(agentID) }); err != nil {
		c.logger.Warn("insight auto-scan: invalid cron expression", "expr", settings.AutoScanCron, "error", err)
		return nil
	}
	cr.Start()
	c.cron = cr
	c.logger.Info("insight auto-scan armed", "cron", settings.AutoScanCron)
	return nil
}

// fire runs one scheduled scan with its own timeout context, so it survives even
// if nothing else holds one. A full-scope scan runs every enabled lens; the
// ledger keeps it incremental (unchanged sessions are skipped).
func (c *InsightCron) fire(agentID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	c.logger.Info("insight auto-scan: begin", "agent", agentID)
	res, err := c.rt.RunInsightScan(ctx, insight.ScanScope{}, agentID)
	if err != nil {
		c.logger.Error("insight auto-scan: failed", "error", err)
		return
	}
	c.logger.Info("insight auto-scan: ok",
		"sessions", res.Sessions, "analyzed", res.Analyzed,
		"skipped", res.Skipped, "findings", res.Findings, "errors", len(res.Errors))
}

// Stop halts auto-scan firing. Safe to call multiple times.
func (c *InsightCron) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cron != nil {
		c.cron.Stop()
		c.cron = nil
	}
}
