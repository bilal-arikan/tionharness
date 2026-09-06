package goals

import (
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Fitness (_Docs/83 §4.2, E1): every metric of the catalog computed, without
// an LLM, from telemetry the runtime already records — per goal scope, over a
// time window, and grouped by the configuration snapshot the sessions ran
// under. The inputs are plain rows so the computation stays pure and testable;
// the api layer gathers them (internal/api/goal_fitness.go).

// SessionRow is what fitness needs from a session.
type SessionRow struct {
	ID           string
	RootID       string // the root coordinator session (itself for a root)
	AgentID      string
	SnapshotHash string
	RunState     string
	Kind         string
	Tags         []string
	CreatedAt    int64
	StuckTurns   int
}

// UsageRow is a session's priced lifetime usage.
type UsageRow struct {
	Tokens      int64
	InputTokens int64
	CacheRead   int64
	CacheWrite  int64
	CostUSD     float64
	Priced      bool
}

// TaskRow is a board card.
type TaskRow struct {
	ID         string
	BoardState string
	CreatedAt  int64
	UpdatedAt  int64
	SessionIDs []string
}

// FireRow is one automation firing.
type FireRow struct {
	AutomationID string
	At           int64
	SessionID    string
	Failed       bool
}

// AskRow is one human ask (ask_user / approval gate).
type AskRow struct {
	SessionID string
}

// FitnessInputs is everything Evaluate reads.
type FitnessInputs struct {
	Now          int64
	Since        int64 // window start (unix seconds); 0 = everything
	Sessions     []SessionRow
	Trajectories []db.TrajectoryIndexEntry
	Usage        map[string]UsageRow // by session id
	Tasks        []TaskRow
	Fires        []FireRow
	Asks         []AskRow
	Snapshots    map[string]ConfigSnapshot // by hash, for config.* metrics and diffs
	CurrentHash  string
}

// MetricValue is one computed metric.
type MetricValue struct {
	Metric    string   `json:"metric"`
	Value     *float64 `json:"value"` // nil = no data in this group
	N         int      `json:"n"`     // sample size the value rests on
	Unit      string   `json:"unit"`
	Available bool     `json:"available"` // false = the catalog cannot measure this yet
	Note      string   `json:"note,omitempty"`
}

// GuardrailStatus is a guardrail evaluated over one group.
type GuardrailStatus struct {
	MetricValue
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// OK is true when the bound holds or there is no data to judge.
	OK       bool `json:"ok"`
	Violated bool `json:"violated"`
}

// SnapshotFitness is the goal's metrics over the sessions of one snapshot.
type SnapshotFitness struct {
	Hash       string            `json:"hash"`
	From       int64             `json:"from"`
	To         int64             `json:"to"`
	Sessions   int               `json:"sessions"`
	Current    bool              `json:"current"`
	Primary    MetricValue       `json:"primary"`
	Guardrails []GuardrailStatus `json:"guardrails"`
	Changes    []SnapshotChange  `json:"changes,omitempty"` // vs the previous snapshot in the series
}

// GoalFitness is the whole answer for one goal.
type GoalFitness struct {
	GoalID     string            `json:"goalId"`
	Since      int64             `json:"since"`
	Now        int64             `json:"now"`
	Sessions   int               `json:"sessions"` // scoped sessions in the window
	Primary    MetricValue       `json:"primary"`
	Guardrails []GuardrailStatus `json:"guardrails"`
	// Direction/Target echo the goal so the UI can judge the primary value.
	Direction  string            `json:"direction"`
	Target     *float64          `json:"target,omitempty"`
	OnTarget   *bool             `json:"onTarget,omitempty"`
	BySnapshot []SnapshotFitness `json:"bySnapshot"`
}

// Evaluate computes the goal's fitness over in.
func Evaluate(g db.Goal, in FitnessInputs) GoalFitness {
	scoped := scopeSessions(g, in)
	out := GoalFitness{
		GoalID: g.ID, Since: in.Since, Now: in.Now, Sessions: len(scoped),
		Direction: g.Primary.Direction, Target: g.Primary.Target,
	}
	out.Primary = computeMetric(g.Primary.Metric, g, scoped, in, in.CurrentHash)
	out.Guardrails = guardrails(g, scoped, in, in.CurrentHash)
	if out.Primary.Value != nil && g.Primary.Target != nil {
		ok := (g.Primary.Direction == db.GoalDirectionMin && *out.Primary.Value <= *g.Primary.Target) ||
			(g.Primary.Direction == db.GoalDirectionMax && *out.Primary.Value >= *g.Primary.Target)
		out.OnTarget = &ok
	}

	// Group by snapshot, ordered by first session time; unstamped sessions
	// form their own "" group at the front.
	groups := map[string][]SessionRow{}
	first := map[string]int64{}
	last := map[string]int64{}
	for _, s := range scoped {
		h := s.SnapshotHash
		groups[h] = append(groups[h], s)
		if f, ok := first[h]; !ok || s.CreatedAt < f {
			first[h] = s.CreatedAt
		}
		if s.CreatedAt > last[h] {
			last[h] = s.CreatedAt
		}
	}
	hashes := make([]string, 0, len(groups))
	for h := range groups {
		hashes = append(hashes, h)
	}
	sort.Slice(hashes, func(i, j int) bool {
		if first[hashes[i]] != first[hashes[j]] {
			return first[hashes[i]] < first[hashes[j]]
		}
		return hashes[i] < hashes[j]
	})
	// The current configuration is listed even with no session yet, so the
	// user sees "no data under the current config" rather than nothing.
	if in.CurrentHash != "" {
		if _, ok := groups[in.CurrentHash]; !ok {
			hashes = append(hashes, in.CurrentHash)
			first[in.CurrentHash] = in.Now
			last[in.CurrentHash] = in.Now
		}
	}
	out.BySnapshot = make([]SnapshotFitness, 0, len(hashes))
	prev := ""
	for _, h := range hashes {
		rows := groups[h]
		sf := SnapshotFitness{
			Hash: h, From: first[h], To: last[h], Sessions: len(rows), Current: h == in.CurrentHash,
			Primary:    computeMetric(g.Primary.Metric, g, rows, in, h),
			Guardrails: guardrails(g, rows, in, h),
		}
		if prev != "" && h != "" {
			if a, okA := in.Snapshots[prev]; okA {
				if b, okB := in.Snapshots[h]; okB {
					sf.Changes = Diff(a, b)
				}
			}
		}
		if h != "" {
			prev = h
		}
		out.BySnapshot = append(out.BySnapshot, sf)
	}
	return out
}

func guardrails(g db.Goal, rows []SessionRow, in FitnessInputs, hash string) []GuardrailStatus {
	out := make([]GuardrailStatus, 0, len(g.Guardrails))
	for _, gr := range g.Guardrails {
		mv := computeMetric(gr.Metric, g, rows, in, hash)
		st := GuardrailStatus{MetricValue: mv, Min: gr.Min, Max: gr.Max, OK: true}
		if mv.Value != nil {
			if gr.Min != nil && *mv.Value < *gr.Min {
				st.OK, st.Violated = false, true
			}
			if gr.Max != nil && *mv.Value > *gr.Max {
				st.OK, st.Violated = false, true
			}
		}
		out = append(out, st)
	}
	return out
}

// scopeSessions keeps the sessions inside the goal's scope and time window.
// Every set scope kind must match (intersection); an empty scope keeps all.
func scopeSessions(g db.Goal, in FitnessInputs) []SessionRow {
	var recipeRoots map[string]bool
	if len(g.Scope.Recipes) > 0 {
		recipeRoots = map[string]bool{}
		want := toSet(g.Scope.Recipes)
		for _, t := range in.Trajectories {
			if want[recipeSlug(t.TemplateRef)] {
				recipeRoots[t.RootSessionID] = true
			}
		}
	}
	var autoSessions map[string]bool
	if len(g.Scope.Automations) > 0 {
		autoSessions = map[string]bool{}
		want := toSet(g.Scope.Automations)
		for _, f := range in.Fires {
			if want[f.AutomationID] && f.SessionID != "" {
				autoSessions[f.SessionID] = true
			}
		}
	}
	agents := toSet(g.Scope.Agents)
	tags := toSet(g.Scope.Tags)
	var out []SessionRow
	for _, s := range in.Sessions {
		if in.Since > 0 && s.CreatedAt < in.Since {
			continue
		}
		if s.Kind == db.SessionKindInsight {
			continue
		}
		if recipeRoots != nil && !recipeRoots[s.RootID] && !recipeRoots[s.ID] {
			continue
		}
		if autoSessions != nil && !autoSessions[s.ID] && !autoSessions[s.RootID] {
			continue
		}
		if len(agents) > 0 && !agents[s.AgentID] {
			continue
		}
		if len(tags) > 0 && !anyIn(s.Tags, tags) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// computeMetric evaluates one catalog metric over rows (sessions) — config.*
// metrics read the snapshot named by hash instead.
func computeMetric(key string, g db.Goal, rows []SessionRow, in FitnessInputs, hash string) MetricValue {
	def, ok := Lookup(key)
	mv := MetricValue{Metric: key, Unit: def.Unit, Available: ok && def.Available}
	if !ok {
		mv.Note = "unknown metric"
		return mv
	}
	if !def.Available {
		mv.Note = "not measured yet"
		return mv
	}
	ids := map[string]bool{}
	var earliest int64
	for _, s := range rows {
		ids[s.ID] = true
		if earliest == 0 || s.CreatedAt < earliest {
			earliest = s.CreatedAt
		}
	}
	days := windowDays(in, earliest)
	set := func(v float64, n int) MetricValue {
		if n == 0 {
			return mv
		}
		mv.Value, mv.N = &v, n
		return mv
	}
	switch {
	case strings.HasPrefix(key, "recipe."):
		return recipeMetric(key, g, ids, in, mv)
	case key == "usage.costUSDPerSession", key == "usage.tokensPerSession", key == "usage.cacheHitRatio", key == "usage.costUSDPerDay":
		var cost, tokens, read, denom float64
		n := 0
		for id := range ids {
			u, ok := in.Usage[id]
			if !ok || u.Tokens == 0 {
				continue
			}
			n++
			cost += u.CostUSD
			tokens += float64(u.Tokens)
			read += float64(u.CacheRead)
			denom += float64(u.InputTokens + u.CacheRead + u.CacheWrite)
		}
		switch key {
		case "usage.costUSDPerSession":
			return set(cost/float64(max(n, 1)), n)
		case "usage.tokensPerSession":
			return set(tokens/float64(max(n, 1)), n)
		case "usage.costUSDPerDay":
			return set(cost/days, n)
		default:
			if denom == 0 {
				return mv
			}
			return set(read/denom, n)
		}
	case key == "board.cycleTimeSec", key == "board.cardsDonePerDay":
		var total float64
		n := 0
		for _, t := range in.Tasks {
			if t.BoardState != db.BoardDone {
				continue
			}
			if in.Since > 0 && t.UpdatedAt < in.Since {
				continue
			}
			if !taskInGroup(t, ids, g, in) {
				continue
			}
			n++
			total += float64(t.UpdatedAt - t.CreatedAt)
		}
		if key == "board.cycleTimeSec" {
			return set(total/float64(max(n, 1)), n)
		}
		return set(float64(n)/days, n)
	case key == "automation.errorRate", key == "automation.firesPerDay":
		want := toSet(g.Scope.Automations)
		n, failed := 0, 0
		for _, f := range in.Fires {
			if in.Since > 0 && f.At < in.Since {
				continue
			}
			if len(want) > 0 && !want[f.AutomationID] {
				continue
			}
			// Attribute a fire to the group through the session it produced;
			// fires without a session count only in the whole-scope group.
			if f.SessionID != "" && !ids[f.SessionID] {
				continue
			}
			if f.SessionID == "" && hash != in.CurrentHash && hash != "" {
				continue
			}
			n++
			if f.Failed {
				failed++
			}
		}
		if key == "automation.errorRate" {
			return set(float64(failed)/float64(max(n, 1)), n)
		}
		return set(float64(n)/days, n)
	case key == "session.errorTurnsRatio":
		failed := 0
		for _, s := range rows {
			if isFailedRunState(s.RunState) {
				failed++
			}
		}
		return set(float64(failed)/float64(max(len(rows), 1)), len(rows))
	case key == "session.humanAsksPerSession":
		asks := 0
		for _, a := range in.Asks {
			if ids[a.SessionID] {
				asks++
			}
		}
		return set(float64(asks)/float64(max(len(rows), 1)), len(rows))
	case key == "session.stuckLoops":
		stuck := 0
		for _, s := range rows {
			stuck += s.StuckTurns
		}
		return set(float64(stuck)/float64(max(len(rows), 1)), len(rows))
	case strings.HasPrefix(key, "config."):
		snap, ok := in.Snapshots[hash]
		if !ok {
			mv.Note = "snapshot unknown"
			return mv
		}
		want := toSet(g.Scope.Agents)
		var total float64
		n := 0
		for id, a := range snap.Agents {
			if len(want) > 0 && !want[id] {
				continue
			}
			if len(want) == 0 && a.SystemKey != "" {
				continue
			}
			switch key {
			case "config.soulChars":
				total += float64(a.SoulChars)
			case "config.skillCount":
				total += float64(len(a.Skills))
			default:
				return mv
			}
			n++
		}
		return set(total/float64(max(n, 1)), n)
	}
	mv.Note = "no evaluator"
	return mv
}

func recipeMetric(key string, g db.Goal, roots map[string]bool, in FitnessInputs, mv MetricValue) MetricValue {
	want := toSet(g.Scope.Recipes)
	var runs, done, summarized int
	var dur, tokens, gate, ghost, unfired, failedSess, sessions float64
	var cost float64
	for _, t := range in.Trajectories {
		if len(want) > 0 && !want[recipeSlug(t.TemplateRef)] {
			continue
		}
		if !roots[t.RootSessionID] {
			continue
		}
		switch t.Status {
		case db.TrajStatusDone:
			done++
		case db.TrajStatusFailed, db.TrajStatusAbandoned:
		default:
			continue
		}
		runs++
		if t.Summary == nil {
			continue
		}
		summarized++
		dur += float64(t.Summary.DurationSec)
		tokens += float64(t.Summary.Tokens)
		cost += t.Summary.CostUSD
		gate += float64(t.Summary.GateWaitSec)
		ghost += float64(len(t.Summary.GhostPhases))
		unfired += float64(len(t.Summary.UnfiredWatchers))
		failedSess += float64(t.Summary.FailedSess)
		sessions += float64(t.Summary.Sessions)
	}
	set := func(v float64, n int) MetricValue {
		if n == 0 {
			return mv
		}
		mv.Value, mv.N = &v, n
		return mv
	}
	if key == "recipe.successRate" {
		return set(float64(done)/float64(max(runs, 1)), runs)
	}
	den := float64(max(summarized, 1))
	switch key {
	case "recipe.avgCostUSD":
		return set(cost/den, summarized)
	case "recipe.avgDurationSec":
		return set(dur/den, summarized)
	case "recipe.avgTokens":
		return set(tokens/den, summarized)
	case "recipe.avgSessions":
		return set(sessions/den, summarized)
	case "recipe.failedSessions":
		return set(failedSess/den, summarized)
	case "recipe.gateWaitSec":
		return set(gate/den, summarized)
	case "recipe.ghostPhases":
		return set(ghost/den, summarized)
	case "recipe.unfiredWatchers":
		return set(unfired/den, summarized)
	}
	mv.Note = "no evaluator"
	return mv
}

// taskInGroup attributes a card to a session group through its sessions. A
// card with no session belongs to the whole-scope group only when the goal
// has no session-level scope.
func taskInGroup(t TaskRow, ids map[string]bool, g db.Goal, in FitnessInputs) bool {
	for _, id := range t.SessionIDs {
		if ids[id] {
			return true
		}
	}
	if len(t.SessionIDs) > 0 {
		return false
	}
	return len(g.Scope.Agents)+len(g.Scope.Tags)+len(g.Scope.Recipes)+len(g.Scope.Automations) == 0 && len(ids) == len(in.Sessions)
}

func windowDays(in FitnessInputs, earliest int64) float64 {
	start := in.Since
	if start == 0 || (earliest > 0 && earliest > start) {
		start = earliest
	}
	if start == 0 || in.Now <= start {
		return 1
	}
	d := float64(in.Now-start) / 86400
	if d < 1 {
		return 1
	}
	return d
}

// isFailedRunState: the run states the runtime writes for a turn that ended
// badly (failed, error, killed); "" and "running"/"done" are clean.
func isFailedRunState(s string) bool {
	switch strings.ToLower(s) {
	case "failed", "error", "killed", "stuck":
		return true
	}
	return false
}

// RecipeSlug strips the "@version" suffix of a trajectory template ref.
func recipeSlug(ref string) string {
	if i := strings.LastIndex(ref, "@"); i > 0 {
		return ref[:i]
	}
	return ref
}

func toSet(l []string) map[string]bool {
	if len(l) == 0 {
		return nil
	}
	m := make(map[string]bool, len(l))
	for _, s := range l {
		m[s] = true
	}
	return m
}

func anyIn(l []string, set map[string]bool) bool {
	for _, s := range l {
		if set[s] {
			return true
		}
	}
	return false
}
