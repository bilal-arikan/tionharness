package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// The scheduled-fire deadline is settings-driven (ScheduleTimeoutMinutes, default
// DefaultScheduleTimeoutMinutes) and read live via s.rt.tun.ScheduleTimeout().

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
	ctx, cancel := context.WithTimeout(context.Background(), s.rt.tun.ScheduleTimeout())
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

	ctx, cancel := context.WithTimeout(context.Background(), s.rt.tun.ScheduleTimeout())
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
	// A wake re-enters a real, human-visible chat session: claim its per-session
	// turn slot so the wake turn never overlaps a concurrent user turn (inbox
	// worker / direct chat) or, for a coordinator, an auto turn — worker
	// notifications arriving meanwhile coalesce and run after release.
	release := s.rt.claimSessionTurnSlot(sc.SessionID, turnqueue.KindWake, "uyandırma")
	defer release()

	// Record the wake prompt as a user turn and tell the open screen to refresh +
	// show a thinking indicator (phase=start). Origin "wake" makes the UI render it
	// as a "⏰ Otomatik devam" note, not a user bubble — the agent resumed itself,
	// the user did not re-ask. Role stays "user" so the model's context is unchanged.
	wakeMsg, err := s.db.AddMessage(ctx, db.Message{
		SessionID: sc.SessionID,
		Role:      "user",
		Origin:    "wake",
		Text:      sc.Prompt,
	})
	if err != nil {
		return err
	}
	// Bridge it to the hub so a window watching this session renders the wake note
	// live and in order before the reply (_Docs/58), not only on reload.
	s.rt.emitInjectedUserNote(sc.SessionID, wakeMsg)
	s.emitWakeEvent(sc, "start", "⏰ Otomatik uyandırma çalışıyor")

	// A wake re-enters a real, human-visible chat session: mark the turn as an
	// asynchronous chat run so interactive-only tools (ask_user/request_confirmation)
	// guide the model to ask in its reply instead of bailing with "proceed without
	// asking" — the user can answer in the chat afterwards.
	// Bound the wake turn with the same hard+idle watchdog as spawn/worker turns: it
	// now holds the per-session turn slot, so a hung wake (a provider that never
	// returns) would otherwise block every other turn on the session indefinitely —
	// the slot's Cond wait ignores ctx, so nothing else could release it.
	hardCap, idleCap := s.rt.tun.SpawnTimeout(), s.rt.tun.SpawnIdleTimeout()
	// Single-shot idle-resume (FND-708844f8): an idle-cut wake turn gets ONE more
	// attempt under a fresh window before reconcileTurnOutcome marks it unfinished.
	// Prefer the history-aware runner (installed by the api server) so the woken agent
	// continues with the FULL conversation — the wake prompt was just persisted as the
	// last user message, so the history already carries it; runSessionTurn falls back
	// to the prompt-only invoke when no runner is wired.
	var wakeMeta *turnMeta
	wakeStart := time.Now()
	// Own cancelable context + active-session tracking so a wake turn is stoppable
	// from the UI, exactly like a scheduled or spawned turn.
	wakeRunCtx, cancelWakeRun := context.WithCancel(ctx)
	defer cancelWakeRun()
	s.rt.trackSession(sc.SessionID, cancelWakeRun)
	defer s.rt.untrackSession(sc.SessionID)
	turnBase, cancelTurn, output, steps, invokeErr := s.rt.runTurnWithIdleResume(wakeRunCtx, hardCap, idleCap, s.rt.tun.IdleResumeMax(),
		func(attemptCtx context.Context, _ context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			wakeCtx := tools.WithAsyncChat(WithSessionID(WithCallKind(attemptCtx, KindSchedule), sc.SessionID))
			wakeCtx, wakeMeta = WithTurnMeta(wakeCtx)
			p := sc.Prompt
			if attempt > 1 {
				p = resumeContinuationPrompt(sc.Prompt, prevOutput)
			}
			return s.rt.runSessionTurn(wakeCtx, agent, sc.SessionID, p, true)
		})
	defer cancelTurn()
	// A watchdog cut / self-truncated loop hands back salvaged text; lead it with the
	// outcome note (nil error) so it records as an explaining reply, not a clean one.
	output, steps, invokeErr, truncated := s.rt.reconcileTurnOutcome(turnBase, output, steps, invokeErr, hardCap, idleCap)
	if invokeErr != nil {
		s.rt.recordTurnError(ctx, sc.SessionID, sc.AgentID, invokeErr, steps, wakeMeta, time.Since(wakeStart).Milliseconds(), "⚠️ Otomatik uyandırma çalıştırılamadı:")
		s.emitWakeEvent(sc, "done", "⏰ Otomatik uyandırma başarısız")
		s.rt.AutoTagTurn(ctx, sc.SessionID, steps, "wake_error")
		return invokeErr
	}
	output, err = s.rt.recordAssistantReply(ctx, sc.SessionID, sc.AgentID, output, steps, wakeMeta, time.Since(wakeStart).Milliseconds(), "ℹ️ Ajan bu uyandırma için boş yanıt döndürdü.")
	s.emitWakeEvent(sc, "done", "⏰ Otomatik uyandırma tamamlandı")
	// Auto-tag tool errors from this wake turn.
	s.rt.AutoTagTurn(ctx, sc.SessionID, steps, "")
	// Tag-triggered automations: a wake continues a real chat session, which may be
	// tagged — signal its completion so an automation can pick up the result. A
	// truncated turn only produced a fragment (already noted), so withhold the signal.
	if !truncated {
		s.rt.FireTurnFinished(sc.SessionID, sc.AgentID, output)
	}
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

	// Common pre-dispatch guard: the workspace autonomy pause must stop a fire
	// before any session, message, or flow run is created. deliverFlow is also
	// gated deeper (LaunchRun/launchGate), but deliverPrompt manages its own
	// session directly and has no such gate — without this check a paused
	// prompt-backed schedule would still open/reuse its session and record the
	// prompt as a new message before eventually failing inside the tool loop.
	if s.rt.Paused() {
		s.logger.Info("schedule fire: skipped (autonomy paused)",
			"schedule", scheduleID, "trigger", trigger, "agent", sc.AgentID)
		next := s.nextRun(scheduleID)
		if derr := s.db.SetScheduleDelivery(ctx, scheduleID, "failure", ErrAutonomyPaused.Error(), next); derr != nil {
			s.logger.Warn("schedule fire: persist delivery failed",
				"schedule", scheduleID, "trigger", trigger, "error", derr)
		}
		return ErrAutonomyPaused
	}

	// Log the start of every fire so a run is traceable even if it later hangs
	// or the process dies mid-flight — the previous code only logged on success.
	s.logger.Info("schedule fire: begin",
		"schedule", scheduleID, "trigger", trigger,
		"agent", sc.AgentID, "flow", sc.FlowID, "cron", sc.CronExpr)

	// Flow-backed schedules run their orchestration flow; prompt-backed ones
	// deliver a standalone prompt to the agent. RunFlowRecorded emits its own
	// desktop notification, so only the prompt path calls emitPromptDelivery.
	var sessionID string
	var fireErr error
	if sc.FlowID != "" {
		sessionID, fireErr = s.deliverFlow(ctx, sc)
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
			Type:   events.TypeSchedule,
			Level:  "error",
			Title:  "⏰ Zamanlama başarısız — " + name,
			Body:   notifyLine(err.Error(), 200),
			Target: target,
		})
		return
	}
	s.rt.publish(events.Event{
		Type:   events.TypeSchedule,
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

// deliverFlow runs a flow-backed schedule: it executes sc.FlowID (with sc.Prompt
// as the flow input) through RunFlowRecorded, which records the run as a turn in
// the flow's transcript session and raises its own desktop notification. It
// returns the flow session id (for delivery bookkeeping) and any run error. A
// flow that finishes with FlowFailure is surfaced as an error so the schedule's
// lastDeliveryStatus reflects it.
func (s *Scheduler) deliverFlow(ctx context.Context, sc db.Schedule) (string, error) {
	// Unified dispatch: LaunchRun validates the flow, runs it (budget-gated
	// autonomous), and normalizes a flow-failure into an error.
	res, err := s.rt.LaunchRun(ctx, RunSpec{
		Trigger:    TriggerSchedule,
		Input:      sc.Prompt,
		Autonomous: true,
		FlowID:     sc.FlowID,
	})
	if err != nil {
		s.logger.Error("schedule deliver: flow run failed",
			"schedule", sc.ID, "flow", sc.FlowID, "session", res.SessionID, "error", err)
		return res.SessionID, err
	}
	return res.SessionID, nil
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
	// Serialize this scheduled turn with any concurrent turn on the same session
	// (user chat / inbox worker / wake) — and, for a coordinator, its auto turns —
	// via the single per-session turn slot.
	release := s.rt.claimSessionTurnSlot(session.ID, turnqueue.KindWake, "zamanlanmış tur")
	defer release()
	// Record the scheduled prompt as a user turn first, so the schedule thread
	// reads as a real conversation (the UI shows what was asked). Origin "schedule"
	// renders it as a "⏰ Zamanlanmış görev" note rather than a user bubble.
	schedMsg, err := s.db.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      "user",
		Origin:    "schedule",
		Text:      sc.Prompt,
	})
	if err != nil {
		return session.ID, err
	}
	// Bridge it to the hub so a window watching this session renders the scheduled
	// prompt live and in order before the reply (_Docs/58), not only on reload.
	s.rt.emitInjectedUserNote(session.ID, schedMsg)
	// Own cancelable context for this turn so a human "Durdur" (CancelSession) can
	// stop a scheduled run that never enters the api server's chatRuns.
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	s.rt.trackSession(session.ID, cancelRun)
	// Bound the scheduled turn with the spawn watchdog: it holds the per-session turn
	// slot, so a hung turn must not block the session's queue forever (the slot's Cond
	// wait ignores ctx).
	hardCap, idleCap := s.rt.tun.SpawnTimeout(), s.rt.tun.SpawnIdleTimeout()
	// Single-shot idle-resume (FND-708844f8): an idle-cut scheduled turn gets ONE more
	// attempt under a fresh window before reconcileTurnOutcome marks it unfinished.
	var (
		overflow *atomic.Bool
		meta     *turnMeta
	)
	turnStart := time.Now()
	turnBase, cancelTurn, output, steps, err := s.rt.runTurnWithIdleResume(runCtx, hardCap, idleCap, s.rt.tun.IdleResumeMax(),
		func(attemptCtx context.Context, _ context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			var turnCtx context.Context
			turnCtx, overflow = withOverflowFlag(WithSessionID(WithCallKind(attemptCtx, KindSchedule), session.ID))
			turnCtx, meta = WithTurnMeta(turnCtx)
			p := sc.Prompt
			if attempt > 1 {
				p = resumeContinuationPrompt(sc.Prompt, prevOutput)
			}
			return s.rt.invokeTraced(turnCtx, agent, p, true) // scheduled = autonomous
		})
	defer cancelTurn()
	s.rt.untrackSession(session.ID)
	// A watchdog cut (hard/idle) or a self-truncated loop returns salvaged text that
	// must not be recorded as a finished result: lead it with the outcome note and
	// suppress the completion signal below. A real fault / human stop is left as an
	// error for the branch that follows.
	output, steps, err, truncated := s.rt.reconcileTurnOutcome(turnBase, output, steps, err, hardCap, idleCap)
	if err != nil {
		// Log the provider/tool-loop failure with the agent + its provider/model,
		// so the logs view pinpoints what failed (e.g. missing key, model error)
		// rather than leaving only a notification behind.
		s.logger.Error("schedule deliver: agent invoke failed",
			"schedule", sc.ID, "agent", sc.AgentID, "session", session.ID,
			"provider", agent.Provider, "model", agent.Model, "error", err)
		// Surface the failure inside the schedule thread itself, not just in the
		// delivery status/logs — otherwise the user opens the session and sees
		// their prompt with no reply and no clue what went wrong.
		s.rt.recordTurnError(ctx, session.ID, sc.AgentID, err, steps, meta, time.Since(turnStart).Milliseconds(), "⚠️ Zamanlanmış prompt çalıştırılamadı:")
		s.rt.AutoTagTurn(ctx, session.ID, steps, "schedule_error")
		return session.ID, err
	}
	// Persist the reply (empty → explicit note; agent id + activity trace stamped
	// so it renders like a normal chat turn).
	output, err = s.rt.recordAssistantReply(ctx, session.ID, sc.AgentID, output, steps, meta, time.Since(turnStart).Milliseconds(), "ℹ️ Ajan bu zamanlanmış prompt için boş yanıt döndürdü.")
	// Self-completion: a scheduled run has no human to send the follow-up, so if the
	// turn stalled with unfinished work (activated tools it never used, or open
	// todos) keep it going until done. No-op on a clean finish. Bounded + budget-gated.
	s.rt.maybeAutoContinue(ctx, agent, session.ID, KindSchedule, steps)
	// Context-reset handoff: if this scheduled turn hit the context limit, optionally
	// continue the work in a fresh session. No-op unless HandoffAuto is enabled.
	s.rt.maybeAutoHandoff(ctx, session.ID, agent, overflow.Load())
	// Auto-tag tool errors from this scheduled turn.
	s.rt.AutoTagTurn(ctx, session.ID, steps, "")
	// Tag-triggered automations: a scheduled delivery's session may be tagged too.
	// A truncated turn produced only a fragment (already led with a "not done" note),
	// so withhold the completion signal — firing it would chain automations onto
	// half-done work.
	if !truncated {
		s.rt.FireTurnFinished(session.ID, agent.ID, output)
	}
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
