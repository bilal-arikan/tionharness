package agent

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
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
	db     *db.DB
	rt     *Runtime
	logger *slog.Logger
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
		if a.TriggerTag == "" || !containsTag(sess.Tags, a.TriggerTag) {
			continue
		}
		e.fire(ctx, a, sess, tf)
	}
}

// fire evaluates one matching automation's guardrails and, if they pass, spawns
// the follow-up session. Guardrail decisions are logged so a stalled loop is
// explainable in the Logs view.
func (e *AutomationEngine) fire(ctx context.Context, a db.Automation, sess db.Session, tf TurnFinished) {
	// Expiry: past its optional end date → auto-disable and stop.
	if a.ExpiresAt > 0 && time.Now().Unix() >= a.ExpiresAt {
		e.logger.Info("automation: past end date; auto-disabling",
			"automation", a.ID, "tag", a.TriggerTag, "expiresAt", a.ExpiresAt)
		if err := e.db.SetAutomationEnabled(ctx, a.ID, false); err != nil {
			e.logger.Warn("automation: expiry auto-disable failed", "automation", a.ID, "error", err)
		}
		return
	}
	// Cooldown: skip if the previous fire was too recent.
	if a.CooldownSec > 0 && a.LastFiredAt > 0 {
		if elapsed := time.Now().Unix() - a.LastFiredAt; elapsed < int64(a.CooldownSec) {
			e.logger.Info("automation: cooldown, skipping",
				"automation", a.ID, "tag", a.TriggerTag, "elapsed", elapsed, "cooldown", a.CooldownSec)
			return
		}
	}
	// Iteration cap: disable and stop once the budget is spent (0 = unlimited).
	if a.MaxIterations > 0 && a.IterationCount >= a.MaxIterations {
		e.logger.Info("automation: max iterations reached; auto-disabling",
			"automation", a.ID, "tag", a.TriggerTag, "iterations", a.IterationCount, "max", a.MaxIterations)
		if err := e.db.SetAutomationEnabled(ctx, a.ID, false); err != nil {
			e.logger.Warn("automation: auto-disable failed", "automation", a.ID, "error", err)
		}
		e.rt.publish(events.Event{
			Type:   "automation",
			Level:  "info",
			Title:  "🔁 Otomasyon durduruldu (limit) — " + automationLabel(a),
			Body:   "Maksimum iterasyon (" + strconv.Itoa(a.MaxIterations) + ") aşıldı; otomasyon devre dışı bırakıldı.",
			Target: map[string]string{"view": "schedules"},
		})
		return
	}

	vars := e.turnVars(ctx, a, sess, tf)
	prompt := renderAutomationPrompt(a.PromptTemplate, vars)
	if strings.TrimSpace(prompt) == "" {
		e.recordFailure(ctx, a, "rendered prompt is empty")
		return
	}

	// Flow-backed automation: run the rendered prompt as the flow's input instead
	// of spawning a single-agent session. This is a per-trigger dispatch (no
	// self-loop via SpawnTags — flow sessions carry no trigger tag).
	if a.FlowID != "" {
		e.fireFlow(ctx, a, prompt)
		return
	}

	agent, err := e.db.GetAgent(ctx, a.TargetAgentID)
	if err != nil {
		e.recordFailure(ctx, a, "target agent gone: "+err.Error())
		return
	}

	// Spawn tags: default to the trigger tag so the new session re-fires this
	// automation (the loop). A non-nil empty slice breaks the loop intentionally.
	spawnTags := a.SpawnTags
	if spawnTags == nil {
		spawnTags = []string{a.TriggerTag}
	}

	res, err := e.rt.SpawnSession(ctx, agent.ID, prompt, SpawnOptions{
		Title:           "🔁 " + automationLabel(a),
		CreatedBy:       "automation:" + a.ID,
		ParentSessionID: sess.ID,
		Tags:            spawnTags,
	})
	if err != nil {
		e.recordFailure(ctx, a, "spawn failed: "+err.Error())
		return
	}

	if err := e.db.RecordAutomationFire(ctx, a.ID, res.SessionID, ""); err != nil {
		e.logger.Warn("automation: record fire failed", "automation", a.ID, "error", err)
	}
	e.logger.Info("automation: fired",
		"automation", a.ID, "tag", a.TriggerTag, "from", sess.ID,
		"spawned", res.SessionID, "agent", agent.ID, "iteration", a.IterationCount+1)
	e.rt.publish(events.Event{
		Type:   "automation",
		Level:  "success",
		Title:  "🔁 Otomasyon tetiklendi — " + automationLabel(a),
		Body:   notifyLine(prompt, 120),
		Target: map[string]string{"view": "executions", "sessionId": res.SessionID},
	})
}

// fireFlow runs a flow-backed automation: it executes a.FlowID with the rendered
// prompt as the flow input (RunFlowRecorded records the run as a turn in the
// flow's transcript session and raises its own notification), then records the
// fire. A missing flow or a failed run is captured via recordFailure so the
// automation's LastError and iteration counter stay accurate.
func (e *AutomationEngine) fireFlow(ctx context.Context, a db.Automation, prompt string) {
	if _, err := e.db.GetFlow(ctx, a.FlowID); err != nil {
		e.recordFailure(ctx, a, "target flow gone: "+err.Error())
		return
	}
	run, sessionID, err := e.rt.RunFlowRecorded(ctx, a.FlowID, prompt, true, nil)
	if err != nil {
		e.recordFailure(ctx, a, "flow run failed: "+err.Error())
		return
	}
	if run.Status == db.FlowFailure {
		e.recordFailure(ctx, a, "flow run failed: "+run.Error)
		return
	}
	if err := e.db.RecordAutomationFire(ctx, a.ID, sessionID, ""); err != nil {
		e.logger.Warn("automation: record fire failed", "automation", a.ID, "error", err)
	}
	e.logger.Info("automation: fired (flow)",
		"automation", a.ID, "tag", a.TriggerTag, "flow", a.FlowID,
		"session", sessionID, "iteration", a.IterationCount+1)
	e.rt.publish(events.Event{
		Type:   "automation",
		Level:  "success",
		Title:  "🔁 Otomasyon tetiklendi (akış) — " + automationLabel(a),
		Body:   notifyLine(prompt, 120),
		Target: map[string]string{"view": "executions", "sessionId": sessionID},
	})
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
		Type:   "automation",
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
	now := time.Now()
	maxIter := strconv.Itoa(a.MaxIterations)
	if a.MaxIterations == 0 {
		maxIter = "∞"
	}
	return map[string]string{
		"result":        tf.Output,
		"title":         sess.Title,
		"tag":           a.TriggerTag,
		"sessionId":     sess.ID,
		"iteration":     strconv.Itoa(a.IterationCount + 1), // 1-based: this fire's number
		"maxIterations": maxIter,
		"agent":         agentName,
		"agentName":     agentName, // alias
		"prevPrompt":    prevPrompt,
		"automation":    automationLabel(a),
		"date":          now.Format("2006-01-02"),
		"time":          now.Format("15:04"),
		"datetime":      now.Format("2006-01-02 15:04"),
	}
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
