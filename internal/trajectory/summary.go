package trajectory

import (
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Trajectory summary (Rota F3, brief §7.1): the deterministic, LLM-free digest
// of one run. Computed when the trajectory reaches a terminal status (on the
// trajectory work queue, before the trajectory_end automations see it) and on
// demand for a live run; stored on the graph and mirrored on the index row so
// recipe statistics and the curator read the index only.

// SessionCostFn returns a session's lifetime tokens, cost and whether every
// model behind it had a price.
type SessionCostFn func(sessionID string) (tokens int64, costUSD float64, priced bool)

// Summarize is the pure part: everything the graph itself carries.
// nowSec closes the duration of a run that has not ended.
func Summarize(t db.Trajectory, cost SessionCostFn, nowSec int64) db.TrajectorySummary {
	s := db.TrajectorySummary{At: nowSec, Priced: true, PerPhase: map[string]db.PhaseStat{}}
	// Duration: root creation → last node end (terminal) or now.
	endMs := int64(0)
	for _, n := range t.Nodes {
		if n.EndMs > endMs {
			endMs = n.EndMs
		}
	}
	if !t.IsTerminal() || endMs == 0 {
		endMs = nowSec * 1000
	}
	if t.CreatedAt > 0 && endMs/1000 > t.CreatedAt {
		s.DurationSec = endMs/1000 - t.CreatedAt
	}
	phaseIDs := map[string]bool{}
	for _, n := range t.Nodes {
		switch n.Kind {
		case db.TrajNodePhase:
			s.Phases++
			id := strings.TrimPrefix(n.ID, "p:")
			phaseIDs[n.ID] = true
			st := s.PerPhase[id]
			if n.StartMs > 0 && n.EndMs >= n.StartMs {
				st.DurationSec = (n.EndMs - n.StartMs) / 1000
			}
			s.PerPhase[id] = st
			switch n.State {
			case db.TrajStateDone, db.TrajStateSkipped:
				s.PhasesDone++
			case db.TrajStatePending, db.TrajStateGhost:
				s.GhostPhases = append(s.GhostPhases, id)
			}
		}
	}
	for _, n := range t.Nodes {
		switch n.Kind {
		case db.TrajNodeSession:
			if n.Lane == 0 {
				continue // the root itself
			}
			s.Sessions++
			failed := n.State == db.TrajStateFailed
			if failed {
				s.FailedSess++
			}
			if n.RefID != "" && cost != nil {
				tok, c, priced := cost(n.RefID)
				s.Tokens += tok
				s.CostUSD += c
				if !priced {
					s.Priced = false
				}
			}
			if n.PhaseID == "" || !phaseIDs[n.PhaseID] {
				s.Unannounced++
			} else {
				id := strings.TrimPrefix(n.PhaseID, "p:")
				st := s.PerPhase[id]
				st.Sessions++
				if failed {
					st.Failed++
				}
				s.PerPhase[id] = st
			}
		case db.TrajNodeFlowRun:
			s.FlowRuns++
			if n.State == db.TrajStateFailed {
				s.FailedRuns++
			}
			if n.PhaseID == "" || !phaseIDs[n.PhaseID] {
				s.Unannounced++
			}
		case db.TrajNodeAutomation:
			if n.Origin != db.TrajOriginDeclared {
				continue
			}
			s.Watchers++
			if n.State != db.TrajStateDone {
				s.UnfiredWatchers = append(s.UnfiredWatchers, n.RefID)
			}
		case db.TrajNodeGate:
			s.Gates++
			end := n.EndMs
			if end == 0 {
				end = nowSec * 1000
			}
			if n.StartMs > 0 && end > n.StartMs {
				s.GateWaitSec += (end - n.StartMs) / 1000
			}
		}
	}
	// The root's own spend counts too.
	if t.RootSessionID != "" && cost != nil {
		tok, c, priced := cost(t.RootSessionID)
		s.Tokens += tok
		s.CostUSD += c
		if !priced {
			s.Priced = false
		}
	}
	sort.Strings(s.GhostPhases)
	sort.Strings(s.UnfiredWatchers)
	if len(s.PerPhase) == 0 {
		s.PerPhase = nil
	}
	return s
}

// sessionCost resolves a session's lifetime spend through the usage rollup
// and the billing price table.
