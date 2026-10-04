package awareness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// OpenLoops is the structured form of the workspace's "what needs a human"
// facts: the same scan the brief's open-loops section and the pulse line
// render as text, kept as ids so a second consumer (the Explorer map's
// attention layer, _Docs/94 §11) can mark the very same entities instead of
// re-deriving them with slightly different rules.
type OpenLoops struct {
	// WaitingAsks are sessions with a pending human answer.
	WaitingAsks []string
	// Stuck sessions: StuckTurns > 0 or tagged "stuck".
	Stuck []string
	// Blocked coordinators: tagged "blocked".
	Blocked []string
	// FailedRuns are flow runs that failed inside the last 24h.
	FailedRuns []string
	// StaleCards are in-progress cards untouched past Settings.StaleCardDays;
	// FailedCards sit in the failed column. Both carry the task for labels.
	StaleCards  []db.Task
	FailedCards []db.Task
}

// Empty reports that nothing needs attention.
func (o OpenLoops) Empty() bool {
	return len(o.WaitingAsks) == 0 && len(o.Stuck) == 0 && len(o.Blocked) == 0 &&
		len(o.FailedRuns) == 0 && len(o.StaleCards) == 0 && len(o.FailedCards) == 0
}

// runFailed accepts both spellings the run status has carried: db.FlowFailure
// ("failure") is the model constant; "failed" was what the first open-loop
// scan compared against, so a store written by either keeps counting.
func runFailed(status string) bool {
	return status == db.FlowFailure || status == "failed"
}

// CollectOpenLoops scans the store for the open loops, excluding one session
// (the asking session, whose own state is not "elsewhere"). A store read that
// fails only empties its slice: a partial scan beats no scan.
func CollectOpenLoops(ctx context.Context, store Store, settings Settings, now time.Time, exclude string) OpenLoops {
	var out OpenLoops
	if asks, err := store.ListWaitingSessionAsks(ctx); err == nil {
		for _, a := range asks {
			if a.SessionID == exclude {
				continue
			}
			out.WaitingAsks = append(out.WaitingAsks, a.SessionID)
		}
	}
	if sessions, err := store.ListSessions(ctx, ""); err == nil {
		for _, s := range sessions {
			if s.ID == exclude || s.State == "archived" {
				continue
			}
			if s.StuckTurns > 0 || hasTag(s.Tags, "stuck") {
				out.Stuck = append(out.Stuck, s.ID)
			}
			if hasTag(s.Tags, "blocked") {
				out.Blocked = append(out.Blocked, s.ID)
			}
		}
	}
	if runs, err := store.ListFlowRuns(ctx, "", 0); err == nil {
		cutoff := now.Add(-24 * time.Hour).Unix()
		for _, r := range runs {
			if runFailed(r.Status) && r.UpdatedAt >= cutoff {
				out.FailedRuns = append(out.FailedRuns, r.ID)
			}
		}
	}
	if tasks, err := store.ListTasks(ctx); err == nil {
		staleAfter := time.Duration(settings.StaleCardDays) * 24 * time.Hour
		for _, t := range tasks {
			if t.Archived {
				continue
			}
			switch t.BoardState {
			case db.BoardInProgress:
				if t.UpdatedAt > 0 && now.Sub(time.Unix(t.UpdatedAt, 0)) > staleAfter {
					out.StaleCards = append(out.StaleCards, t)
				}
			case db.BoardFailed:
				out.FailedCards = append(out.FailedCards, t)
			}
		}
	}
	return out
}

// renderOpenLoops is the text form the brief and the pulse use: one line per
// kind with the first few ids, and the compact "N waiting · M stuck" counts.
func renderOpenLoops(o OpenLoops, staleDays int) (lines []string, counts string) {
	const shown = 5
	var parts []string
	if n := len(o.WaitingAsks); n > 0 {
		lines = append(lines, fmt.Sprintf("%d session(s) waiting for a human answer: %s", n, joinCapped(o.WaitingAsks, shown)))
		parts = append(parts, fmt.Sprintf("%d waiting", n))
	}
	if n := len(o.Stuck); n > 0 {
		lines = append(lines, fmt.Sprintf("%d stuck session(s): %s", n, joinCapped(o.Stuck, shown)))
		parts = append(parts, fmt.Sprintf("%d stuck", n))
	}
	if n := len(o.Blocked); n > 0 {
		lines = append(lines, fmt.Sprintf("%d blocked coordinator(s): %s", n, joinCapped(o.Blocked, shown)))
		parts = append(parts, fmt.Sprintf("%d blocked", n))
	}
	if n := len(o.FailedRuns); n > 0 {
		lines = append(lines, fmt.Sprintf("%d flow run(s) failed in the last 24h", n))
		parts = append(parts, fmt.Sprintf("%d failed runs", n))
	}
	if n := len(o.StaleCards); n > 0 {
		lines = append(lines, fmt.Sprintf("%d card(s) in progress for over %dd: %s", n, staleDays, joinCapped(taskLabels(o.StaleCards), shown)))
		parts = append(parts, fmt.Sprintf("%d stale cards", n))
	}
	if n := len(o.FailedCards); n > 0 {
		lines = append(lines, fmt.Sprintf("%d failed card(s): %s", n, joinCapped(taskLabels(o.FailedCards), shown)))
		parts = append(parts, fmt.Sprintf("%d failed cards", n))
	}
	return lines, strings.Join(parts, " · ")
}

func taskLabels(tasks []db.Task) []string {
	out := make([]string, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t.ID+" "+clip(t.Title, 40))
	}
	return out
}
