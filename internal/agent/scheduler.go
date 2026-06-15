package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/bilal/swarmgo/internal/db"
)

// scheduleTimeout bounds a single scheduled fire (task run or prompt delivery).
const scheduleTimeout = 120 * time.Second

// Scheduler runs a workspace's enabled schedules on their cron expressions.
// Cron expressions use the standard 5-field format (minute hour dom month dow).
// Each workspace owns one Scheduler, so timers never cross workspace boundaries.
type Scheduler struct {
	db     *db.DB
	rt     *Runtime
	logger *slog.Logger

	mu      sync.Mutex
	cron    *cron.Cron
	entries map[string]cron.EntryID // schedule id -> cron entry
}

// NewScheduler constructs a scheduler bound to a workspace's DB and runtime.
func NewScheduler(database *db.DB, rt *Runtime, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		db:      database,
		rt:      rt,
		logger:  logger,
		entries: make(map[string]cron.EntryID),
	}
}

// Start loads enabled schedules from the DB and begins firing them.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rebuildLocked(ctx)
}

// Reload rebuilds the cron table from the current DB state. Call after any
// schedule create/toggle/delete so timers reflect the latest config.
func (s *Scheduler) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rebuildLocked(ctx)
}

// rebuildLocked stops the running cron and reconstructs it from enabled rows.
func (s *Scheduler) rebuildLocked(ctx context.Context) error {
	if s.cron != nil {
		s.cron.Stop()
	}
	s.cron = cron.New()
	s.entries = make(map[string]cron.EntryID)

	schedules, err := s.db.ListEnabledSchedules(ctx)
	if err != nil {
		return err
	}

	for _, sc := range schedules {
		id := sc.ID // capture for the closure
		entryID, err := s.cron.AddFunc(sc.CronExpr, func() { s.fire(id) })
		if err != nil {
			s.logger.Warn("invalid cron expression", "schedule", id, "expr", sc.CronExpr, "error", err)
			_ = s.db.SetScheduleDelivery(ctx, id, "error", "invalid cron: "+err.Error(), 0)
			continue
		}
		s.entries[id] = entryID
	}

	s.cron.Start()
	s.syncNextRunLocked(ctx)
	s.logger.Info("scheduler started", "schedules", len(s.entries))
	return nil
}

// syncNextRunLocked persists each entry's next fire time for display.
func (s *Scheduler) syncNextRunLocked(ctx context.Context) {
	for id, entryID := range s.entries {
		entry := s.cron.Entry(entryID)
		if entry.Valid() && !entry.Next.IsZero() {
			_ = s.db.SetScheduleDelivery(ctx, id, "", "", entry.Next.Unix())
		}
	}
}

// fire executes a schedule: run its task or deliver its prompt to the agent.
func (s *Scheduler) fire(scheduleID string) {
	ctx, cancel := context.WithTimeout(context.Background(), scheduleTimeout)
	defer cancel()

	sc, err := s.db.GetSchedule(ctx, scheduleID)
	if err != nil {
		s.logger.Warn("schedule fire: lookup failed", "schedule", scheduleID, "error", err)
		return
	}

	var fireErr error
	if sc.TaskID != "" {
		_, fireErr = s.rt.RunTask(ctx, sc.TaskID, "schedule")
	} else {
		fireErr = s.deliverPrompt(ctx, sc)
	}

	status := "success"
	errText := ""
	if fireErr != nil {
		status = "failure"
		errText = fireErr.Error()
	}

	next := s.nextRun(scheduleID)
	_ = s.db.SetScheduleDelivery(ctx, scheduleID, status, errText, next)
	s.logger.Info("schedule fired", "schedule", scheduleID, "status", status)
}

// deliverPrompt sends a standalone scheduled prompt to the agent and logs the
// reply in the agent's dedicated "schedule" session.
func (s *Scheduler) deliverPrompt(ctx context.Context, sc db.Schedule) error {
	if sc.Prompt == "" {
		return fmt.Errorf("schedule %s has neither task nor prompt", sc.ID)
	}
	agent, err := s.db.GetAgent(ctx, sc.AgentID)
	if err != nil {
		return err
	}
	output, err := s.rt.invoke(ctx, agent, sc.Prompt, true) // scheduled = autonomous
	if err != nil {
		return err
	}
	session, err := s.db.GetOrCreateKindSession(ctx, sc.AgentID, "schedule", "⏰ Schedule")
	if err != nil {
		return err
	}
	_, err = s.db.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      "assistant",
		Text:      "[schedule] " + output,
	})
	return err
}

// nextRun returns the unix time of a schedule's next fire (0 if unknown).
func (s *Scheduler) nextRun(scheduleID string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron == nil {
		return 0
	}
	entryID, ok := s.entries[scheduleID]
	if !ok {
		return 0
	}
	entry := s.cron.Entry(entryID)
	if entry.Valid() && !entry.Next.IsZero() {
		return entry.Next.Unix()
	}
	return 0
}

// Stop halts all scheduled firing. Safe to call multiple times.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		s.cron.Stop()
		s.cron = nil
	}
	s.entries = make(map[string]cron.EntryID)
}
