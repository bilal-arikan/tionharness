package view

import (
	"fmt"
	"sort"
	"time"

	"github.com/bilal-arikan/tionharness/internal/billing"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// AgentInput is everything the agent projection reads: the agent record, the
// sessions bound to it, and its usage rollup for today. All three are in-memory
// store reads, so an agent view — like the workspace roll-up — costs no disk I/O.
type AgentInput struct {
	Agent    db.Agent
	Sessions []db.Session
	// Usage is the agent's usage row for the current day (zero-valued when the
	// agent has run nothing today).
	Usage db.Usage
	// Now is the clock used for age computations. Zero means time.Now().
	Now time.Time
}

// agentSessionHandles is how many session handles an agent view lists before the
// rest are reported as Elided.
const agentSessionHandles = 20

// ProjectAgent renders one agent: who it is, what it cost today, how many of its
// sessions are open, and when it last did anything. It re-uses the same L0/L1
// discipline as every other projection — every number is counted in Go, the
// cost comes from billing.RollupOf (never re-priced here), nothing is narrated.
func ProjectAgent(in AgentInput, level Level) (View, error) {
	if in.Agent.ID == "" {
		return View{}, fmt.Errorf("agent input has no agent")
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	v := View{
		Ref:    Ref{Kind: KindAgent, ID: in.Agent.ID},
		Level:  level,
		AsOf:   now,
		Source: fmt.Sprintf("%d@%d", len(in.Sessions), in.Usage.Calls),
	}

	st := computeAgentStats(in, now)
	tokens := usageTokens(in.Usage)
	// Cost rides next to the token figure, priced exactly as every budget surface
	// prices it. Omitted (not "$0.00") when there is no priced spend today: a bare
	// $0.00 next to a non-zero token count would read as "free" when it actually
	// means "this provider has no list price".
	cost := ""
	roll := billing.RollupOf(in.Usage.ByModel)
	if roll.CostUSD > 0 {
		cost = " · " + usd(roll.CostUSD, roll.Estimated || !roll.Priced) + " bugün"
	}
	v.Header = fmt.Sprintf("AGENT:%s %q · %s · %d oturum (%d aktif) · %s tok bugün%s · son etkinlik %s",
		in.Agent.ID, clip(agentName(in.Agent), 60), modelLabel(in.Agent),
		st.Total, st.Active, compactCount(tokens), cost, agentLastActivity(st.LastActivity, now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	for _, s := range agentSignals(st) {
		l.add("%s", s)
	}

	v.Handles, v.Elided = agentSessionHandleList(in.Sessions, now)
	v.ElidedUnit = "oturum"
	v.Body = l.String()
	v.finalize()
	return v, nil
}

// agentStats is the L0 counting pass over an agent's sessions.
type agentStats struct {
	Total        int
	Active       int
	Coordinators int
	Stuck        []db.Session
	LastActivity int64
}

func computeAgentStats(in AgentInput, now time.Time) agentStats {
	var st agentStats
	activeCutoff := now.Add(-wsActiveWindow).Unix()
	for _, s := range in.Sessions {
		if s.State == "archived" {
			continue
		}
		st.Total++
		if s.UpdatedAt >= activeCutoff {
			st.Active++
		}
		if s.StuckTurns > 0 {
			st.Stuck = append(st.Stuck, s)
		}
		if s.IsCoordinator() {
			st.Coordinators++
		}
		if s.UpdatedAt > st.LastActivity {
			st.LastActivity = s.UpdatedAt
		}
	}
	sort.Slice(st.Stuck, func(i, j int) bool {
		return st.Stuck[i].UpdatedAt > st.Stuck[j].UpdatedAt
	})
	return st
}

// agentSignals is the L1 layer: what about this agent deserves attention.
func agentSignals(st agentStats) []string {
	var out []string
	if n := len(st.Stuck); n > 0 {
		out = append(out, fmt.Sprintf("⚠ %d oturum takılmış (StuckTurns>0): %s",
			n, sessionNames(st.Stuck, wsSignalNames)))
	}
	if st.Coordinators > 0 {
		out = append(out, fmt.Sprintf("⇵ %d koordinatör oturumu", st.Coordinators))
	}
	return out
}

// agentSessionHandleList returns up to agentSessionHandles session handles (most
// recently active first) and how many sessions were left out. Archived sessions
// are excluded — they are not part of the agent's live surface.
func agentSessionHandleList(sessions []db.Session, now time.Time) ([]Handle, int) {
	live := make([]db.Session, 0, len(sessions))
	for _, s := range sessions {
		if s.State == "archived" {
			continue
		}
		live = append(live, s)
	}
	sort.Slice(live, func(i, j int) bool { return live[i].UpdatedAt > live[j].UpdatedAt })

	elided := 0
	if len(live) > agentSessionHandles {
		elided = len(live) - agentSessionHandles
		live = live[:agentSessionHandles]
	}
	hs := make([]Handle, 0, len(live))
	for _, s := range live {
		hs = append(hs, Handle{
			Label: "session:" + s.ID + " " + clip(orDash(s.Title), 40),
			Ref:   Ref{Kind: KindSession, ID: s.ID},
			Level: LevelCard,
		})
	}
	return hs, elided
}

// agentName falls back to the id-derived placeholder when an agent has no name.
func agentName(a db.Agent) string {
	if a.Name == "" {
		return "(isimsiz)"
	}
	return a.Name
}

// modelLabel renders the agent's provider/model binding, collapsing an empty
// model (e.g. claude-cli's session default) to just the provider.
func modelLabel(a db.Agent) string {
	switch {
	case a.Provider == "" && a.Model == "":
		return "-"
	case a.Model == "":
		return a.Provider
	case a.Provider == "":
		return a.Model
	default:
		return a.Provider + "/" + a.Model
	}
}

// agentLastActivity renders the most recent session activity, or "-" when the
// agent has no (non-archived) sessions at all.
func agentLastActivity(ts int64, now time.Time) string {
	if ts <= 0 {
		return "-"
	}
	return age(tsSec(ts), now) + " önce"
}

// usageTokens sums every token class of a day's usage row.
func usageTokens(u db.Usage) int64 {
	return int64(u.InputTokens) + int64(u.OutputTokens) +
		int64(u.CacheReadTokens) + int64(u.CacheWriteTokens)
}
