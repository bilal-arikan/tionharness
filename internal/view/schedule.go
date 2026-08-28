package view

import (
	"fmt"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// ScheduleInput is everything the schedule projection reads — just the schedule
// row itself. A schedule is a small entity, so this projection is L0/L1 only
// (no transcript, no narrative): it exists so an action-queue row pointing at a
// broken schedule can drill down in place instead of jumping to the full screen.
type ScheduleInput struct {
	Schedule db.Schedule
	// Now is the clock used for age computations. Zero means time.Now().
	Now time.Time
}

// ProjectSchedule renders one cron schedule: whether it is armed, when it last
// fired and whether that fire failed, when it runs next, and what it delivers.
//
// A DISABLED schedule's last error is still shown here (unlike the workspace
// roll-up, which suppresses it): the roll-up hides it so a deliberately paused
// schedule does not read as "broken" in a list of real problems, but when the
// user has drilled into this specific schedule they are asking about exactly
// that, so withholding it would be the surprise.
func ProjectSchedule(in ScheduleInput, level Level) (View, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	sc := in.Schedule

	v := View{
		Ref:    Ref{Kind: KindSchedule, ID: sc.ID},
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("%d/%s", sc.LastRunAt, sc.LastDeliveryStatus),
	}

	state := "enabled"
	if !sc.Enabled {
		state = "disabled"
	}
	// A one-shot wake carries NO cron expression, so the old unconditional
	// `cron ""` asserted a broken schedule for every wake the schedule_wake tool
	// created. The rhythm is named for what it actually is.
	rhythm := fmt.Sprintf("cron %q", sc.CronExpr)
	if sc.OneShot {
		rhythm = "tek seferlik"
	}
	if sc.Name != "" {
		v.Header = fmt.Sprintf("SCHEDULE %s %q · %s · %s · asOf %s",
			sc.ID, clip(sc.Name, 60), state, rhythm, hhmmss(now))
	} else {
		v.Header = fmt.Sprintf("SCHEDULE %s · %s · %s · asOf %s",
			sc.ID, state, rhythm, hhmmss(now))
	}

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	// The most important line first: did the last fire fail?
	if sc.LastDeliveryStatus == "error" {
		l.add("✗ son çalışmada hata: %s", clip(sc.LastDeliveryError, 120))
	} else if sc.LastRunAt > 0 {
		l.add("son çalışma: %s önce · %s", age(tsSec(sc.LastRunAt), now),
			firstNonBlank(sc.LastDeliveryStatus, "ok"))
	}
	// A one-shot fires at FireAt, not on the cron cursor NextRunAt.
	if sc.Enabled && sc.OneShot && sc.FireAt > 0 {
		l.add("ateşleme: %s sonra", dur(time.Until(time.Unix(sc.FireAt, 0))))
	} else if sc.Enabled && sc.NextRunAt > 0 {
		l.add("sıradaki: %s sonra", dur(time.Until(time.Unix(sc.NextRunAt, 0))))
	}
	// A one-shot wake re-delivers into the ORIGINATING session — which session that
	// is, is the whole point of the wake — so it is named ahead of the agent.
	switch {
	case sc.FlowID != "":
		l.add("hedef: flow:%s", sc.FlowID)
	case sc.OneShot && sc.SessionID != "":
		l.add("hedef: session:%s", sc.SessionID)
	case sc.AgentID != "":
		l.add("hedef: agent:%s", sc.AgentID)
	}
	l.addIf(sc.Reason != "", "neden: %s", clip(sc.Reason, 120))
	l.addIf(sc.Prompt != "", "prompt: %s", clip(sc.Prompt, 140))
	if sc.ExpiresAt > 0 {
		l.add("bitiş: %s sonra", dur(time.Until(time.Unix(sc.ExpiresAt, 0))))
	}
	l.addIf(len(sc.Tags) > 0, "etiket: %v", sc.Tags)
	// Whether every fire adds a turn to the agent's shared schedule thread or opens
	// a fresh session decides what the delivered prompt may assume about its
	// context. It is long-tail detail and only meaningful for an agent-backed
	// recurring schedule (a flow always records into its own per-run transcript).
	if level == LevelFull && !sc.OneShot && sc.FlowID == "" && sc.AgentID != "" {
		l.add("oturum modu: %s", sc.EffectiveSessionMode())
	}

	v.Body = l.String()
	v.finalize()
	return v, nil
}
