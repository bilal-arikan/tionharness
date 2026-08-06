package view

import (
	"fmt"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
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
func ProjectSchedule(in ScheduleInput, level Level, lens Lens) (View, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	sc := in.Schedule

	v := View{
		Ref:    Ref{Kind: KindSchedule, ID: sc.ID},
		Level:  level,
		Lens:   lens,
		AsOf:   now,
		Source: fmt.Sprintf("%d/%s", sc.LastRunAt, sc.LastDeliveryStatus),
	}

	state := "enabled"
	if !sc.Enabled {
		state = "disabled"
	}
	if sc.Name != "" {
		v.Header = fmt.Sprintf("SCHEDULE %s %q · %s · cron %q · asOf %s",
			sc.ID, sc.Name, state, sc.CronExpr, hhmmss(now))
	} else {
		v.Header = fmt.Sprintf("SCHEDULE %s · %s · cron %q · asOf %s",
			sc.ID, state, sc.CronExpr, hhmmss(now))
	}

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	// The most important line first: did the last fire fail? errors lens keeps
	// only this.
	if sc.LastDeliveryStatus == "error" {
		l.add("✗ son çalışmada hata: %s", clip(sc.LastDeliveryError, 120))
	} else if sc.LastRunAt > 0 {
		l.add("son çalışma: %s önce · %s", age(tsSec(sc.LastRunAt), now),
			firstNonBlank(sc.LastDeliveryStatus, "ok"))
	}
	if lens == LensErrors {
		v.Body = l.String()
		v.finalize()
		return v, nil
	}

	if sc.Enabled && sc.NextRunAt > 0 {
		l.add("sıradaki: %s sonra", dur(time.Until(time.Unix(sc.NextRunAt, 0))))
	}
	if sc.FlowID != "" {
		l.add("hedef: flow:%s", sc.FlowID)
	} else if sc.AgentID != "" {
		l.add("hedef: agent:%s", sc.AgentID)
	}
	l.addIf(sc.Prompt != "", "prompt: %s", clip(sc.Prompt, 140))
	if sc.ExpiresAt > 0 {
		l.add("bitiş: %s sonra", dur(time.Until(time.Unix(sc.ExpiresAt, 0))))
	}
	l.addIf(len(sc.Tags) > 0, "etiket: %v", sc.Tags)

	v.Body = l.String()
	v.finalize()
	return v, nil
}
