package api

// handleDashboard is the workspace overview screen's single data call: the
// counters, the chart series and the workspace PROJECTION text, in one response.
//
//	GET /api/dashboard?days=14
//
// Two deliberate choices:
//
//   - The series are aggregated HERE, not in the browser. The client would
//     otherwise download every session, task and flow run just to count them —
//     which is the exact cost the projection layer exists to avoid (_Docs/66).
//   - The `summary` field is the workspace view rendered by internal/view, i.e.
//     byte-identical to what an agent gets from get_view{kind:"workspace"}. The
//     dashboard shows the agent's own summary rather than a prettier parallel
//     one, so a wrong projection is visible to the user.

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/view"
)

// dashboardMaxDays bounds the trend window. 90 days matches /api/usage so the
// two screens can never disagree about how far back "the trend" goes.
const dashboardMaxDays = 90

// daySeriesPoint is one bucket of a daily count series. Distinct from budget.go's
// dayPoint, which carries per-day cost/cache detail the dashboard does not plot.
type daySeriesPoint struct {
	Day   string `json:"day"` // YYYY-MM-DD
	Value int64  `json:"value"`
}

// namedCount is one slice of a categorical breakdown (column, status, agent).
type namedCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()
	now := time.Now()

	days := 14
	if q := r.URL.Query().Get("days"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n >= 1 && n <= dashboardMaxDays {
			days = n
		}
	}

	sessions, err := wsp.DB.ListSessions(ctx, "")
	if writeDBError(w, err, "") {
		return
	}
	tasks, err := wsp.DB.ListTasks(ctx)
	if writeDBError(w, err, "") {
		return
	}
	runs, err := wsp.DB.ListFlowRuns(ctx, "")
	if writeDBError(w, err, "") {
		return
	}
	agents, err := wsp.DB.ListAgents(ctx)
	if writeDBError(w, err, "") {
		return
	}
	// Schedules and pending asks feed the action queue only; a failure there must
	// not take the whole dashboard down, so they degrade to empty rather than 500.
	schedules, _ := wsp.DB.ListSchedules(ctx)
	asks, _ := wsp.DB.ListWaitingSessionAsks(ctx)

	cost, costByDay := dashboardCost(ctx, wsp.DB, days, now)

	// The workspace projection: the same text an agent reads, from the same
	// fully-wired projector every other surface uses (so the header carries the
	// workspace name here too, and this block stays byte-identical to
	// GET /api/views/workspace). The counters come out of the SAME load — the
	// stat tiles and the summary text can therefore never disagree. A failure is
	// reported rather than swallowed: a dashboard with a blank summary looks like
	// an idle workspace.
	v, counts, err := s.viewProjector(r).Workspace(ctx, view.LevelCard, view.LensHealth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"asOf": now,
		"summary": map[string]any{
			"text":       v.Text(),
			"tokens":     v.Tokens,
			"elided":     v.Elided,
			"elidedUnit": v.ElidedUnit,
			"handles":    v.Handles,
		},
		"counters":       counts,
		"sessionsByDay":  sessionsByDay(sessions, days, now),
		"runsByDay":      runsByDay(runs, days, now),
		"tokensByDay":    tokensByDay(ctx, wsp.DB, days, now),
		"boardByColumn":  boardByColumn(tasks),
		"runsByStatus":   runsByStatus(runs),
		"sessionsByKind": sessionsByKind(sessions),
		"topAgents":      topAgents(sessions, agents),
		// Item 1 — cost. Item 2 — action queue. Item 3 — period deltas. Item 4 —
		// outcomes. All priced/counted on the backend so the browser never
		// downloads the workspace to compute them.
		"cost":          cost,
		"costByDay":     costByDay,
		"topAgentsCost": topAgentsByCost(ctx, wsp.DB, agents, days, now),
		"actions":       dashboardActions(sessions, tasks, runs, schedules, asks, now),
		"deltas":        dashboardDeltas(ctx, wsp.DB, sessions, runs, days, now),
		"outcomes":      dashboardOutcomes(tasks, runs, days, now),
	})
}

// The stat tiles are counted by view.CountWorkspace, not here: this handler used
// to run its own tally of the same sessions/tasks/runs it then asked the view
// layer to summarise, and two independent counts of one set of facts can drift
// apart in front of the user. See internal/view.WorkspaceCounts.

// dayKeys returns the last `days` day labels, oldest first, so a chart always
// has a full x-axis even on days where nothing happened. Without this a quiet
// weekend silently disappears from the trend instead of showing as a gap.
func dayKeys(days int, now time.Time) []string {
	keys := make([]string, 0, days)
	for i := days - 1; i >= 0; i-- {
		keys = append(keys, now.AddDate(0, 0, -i).Format("2006-01-02"))
	}
	return keys
}

// bucketByDay counts unix-SECOND timestamps into the trailing day window.
func bucketByDay(stamps []int64, days int, now time.Time) []daySeriesPoint {
	keys := dayKeys(days, now)
	idx := make(map[string]int, len(keys))
	for i, k := range keys {
		idx[k] = i
	}
	out := make([]daySeriesPoint, len(keys))
	for i, k := range keys {
		out[i] = daySeriesPoint{Day: k}
	}
	for _, ts := range stamps {
		if ts <= 0 {
			continue
		}
		if i, ok := idx[time.Unix(ts, 0).Format("2006-01-02")]; ok {
			out[i].Value++
		}
	}
	return out
}

func sessionsByDay(sessions []db.Session, days int, now time.Time) []daySeriesPoint {
	stamps := make([]int64, 0, len(sessions))
	for _, s := range sessions {
		stamps = append(stamps, s.CreatedAt)
	}
	return bucketByDay(stamps, days, now)
}

func runsByDay(runs []db.FlowRun, days int, now time.Time) []daySeriesPoint {
	stamps := make([]int64, 0, len(runs))
	for _, r := range runs {
		stamps = append(stamps, r.CreatedAt)
	}
	return bucketByDay(stamps, days, now)
}

// tokensByDay reuses the usage rollups the Budget screen reads, so the two
// screens report the same spend rather than two independent tallies.
func tokensByDay(ctx context.Context, database *db.DB, days int, now time.Time) []daySeriesPoint {
	keys := dayKeys(days, now)
	idx := make(map[string]int, len(keys))
	for i, k := range keys {
		idx[k] = i
	}
	out := make([]daySeriesPoint, len(keys))
	for i, k := range keys {
		out[i] = daySeriesPoint{Day: k}
	}

	rows, err := database.UsageHistory(ctx, keys[0])
	if err != nil {
		// The trend degrades to zeros rather than failing the dashboard; the
		// counters and the projection above it are still correct and useful.
		return out
	}
	for _, u := range rows {
		if i, ok := idx[u.Day]; ok {
			out[i].Value += int64(u.InputTokens) + int64(u.OutputTokens) +
				int64(u.CacheReadTokens) + int64(u.CacheWriteTokens)
		}
	}
	return out
}

// boardByColumn mirrors the board projection's ordering: built-in columns in
// their canonical board order, workspace-custom keys after them alphabetically.
func boardByColumn(tasks []db.Task) []namedCount {
	counts := map[string]int{}
	for _, t := range tasks {
		key := t.BoardState
		if key == "" {
			key = "(boş)"
		}
		counts[key]++
	}

	order := map[string]int{}
	for i, c := range db.DefaultBoardColumns() {
		order[c.Key] = i
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		oi, ki := order[keys[i]]
		oj, kj := order[keys[j]]
		if ki != kj {
			return ki
		}
		if ki && kj {
			return oi < oj
		}
		return keys[i] < keys[j]
	})

	out := make([]namedCount, 0, len(keys))
	for _, k := range keys {
		out = append(out, namedCount{Name: k, Count: counts[k]})
	}
	return out
}

func runsByStatus(runs []db.FlowRun) []namedCount {
	counts := map[string]int{}
	for _, r := range runs {
		counts[r.Status]++
	}
	// Fixed order so the chart's colours stay stable between refreshes.
	out := []namedCount{}
	for _, st := range []string{db.FlowSuccess, db.FlowRunning, db.FlowWaiting, db.FlowFailure} {
		if n := counts[st]; n > 0 {
			out = append(out, namedCount{Name: st, Count: n})
		}
		delete(counts, st)
	}
	return append(out, sortedCounts(counts)...)
}

func sessionsByKind(sessions []db.Session) []namedCount {
	counts := map[string]int{}
	for _, s := range sessions {
		kind := s.Kind
		if kind == "" {
			kind = "chat"
		}
		counts[kind]++
	}
	return sortedCounts(counts)
}

// topAgents ranks agents by how many sessions they own, resolving display names.
func topAgents(sessions []db.Session, agents []db.Agent) []namedCount {
	name := make(map[string]string, len(agents))
	for _, a := range agents {
		name[a.ID] = a.Name
	}
	counts := map[string]int{}
	for _, s := range sessions {
		if s.AgentID == "" {
			continue
		}
		n := name[s.AgentID]
		if n == "" {
			// A deleted agent still owns history; showing its id beats dropping
			// the sessions from the chart and under-reporting the total.
			n = s.AgentID
		}
		counts[n]++
	}
	out := sortedCounts(counts)
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// sortedCounts renders a count map biggest-first, ties broken by name so the
// output is stable across refreshes.
func sortedCounts(counts map[string]int) []namedCount {
	out := make([]namedCount, 0, len(counts))
	for k, v := range counts {
		out = append(out, namedCount{Name: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}
