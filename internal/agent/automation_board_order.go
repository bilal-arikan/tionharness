package agent

import (
	"sort"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// selectBoardAutomations decides WHICH board automations fire for one card
// change, and in WHAT order.
//
// Two rules watching the same column (e.g. a classifier and a planner both on
// move→todo) used to fire together, in whatever order the store happened to
// return them, and race on the same card. This resolves that in two steps:
//
//  1. Ordering — matches are sorted by BoardPriority ascending, ties broken on
//     ID. Map iteration order is not stable across restarts, so without this the
//     sequence was arbitrary; now it is deterministic and configurable.
//
//  2. Exclusivity — if any match sets BoardExclusive, only the winning one (the
//     lowest priority among the exclusive matches, after sorting) is returned.
//     That is the "single owner per column" guarantee: a column's owner runs
//     alone instead of the loser having to be disabled by hand.
//
// The returned slice is in fire order. A nil/empty input returns nil.
func selectBoardAutomations(autos []db.Automation, ev db.BoardChangeEvent) []db.Automation {
	matches := make([]db.Automation, 0, len(autos))
	for _, a := range autos {
		if a.TriggerKind != db.TriggerBoard || !boardMatches(a, ev) {
			continue
		}
		matches = append(matches, a)
	}
	if len(matches) == 0 {
		return nil
	}
	sortBoardAutomations(matches)
	// Exclusivity: the first exclusive match (already the highest-precedence one
	// after sorting) claims the event alone.
	for _, a := range matches {
		if a.BoardExclusive {
			return []db.Automation{a}
		}
	}
	return matches
}

// sortBoardAutomations orders matches by BoardPriority ascending, then by ID, so
// the fire sequence is stable across process restarts.
func sortBoardAutomations(matches []db.Automation) {
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].BoardPriority != matches[j].BoardPriority {
			return matches[i].BoardPriority < matches[j].BoardPriority
		}
		return matches[i].ID < matches[j].ID
	})
}
