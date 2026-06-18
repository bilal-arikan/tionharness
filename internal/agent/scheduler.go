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
	// wakeTimers holds the armed one-shot (schedule_wake) timers, keyed by
	// schedule id, so a rebuild can cancel and re-arm them without leaking.
	wakeTimers map[string]*time.Timer
}

// NewScheduler constructs a scheduler bound to a workspace's DB and runtime.
func NewScheduler(database *db.DB, rt *Runtime, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		db:         database,
		rt:         rt,
		logger:     logger,
		entries:    make(map[string]cron.EntryID),
		wakeTimers: make(map[string]*time.Timer),
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
	// Cancel any previously armed one-shot wake timers before re-arming from the
	// current DB state, so a reload never leaves a stale timer running.
	for id, t := range s.wakeTimers {
		t.Stop()
		delete(s.wakeTimers, id)
	}

	schedules, err := s.db.ListEnabledSchedules(ctx)
	if err != nil {
		return err
	}

	for _, sc := range schedules {
		// One-shot wakes are timer-driven, not cron-driven: (re-)arm them and skip
		// the cron table entirely (their CronExpr is empty).
		if sc.OneShot {
			s.armWakeLocked(sc)
			continue
		}
		// Past its optional end date: auto-disable so it leaves the cron table for
		// good, and skip adding a timer.
		if scheduleExpired(sc) {
			if err := s.db.SetScheduleEnabled(ctx, sc.ID, false); err != nil {
				s.logger.Warn("expired schedule disable failed", "schedule", sc.ID, "error", err)
			}
			s.logger.Info("schedule expired; auto-disabled", "schedule", sc.ID, "expiresAt", sc.ExpiresAt)
			continue
		}
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

// scheduleExpired reports whether a schedule's optional end date has passed.
// A zero ExpiresAt means "no end date" (never expires).
func scheduleExpired(sc db.Schedule) bool {
	return sc.ExpiresAt > 0 && time.Now().Unix() >= sc.ExpiresAt
}

// fire executes a schedule on its cron tick: it owns its own timeout context so
// the run survives even if nothing else holds one. A cron tick that lands after
// the schedule's end date is skipped and the schedule is auto-disabled (a reload
// then drops it from the cron table).
func (s *Scheduler) fire(scheduleID string) {
	ctx, cancel := context.WithTimeout(context.Background(), scheduleTimeout)
	defer cancel()
	if sc, err := s.db.GetSchedule(ctx, scheduleID); err == nil && scheduleExpired(sc) {
		if err := s.db.SetScheduleEnabled(ctx, scheduleID, false); err != nil {
			s.logger.Warn("expired schedule disable failed", "schedule", scheduleID, "error", err)
		}
		s.logger.Info("schedule expired; skipping fire", "schedule", scheduleID, "expiresAt", sc.ExpiresAt)
		go func() { _ = s.Reload(context.Background()) }()
		return
	}
	_ = s.run(ctx, scheduleID, "schedule")
}

// maxWakeDelay bounds how far in the future a one-shot wake may be scheduled, so
// a runaway delay can never pin a timer for days.
const maxWakeDelay = time.Hour

// armWakeLocked schedules a one-shot wake to fire at sc.FireAt (clamped to a
// sane window). Caller holds s.mu. An overdue wake fires almost immediately.
func (s *Scheduler) armWakeLocked(sc db.Schedule) {
	delay := time.Until(time.Unix(sc.FireAt, 0))
	if delay < time.Second {
		delay = time.Second // overdue (e.g. recovered after a restart) → fire now-ish
	}
	if delay > maxWakeDelay {
		delay = maxWakeDelay
	}
	id := sc.ID
	s.wakeTimers[id] = time.AfterFunc(delay, func() { s.fireWake(id) })
}

// fireWake runs a one-shot wake on its timer: it delivers the wake prompt back
// into its originating chat session, then removes the spent schedule (a wake is
// single-use). It owns its own timeout context so the run survives independently.
func (s *Scheduler) fireWake(scheduleID string) {
	s.mu.Lock()
	delete(s.wakeTimers, scheduleID)
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), scheduleTimeout)
	defer cancel()

	sc, err := s.db.GetSchedule(ctx, scheduleID)
	if err != nil {
		s.logger.Warn("wake fire: lookup failed", "schedule", scheduleID, "error", err)
		return
	}
	s.logger.Info("wake fire: begin", "schedule", scheduleID, "agent", sc.AgentID, "session", sc.SessionID)

	fireErr := s.deliverWake(ctx, sc)
	if fireErr != nil {
		s.logger.Error("wake fire: failed",
			"schedule", scheduleID, "agent", sc.AgentID, "session", sc.SessionID, "error", fireErr)
	} else {
		s.logger.Info("wake fire: ok", "schedule", scheduleID, "agent", sc.AgentID, "session", sc.SessionID)
	}
	// A wake is single-use: drop the spent row so it never lingers in the routine
	// list or re-fires after a restart.
	if err := s.db.DeleteSchedule(ctx, scheduleID); err != nil {
		s.logger.Warn("wake fire: cleanup failed", "schedule", scheduleID, "error", err)
	}
}

// deliverWake re-delivers a wake prompt into its originating chat session as a
// fresh turn: it records the prompt as a user message, runs the agent (tracing
// its activity), and records the reply — exactly like a normal chat turn, so the
// conversation visibly continues. It brackets the run with "chat" events tagged
// phase=start / phase=done so an open session screen reloads the transcript and
// shows a thinking indicator while the wake runs.
func (s *Scheduler) deliverWake(ctx context.Context, sc db.Schedule) error {
	if sc.SessionID == "" {
		return fmt.Errorf("wake %s has no session to resume", sc.ID)
	}
	if strings.TrimSpace(sc.Prompt) == "" {
		return fmt.Errorf("wake %s has no prompt", sc.ID)
	}
	agent, err := s.db.GetAgent(ctx, sc.AgentID)
	if err != nil {
		return err
	}
	if _, err := s.db.GetSession(ctx, sc.SessionID); err != nil {
		return fmt.Errorf("wake session %s gone: %w", sc.SessionID, err)
	}

	// Record the wake prompt as a user turn and tell the open screen to refresh +
	// show a thinking indicator (phase=start).
	if _, err := s.db.AddMessage(ctx, db.Message{
		SessionID: sc.SessionID,
		Role:      "user",
		Text:      sc.Prompt,
	}); err != nil {
		return err
	}
	s.emitWakeEvent(sc, "start", "⏰ Otomatik uyandırma çalışıyor")

	output, steps, invokeErr := s.rt.invokeTraced(WithCallKind(ctx, KindSchedule), agent, sc.Prompt, true)
	if invokeErr != nil {
		if _, addErr := s.db.AddMessage(ctx, db.Message{
			SessionID: sc.SessionID,
			AgentID:   sc.AgentID,
			Role:      "assistant",
			Text:      "⚠️ Otomatik uyandırma çalıştırılamadı:\n\n" + invokeErr.Error(),
			Steps:     encodeSteps(steps),
		}); addErr != nil {
			s.logger.Warn("wake: failed to record error reply", "schedule", sc.ID, "error", addErr)
		}
		s.emitWakeEvent(sc, "done", "⏰ Otomatik uyandırma başarısız")
		return invokeErr
	}
	if strings.TrimSpace(output) == "" {
		output = "ℹ️ Ajan bu uyandırma için boş yanıt döndürdü."
	}
	_, err = s.db.AddMessage(ctx, db.Message{
		SessionID: sc.SessionID,
		AgentID:   sc.AgentID,
		Role:      "assistant",
		Text:      output,
		Steps:     encodeSteps(steps),
	})
	s.emitWakeEvent(sc, "done", "⏰ Otomatik uyandırma tamamlandı")
	return err
}

// emitWakeEvent publishes a "chat"-typed event for a wake phase. The frontend
// reloads the open session's transcript on any chat event; phase=start also
// raises the thinking indicator, phase=done clears it.
func (s *Scheduler) emitWakeEvent(sc db.Schedule, phase, title string) {
	s.rt.publish(events.Event{
		Type:   "chat",
		Level:  "success",
		Title:  title,
		Body:   notifyLine(sc.Prompt, 120),
		Target: map[string]string{"view": "chat", "sessionId": sc.SessionID, "phase": phase},
	})
}

// RunNow fires a schedule immediately on demand (manual "Run" button), regardless
// of whether it is enabled, and returns any execution error. The attempt's
// outcome is persisted on the schedule (lastDeliveryStatus/error) just like a
// cron tick, so the UI reflects it after a reload.
func (s *Scheduler) RunNow(ctx context.Context, scheduleID string) error {
	return s.run(ctx, scheduleID, "manual")
}

// run executes a schedule: it delivers the schedule's prompt to its agent, then
// records the outcome. trigger labels what initiated it (schedule | manual).
func (s *Scheduler) run(ctx context.Context, scheduleID, trigger string) error {
	sc, err := s.db.GetSchedule(ctx, scheduleID)
	if err != nil {
		s.logger.Warn("schedule fire: lookup failed", "schedule", scheduleID, "trigger", trigger, "error", err)
		return err
	}

	// Log the start of every fire so a run is traceable even if it later hangs
	// or the process dies mid-flight — the previous code only logged on success.
	s.logger.Info("schedule fire: begin",
		"schedule", scheduleID, "trigger", trigger,
		"agent", sc.AgentID, "cron", sc.CronExpr)

	sessionID, fireErr := s.deliverPrompt(ctx, sc)
	s.emitPromptDelivery(sc, sessionID, fireErr)

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
	// full message + the affected agent/session, so the logs view actually
	// explains what the desktop notification only hinted at. Successes stay Info.
	if fireErr != nil {
		s.logger.Error("schedule fire: failed",
			"schedule", scheduleID, "trigger", trigger,
			"agent", sc.AgentID, "session", sessionID,
			"error", fireErr)
	} else {
		s.logger.Info("schedule fire: ok",
			"schedule", scheduleID, "trigger", trigger,
			"agent", sc.AgentID, "session", sessionID)
	}
	return fireErr
}

// emitPromptDelivery publishes the outcome of a scheduled prompt as a desktop
// notification. Both success and failure deep-link to the run's transcript in
// the executions feed (Session.Kind "schedule"), since deliverPrompt records the
// reply (or the error) there as a turn — clicking opens it with the outcome shown
// inline. Only when no session was reached does a failure fall back to the logs
// view.
func (s *Scheduler) emitPromptDelivery(sc db.Schedule, sessionID string, err error) {
	name := s.rt.agentName(sc.AgentID)
	if err != nil {
		target := map[string]string{"view": "logs", "agentId": sc.AgentID}
		if sessionID != "" {
			target = map[string]string{"view": "executions", "sessionId": sessionID}
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
		Target: map[string]string{"view": "executions", "sessionId": sessionID},
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
	s.rt.trackSession(session.ID)
	output, steps, err := s.rt.invokeTraced(WithCallKind(ctx, KindSchedule), agent, sc.Prompt, true) // scheduled = autonomous
	s.rt.untrackSession(session.ID)
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
	for id, t := range s.wakeTimers {
		t.Stop()
		delete(s.wakeTimers, id)
	}
}
