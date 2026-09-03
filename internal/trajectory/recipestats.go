package trajectory

import (
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Recipe statistics (Rota F3): the per-recipe-version rollup of finished
// trajectories, computed from the index rows' summaries only. "plan-dev@3 ran
// 5 times, 4 done, avg 12 min / 40k tokens, the docs watcher never fired,
// the ship phase was never reached" — the numbers the curator's recipe
// suggestions and the Skills screen rest on.

// RecipeStats is one recipe version's rollup.
type RecipeStats struct {
	TemplateRef string `json:"templateRef"`
	Slug        string `json:"slug"`
	Version     string `json:"version,omitempty"`
	Runs        int    `json:"runs"` // terminal runs (done + failed + abandoned)
	Done        int    `json:"done"`
	Failed      int    `json:"failed"`
	Abandoned   int    `json:"abandoned"`
	Live        int    `json:"live"` // planned / running / waiting
	// Averages over the terminal runs that carry a summary.
	Summarized     int     `json:"summarized"`
	AvgDurationSec int64   `json:"avgDurationSec"`
	AvgTokens      int64   `json:"avgTokens"`
	AvgCostUSD     float64 `json:"avgCostUsd"`
	Priced         bool    `json:"priced"`
	AvgSessions    float64 `json:"avgSessions"`
	FailedSessions int     `json:"failedSessions"`
	Unannounced    int     `json:"unannounced"`
	GateWaitSec    int64   `json:"gateWaitSec"`
	// UnfiredWatchers / GhostPhases: how many summarized runs each declared
	// watcher / phase stayed unfired / unreached in.
	UnfiredWatchers map[string]int `json:"unfiredWatchers,omitempty"`
	GhostPhases     map[string]int `json:"ghostPhases,omitempty"`
	LastAt          int64          `json:"lastAt"`
	LatestID        string         `json:"latestId,omitempty"`
}

// RecipeStatsFromIndex groups index rows by TemplateRef. Rows without a
// recipe are reported under the empty ref ("plansız") so agent-planned runs
// still have a line.
func RecipeStatsFromIndex(rows []db.TrajectoryIndexEntry) []RecipeStats {
	by := map[string]*RecipeStats{}
	for _, e := range rows {
		st := by[e.TemplateRef]
		if st == nil {
			slug, version := e.TemplateRef, ""
			if i := strings.LastIndex(e.TemplateRef, "@"); i > 0 {
				slug, version = e.TemplateRef[:i], e.TemplateRef[i+1:]
			}
			st = &RecipeStats{TemplateRef: e.TemplateRef, Slug: slug, Version: version, Priced: true,
				UnfiredWatchers: map[string]int{}, GhostPhases: map[string]int{}}
			by[e.TemplateRef] = st
		}
		if e.UpdatedAt > st.LastAt {
			st.LastAt = e.UpdatedAt
			st.LatestID = e.ID
		}
		switch e.Status {
		case db.TrajStatusDone:
			st.Runs++
			st.Done++
		case db.TrajStatusFailed:
			st.Runs++
			st.Failed++
		case db.TrajStatusAbandoned:
			st.Runs++
			st.Abandoned++
		default:
			st.Live++
			continue
		}
		s := e.Summary
		if s == nil {
			continue
		}
		st.Summarized++
		st.AvgDurationSec += s.DurationSec
		st.AvgTokens += s.Tokens
		st.AvgCostUSD += s.CostUSD
		if !s.Priced {
			st.Priced = false
		}
		st.AvgSessions += float64(s.Sessions)
		st.FailedSessions += s.FailedSess
		st.Unannounced += s.Unannounced
		st.GateWaitSec += s.GateWaitSec
		for _, w := range s.UnfiredWatchers {
			st.UnfiredWatchers[w]++
		}
		for _, p := range s.GhostPhases {
			st.GhostPhases[p]++
		}
	}
	out := make([]RecipeStats, 0, len(by))
	for _, st := range by {
		if n := int64(st.Summarized); n > 0 {
			st.AvgDurationSec /= n
			st.AvgTokens /= n
			st.AvgCostUSD /= float64(n)
			st.AvgSessions /= float64(n)
		}
		if len(st.UnfiredWatchers) == 0 {
			st.UnfiredWatchers = nil
		}
		if len(st.GhostPhases) == 0 {
			st.GhostPhases = nil
		}
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slug != out[j].Slug {
			return out[i].Slug < out[j].Slug
		}
		return out[i].Version < out[j].Version
	})
	return out
}
