package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/goals"
	"github.com/bilal-arikan/tionharness/internal/insight"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Workspace evolver (_Docs/83 §4.4, E2). One rare LLM pass per goal: it reads
// the goal's per-snapshot fitness (E1), the current values of the surfaces in
// scope, the worst recent runs and the fate of earlier proposals, and files
// PROPOSALS as insight findings on the evolution channel. It never applies
// anything (E3 does, under the goal's policy).
//
// Invariants enforced in code (goals.CheckProposal + this file): only the
// editable surfaces/fields; measured evidence; no general negative judgement;
// entity exists and is inside the goal's scope; growth budgets; the expected
// metric belongs to the goal and moves the right way; side effects on the
// goal's own guardrails refuse the proposal, on another goal's metrics mark a
// conflict; a proposal the user dismissed is not re-filed; a proposal that
// keeps returning open is escalated once and then silenced.

const (
	evolverSystemKey  = "workspace-evolver"
	evolverLensID     = "workspace-evolver"
	evolverMaxTokens  = 3000
	evolverWorstRuns  = 4
	evolverMaxAgents  = 8
	evolverMaxAutos   = 10
	evolverMaxSkills  = 12
	evolverSoulPeek   = 400
	evolverWindow     = 30 * 24 * time.Hour
	evolverTrigManual = "manual"
	evolverTrigRuns   = "runs"
	evolverTrigGuard  = "guardrail"
)

var evolverFlights sync.Map // goal id → in flight

var evolverSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["proposals"],
  "properties": {
    "proposals": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["surface", "entityId", "field", "action", "value", "removes", "title", "rationale", "evidence", "expectedMetric", "expectedDelta", "sideEffects", "severity"],
        "properties": {
          "surface": {"type": "string"},
          "entityId": {"type": "string"},
          "field": {"type": "string"},
          "action": {"type": "string"},
          "value": {"type": "string"},
          "removes": {"type": "string"},
          "title": {"type": "string"},
          "rationale": {"type": "string"},
          "evidence": {"type": "string"},
          "expectedMetric": {"type": "string"},
          "expectedDelta": {"type": "number"},
          "sideEffects": {"type": "array", "items": {"type": "string"}},
          "severity": {"type": "string"}
        }
      }
    }
  }
}`)

// EvolutionResult is what one pass returns.
type EvolutionResult struct {
	GoalID        string            `json:"goalId"`
	Trigger       string            `json:"trigger"`
	Ran           bool              `json:"ran"`
	Skipped       string            `json:"skipped,omitempty"`
	LowConfidence bool              `json:"lowConfidence,omitempty"`
	Proposals     []insight.Finding `json:"proposals"`
	Dropped       int               `json:"dropped"`
	DropReasons   []string          `json:"dropReasons,omitempty"`
	Sessions      int               `json:"sessions"`
}

// SweepGoals runs MaybeEvolveGoal for every active goal (called from the
// trajectory work queue when a run ends).
func (r *Runtime) SweepGoals(ctx context.Context) {
	list, err := r.db.ListGoalsByStatus(ctx, db.GoalStatusActive)
	if err != nil {
		return
	}
	for _, g := range list {
		r.MaybeEvolveGoal(ctx, g.ID)
	}
}

// MaybeEvolveGoal runs a pass when the goal's own thresholds allow: mode not
// off, cooldown elapsed, and at least minRuns new scoped sessions since the
// last pass — or a guardrail currently violated. The LLM call leaves the
// caller's goroutine.
func (r *Runtime) MaybeEvolveGoal(ctx context.Context, goalID string) {
	g, err := r.db.GetGoal(ctx, goalID)
	if err != nil || g.Status != db.GoalStatusActive || g.Policy.Mode == db.GoalModeOff {
		return
	}
	st, _ := r.db.GetEvolutionState(ctx)
	prev := st.Goals[g.ID]
	now := time.Now()
	if prev.LastAt > 0 && now.Sub(time.Unix(prev.LastAt, 0)) < time.Duration(goals.EffectiveCooldownHours(g))*time.Hour {
		return
	}
	in := r.FitnessInputs(ctx, now.Unix(), now.Add(-evolverWindow).Unix())
	fit := goals.Evaluate(g, in)
	trigger := ""
	if fit.Sessions-prev.SessionsSeen >= goals.EffectiveMinRuns(g) {
		trigger = evolverTrigRuns
	}
	for _, gr := range fit.Guardrails {
		if gr.Violated {
			trigger = evolverTrigGuard
		}
	}
	if trigger == "" {
		return
	}
	go func() {
		if _, err := r.RunGoalEvolver(context.Background(), g.ID, trigger); err != nil {
			r.logger.Warn("evolver: pass failed", "goal", g.ID, "error", err)
		}
	}()
}

// RunGoalEvolver runs one pass now. A manual trigger ignores cooldown and the
// minRuns threshold (the proposals are flagged low-confidence under it).
func (r *Runtime) RunGoalEvolver(ctx context.Context, goalID, trigger string) (EvolutionResult, error) {
	res := EvolutionResult{GoalID: goalID, Trigger: trigger, Proposals: []insight.Finding{}}
	g, err := r.db.GetGoal(ctx, goalID)
	if err != nil {
		return res, err
	}
	if _, busy := evolverFlights.LoadOrStore(goalID, true); busy {
		res.Skipped = "a pass is already running for this goal"
		return res, nil
	}
	defer evolverFlights.Delete(goalID)

	now := time.Now()
	st, _ := r.db.GetEvolutionState(ctx)
	state := st.Goals[goalID]
	if state.Repeats == nil {
		state.Repeats = map[string]int{}
	}
	if state.Escalated == nil {
		state.Escalated = map[string]bool{}
	}
	in := r.FitnessInputs(ctx, now.Unix(), now.Add(-evolverWindow).Unix())
	fit := goals.Evaluate(g, in)
	res.Sessions = fit.Sessions
	finish := func(skipped string) (EvolutionResult, error) {
		res.Skipped = skipped
		res.Ran = skipped == ""
		state.LastAt = now.Unix()
		state.SessionsSeen = fit.Sessions
		state.Trigger = trigger
		state.Proposals = len(res.Proposals)
		state.Dropped = res.Dropped
		state.Skipped = skipped
		state.SnapshotHash = in.CurrentHash
		_ = r.db.SetEvolutionGoalState(ctx, goalID, state)
		return res, nil
	}
	if g.Status != db.GoalStatusActive {
		return finish("goal is not active")
	}
	if g.Policy.Mode == db.GoalModeOff {
		return finish("goal policy is off")
	}
	if fit.Sessions == 0 {
		return finish("no sessions in scope yet")
	}
	res.LowConfidence = fit.Sessions < goals.EffectiveMinRuns(g)
	if res.LowConfidence && trigger != evolverTrigManual {
		return finish(fmt.Sprintf("only %d scoped sessions (min %d)", fit.Sessions, goals.EffectiveMinRuns(g)))
	}
	base, err := r.pickInsightAgent(ctx, "")
	if err != nil {
		return finish("no agent available")
	}
	agent, system, err := r.resolveAnalysisSystemAgent(evolverSystemKey, base)
	if err != nil {
		return finish("evolver agent unavailable")
	}
	if system == "" || strings.Contains(system, "retrospective analyst") {
		system = r.readPrompt(evolverSystemKey)
	}
	store, err := insight.OpenFindingStore(r.db.Root())
	if err != nil {
		return res, err
	}
	previous := store.List(evolverLensID, insight.ChannelEvolution)
	others, _ := r.db.ListGoalsByStatus(ctx, db.GoalStatusActive)
	ctxs := r.evolverSurfaces(ctx, g)
	lang := ""
	if r.tun != nil {
		lang = r.tun.Language()
	}
	user := evolverUserPrompt(g, fit, ctxs, worstRuns(in, g), previous, lang)
	resp, err := r.guardedComplete(WithPromptTrace(WithCallKind(ctx, KindReflect), evolverSystemKey, system), agent, providers.Request{
		Model: agent.Model, System: system, MaxTokens: evolverMaxTokens, OutputSchema: evolverSchema,
		Messages: []providers.Message{{Role: providers.RoleUser, Text: user}},
	}, false)
	if err != nil {
		_, _ = finish("model call failed: " + err.Error())
		return res, err
	}
	raw := extractJSONObject(resp.Text)
	var parsed struct {
		Proposals []goals.RawProposal `json:"proposals"`
	}
	if raw == "" || json.Unmarshal([]byte(raw), &parsed) != nil {
		_, _ = finish("no JSON in reply")
		return res, errors.New("evolver: no JSON in reply")
	}
	r.fileEvolverProposals(ctx, g, parsed.Proposals, others, in, &state, store, previous, &res, now.Unix())
	r.logger.Info("evolver pass", "goal", goalID, "trigger", trigger, "proposals", len(res.Proposals), "dropped", res.Dropped)
	if len(res.Proposals) > 0 {
		r.publish(events.Event{
			Type: "insight", Level: "info",
			Title:  fmt.Sprintf("🧬 Evrim — %s için %d öneri", g.Name, len(res.Proposals)),
			Body:   "Hedefler ▸ " + g.Name + " ▸ Öneriler",
			Target: map[string]string{"view": "goals", "id": g.ID},
		})
	}
	return finish("")
}

// surfaceContext is what the evolver sees of the workspace inside the scope.
type surfaceContext struct {
	Agents      []string
	Skills      []string
	Recipes     []string
	Automations []string
	Schedules   []string
	Settings    []string
}

func (r *Runtime) evolverSurfaces(ctx context.Context, g db.Goal) surfaceContext {
	var out surfaceContext
	agentScope := toSet(g.Scope.Agents)
	agents, _ := r.db.ListAgents(ctx)
	skillSlugs := map[string]bool{}
	n := 0
	for _, a := range agents {
		if a.Locked || a.Deleted {
			continue
		}
		if len(agentScope) > 0 && !agentScope[a.ID] {
			continue
		}
		if len(agentScope) == 0 && a.System {
			continue
		}
		if n >= evolverMaxAgents {
			out.Agents = append(out.Agents, "…")
			break
		}
		n++
		soul := strings.TrimSpace(a.Soul)
		out.Agents = append(out.Agents, fmt.Sprintf("%s %q: model=%s thinking=%s soulChars=%d skills=%v coordinatorWorkflow=%q toolOverrides=%s\n  soul: %s",
			a.ID, a.Name, orDash(a.Model), orDash(a.ThinkingLevel), len([]rune(soul)), a.Skills, a.CoordinatorWorkflow,
			clipText(strings.TrimSpace(a.ToolOverrides), 160), clipText(soul, evolverSoulPeek)))
		for _, s := range a.Skills {
			skillSlugs[s] = true
		}
	}
	if r.skills != nil {
		recipeScope := toSet(g.Scope.Recipes)
		k := 0
		for _, sk := range r.skills.List() {
			if sk.Recipe != nil {
				if len(recipeScope) > 0 && !recipeScope[sk.Slug] {
					continue
				}
				var phases []string
				for _, ph := range sk.Recipe.Phases {
					phases = append(phases, fmt.Sprintf("%s(%s)", ph.ID, orDash(ph.Profile)))
				}
				out.Recipes = append(out.Recipes, fmt.Sprintf("%s v%s: phases=%s watchers=%v", sk.Slug, sk.Recipe.Version, strings.Join(phases, ","), sk.Recipe.Watchers))
				continue
			}
			if !skillSlugs[sk.Slug] && len(agentScope) > 0 {
				continue
			}
			if k >= evolverMaxSkills {
				continue
			}
			k++
			out.Skills = append(out.Skills, fmt.Sprintf("%s: visibility=%s autoSummary=%v nameOnly=%v", sk.Slug, orDash(sk.Visibility), sk.AutoSummary, sk.NameOnly))
		}
	}
	autoScope := toSet(g.Scope.Automations)
	if autos, err := r.db.ListAutomations(ctx); err == nil {
		m := 0
		for _, a := range autos {
			if a.Archived || !a.Enabled || (len(autoScope) > 0 && !autoScope[a.ID]) {
				continue
			}
			if m >= evolverMaxAutos {
				break
			}
			m++
			out.Automations = append(out.Automations, fmt.Sprintf("%s %q: trigger=%s cooldownSec=%d maxIterations=%d fired=%d target=%s/%s lastError=%q",
				a.ID, a.Name, orDash(a.TriggerKind), a.CooldownSec, a.MaxIterations, a.IterationCount, orDash(a.TargetAgentID), orDash(a.FlowID), clipText(a.LastError, 80)))
		}
	}
	if scheds, err := r.db.ListSchedules(ctx); err == nil {
		for i, s := range scheds {
			if s.Archived || !s.Enabled || i >= 5 {
				continue
			}
			out.Schedules = append(out.Schedules, fmt.Sprintf("%s %q: cron=%s agent=%s flow=%s", s.ID, s.Name, s.CronExpr, orDash(s.AgentID), orDash(s.FlowID)))
		}
	}
	out.Settings = append(out.Settings, fmt.Sprintf("terseMode=%v", r.TerseModeEnabled()))
	return out
}

// proposalContext wires the scope / existence / budget checks to the store.
func (r *Runtime) proposalContext(ctx context.Context, g db.Goal, others []db.Goal) goals.ProposalContext {
	agentScope := toSet(g.Scope.Agents)
	recipeScope := toSet(g.Scope.Recipes)
	autoScope := toSet(g.Scope.Automations)
	return goals.ProposalContext{
		Goal:       g,
		OtherGoals: others,
		InScope: func(surface, id string) bool {
			switch surface {
			case goals.SurfaceAgent:
				return len(agentScope) == 0 || agentScope[id]
			case goals.SurfaceRecipe:
				return len(recipeScope) == 0 || recipeScope[id]
			case goals.SurfaceAutomation:
				return len(autoScope) == 0 || autoScope[id]
			}
			return true // workspace-wide surfaces
		},
		Exists: func(surface, id string) bool {
			switch surface {
			case goals.SurfaceAgent:
				a, err := r.db.GetAgent(ctx, id)
				return err == nil && !a.Locked && !a.Deleted
			case goals.SurfaceSkill, goals.SurfaceRecipe:
				if r.skills == nil {
					return false
				}
				_, ok := r.skills.Get(id)
				return ok
			case goals.SurfaceAutomation:
				_, err := r.db.GetAutomation(ctx, id)
				return err == nil
			case goals.SurfaceSchedule:
				_, err := r.db.GetSchedule(ctx, id)
				return err == nil
			}
			return true
		},
		SkillCount: func(agentID string) int {
			a, err := r.db.GetAgent(ctx, agentID)
			if err != nil {
				return 0
			}
			return len(a.Skills)
		},
	}
}

// evolverFinding turns an accepted proposal into the finding card.
func evolverFinding(g db.Goal, p insight.EvolutionProposal, raw goals.RawProposal, now int64, evidenceSessions []string) insight.Finding {
	title := strings.TrimSpace(raw.Title)
	if title == "" {
		title = fmt.Sprintf("%s %s.%s %s", p.Action, p.Surface, p.Field, p.EntityID)
	}
	if p.Kind == "conflict" {
		title = "Hedef çatışması: " + title
	}
	if p.LowConfidence {
		title = "[düşük güven] " + title
	}
	fix := strings.TrimSpace(raw.Rationale)
	if p.Value != "" {
		fix = strings.TrimSpace(fix + "\nDeğer: " + clipText(p.Value, 600))
	}
	if p.Removes != "" {
		fix = strings.TrimSpace(fix + "\nKaldırır: " + p.Removes)
	}
	fix = strings.TrimSpace(fix + fmt.Sprintf("\nBeklenen etki: %s %+g", p.ExpectedMetric, p.ExpectedDelta))
	pointer := p.Surface
	if p.EntityID != "" {
		pointer += "/" + p.EntityID
	}
	sev := normalizeSeverity(raw.Severity)
	if p.Kind == "conflict" {
		sev = "high"
	}
	pp := p
	return insight.Finding{
		LensID:             evolverLensID,
		Channel:            insight.ChannelEvolution,
		Signature:          goals.Signature(p),
		Title:              title,
		RootCause:          p.Evidence,
		ProposedFix:        fix,
		FilePointer:        pointer,
		Severity:           sev,
		EvidenceSessionIDs: evidenceSessions,
		Occurrences:        1,
		Status:             insight.StatusNew,
		FirstSeen:          now,
		LastSeen:           now,
		Evolution:          &pp,
	}
}

func findBySig(list []insight.Finding, sig string) (insight.Finding, bool) {
	for _, f := range list {
		if strings.EqualFold(f.Signature, sig) {
			return f, true
		}
	}
	return insight.Finding{}, false
}

// worstRuns picks the scoped terminal trajectories that hurt most (failed
// first, then costliest) for the prompt.
func worstRuns(in goals.FitnessInputs, g db.Goal) []db.TrajectoryIndexEntry {
	want := toSet(g.Scope.Recipes)
	var rows []db.TrajectoryIndexEntry
	for _, t := range in.Trajectories {
		if t.Summary == nil || (t.Status != db.TrajStatusDone && t.Status != db.TrajStatusFailed && t.Status != db.TrajStatusAbandoned) {
			continue
		}
		if len(want) > 0 && !want[recipeSlugOf(t.TemplateRef)] {
			continue
		}
		if in.Since > 0 && t.CreatedAt < in.Since {
			continue
		}
		rows = append(rows, t)
	}
	sort.Slice(rows, func(i, j int) bool {
		fi, fj := rows[i].Status != db.TrajStatusDone, rows[j].Status != db.TrajStatusDone
		if fi != fj {
			return fi
		}
		return rows[i].Summary.CostUSD > rows[j].Summary.CostUSD
	})
	if len(rows) > evolverWorstRuns {
		rows = rows[:evolverWorstRuns]
	}
	return rows
}

func worstRunRoots(in goals.FitnessInputs, g db.Goal) []string {
	var out []string
	for _, t := range worstRuns(in, g) {
		if t.RootSessionID != "" {
			out = append(out, t.RootSessionID)
		}
	}
	return out
}

func recipeSlugOf(ref string) string {
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

// evolverUserPrompt renders one pass's request.
func evolverUserPrompt(g db.Goal, fit goals.GoalFitness, sc surfaceContext, worst []db.TrajectoryIndexEntry, previous []insight.Finding, lang string) string {
	var b strings.Builder
	if lang != "" {
		fmt.Fprintf(&b, "Reply language for title/rationale/evidence: %s.\n\n", lang)
	}
	fmt.Fprintf(&b, "## Goal %s — %s\n\n", g.ID, g.Name)
	if g.Summary != "" {
		b.WriteString(g.Summary + "\n")
	}
	if g.Description != "" {
		b.WriteString(g.Description + "\n")
	}
	fmt.Fprintf(&b, "\nPrimary: %s (%s", g.Primary.Metric, g.Primary.Direction)
	if g.Primary.Target != nil {
		fmt.Fprintf(&b, ", target %g", *g.Primary.Target)
	}
	b.WriteString(")\nGuardrails:")
	if len(g.Guardrails) == 0 {
		b.WriteString(" none")
	}
	for _, gr := range g.Guardrails {
		fmt.Fprintf(&b, " %s[", gr.Metric)
		if gr.Min != nil {
			fmt.Fprintf(&b, "min %g", *gr.Min)
		}
		if gr.Max != nil {
			if gr.Min != nil {
				b.WriteString(" ")
			}
			fmt.Fprintf(&b, "max %g", *gr.Max)
		}
		b.WriteString("]")
	}
	fmt.Fprintf(&b, "\nScope: recipes=%v agents=%v automations=%v tags=%v\n", g.Scope.Recipes, g.Scope.Agents, g.Scope.Automations, g.Scope.Tags)

	b.WriteString("\n## Measured fitness (window)\n\n")
	fmt.Fprintf(&b, "%d scoped sessions. %s\n", fit.Sessions, metricLine(fit.Primary))
	for _, gr := range fit.Guardrails {
		status := "ok"
		if gr.Violated {
			status = "VIOLATED"
		}
		fmt.Fprintf(&b, "guardrail %s — %s\n", metricLine(gr.MetricValue), status)
	}
	b.WriteString("\nPer configuration snapshot (oldest first):\n")
	for _, s := range fit.BySnapshot {
		hash := s.Hash
		if hash == "" {
			hash = "unstamped"
		} else if len(hash) > 8 {
			hash = hash[:8]
		}
		cur := ""
		if s.Current {
			cur = " (current)"
		}
		fmt.Fprintf(&b, "- %s%s: %d sessions; %s", hash, cur, s.Sessions, metricLine(s.Primary))
		for _, gr := range s.Guardrails {
			fmt.Fprintf(&b, "; guardrail %s", metricLine(gr.MetricValue))
			if gr.Violated {
				b.WriteString(" VIOLATED")
			}
		}
		b.WriteString("\n")
		for i, c := range s.Changes {
			if i >= 6 {
				b.WriteString("    …\n")
				break
			}
			fmt.Fprintf(&b, "    changed: %s %s %s: %s → %s\n", c.Surface, c.Entity, c.Field, clipText(c.Before, 40), clipText(c.After, 40))
		}
	}

	b.WriteString("\n## Surfaces in scope (current values)\n\n")
	writeList := func(title string, items []string) {
		fmt.Fprintf(&b, "%s:\n", title)
		if len(items) == 0 {
			b.WriteString("  (none)\n")
		}
		for _, it := range items {
			b.WriteString("- " + it + "\n")
		}
	}
	writeList("Agents", sc.Agents)
	writeList("Skills", sc.Skills)
	writeList("Recipes", sc.Recipes)
	writeList("Automations", sc.Automations)
	writeList("Schedules", sc.Schedules)
	writeList("Settings", sc.Settings)

	b.WriteString("\n## Worst recent runs\n\n")
	if len(worst) == 0 {
		b.WriteString("(none)\n")
	}
	for _, t := range worst {
		s := t.Summary
		fmt.Fprintf(&b, "- %s %s [%s] root %s: %ds, %d tokens, $%.2f, %d sessions (%d failed), gateWait %ds, ghost %v, unfired %v\n",
			t.ID, t.TemplateRef, t.Status, t.RootSessionID, s.DurationSec, s.Tokens, s.CostUSD, s.Sessions, s.FailedSess, s.GateWaitSec, s.GhostPhases, s.UnfiredWatchers)
	}

	b.WriteString("\n## Earlier proposals for this goal\n\n")
	n := 0
	for _, f := range previous {
		if f.Evolution == nil || f.Evolution.GoalID != g.ID {
			continue
		}
		n++
		if n > 12 {
			b.WriteString("…\n")
			break
		}
		fmt.Fprintf(&b, "- [%s] %s (%s.%s %s %s)\n", f.Status, f.Title, f.Evolution.Surface, f.Evolution.Field, f.Evolution.Action, f.Evolution.EntityID)
	}
	if n == 0 {
		b.WriteString("(none)\n")
	}
	b.WriteString("Do not repeat a dismissed proposal; an open one may be re-stated only with new evidence.\n")

	b.WriteString("\n## Editable surfaces\n\n")
	b.WriteString(goals.RulesForPrompt())
	b.WriteString("\nAnswer with ONE JSON object {\"proposals\": [...]}, at most 3 entries.\n")
	return b.String()
}

func metricLine(m goals.MetricValue) string {
	if !m.Available {
		return m.Metric + " = (not measured)"
	}
	if m.Value == nil {
		return m.Metric + " = (no data)"
	}
	return fmt.Sprintf("%s = %.4g %s (n=%d)", m.Metric, *m.Value, m.Unit, m.N)
}

// fileEvolverProposals checks every raw proposal and files the accepted ones,
// applying the dismiss / repeat-escalation / per-pass-cap rules. Split from
// the LLM call so the rules are testable without a model.
func (r *Runtime) fileEvolverProposals(ctx context.Context, g db.Goal, raws []goals.RawProposal, others []db.Goal, in goals.FitnessInputs, state *db.EvolutionGoalState, store *insight.FindingStore, previous []insight.Finding, res *EvolutionResult, now int64) {
	pcx := r.proposalContext(ctx, g, others)
	evidenceSessions := worstRunRoots(in, g)
	seenThisPass := map[string]bool{}
	for _, rp := range raws {
		if len(res.Proposals) >= goals.MaxProposalsPerPass {
			res.Dropped++
			res.DropReasons = append(res.DropReasons, "over the per-pass cap")
			continue
		}
		p, why := goals.CheckProposal(rp, pcx)
		if why != "" {
			res.Dropped++
			res.DropReasons = append(res.DropReasons, why)
			continue
		}
		p.SnapshotHash = in.CurrentHash
		p.LowConfidence = res.LowConfidence
		sig := goals.Signature(p)
		if seenThisPass[sig] {
			res.Dropped++
			res.DropReasons = append(res.DropReasons, "duplicate in the same pass")
			continue
		}
		seenThisPass[sig] = true
		if state.Escalated[sig] {
			res.Dropped++
			res.DropReasons = append(res.DropReasons, "already escalated to a human")
			continue
		}
		if prevF, ok := findBySig(previous, sig); ok && prevF.Status == insight.StatusDismissed {
			res.Dropped++
			res.DropReasons = append(res.DropReasons, "dismissed by the user earlier")
			continue
		}
		f := evolverFinding(g, p, rp, now, evidenceSessions)
		if prevF, ok := findBySig(previous, sig); ok && !insight.ClosedStatus(prevF.Status) {
			state.Repeats[sig]++
			if state.Repeats[sig] >= goals.EscalateAfterRepeats {
				// The same open proposal came back again: hand it to a human
				// once instead of re-proposing forever.
				esc := f
				esc.Signature = sig + ":escalation"
				esc.Title = "İnsan kararı gerekiyor: " + f.Title
				esc.ProposedFix = fmt.Sprintf("Bu öneri %d geçiştir açık kalıyor. Kabul et, reddet ya da hedefi değiştir; evolver bunu artık önermeyecek.\n\n%s", state.Repeats[sig], f.ProposedFix)
				esc.Severity = "high"
				esc.Evolution.Kind = "escalation"
				state.Escalated[sig] = true
				if stored, err := store.Upsert(esc); err == nil {
					res.Proposals = append(res.Proposals, stored)
				}
				continue
			}
		} else {
			delete(state.Repeats, sig)
		}
		stored, err := store.Upsert(f)
		if err != nil {
			r.logger.Warn("evolver: finding upsert failed", "goal", g.ID, "error", err)
			continue
		}
		res.Proposals = append(res.Proposals, stored)
	}
}
