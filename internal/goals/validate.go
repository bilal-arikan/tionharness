package goals

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// ErrInvalid wraps every validation failure so callers can map it to 400.
var ErrInvalid = errors.New("goal invalid")

// MaxNameLen / MaxTextLen cap the free-text fields so a goal can never itself
// become the bloat it is meant to guard against.
const (
	MaxNameLen = 120
	MaxTextLen = 4000
	MaxLists   = 32
)

// Normalize trims and defaults the fields of g in place. It is applied before
// Validate on every write path (writer draft and user edit alike).
func Normalize(g *db.Goal) {
	g.Name = strings.TrimSpace(g.Name)
	g.Summary = strings.TrimSpace(g.Summary)
	g.Description = strings.TrimSpace(g.Description)
	g.Rubric = strings.TrimSpace(g.Rubric)
	g.Notes = strings.TrimSpace(g.Notes)
	g.Status = strings.ToLower(strings.TrimSpace(g.Status))
	if g.Status == "" {
		g.Status = db.GoalStatusDraft
	}
	g.Kind = strings.ToLower(strings.TrimSpace(g.Kind))
	g.Primary.Metric = strings.TrimSpace(g.Primary.Metric)
	g.Primary.Direction = strings.ToLower(strings.TrimSpace(g.Primary.Direction))
	if g.Primary.Direction == "" {
		if m, ok := Lookup(g.Primary.Metric); ok {
			g.Primary.Direction = m.DefaultDirection
		}
	}
	if g.Kind == "" {
		switch {
		case g.Primary.Metric == "judge.rubricScore" && g.Rubric != "":
			g.Kind = db.GoalKindRubric
		case g.Rubric != "":
			g.Kind = db.GoalKindMixed
		default:
			g.Kind = db.GoalKindMetric
		}
	}
	g.Policy.Mode = strings.ToLower(strings.TrimSpace(g.Policy.Mode))
	if g.Policy.Mode == "" {
		g.Policy.Mode = db.GoalModePropose
	}
	g.Scope.Recipes = cleanList(g.Scope.Recipes)
	g.Scope.Agents = cleanList(g.Scope.Agents)
	g.Scope.Automations = cleanList(g.Scope.Automations)
	g.Scope.Tags = cleanList(g.Scope.Tags)
	g.Policy.AutoApplySurfaces = cleanList(g.Policy.AutoApplySurfaces)
	g.Questions = cleanList(g.Questions)
	if g.Guardrails == nil {
		g.Guardrails = []db.GoalGuardrail{}
	}
	for i := range g.Guardrails {
		g.Guardrails[i].Metric = strings.TrimSpace(g.Guardrails[i].Metric)
	}
}

// Validate reports the first rule g breaks. Rules are enforced here, not in
// the writer prompt: an unknown metric, a wrong direction, an unsafe policy or
// an over-long text is refused whoever wrote it.
func Validate(g db.Goal) error {
	fail := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
	}
	if g.Name == "" {
		return fail("name is required")
	}
	if len(g.Name) > MaxNameLen {
		return fail("name longer than %d characters", MaxNameLen)
	}
	for _, f := range []struct{ name, v string }{{"summary", g.Summary}, {"description", g.Description}, {"rubric", g.Rubric}, {"notes", g.Notes}, {"rawText", g.RawText}} {
		if len(f.v) > MaxTextLen {
			return fail("%s longer than %d characters", f.name, MaxTextLen)
		}
	}
	switch g.Status {
	case db.GoalStatusDraft, db.GoalStatusActive, db.GoalStatusPaused, db.GoalStatusArchived:
	default:
		return fail("unknown status %q", g.Status)
	}
	switch g.Kind {
	case db.GoalKindMetric, db.GoalKindRubric, db.GoalKindMixed:
	default:
		return fail("unknown kind %q", g.Kind)
	}
	if g.Priority < 0 || g.Priority > 5 {
		return fail("priority must be 1..5 (0 = unset)")
	}
	if g.Primary.Metric == "" {
		return fail("primary metric is required")
	}
	if _, ok := Lookup(g.Primary.Metric); !ok {
		return fail("unknown primary metric %q", g.Primary.Metric)
	}
	if g.Primary.Direction != db.GoalDirectionMin && g.Primary.Direction != db.GoalDirectionMax {
		return fail("primary direction must be min or max")
	}
	if g.Primary.Metric == "judge.rubricScore" && g.Rubric == "" {
		return fail("a judge.rubricScore goal needs a rubric")
	}
	if len(g.Guardrails) > MaxLists {
		return fail("too many guardrails")
	}
	seen := map[string]bool{}
	for i, gr := range g.Guardrails {
		if gr.Metric == "" {
			return fail("guardrail %d: metric is required", i+1)
		}
		if _, ok := Lookup(gr.Metric); !ok {
			return fail("guardrail %d: unknown metric %q", i+1, gr.Metric)
		}
		if gr.Metric == g.Primary.Metric {
			return fail("guardrail %d: %s is already the primary metric", i+1, gr.Metric)
		}
		if seen[gr.Metric] {
			return fail("guardrail %d: %s listed twice", i+1, gr.Metric)
		}
		seen[gr.Metric] = true
		if gr.Min == nil && gr.Max == nil {
			return fail("guardrail %d: needs a min or a max", i+1)
		}
		if gr.Min != nil && gr.Max != nil && *gr.Min > *gr.Max {
			return fail("guardrail %d: min above max", i+1)
		}
	}
	switch g.Policy.Mode {
	case db.GoalModePropose, db.GoalModeOff:
	case db.GoalModeAuto:
		if len(g.Policy.AutoApplySurfaces) == 0 {
			return fail("auto mode needs at least one auto-apply surface")
		}
	default:
		return fail("unknown policy mode %q", g.Policy.Mode)
	}
	for _, s := range g.Policy.AutoApplySurfaces {
		if !IsAutoApplySurface(s) {
			return fail("surface %q may not be auto-applied", s)
		}
	}
	if g.Policy.CooldownHours < 0 || g.Policy.MinRuns < 0 {
		return fail("cooldown and minRuns must not be negative")
	}
	for _, l := range [][]string{g.Scope.Recipes, g.Scope.Agents, g.Scope.Automations, g.Scope.Tags, g.Questions} {
		if len(l) > MaxLists {
			return fail("a list has more than %d entries", MaxLists)
		}
	}
	return nil
}

// CanActivate reports whether a draft is complete enough to become active:
// validation passes and no clarifying question is still open.
func CanActivate(g db.Goal) error {
	if err := Validate(g); err != nil {
		return err
	}
	if len(g.Questions) > 0 {
		return fmt.Errorf("%w: %d open question(s) — answer or clear them first", ErrInvalid, len(g.Questions))
	}
	return nil
}

func cleanList(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
