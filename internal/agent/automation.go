package agent

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// TurnFinished is the signal a completed agent turn delivers to the automation
// engine: the session that finished, the agent that answered, and the final
// reply text ("result"). Emitted from every completion path via
// Runtime.FireTurnFinished.
type TurnFinished struct {
	SessionID string
	AgentID   string
	Output    string
}

// UsageRecorded is the signal a recorded provider call delivers to the automation
// engine so a token-triggered automation can detect a threshold crossing: the
// session the tokens were attributed to (may be empty for a detached aux call),
// this call's token delta, and the session's NEW cumulative lifetime total after
// the delta. The workspace-scope total is resolved by the engine from the DB (it
// is not carried here, so a session-less call still drives workspace crossings).
// Emitted from Runtime.RecordUsage via Runtime.FireUsageRecorded.
type UsageRecorded struct {
	SessionID       string
	DeltaTokens     int64
	SessionNewTotal int64
}

// AutomationEngine reacts to finished turns: when a finishing session carries a
// tag some enabled Automation watches, it renders that automation's prompt (with
// the finishing session's result substituted) and spawns a new session for the
// target agent. The spawned session is tagged so — by default — it re-triggers
// the same automation on its own completion, forming a bounded self-continuing
// loop. Guardrails (Enabled, MaxIterations, CooldownSec) keep the loop finite.
//
// It is purely event-driven (no cron): the workspace manager wires
// engine.OnTurnFinished as the runtime's turn hook. Firing runs on the detached
// goroutine FireTurnFinished spawns, so it never blocks the finishing turn.
type AutomationEngine struct {
	db         *db.DB
	rt         *Runtime
	logger     *slog.Logger
	activityMu sync.Mutex
}

// NewAutomationEngine constructs an engine bound to a workspace's DB and runtime.
func NewAutomationEngine(database *db.DB, rt *Runtime, logger *slog.Logger) *AutomationEngine {
	return &AutomationEngine{db: database, rt: rt, logger: logger}
}

// OnTurnFinished is the turn hook. It loads the finished session, and for every
// enabled automation whose trigger tag the session carries, evaluates guardrails
// and (if clear) fires. A session with no tags returns immediately (the common
// case), so untagged chat traffic pays almost nothing.
func (e *AutomationEngine) OnTurnFinished(ctx context.Context, tf TurnFinished) {
	sess, err := e.db.GetSession(ctx, tf.SessionID)
	if err != nil {
		return // session gone (e.g. deleted mid-turn) — nothing to match
	}
	if len(sess.Tags) == 0 {
		return
	}
	autos, err := e.db.ListEnabledAutomations(ctx)
	if err != nil {
		e.logger.Warn("automation: list failed", "session", tf.SessionID, "error", err)
		return
	}
	for _, a := range autos {
		if a.TriggerKind == db.TriggerBoard || a.TriggerKind == db.TriggerToken || a.TriggerKind == db.TriggerCounter {
			continue // board/token/counter automations react to their own events, not turns
		}
		if a.TriggerTag == "" || !containsTag(sess.Tags, a.TriggerTag) {
			continue
		}
		e.fire(ctx, a, sess, tf)
	}
}

// OnBoardChange is the board hook (see db.SetBoardHook). It selects the board
// automations that match the change — deterministically ordered, and narrowed to
// a single owner when one of them is exclusive (see selectBoardAutomations) —
// then evaluates the shared guardrails and fires each in turn with the card
// context rendered into the prompt. Runs on its own detached goroutine (wired by
// the workspace manager) so a card mutation is never blocked.
//
// Firing is SEQUENTIAL: two rules on the same column run one after the other, so
// a later rule observes the card state the earlier one left behind instead of
// racing it.
func (e *AutomationEngine) OnBoardChange(ctx context.Context, ev db.BoardChangeEvent) {
	autos, err := e.db.ListEnabledAutomations(ctx)
	if err != nil {
		e.logger.Warn("automation: list failed (board)", "task", ev.TaskID, "error", err)
		return
	}
	selected := selectBoardAutomations(autos, ev)
	if len(selected) == 0 {
		return
	}
	if len(selected) == 1 && selected[0].BoardExclusive {
		e.logger.Info("automation: exclusive owner claimed board event",
			"automation", selected[0].ID, "task", ev.TaskID, "op", ev.Op, "to", ev.ToState)
	}
	for _, a := range selected {
		e.fireBoard(ctx, a, ev)
	}
}

// OnUsageRecorded is the usage hook (see Runtime.FireUsageRecorded). It fires
// after every provider call's tokens are recorded and, for each enabled token
// automation, checks whether this call pushed the watched cumulative total across
// another TokenThreshold multiple. The crossing is detected statelessly from the
// previous vs new total (new − delta = previous), so no per-scope ledger is kept.
// Runs on its own detached goroutine (wired by the workspace manager) so token
// recording is never blocked.
//
// Session-scoped rules watch the finishing session's lifetime total (needs a
// session id); workspace-scoped rules watch the whole workspace's spend for the
// day, resolved fresh from the DB. Guardrails (cooldown/maxIterations/expiry) are
// shared with the tag and board paths and bound how often a crossing may fire.
func (e *AutomationEngine) OnUsageRecorded(ctx context.Context, sig UsageRecorded) {
	if sig.DeltaTokens <= 0 {
		return // no token movement this call → no boundary can be crossed
	}
	autos, err := e.db.ListEnabledAutomations(ctx)
	if err != nil {
		e.logger.Warn("automation: list failed (token)", "session", sig.SessionID, "error", err)
		return
	}
	// Resolve the workspace-day total once, lazily: only summed if some enabled
	// automation actually watches the workspace scope.
	var wsTotal int64 = -1
	// Self-amplification guard: a token automation now delivers into its OWN
	// persistent maintenance session (SessionKindAutomation), which spends tokens on
	// every fire. For a session-scoped rule those upkeep tokens would push that same
	// session across the next threshold multiple and re-fire it on a tight loop
	// (bounded only by cooldown/maxIterations) — a self-sustaining loop the old
	// fresh-session spawn never had. So skip session-scope crossings attributed to a
	// maintenance session. Resolved lazily (one in-memory lookup) and only when a
	// session-scoped rule is actually present. Workspace scope is intentionally NOT
	// guarded: those tokens are real workspace spend and belong in the day total.
	crossingIsMaint := false
	maintResolved := false
	isMaintSession := func() bool {
		if !maintResolved {
			maintResolved = true
			if sig.SessionID != "" {
				if s, err := e.db.GetSession(ctx, sig.SessionID); err == nil {
					crossingIsMaint = s.Kind == SessionKindAutomation
				}
			}
		}
		return crossingIsMaint
	}
	for _, a := range autos {
		if a.TriggerKind != db.TriggerToken || a.TokenThreshold <= 0 {
			continue
		}
		interval := int64(a.TokenThreshold)
		switch a.TokenScope {
		case db.TokenScopeWorkspace:
			if wsTotal < 0 {
				wsTotal = e.db.WorkspaceTokensToday(ctx)
			}
			if crossedMultiple(wsTotal-sig.DeltaTokens, wsTotal, interval) {
				e.fireToken(ctx, a, "", wsTotal)
			}
		default: // "" or TokenScopeSession
			if sig.SessionID == "" {
				continue // a session-scoped rule needs a session to attribute to
			}
			if isMaintSession() {
				continue // don't let a maintenance session re-trigger its own rule
			}
			if crossedMultiple(sig.SessionNewTotal-sig.DeltaTokens, sig.SessionNewTotal, interval) {
				e.fireToken(ctx, a, sig.SessionID, sig.SessionNewTotal)
			}
		}
	}
}

// crossedMultiple reports whether the interval boundary between prev and now was
// passed — i.e. now reached a higher multiple of interval than prev did. Both
// totals are non-negative token counts; a non-positive interval never crosses.
func crossedMultiple(prev, now, interval int64) bool {
	if interval <= 0 || now <= prev {
		return false
	}
	if prev < 0 {
		prev = 0
	}
	return prev/interval < now/interval
}

// fireToken evaluates a token automation's shared guardrails and, if they pass,
// runs its target flow or spawns its target agent with the token context in the
// prompt. sessionID is the crossing session for session scope (empty for
// workspace scope); total is the cumulative token count that crossed the boundary.
// dispatchFire runs an automation's rendered prompt against its target, choosing
// the session strategy from EffectiveSessionMode. Three routes, checked in order:
//   - flow-backed → LaunchRun runs the flow (its own transcript; session mode n/a);
//   - continue    → reuse the persistent per-automation thread (deliverAutomationTurn,
//     history-aware). It bypasses LaunchRun's launch brake, so the workspace
//     autonomy pause is honored here instead; SpawnOptions are ignored (no fresh
//     session to tag/parent);
//   - spawn        → LaunchRun spawns a FRESH session with the caller's SpawnOptions.
//
// Returns the fired session id and a driver label ("flow"|"session"). Shared by
// all four fire paths so the mode choice lives in one place.
func (e *AutomationEngine) dispatchFire(ctx context.Context, a db.Automation, prompt string, trigger RunTrigger, spawn SpawnOptions) (sessionID, driver string, err error) {
	// Lineage: whichever driver runs, the session it produces was started by THIS
	// automation, tripped (for a session-scoped trigger) by the session the caller
	// put in ParentSessionID. The flow driver threads it through LaunchRun into
	// the run's transcript session; the session driver stamps it on the spawn.
	origin := &db.SessionOrigin{Kind: db.OriginAutomation, EntityID: a.ID, TriggerSessionID: spawn.ParentSessionID}
	if a.FlowID != "" {
		res, ferr := e.rt.LaunchRun(ctx, RunSpec{
			Trigger:    trigger,
			Input:      prompt,
			Autonomous: true,
			FlowID:     a.FlowID,
			Origin:     origin,
		})
		return res.SessionID, "flow", ferr
	}
	if spawn.Origin == nil {
		spawn.Origin = origin
	}
	if a.EffectiveSessionMode() == db.SessionModeContinue {
		if e.rt.Paused() {
			return "", "session", ErrAutonomyPaused
		}
		sid, derr := e.rt.deliverAutomationTurn(ctx, a, prompt)
		return sid, "session", derr
	}
	// A one-shot automation fire (session mode != continue) still opens a NEW
	// session via SpawnSession, whose default Kind is "spawned" — landing under the
	// sidebar's generic "Spawn" chip instead of "Otomasyon". Tag it explicitly so it
	// groups with the persistent maintenance thread (see SessionKindAutomationRun).
	if spawn.Kind == "" {
		spawn.Kind = SessionKindAutomationRun
	}
	res, serr := e.rt.LaunchRun(ctx, RunSpec{
		Trigger:    trigger,
		Input:      prompt,
		Autonomous: true,
		AgentID:    a.TargetAgentID,
		Spawn:      spawn,
	})
	return res.SessionID, res.Driver, serr
}

// notifyFired records a successful fire and publishes the standard success event.
// The caller supplies the notification icon and the already-built title suffix so
// each trigger kind keeps its own wording; the record + publish boilerplate is
// shared by every fire path.
func (e *AutomationEngine) notifyFired(ctx context.Context, a db.Automation, sessionID, icon, suffix, prompt string) {
	if err := e.db.RecordAutomationFire(ctx, a.ID, sessionID, ""); err != nil {
		e.logger.Warn("automation: record fire failed", "automation", a.ID, "error", err)
	}
	e.rt.publish(events.Event{
		Type:   events.TypeAutomation,
		Level:  "success",
		Title:  icon + " Otomasyon tetiklendi" + suffix + " — " + automationLabel(a),
		Body:   notifyLine(prompt, 120),
		Target: map[string]string{"view": "executions", "sessionId": sessionID},
	})
}

func (e *AutomationEngine) fireToken(ctx context.Context, a db.Automation, sessionID string, total int64) {
	if !e.guardsPass(ctx, a) {
		return
	}
	prompt := renderAutomationPrompt(a.PromptTemplate, e.tokenVars(a, sessionID, total))
	if strings.TrimSpace(prompt) == "" {
		e.recordFailure(ctx, a, "rendered prompt is empty")
		return
	}
	firedSessionID, driver, err := e.dispatchFire(ctx, a, prompt, TriggerAutomationToken, SpawnOptions{
		Title:     "⚡ " + automationLabel(a),
		CreatedBy: "automation:" + a.ID,
		Tags:      a.SpawnTags, // token rules do not self-loop; nil = no tag
	})
	if err != nil {
		e.recordFailure(ctx, a, err.Error())
		return
	}
	scope := a.EffectiveTokenScope()
	suffix := " (token·" + scope + ")"
	if driver == "flow" {
		suffix = " (token·" + scope + "·akış)"
	}
	e.logger.Info("automation: fired (token)",
		"automation", a.ID, "scope", scope, "total", total, "threshold", a.TokenThreshold,
		"session", firedSessionID, "driver", driver, "iteration", a.IterationCount+1)
	e.notifyFired(ctx, a, firedSessionID, "⚡", suffix, prompt)
}

// commonVars are the placeholder values every trigger kind shares: the iteration
// bookkeeping ({{iteration}}/{{maxIterations}}), the automation name, and the
// current date/time. Each kind's *Vars function starts from these and layers its
// own trigger-specific keys on top, so the shared tail lives in one place.
func commonVars(a db.Automation) map[string]string {
	now := time.Now()
	maxIter := strconv.Itoa(a.MaxIterations)
	if a.MaxIterations == 0 {
		maxIter = "∞"
	}
	return map[string]string{
		"iteration":     strconv.Itoa(a.IterationCount + 1), // 1-based: this fire's number
		"maxIterations": maxIter,
		"automation":    automationLabel(a),
		"date":          now.Format("2006-01-02"),
		"time":          now.Format("15:04"),
		"datetime":      now.Format("2006-01-02 15:04"),
	}
}

// tokenVars assembles the placeholder values for a token automation's prompt.
// There is no session result, so {{result}} is absent (nothing is appended).
func (e *AutomationEngine) tokenVars(a db.Automation, sessionID string, total int64) map[string]string {
	v := commonVars(a)
	v["tokens"] = strconv.FormatInt(total, 10)
	v["threshold"] = strconv.Itoa(a.TokenThreshold)
	v["scope"] = a.EffectiveTokenScope()
	v["sessionId"] = sessionID // empty for workspace scope
	return v
}

// boardMatches reports whether a board automation's op and column filters accept
// this card change. An empty BoardOp defaults to move; BoardOpAny matches all
// ops. Empty From/To filters match any column.
func boardMatches(a db.Automation, ev db.BoardChangeEvent) bool {
	op := a.BoardOp
	if op == "" {
		op = db.BoardOpMove
	}
	if op != db.BoardOpAny && op != ev.Op {
		return false
	}
	if a.BoardFromState != "" && a.BoardFromState != ev.FromState {
		return false
	}
	if a.BoardToState != "" && a.BoardToState != ev.ToState {
		return false
	}
	return true
}

// fireBoard evaluates a board automation's guardrails and, if they pass, runs its
// target flow or spawns its target agent with the card context in the prompt.
// Unlike tag automations there is no originating session and no self-tagging loop
// (the spawned session carries no trigger tag); the guardrails still bound how
// often card changes may fire it.
func (e *AutomationEngine) fireBoard(ctx context.Context, a db.Automation, ev db.BoardChangeEvent) {
	if !e.guardsPass(ctx, a) {
		return
	}

	// Archive action: a lightweight, no-LLM bookkeeping fire (the "done → archive"
	// cleanup). It hides the card off the active board instead of spawning a
	// session. A delete event has no card to archive, and archiving an already
	// archived card is a no-op inside SetTaskArchived.
	if a.BoardAction == db.BoardActionArchive {
		if ev.Op == db.BoardOpDelete || ev.TaskID == "" {
			return
		}
		// Board events are dispatched asynchronously. The card may have left done
		// while this event was queued, so authorize cleanup against current state.
		current, err := e.db.GetTask(ctx, ev.TaskID)
		if err != nil {
			e.recordFailure(ctx, a, err.Error())
			return
		}
		if current.BoardState != db.BoardDone {
			return
		}
		if err := e.db.SetTaskArchived(ctx, ev.TaskID, true); err != nil {
			e.recordFailure(ctx, a, err.Error())
			return
		}
		if err := e.db.RecordAutomationFire(ctx, a.ID, "", ""); err != nil {
			e.logger.Warn("automation: record fire failed", "automation", a.ID, "error", err)
		}
		e.logger.Info("automation: fired (board·archive)",
			"automation", a.ID, "op", ev.Op, "task", ev.TaskID, "iteration", a.IterationCount+1)
		e.rt.publish(events.Event{
			Type:   events.TypeAutomation,
			Level:  "success",
			Title:  "🗄 Otomasyon: kart arşivlendi — " + automationLabel(a),
			Body:   ev.Title,
			Target: map[string]string{"view": "tasks"},
		})
		return
	}

	// Move action: a lightweight, no-LLM bookkeeping fire. The destination is
	// explicit on the automation; MoveTask validates its key and emits the next
	// board event so chained rules can run under the same guardrails.
	if a.BoardAction == db.BoardActionMove {
		if ev.Op == db.BoardOpDelete || ev.TaskID == "" {
			return
		}
		if err := e.db.MoveTask(ctx, ev.TaskID, a.BoardMoveToState); err != nil {
			e.recordFailure(ctx, a, err.Error())
			return
		}
		if err := e.db.RecordAutomationFire(ctx, a.ID, "", ""); err != nil {
			e.logger.Warn("automation: record fire failed", "automation", a.ID, "error", err)
		}
		e.logger.Info("automation: fired (board·move)",
			"automation", a.ID, "op", ev.Op, "task", ev.TaskID,
			"to", a.BoardMoveToState, "iteration", a.IterationCount+1)
		e.rt.publish(events.Event{
			Type:   events.TypeAutomation,
			Level:  "success",
			Title:  "Otomasyon: kart taşındı — " + automationLabel(a),
			Body:   ev.Title,
			Target: map[string]string{"view": "tasks"},
		})
		return
	}

	prompt := renderAutomationPrompt(a.PromptTemplate, e.boardVars(ctx, a, ev))
	if strings.TrimSpace(prompt) == "" {
		e.recordFailure(ctx, a, "rendered prompt is empty")
		return
	}

	// SpawnTags default to none for board automations (an empty/nil slice) so a
	// board fire does not tag its spawned session — board rules match on card
	// changes, not tags, so there is no self-loop to seed. (Ignored on the flow and
	// continue drivers.)
	firedSessionID, driver, err := e.dispatchFire(ctx, a, prompt, TriggerAutomationBoard, SpawnOptions{
		Title:     "🗂 " + automationLabel(a),
		CreatedBy: "automation:" + a.ID,
		Tags:      a.SpawnTags,
	})
	if err != nil {
		e.recordFailure(ctx, a, err.Error())
		return
	}
	suffix := " (pano)"
	if driver == "flow" {
		suffix = " (pano·akış)"
	}
	e.logger.Info("automation: fired (board)",
		"automation", a.ID, "op", ev.Op, "task", ev.TaskID,
		"session", firedSessionID, "driver", driver, "iteration", a.IterationCount+1)
	e.notifyFired(ctx, a, firedSessionID, "🗂", suffix, prompt)
}

// guardsPass evaluates an automation's shared runtime guardrails (expiry,
// cooldown, iteration cap) and returns true when it is clear to fire. Expiry and
// the iteration cap auto-disable the automation as a side effect; every decision
// is logged so a stalled loop is explainable in the Logs view. Shared by both the
// tag (fire) and board (fireBoard) paths.
func (e *AutomationEngine) guardsPass(ctx context.Context, a db.Automation) bool {
	// Expiry: past its optional end date → auto-disable and stop.
	if a.ExpiresAt > 0 && time.Now().Unix() >= a.ExpiresAt {
		e.logger.Info("automation: past end date; auto-disabling",
			"automation", a.ID, "trigger", automationTrigger(a), "expiresAt", a.ExpiresAt)
		if err := e.db.SetAutomationEnabled(ctx, a.ID, false); err != nil {
			e.logger.Warn("automation: expiry auto-disable failed", "automation", a.ID, "error", err)
		}
		return false
	}
	// Cooldown: skip if the previous fire was too recent.
	if a.CooldownSec > 0 && a.LastFiredAt > 0 {
		if elapsed := time.Now().Unix() - a.LastFiredAt; elapsed < int64(a.CooldownSec) {
			e.logger.Info("automation: cooldown, skipping",
				"automation", a.ID, "trigger", automationTrigger(a), "elapsed", elapsed, "cooldown", a.CooldownSec)
			return false
		}
	}
	// Absolute backstop for automations stored with MaxIterations <= 0, the old
	// "unlimited" value. Those rows never went through db.ValidateMaxIterations —
	// they were written before the rule, imported from a market package, or
	// hand-edited — so entry validation cannot help them and they would otherwise
	// loop with no lifetime brake at all. (Four such rows existed when this landed,
	// three of them enabled.)
	//
	// The threshold sits ABOVE the hard cap on purpose: this is a last resort for
	// data nobody chose, so it must not stop a working board automation sooner than
	// an explicit maximum would have. It is a warn, not an info: unlike a normal cap
	// this is TionHarness ending something the user never bounded.
	if a.MaxIterations <= 0 && a.IterationCount >= db.AbsoluteIterationBackstop {
		e.logger.Warn("automation: absolute iteration backstop reached (stored maxIterations<=0); auto-disabling",
			"automation", a.ID, "trigger", automationTrigger(a),
			"iterations", a.IterationCount, "backstop", db.AbsoluteIterationBackstop)
		if err := e.db.SetAutomationEnabled(ctx, a.ID, false); err != nil {
			e.logger.Warn("automation: auto-disable failed", "automation", a.ID, "error", err)
		}
		e.rt.publish(events.Event{
			Type:  events.TypeAutomation,
			Level: "warn",
			Title: "🛑 Otomasyon durduruldu (mutlak fren) — " + automationLabel(a),
			Body: "Bu otomasyon sınırsız (maxIterations=0) kayıtlıydı ve " +
				strconv.Itoa(db.AbsoluteIterationBackstop) + " tetiğe ulaştı. Devre dışı bırakıldı; " +
				"düzenleyip 1–" + strconv.Itoa(db.MaxIterationsHardCap) + " arası bir üst sınır verin.",
			Target: map[string]string{"view": "schedules"},
		})
		return false
	}
	// Iteration cap: disable and stop once the budget is spent (0 = unlimited,
	// bounded by the backstop above).
	if a.MaxIterations > 0 && a.IterationCount >= a.MaxIterations {
		e.logger.Info("automation: max iterations reached; auto-disabling",
			"automation", a.ID, "trigger", automationTrigger(a), "iterations", a.IterationCount, "max", a.MaxIterations)
		if err := e.db.SetAutomationEnabled(ctx, a.ID, false); err != nil {
			e.logger.Warn("automation: auto-disable failed", "automation", a.ID, "error", err)
		}
		e.rt.publish(events.Event{
			Type:   events.TypeAutomation,
			Level:  "info",
			Title:  "🔁 Otomasyon durduruldu (limit) — " + automationLabel(a),
			Body:   "Maksimum iterasyon (" + strconv.Itoa(a.MaxIterations) + ") aşıldı; otomasyon devre dışı bırakıldı.",
			Target: map[string]string{"view": "schedules"},
		})
		return false
	}
	return true
}

// fire evaluates one matching tag automation's guardrails and, if they pass,
// spawns the follow-up session.
func (e *AutomationEngine) fire(ctx context.Context, a db.Automation, sess db.Session, tf TurnFinished) {
	if !e.guardsPass(ctx, a) {
		return
	}

	vars := e.turnVars(ctx, a, sess, tf)
	prompt := renderAutomationPrompt(a.PromptTemplate, vars)
	if strings.TrimSpace(prompt) == "" {
		e.recordFailure(ctx, a, "rendered prompt is empty")
		return
	}

	// Spawn tags: default to the trigger tag so the new session re-fires this
	// automation (the loop). A non-nil empty slice breaks the loop intentionally.
	// (Ignored on the flow driver — flow sessions carry no trigger tag.)
	spawnTags := a.SpawnTags
	if spawnTags == nil {
		spawnTags = []string{a.TriggerTag}
	}

	// Error-repair semantic: when the trigger is an error-class tag, the spawned
	// fixer is meant to clear that tag from the ERRORED (parent) session once it is
	// repaired. The fixer runs in its OWN session, so its update_session tool can't
	// reach the parent — the framework clears it on the fixer's success instead. This
	// fixes the fixer's old habit of stripping the tag off itself (the wrong session)
	// and leaving the parent flagged forever. auth-error is deliberately excluded:
	// it is terminal (needs /login), not something a repair turn can clear.
	var clearParentTags []string
	if a.TriggerTag == TagToolError || a.TriggerTag == TagError || a.TriggerTag == TagStuck {
		// For "stuck", RemoveSessionTags also resets the StuckTurns counter, so
		// a successful fixer re-opens the parent's autonomy in one step.
		clearParentTags = []string{a.TriggerTag}
	}

	// Dispatch by session mode: spawn a fresh session (default) or continue the
	// persistent per-automation thread. In continue mode the spawn-only options
	// (parent link, loop tags) are ignored — the maintenance thread carries no
	// trigger tag, so the tag self-loop is not seeded (like token/counter).
	firedSessionID, driver, err := e.dispatchFire(ctx, a, prompt, TriggerAutomationTag, SpawnOptions{
		Title:                    "🔁 " + automationLabel(a),
		CreatedBy:                "automation:" + a.ID,
		ParentSessionID:          sess.ID,
		Tags:                     spawnTags,
		ClearParentTagsOnSuccess: clearParentTags,
	})
	if err != nil {
		e.recordFailure(ctx, a, err.Error())
		return
	}

	suffix := ""
	if driver == "flow" {
		suffix = " (akış)"
	}
	e.logger.Info("automation: fired"+suffix,
		"automation", a.ID, "tag", a.TriggerTag, "from", sess.ID,
		"session", firedSessionID, "driver", driver, "iteration", a.IterationCount+1)
	e.notifyFired(ctx, a, firedSessionID, "🔁", suffix, prompt)
}

// recordFailure logs a fire failure, persists it on the automation (LastError,
// and still bumps the iteration counter so a persistently-failing loop can't spin
// forever), and notifies.
func (e *AutomationEngine) recordFailure(ctx context.Context, a db.Automation, msg string) {
	e.logger.Error("automation: fire failed", "automation", a.ID, "tag", a.TriggerTag, "error", msg)
	if err := e.db.RecordAutomationFire(ctx, a.ID, "", msg); err != nil {
		e.logger.Warn("automation: record failure failed", "automation", a.ID, "error", err)
	}
	e.rt.publish(events.Event{
		Type:   events.TypeAutomation,
		Level:  "error",
		Title:  "🔁 Otomasyon başarısız — " + automationLabel(a),
		Body:   notifyLine(msg, 200),
		Target: map[string]string{"view": "schedules"},
	})
}

// turnVars assembles the placeholder values available to an automation's prompt
// template for one fire. It resolves a couple of extras from the DB (the finishing
// agent's name, the prompt that produced the result) best-effort — a lookup miss
// just leaves that variable empty rather than aborting the fire.
func (e *AutomationEngine) turnVars(ctx context.Context, a db.Automation, sess db.Session, tf TurnFinished) map[string]string {
	agentName := tf.AgentID
	if ag, err := e.db.GetAgent(ctx, tf.AgentID); err == nil {
		agentName = ag.Name
	}
	// prevPrompt: the last user turn of the finishing session — the input that
	// produced {{result}}. Useful for carrying the original instruction forward.
	prevPrompt := ""
	if msgs, err := e.db.ListMessages(ctx, sess.ID); err == nil {
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "user" {
				prevPrompt = msgs[i].Text
				break
			}
		}
	}
	v := commonVars(a)
	v["result"] = tf.Output
	v["title"] = sess.Title
	v["tag"] = a.TriggerTag
	v["sessionId"] = sess.ID
	v["agent"] = agentName
	v["agentName"] = agentName // alias
	v["prevPrompt"] = prevPrompt
	return v
}

// boardVars assembles the placeholder values available to a board automation's
// prompt template for one card change. There is no session result, so {{result}}
// is absent (renderAutomationPrompt appends nothing).
func (e *AutomationEngine) boardVars(ctx context.Context, a db.Automation, ev db.BoardChangeEvent) map[string]string {
	// Resolve the card owner to a human name (falling back to the raw id); empty
	// when the card is unassigned.
	owner := ev.OwnerAgentID
	if owner != "" {
		if ag, err := e.db.GetAgent(ctx, owner); err == nil {
			owner = ag.Name
		}
	}
	v := commonVars(a)
	v["taskId"] = ev.TaskID
	v["title"] = ev.Title
	v["op"] = ev.Op
	v["from"] = ev.FromState
	v["to"] = ev.ToState
	v["fromLabel"] = boardLabel(ev.FromState)
	v["toLabel"] = boardLabel(ev.ToState)
	v["board"] = ev.ToState // convenience alias for the current column
	v["tags"] = strings.Join(ev.Tags, ", ")
	if v["tags"] == "" {
		v["tags"] = "none"
	}
	if owner == "" {
		owner = "unassigned"
	}
	v["owner"] = owner
	v["priority"] = ev.Priority // critical/high/medium/low
	if v["priority"] == "" {
		v["priority"] = "unset"
	}
	return v
}

// boardLabel resolves a column key to its human label using the built-in column
// set, falling back to the key itself (custom columns keep their key). Empty key
// (e.g. the source of a create, or target of a delete) renders as "—".
func boardLabel(key string) string {
	if key == "" {
		return "—"
	}
	for _, c := range db.DefaultBoardColumns() {
		if c.Key == key {
			return c.Label
		}
	}
	return key
}

// automationTrigger returns a short trigger descriptor for logging (the tag for
// tag automations, the board op for board ones).
func automationTrigger(a db.Automation) string {
	switch a.TriggerKind {
	case db.TriggerBoard:
		op := a.BoardOp
		if op == "" {
			op = db.BoardOpMove
		}
		return "board:" + op
	case db.TriggerToken:
		scope := a.TokenScope
		if scope == "" {
			scope = db.TokenScopeSession
		}
		return "token:" + scope
	}
	return "tag:" + a.TriggerTag
}

// renderAutomationPrompt substitutes {{name}} placeholders (see turnVars) into the
// template. A template that references no {{result}} still gets the result appended
// so the loop always carries its output forward.
func renderAutomationPrompt(tmpl string, vars map[string]string) string {
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	out := strings.NewReplacer(pairs...).Replace(tmpl)
	if !strings.Contains(tmpl, "{{result}}") && strings.TrimSpace(vars["result"]) != "" {
		out = strings.TrimRight(out, "\n") + "\n\n--- Önceki sonuç ---\n" + vars["result"]
	}
	return out
}

// containsTag reports whether tags contains tag (exact match).
func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// automationLabel returns a human label for an automation (name, else trigger tag).
func automationLabel(a db.Automation) string {
	if s := strings.TrimSpace(a.Name); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.TriggerTag); s != "" {
		return "#" + s
	}
	return a.ID
}
