package db

import (
	"context"
	"strings"
)

// Goal store: one JSON file per goal under goals/, loaded into d.goals at
// Open() like every other entity. Writes go through the shared d.mu.

func (d *DB) persistGoalLocked(g Goal) error {
	return dbPersistLocked(d, d.goals, dirGoals, g.ID, g)
}

// normalizeGoal fills the defaults every stored goal carries so readers never
// see nil slices or an empty status.
func normalizeGoal(g *Goal) {
	if g.Status == "" {
		g.Status = GoalStatusDraft
	}
	if g.Policy.Mode == "" {
		g.Policy.Mode = GoalModePropose
	}
	if g.Guardrails == nil {
		g.Guardrails = []GoalGuardrail{}
	}
	if g.History == nil {
		g.History = []GoalRevision{}
	}
}

// CreateGoal inserts a goal and returns the stored row. The first revision
// records who wrote it.
func (d *DB) CreateGoal(ctx context.Context, g Goal, by, note string) (Goal, error) {
	g.ID = d.nextID(idGoal)
	g.CreatedAt = now()
	g.UpdatedAt = g.CreatedAt
	if g.CreatedBy == "" {
		g.CreatedBy = by
	}
	normalizeGoal(&g)
	g.History = append(g.History, GoalRevision{At: g.CreatedAt, By: by, Note: note, Fields: []string{"created"}})
	d.mu.Lock()
	defer d.mu.Unlock()
	return g, d.persistGoalLocked(g)
}

// GetGoal loads a goal by id.
func (d *DB) GetGoal(ctx context.Context, id string) (Goal, error) {
	return dbGet(d, d.goals, id)
}

// ListGoals returns every goal, active first, then by priority, then newest.
func (d *DB) ListGoals(ctx context.Context) ([]Goal, error) {
	return dbList(d, d.goals, func(a, b Goal) bool {
		if ra, rb := goalStatusRank(a.Status), goalStatusRank(b.Status); ra != rb {
			return ra < rb
		}
		if pa, pb := goalPriority(a.Priority), goalPriority(b.Priority); pa != pb {
			return pa < pb
		}
		return a.CreatedAt > b.CreatedAt
	}), nil
}

// ListGoalsByStatus returns the goals in one status, same order as ListGoals.
func (d *DB) ListGoalsByStatus(ctx context.Context, status string) ([]Goal, error) {
	all, _ := d.ListGoals(ctx)
	out := make([]Goal, 0, len(all))
	for _, g := range all {
		if g.Status == status {
			out = append(out, g)
		}
	}
	return out, nil
}

// UpdateGoal replaces the editable fields of a goal and appends a revision.
// Id, CreatedAt, CreatedBy, RawText and History are preserved from the stored
// row (RawText is the user's original words and never rewritten by an edit;
// the writer's rewrite path sets it explicitly through ReplaceGoalRawText).
func (d *DB) UpdateGoal(ctx context.Context, g Goal, by, note string) (Goal, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cur, ok := d.goals[g.ID]
	if !ok {
		return Goal{}, ErrNotFound
	}
	fields := goalChangedFields(cur, g)
	g.CreatedAt = cur.CreatedAt
	g.CreatedBy = cur.CreatedBy
	g.RawText = cur.RawText
	g.History = cur.History
	g.UpdatedAt = now()
	normalizeGoal(&g)
	if len(fields) > 0 || note != "" {
		g.History = append(g.History, GoalRevision{At: g.UpdatedAt, By: by, Note: note, Fields: fields})
	}
	return g, d.persistGoalLocked(g)
}

// ReplaceGoalRawText records a fresh statement of the goal in the user's words
// (a re-intake on an existing goal) alongside the writer's rewrite.
func (d *DB) ReplaceGoalRawText(ctx context.Context, id, rawText string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	g, ok := d.goals[id]
	if !ok {
		return ErrNotFound
	}
	g.RawText = rawText
	return d.persistGoalLocked(g)
}

// SetGoalStatus moves a goal between draft/active/paused/archived.
func (d *DB) SetGoalStatus(ctx context.Context, id, status, by string) (Goal, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	g, ok := d.goals[id]
	if !ok {
		return Goal{}, ErrNotFound
	}
	if g.Status == status {
		return g, nil
	}
	prev := g.Status
	g.Status = status
	g.UpdatedAt = now()
	normalizeGoal(&g)
	g.History = append(g.History, GoalRevision{At: g.UpdatedAt, By: by, Note: prev + " → " + status, Fields: []string{"status"}})
	return g, d.persistGoalLocked(g)
}

// DeleteGoal removes a goal permanently (archiving is the reversible path).
func (d *DB) DeleteGoal(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return dbDeleteLocked(d, d.goals, dirGoals, id)
}

func goalStatusRank(s string) int {
	switch s {
	case GoalStatusActive:
		return 0
	case GoalStatusDraft:
		return 1
	case GoalStatusPaused:
		return 2
	case GoalStatusArchived:
		return 3
	}
	return 4
}

func goalPriority(p int) int {
	if p <= 0 {
		return 3
	}
	return p
}

// goalChangedFields names the editable fields that differ between two goals,
// for the revision log.
func goalChangedFields(a, b Goal) []string {
	var out []string
	add := func(name string, changed bool) {
		if changed {
			out = append(out, name)
		}
	}
	add("name", a.Name != b.Name)
	add("summary", a.Summary != b.Summary)
	add("description", a.Description != b.Description)
	add("status", a.Status != b.Status)
	add("kind", a.Kind != b.Kind)
	add("priority", a.Priority != b.Priority)
	add("scope", !stringSliceEq(a.Scope.Recipes, b.Scope.Recipes) || !stringSliceEq(a.Scope.Agents, b.Scope.Agents) ||
		!stringSliceEq(a.Scope.Automations, b.Scope.Automations) || !stringSliceEq(a.Scope.Tags, b.Scope.Tags))
	add("primary", a.Primary.Metric != b.Primary.Metric || a.Primary.Direction != b.Primary.Direction || !floatPtrEq(a.Primary.Target, b.Primary.Target))
	add("guardrails", !guardrailsEq(a.Guardrails, b.Guardrails))
	add("rubric", a.Rubric != b.Rubric)
	add("policy", a.Policy.Mode != b.Policy.Mode || a.Policy.CooldownHours != b.Policy.CooldownHours || a.Policy.MinRuns != b.Policy.MinRuns ||
		!stringSliceEq(a.Policy.AutoApplySurfaces, b.Policy.AutoApplySurfaces))
	add("questions", !stringSliceEq(a.Questions, b.Questions))
	add("notes", a.Notes != b.Notes)
	return out
}

func stringSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != strings.TrimSpace(b[i]) {
			return false
		}
	}
	return true
}

func floatPtrEq(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func guardrailsEq(a, b []GoalGuardrail) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Metric != b[i].Metric || !floatPtrEq(a[i].Min, b[i].Min) || !floatPtrEq(a[i].Max, b[i].Max) {
			return false
		}
	}
	return true
}
