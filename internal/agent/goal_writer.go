package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/goals"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Goal writer (_Docs/83 §4.1). Goals are never created straight from a form:
// the user states the goal in their own words, the goal-writer system agent
// maps it onto the closed metric catalog, the workspace's scope candidates and
// a propose-only policy, and the code validates and stores the result as a
// DRAFT. The user then reviews, edits and activates it in the Goals screen.
//
// Invariants enforced in code: unknown metrics / scopes / surfaces are refused
// by goals.Validate, the writer can never set an auto policy (goals.FromDraft
// downgrades it), and the user's original words are kept verbatim as RawText.

const (
	goalWriterSystemKey = "goal-writer"
	goalWriterMaxTokens = 2000
	goalWriterMaxRaw    = 4000
	goalWriterMaxGoals  = 12
	goalWriterMaxItems  = 40
)

// GoalIntakeResult is what an intake returns to the API.
type GoalIntakeResult struct {
	Goal db.Goal `json:"goal"`
	// Created is true for a new draft, false for a rewrite of an existing goal.
	Created bool `json:"created"`
}

// GoalScopeCandidates are the ids the writer may reference in a scope.
type GoalScopeCandidates struct {
	Recipes     []goalCandidate `json:"recipes"`
	Agents      []goalCandidate `json:"agents"`
	Automations []goalCandidate `json:"automations"`
	Tags        []string        `json:"tags"`
}

type goalCandidate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WriteGoal runs one intake: rawText in the user's words, optionally refining
// existing goal goalID. The stored draft is returned.
func (r *Runtime) WriteGoal(ctx context.Context, rawText, goalID string) (GoalIntakeResult, error) {
	rawText = strings.TrimSpace(rawText)
	if rawText == "" {
		return GoalIntakeResult{}, fmt.Errorf("%w: empty goal statement", goals.ErrInvalid)
	}
	if len(rawText) > goalWriterMaxRaw {
		return GoalIntakeResult{}, fmt.Errorf("%w: statement longer than %d characters", goals.ErrInvalid, goalWriterMaxRaw)
	}
	var base *db.Goal
	if goalID != "" {
		g, err := r.db.GetGoal(ctx, goalID)
		if err != nil {
			return GoalIntakeResult{}, err
		}
		base = &g
	}
	agent, err := r.pickInsightAgent(ctx, "")
	if err != nil {
		return GoalIntakeResult{}, errors.New("goal writer: no agent available")
	}
	agent, system, err := r.resolveAnalysisSystemAgent(goalWriterSystemKey, agent)
	if err != nil {
		return GoalIntakeResult{}, errors.New("goal writer: system agent unavailable")
	}
	if system == "" || strings.Contains(system, "retrospective analyst") {
		system = r.readPrompt(goalWriterSystemKey)
	}
	existing, _ := r.db.ListGoals(ctx)
	cands := r.GoalScopeCandidates(ctx)
	lang := ""
	if r.tun != nil {
		lang = r.tun.Language()
	}
	user := goalWriterUserPrompt(rawText, base, existing, cands, lang)
	resp, err := r.guardedComplete(WithPromptTrace(WithCallKind(ctx, KindReflect), goalWriterSystemKey, system), agent, providers.Request{
		Model: agent.Model, System: system, MaxTokens: goalWriterMaxTokens, OutputSchema: goals.DraftSchema,
		Messages: []providers.Message{{Role: providers.RoleUser, Text: user}},
	}, false)
	if err != nil {
		return GoalIntakeResult{}, fmt.Errorf("goal writer: model call failed: %w", err)
	}
	raw := extractJSONObject(resp.Text)
	var draft goals.Draft
	if raw == "" || json.Unmarshal([]byte(raw), &draft) != nil {
		return GoalIntakeResult{}, errors.New("goal writer: no JSON draft in reply")
	}
	g := goals.FromDraft(draft, rawText, base)
	if err := goals.Validate(g); err != nil {
		return GoalIntakeResult{}, fmt.Errorf("goal writer produced an invalid draft: %w", err)
	}
	if base == nil {
		stored, err := r.db.CreateGoal(ctx, g, db.GoalByWriter, "written from the user's statement")
		if err != nil {
			return GoalIntakeResult{}, err
		}
		return GoalIntakeResult{Goal: stored, Created: true}, nil
	}
	if err := r.db.ReplaceGoalRawText(ctx, base.ID, rawText); err != nil {
		return GoalIntakeResult{}, err
	}
	stored, err := r.db.UpdateGoal(ctx, g, db.GoalByWriter, "rewritten from a new statement")
	if err != nil {
		return GoalIntakeResult{}, err
	}
	return GoalIntakeResult{Goal: stored}, nil
}

// GoalScopeCandidates collects the recipe slugs, agents, automations and
// session tags of the workspace — the only scope values a goal may name.
func (r *Runtime) GoalScopeCandidates(ctx context.Context) GoalScopeCandidates {
	out := GoalScopeCandidates{Recipes: []goalCandidate{}, Agents: []goalCandidate{}, Automations: []goalCandidate{}, Tags: []string{}}
	if r.skills != nil {
		for _, sk := range r.skills.List() {
			if sk.Recipe != nil {
				out.Recipes = append(out.Recipes, goalCandidate{ID: sk.Slug, Name: sk.Name})
			}
		}
	}
	if agents, err := r.db.ListAgents(ctx); err == nil {
		for _, a := range agents {
			if a.System {
				continue
			}
			out.Agents = append(out.Agents, goalCandidate{ID: a.ID, Name: a.Name})
		}
	}
	if autos, err := r.db.ListAutomations(ctx); err == nil {
		for _, a := range autos {
			out.Automations = append(out.Automations, goalCandidate{ID: a.ID, Name: a.Name})
		}
	}
	if sessions, err := r.db.ListSessions(ctx, ""); err == nil {
		seen := map[string]bool{}
		for _, s := range sessions {
			for _, t := range s.Tags {
				if t = strings.TrimSpace(t); t != "" && !seen[t] {
					seen[t] = true
					out.Tags = append(out.Tags, t)
				}
			}
		}
		sort.Strings(out.Tags)
	}
	return out
}

// goalWriterUserPrompt renders the intake request: the statement, the catalog,
// the candidates, the existing goals and (for a rewrite) the base goal.
func goalWriterUserPrompt(rawText string, base *db.Goal, existing []db.Goal, cands GoalScopeCandidates, lang string) string {
	var b strings.Builder
	if lang != "" {
		fmt.Fprintf(&b, "Reply language for name/summary/description/rubric/questions/notes: %s.\n\n", lang)
	}
	b.WriteString("## User statement (verbatim)\n\n")
	b.WriteString(rawText)
	b.WriteString("\n\n")
	if base != nil {
		b.WriteString("## Existing goal this statement refines\n\n")
		if js, err := json.MarshalIndent(goalForPrompt(*base), "", "  "); err == nil {
			b.Write(js)
		}
		b.WriteString("\n\nKeep what the statement does not change; the user is refining, not restarting.\n\n")
	}
	b.WriteString("## Metric catalog (the only allowed metric keys)\n\n")
	for _, m := range goals.Catalog() {
		fmt.Fprintf(&b, "- `%s` — %s (%s; default %s; source %s", m.Key, m.Label, m.Unit, m.DefaultDirection, m.Source)
		if len(m.Scopes) > 0 {
			fmt.Fprintf(&b, "; scopes %s", strings.Join(m.Scopes, ","))
		}
		if m.Hint != "" {
			fmt.Fprintf(&b, ") — %s\n", m.Hint)
		} else {
			b.WriteString(")\n")
		}
	}
	b.WriteString("\n## Scope candidates\n\n")
	writeCands := func(title string, list []goalCandidate) {
		fmt.Fprintf(&b, "%s: ", title)
		if len(list) == 0 {
			b.WriteString("(none)\n")
			return
		}
		for i, c := range list {
			if i >= goalWriterMaxItems {
				b.WriteString("…")
				break
			}
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s (%s)", c.ID, c.Name)
		}
		b.WriteString("\n")
	}
	writeCands("Recipes (slug)", cands.Recipes)
	writeCands("Agents (id)", cands.Agents)
	writeCands("Automations (id)", cands.Automations)
	b.WriteString("Tags: ")
	if len(cands.Tags) == 0 {
		b.WriteString("(none)\n")
	} else {
		tags := cands.Tags
		if len(tags) > goalWriterMaxItems {
			tags = tags[:goalWriterMaxItems]
		}
		b.WriteString(strings.Join(tags, ", "))
		b.WriteString("\n")
	}
	b.WriteString("\n## Existing goals\n\n")
	n := 0
	for _, g := range existing {
		if base != nil && g.ID == base.ID {
			continue
		}
		if g.Status == db.GoalStatusArchived {
			continue
		}
		if n >= goalWriterMaxGoals {
			b.WriteString("…\n")
			break
		}
		n++
		fmt.Fprintf(&b, "- %s [%s] %s — primary %s (%s)", g.ID, g.Status, g.Name, g.Primary.Metric, g.Primary.Direction)
		if len(g.Scope.Recipes)+len(g.Scope.Agents)+len(g.Scope.Automations)+len(g.Scope.Tags) > 0 {
			fmt.Fprintf(&b, "; scope recipes=%v agents=%v automations=%v tags=%v", g.Scope.Recipes, g.Scope.Agents, g.Scope.Automations, g.Scope.Tags)
		}
		b.WriteString("\n")
	}
	if n == 0 {
		b.WriteString("(none)\n")
	}
	b.WriteString("\nWrite the goal draft as ONE JSON object. Policy mode must be \"propose\".\n")
	return b.String()
}

// goalForPrompt strips history and provenance before a goal is shown to the
// writer (a map, so the always-present keys are really absent).
func goalForPrompt(g db.Goal) map[string]any {
	js, err := json.Marshal(g)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(js, &m) != nil {
		return nil
	}
	for _, k := range []string{"history", "createdBy", "createdAt", "updatedAt"} {
		delete(m, k)
	}
	return m
}
