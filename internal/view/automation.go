package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// AutomationInput is one automation rule. The projection renders trigger,
// target, enabled state and fire bookkeeping — the same facts the Otomasyon
// screen shows, in the map's compact DSL.
type AutomationInput struct {
	Automation db.Automation
	// Now is the clock used for age stamps. Zero means time.Now().
	Now time.Time
}

// ProjectAutomation renders one automation rule's configuration and runtime
// state: what fires it, what it runs, and whether the last fire failed.
func ProjectAutomation(in AutomationInput, level Level, lens Lens) (View, error) {
	a := in.Automation
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	if a.ID == "" {
		return View{}, fmt.Errorf("view: automation has no id")
	}

	v := View{
		Ref:    Ref{Kind: KindAutomation, ID: a.ID},
		Level:  level,
		Lens:   lens,
		AsOf:   now,
		Source: fmt.Sprintf("%d", a.UpdatedAt),
	}
	v.Header = fmt.Sprintf("AUTOMATION · %s · %s · asOf %s",
		a.ID, clip(orDash(a.Name), 60), hhmmss(now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	l.add("tetik: %s", automationTrigger(a))
	l.add("hedef: %s", automationTarget(a))
	state := "○ kapalı"
	if a.Enabled {
		state = "● aktif"
	}
	last := "hiç ateşlenmedi"
	if a.LastFiredAt > 0 {
		last = "son " + age(tsSec(a.LastFiredAt), now) + " önce"
	}
	l.add("durum: %s · %d ateşleme · %s", state, a.IterationCount, last)
	if a.LastError != "" {
		l.add("hata: %s", clip(a.LastError, 80))
	}
	if level == LevelFull {
		if a.MaxIterations > 0 {
			l.add("guardrail: max %d ateşleme · %d sn soğuma", a.MaxIterations, a.CooldownSec)
		}
		if a.PromptTemplate != "" {
			l.add("prompt: %s", clip(a.PromptTemplate, 100))
		}
	}

	v.Body = l.String()
	v.finalize()
	return v, nil
}

// automationTrigger renders the trigger kind with the config that shapes it —
// the tag for a tag rule, the op + columns for a board rule, the interval for
// token/counter rules. Empty fields resolve to their documented defaults so the
// line always shows what actually fires.
func automationTrigger(a db.Automation) string {
	switch a.TriggerKind {
	case db.TriggerBoard:
		op := a.BoardOp
		if op == "" {
			op = db.BoardOpMove
		}
		var parts []string
		parts = append(parts, "board ("+op+")")
		if a.BoardFromState != "" {
			parts = append(parts, "from:"+a.BoardFromState)
		}
		if a.BoardToState != "" {
			parts = append(parts, "to:"+a.BoardToState)
		}
		return strings.Join(parts, " ")
	case db.TriggerToken:
		scope := a.EffectiveTokenScope()
		return fmt.Sprintf("token (%s) her %d tok", scope, a.TokenThreshold)
	case db.TriggerCounter:
		metric := a.CounterMetric
		if metric == "" {
			metric = db.CounterMetricMessage
		}
		return fmt.Sprintf("counter (%s, %s) her %d", a.EffectiveCounterScope(), metric, a.CounterInterval)
	default:
		return "tag:" + orDash(a.TriggerTag)
	}
}

// automationTarget renders who runs on fire: an agent (the spawned session's
// owner) or a flow. The empty target id is shown as "?" rather than as a blank —
// a rule with no target is a misconfiguration the reader should see.
func automationTarget(a db.Automation) string {
	if a.FlowID != "" {
		return "flow:" + a.FlowID
	}
	if a.TargetAgentID != "" {
		return "agent:" + a.TargetAgentID
	}
	return "?"
}
