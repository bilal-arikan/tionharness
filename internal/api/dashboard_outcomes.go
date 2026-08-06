package api

// Outcome metrics: the overview's answer to "are things finishing, or just
// starting?". The existing trends count volume (sessions opened, runs started);
// these count completion — card throughput, how long a card takes end to end,
// and the flow-run success rate.

import (
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// outcomeBlock is the completion-side summary shown next to the volume trends.
type outcomeBlock struct {
	// VelocityByDay is cards finishing per day, bucketed by the day they reached
	// the done column (approximated by UpdatedAt — a done card's last touch). A
	// card later moved back out is no longer counted, which is the honest reading
	// of "done right now".
	VelocityByDay   []daySeriesPoint `json:"velocityByDay"`
	CardsDoneWindow int              `json:"cardsDoneWindow"`
	// CycleTimeAvgSec is the mean (UpdatedAt − CreatedAt) over cards done within
	// the window, in seconds. 0 means no card finished in the window (the UI reads
	// CardsDoneWindow to tell "0 seconds" from "no data").
	CycleTimeAvgSec int64 `json:"cycleTimeAvgSec"`
	// RunsClosedWindow is how many flow runs reached a terminal state (success or
	// failure) with a creation stamp in the window — the denominator behind the
	// rate.
	RunsClosedWindow int `json:"runsClosedWindow"`
	// RunSuccessRate is success / (success + failure) over the window, in 0..1.
	// nil when nothing closed: a rate off zero runs is undefined, and showing 0%
	// or 100% would both mislead.
	RunSuccessRate *float64 `json:"runSuccessRate"`
}

// dashboardOutcomes computes the completion metrics over the trailing window.
func dashboardOutcomes(tasks []db.Task, runs []db.FlowRun, days int, now time.Time) outcomeBlock {
	inWindow := keySet(dayKeys(days, now))

	velocity := make([]daySeriesPoint, len(dayKeys(days, now)))
	idx := make(map[string]int, len(velocity))
	for i, k := range dayKeys(days, now) {
		velocity[i] = daySeriesPoint{Day: k}
		idx[k] = i
	}

	var cycleSum, cycleN int64
	done := 0
	for _, t := range tasks {
		if t.BoardState != db.BoardDone || t.UpdatedAt <= 0 {
			continue
		}
		day := time.Unix(t.UpdatedAt, 0).Format("2006-01-02")
		if _, ok := inWindow[day]; !ok {
			continue
		}
		done++
		if i, ok := idx[day]; ok {
			velocity[i].Value++
		}
		// Cycle time only when the creation stamp is sane and not after completion:
		// a negative span would be a clock/decode bug, and folding it in would drag
		// the mean toward a made-up number.
		if t.CreatedAt > 0 && t.CreatedAt <= t.UpdatedAt {
			cycleSum += t.UpdatedAt - t.CreatedAt
			cycleN++
		}
	}

	block := outcomeBlock{VelocityByDay: velocity, CardsDoneWindow: done}
	if cycleN > 0 {
		block.CycleTimeAvgSec = cycleSum / cycleN
	}

	var success, failure int
	for _, r := range runs {
		if r.CreatedAt <= 0 {
			continue
		}
		day := time.Unix(r.CreatedAt, 0).Format("2006-01-02")
		if _, ok := inWindow[day]; !ok {
			continue
		}
		switch r.Status {
		case db.FlowSuccess:
			success++
		case db.FlowFailure:
			failure++
		}
	}
	if closed := success + failure; closed > 0 {
		block.RunsClosedWindow = closed
		rate := float64(success) / float64(closed)
		block.RunSuccessRate = &rate
	}
	return block
}
