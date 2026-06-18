package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/events"
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

// fire executes a schedule on its cron tick: it owns its own timeout context so
// the run survives even if nothing else holds one.
func (s *Scheduler) fire(scheduleID string) {
	ctx, cancel := context.WithTimeout(context.Background(), scheduleTimeout)
	defer cancel()
	_ = s.run(ctx, scheduleID, "schedule")
}

// RunNow fires a schedule immediately on demand (manual "Run" button), regardless
// of whether it is enabled, and returns any execution error. The attempt's
// outcome is persisted on the schedule (lastDeliveryStatus/error) just like a
// cron tick, so the UI reflects it after a reload.
func (s *Scheduler) RunNow(ctx context.Context, scheduleID string) error {
	return s.run(ctx, scheduleID, "manual")
}

// run executes a schedule: run its task or deliver its prompt to the agent, then
// record the outcome. trigger labels what initiated it (schedule | manual).
func (s *Scheduler) run(ctx context.Context, scheduleID, trigger string) error {
	sc, err := s.db.GetSchedule(ctx, scheduleID)
	if err != nil {
		s.logger.Warn("schedule fire: lookup failed", "schedule", scheduleID, "trigger", trigger, "error", err)
		return err
	}

	// Log the start of every fire so a run is traceable even if it later hangs
	// or the process dies mid-flight — the previous code only logged on success.
	kind := "prompt"
	if sc.TaskID != "" {
		kind = "task"
	}
	s.logger.Info("schedule fire: begin",
		"schedule", scheduleID, "trigger", trigger, "kind", kind,
		"agent", sc.AgentID, "task", sc.TaskID, "cron", sc.CronExpr)

	var fireErr error
	var sessionID string
	if sc.TaskID != "" {
		// Task runs publish their own outcome event via RunTask.
		_, fireErr = s.rt.RunTask(ctx, sc.TaskID, trigger)
	} else {
		sessionID, fireErr = s.deliverPrompt(ctx, sc)
		s.emitPromptDelivery(sc, sessionID, fireErr)
	}

	status := "success"
	errText := ""
	if fireErr != nil {
		status = "failure"
		errText = fireErr.Error()
	}

	next := s.nextRun(scheduleID)
	if err := s.db.SetScheduleDelivery(ctx, scheduleID, status, errText, next); err != nil {
		s.logger.Warn("schedule fire: persist delivery failed",
			"schedule", scheduleID, "trigger", trigger, "error", err)
	}

	// Log the outcome at a level that matches it: a failure is an Error with the
	// full message + the affected agent/task/session, so the logs view actually
	// explains what the desktop notification only hinted at. Successes stay Info.
	if fireErr != nil {
		s.logger.Error("schedule fire: failed",
			"schedule", scheduleID, "trigger", trigger, "kind", kind,
			"agent", sc.AgentID, "task", sc.TaskID, "session", sessionID,
			"error", fireErr)
	} else {
		s.logger.Info("schedule fire: ok",
			"schedule", scheduleID, "trigger", trigger, "kind", kind,
			"agent", sc.AgentID, "session", sessionID)
	}
	return fireErr
}

// emitPromptDelivery publishes the outcome of a scheduled prompt as a desktop
// notification. Both success and failure now deep-link to the agent's schedule
// session, since deliverPrompt records the reply (or the error) there as a chat
// turn — clicking opens the thread with the outcome shown inline. Only when no
// session was reached does a failure fall back to the logs view.
func (s *Scheduler) emitPromptDelivery(sc db.Schedule, sessionID string, err error) {
	name := s.rt.agentName(sc.AgentID)
	if err != nil {
		target := map[string]string{"view": "logs", "agentId": sc.AgentID}
		if sessionID != "" {
			target = map[string]string{"view": "chat", "sessionId": sessionID}
		}
		s.rt.publish(events.Event{
			Type:   "schedule",
			Level:  "error",
			Title:  "⏰ Zamanlama başarısız — " + name,
			Body:   notifyLine(err.Error(), 200),
			Target: target,
		})
		return
	}
	s.rt.publish(events.Event{
		Type:   "schedule",
		Level:  "success",
		Title:  "⏰ Zamanlanmış prompt çalıştı — " + name,
		Body:   notifyLine(sc.Prompt, 120),
		Target: map[string]string{"view": "chat", "sessionId": sessionID},
	})
}

// notifyLine condenses a string into a single, rune-capped line fit for a
// desktop notification body: it keeps only the first line and appends an
// ellipsis when truncated. Rune-based so Turkish characters never get split.
func notifyLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if r := []rune(s); len(r) > max {
		return strings.TrimSpace(string(r[:max])) + "…"
	}
	return s
}

// deliverPrompt sends a standalone scheduled prompt to the agent and logs the
// reply in the agent's dedicated "schedule" session. It returns the session id
// (when reached) so callers can deep-link a notification to it.
func (s *Scheduler) deliverPrompt(ctx context.Context, sc db.Schedule) (string, error) {
	if sc.Prompt == "" {
		return "", fmt.Errorf("schedule %s has neither task nor prompt", sc.ID)
	}
	agent, err := s.db.GetAgent(ctx, sc.AgentID)
	if err != nil {
		s.logger.Warn("schedule deliver: agent lookup failed",
			"schedule", sc.ID, "agent", sc.AgentID, "error", err)
		return "", err
	}
	session, err := s.db.GetOrCreateKindSession(ctx, sc.AgentID, "schedule", "⏰ Schedule")
	if err != nil {
		s.logger.Warn("schedule deliver: session open failed",
			"schedule", sc.ID, "agent", sc.AgentID, "error", err)
		return "", err
	}
	// Record the scheduled prompt as a user turn first, so the schedule thread
	// reads as a real conversation (the UI shows what was asked).
	if _, err := s.db.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      "user",
		Text:      sc.Prompt,
	}); err != nil {
		return session.ID, err
	}
	output, steps, err := s.rt.invokeTraced(WithCallKind(ctx, KindSchedule), agent, sc.Prompt, true) // scheduled = autonomous
	if err != nil {
		// Log the provider/tool-loop failure with the agent + its provider/model,
		// so the logs view pinpoints what failed (e.g. missing key, model error)
		// rather than leaving only a notification behind.
		s.logger.Error("schedule deliver: agent invoke failed",
			"schedule", sc.ID, "agent", sc.AgentID, "session", session.ID,
			"provider", agent.Provider, "model", agent.Model, "error", err)
		// Surface the failure inside the schedule thread itself, not just in the
		// delivery status/logs — otherwise the user opens the session and sees
		// their prompt with no reply and no clue what went wrong. Persist the
		// error as an assistant turn so the chat reads the failure inline.
		if _, addErr := s.db.AddMessage(ctx, db.Message{
			SessionID: session.ID,
			AgentID:   sc.AgentID,
			Role:      "assistant",
			Text:      "⚠️ Zamanlanmış prompt çalıştırılamadı:\n\n" + err.Error(),
			Steps:     encodeSteps(steps),
		}); addErr != nil {
			s.logger.Warn("schedule: failed to record error reply", "schedule", sc.ID, "error", addErr)
		}
		return session.ID, err
	}
	// A successful provider call that yields no text still leaves the thread
	// looking unanswered; make the empty turn explicit so it never reads as a
	// silent no-reply.
	if strings.TrimSpace(output) == "" {
		output = "ℹ️ Ajan bu zamanlanmış prompt için boş yanıt döndürdü."
	}
	// Stamp the reply with the agent id (so its avatar/identity renders) and its
	// activity trace (so tool/thinking steps show like a normal chat turn).
	_, err = s.db.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		AgentID:   sc.AgentID,
		Role:      "assistant",
		Text:      output,
		Steps:     encodeSteps(steps),
	})
	return session.ID, err
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
