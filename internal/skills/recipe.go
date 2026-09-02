package skills

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Coordinator recipe schema (_Docs/77 R6).
//
// A coordinator-workflow skill has always been prose: the coordinator reads the
// body and follows it. The Rota work needs the same recipe to also carry a
// STRUCTURED plan the runtime can seed a trajectory from — the phases the work
// is expected to pass through, which worker profile each phase wants, what
// gate closes it, which automations watch it, and which optimizer runs at the
// end. That structure lives in the frontmatter next to the prose:
//
//	version: 3
//	phases:
//	  - id: plan
//	    profile: planner
//	    gate: { kind: artifact, value: plan }
//	  - id: code
//	    profile: coder
//	    watchers: [summarize-board]
//	  - id: review
//	    profile: validator
//	    gate: { kind: verdict, value: "VERDICT: PASS" }
//	    max_rounds: 2
//	  - id: ship
//	    optional: true
//	watchers: [update-docs]
//	optimizer: recipe-optimizer
//
// The block is parsed by the dependency-free frontmatter parser below and
// validated at load. An INVALID block never drops the skill: the recipe still
// loads and works as prose, Skill.RecipeError says what is wrong, and the UI
// shows it — but no trajectory is seeded from it.

// RecipeSpec is the structured part of a coordinator recipe.
type RecipeSpec struct {
	// Version is the recipe's own version (frontmatter `version`), carried in
	// RecipeRef so a trajectory records which revision it was seeded from.
	Version string      `json:"version,omitempty"`
	Phases  []PhaseSpec `json:"phases"`
	// Watchers are automation ids/names that fire when the whole trajectory ends.
	Watchers []string `json:"watchers,omitempty"`
	// Optimizer names the system agent (or automation) to run at trajectory end.
	Optimizer string `json:"optimizer,omitempty"`
}

// PhaseSpec is one declared phase.
type PhaseSpec struct {
	ID      string    `json:"id"`
	Label   string    `json:"label,omitempty"`
	Profile string    `json:"profile,omitempty"` // expected worker profile (planner, coder, validator, …)
	Gate    *GateSpec `json:"gate,omitempty"`
	// Watchers are automation ids/names that fire when this phase exits.
	Watchers  []string `json:"watchers,omitempty"`
	MaxRounds int      `json:"maxRounds,omitempty"` // bounded repair loops (0 = unbounded / n.a.)
	Optional  bool     `json:"optional,omitempty"`
}

// GateSpec is a phase's exit condition.
type GateSpec struct {
	Kind  string `json:"kind"` // artifact | verdict | human | schema
	Value string `json:"value,omitempty"`
}

// GateKinds are the accepted gate kinds.
var GateKinds = []string{"artifact", "verdict", "human", "schema"}

var phaseIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidPhaseID reports whether id is an acceptable phase id (the same rule the
// recipe parser applies), so an agent-planned trajectory follows the recipe
// convention.
func ValidPhaseID(id string) bool { return phaseIDRe.MatchString(id) }

// RecipeRef renders "slug@version" (or just the slug when the recipe has no
// version) — the value Session.CoordinatorWorkflow / Trajectory.TemplateRef
// carry so a run records the exact recipe revision it followed.
func RecipeRef(slug, version string) string {
	slug = strings.TrimSpace(slug)
	version = strings.TrimSpace(version)
	if slug == "" || version == "" {
		return slug
	}
	return slug + "@" + version
}

// ParseRecipeRef splits "slug@version" into its parts; a bare slug has an
// empty version. Readers accept both spellings (sessions written before R6
// carry bare slugs).
func ParseRecipeRef(ref string) (slug, version string) {
	ref = strings.TrimSpace(ref)
	if i := strings.LastIndex(ref, "@"); i > 0 {
		return ref[:i], ref[i+1:]
	}
	return ref, ""
}

// RecipeRefFor returns the ref a session should be stamped with for slug: the
// versioned form when the recipe declares a version, the bare slug otherwise
// (and for an unknown slug, so a bad selection still fails at resolve time).
func RecipeRefFor(store *Store, slug string) string {
	slug, _ = ParseRecipeRef(slug)
	if store == nil || slug == "" {
		return slug
	}
	if sk, ok := store.Get(slug); ok {
		return RecipeRef(slug, sk.Version)
	}
	return slug
}

// parseRecipeSpec builds the RecipeSpec from a coordinator-workflow skill's
// frontmatter. ok=false means the frontmatter carries no structured plan at all
// (a prose-only recipe — not an error). An error means a block IS present but
// invalid; the caller records it on the skill.
func parseRecipeSpec(fm frontmatter, version string) (spec *RecipeSpec, err error) {
	block, hasPhases := fm.blocks["phases"]
	watchers := fm.list("watchers")
	optimizer := strings.TrimSpace(fm.scalar("optimizer"))
	if !hasPhases && len(watchers) == 0 && optimizer == "" {
		return nil, nil
	}
	spec = &RecipeSpec{Version: strings.TrimSpace(version), Watchers: watchers, Optimizer: optimizer, Phases: []PhaseSpec{}}
	if hasPhases {
		phases, perr := parsePhaseBlock(block)
		if perr != nil {
			return nil, perr
		}
		spec.Phases = phases
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return spec, nil
}

// Validate checks the structural invariants: at least one phase when a phases
// block exists, unique slug-like ids, known gate kinds, non-negative rounds.
func (s *RecipeSpec) Validate() error {
	if s == nil {
		return nil
	}
	seen := map[string]bool{}
	for i, p := range s.Phases {
		if !phaseIDRe.MatchString(p.ID) {
			return fmt.Errorf("phases[%d]: id %q must be lowercase [a-z0-9_-], 1..64 chars", i, p.ID)
		}
		if seen[p.ID] {
			return fmt.Errorf("phases[%d]: duplicate id %q", i, p.ID)
		}
		seen[p.ID] = true
		if p.MaxRounds < 0 {
			return fmt.Errorf("phase %q: max_rounds must be >= 0", p.ID)
		}
		if p.Gate != nil {
			known := false
			for _, k := range GateKinds {
				if p.Gate.Kind == k {
					known = true
				}
			}
			if !known {
				return fmt.Errorf("phase %q: gate kind %q must be one of %s", p.ID, p.Gate.Kind, strings.Join(GateKinds, "|"))
			}
		}
	}
	return nil
}

// parsePhaseBlock parses the raw indented `phases:` block: a list whose items
// are small maps. Supported value forms per key: scalar, inline array
// `[a, b]`, inline map `{ k: v, k2: v2 }`. Anything else is an error rather
// than a silent partial read — a recipe author must be able to trust that what
// loaded is what they wrote.
func parsePhaseBlock(block string) ([]PhaseSpec, error) {
	lines := strings.Split(block, "\n")
	var phases []PhaseSpec
	var cur map[string]string
	var curOrder []string
	flush := func() error {
		if cur == nil {
			return nil
		}
		p, err := phaseFromMap(cur)
		if err != nil {
			return err
		}
		phases = append(phases, p)
		cur, curOrder = nil, nil
		return nil
	}
	for n, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			if err := flush(); err != nil {
				return nil, err
			}
			cur = map[string]string{}
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if trimmed == "" {
				continue
			}
		}
		if cur == nil {
			return nil, fmt.Errorf("phases line %d: expected a list item (\"- id: …\"), got %q", n+1, trimmed)
		}
		key, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, fmt.Errorf("phases line %d: expected key: value, got %q", n+1, trimmed)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		cur[key] = strings.TrimSpace(val)
		curOrder = append(curOrder, key)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	_ = curOrder
	if len(phases) == 0 {
		return nil, fmt.Errorf("phases: block is present but lists no phases")
	}
	return phases, nil
}

// phaseFromMap converts one parsed item into a PhaseSpec.
func phaseFromMap(m map[string]string) (PhaseSpec, error) {
	p := PhaseSpec{
		ID:      strings.ToLower(unquote(m["id"])),
		Label:   unquote(m["label"]),
		Profile: unquote(m["profile"]),
	}
	if v, ok := m["watchers"]; ok {
		if !strings.HasPrefix(v, "[") {
			return p, fmt.Errorf("phase %q: watchers must be an inline array [a, b]", p.ID)
		}
		p.Watchers = parseInlineArray(v)
	}
	if v, ok := m["max_rounds"]; ok {
		n, err := strconv.Atoi(unquote(v))
		if err != nil {
			return p, fmt.Errorf("phase %q: max_rounds %q is not an integer", p.ID, v)
		}
		p.MaxRounds = n
	}
	if v, ok := m["optional"]; ok {
		p.Optional = strings.EqualFold(unquote(v), "true")
	}
	if v, ok := m["gate"]; ok {
		g, err := parseGate(v)
		if err != nil {
			return p, fmt.Errorf("phase %q: %w", p.ID, err)
		}
		p.Gate = g
	}
	return p, nil
}

// parseGate accepts `{ kind: verdict, value: "VERDICT: PASS" }` or a bare kind.
func parseGate(v string) (*GateSpec, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("gate must not be empty")
	}
	if !strings.HasPrefix(v, "{") {
		return &GateSpec{Kind: strings.ToLower(unquote(v))}, nil
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(v, "{"), "}")
	g := &GateSpec{}
	for _, part := range splitTopLevel(inner, ',') {
		k, val, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("gate: expected key: value in %q", part)
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "kind":
			g.Kind = strings.ToLower(unquote(strings.TrimSpace(val)))
		case "value":
			g.Value = unquote(strings.TrimSpace(val))
		default:
			return nil, fmt.Errorf("gate: unknown key %q", strings.TrimSpace(k))
		}
	}
	if g.Kind == "" {
		return nil, fmt.Errorf("gate: kind is required")
	}
	return g, nil
}

// splitTopLevel splits s on sep outside single/double quotes.
func splitTopLevel(s string, sep rune) []string {
	var out []string
	var b strings.Builder
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			b.WriteRune(r)
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
			b.WriteRune(r)
		case r == sep:
			out = append(out, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if strings.TrimSpace(b.String()) != "" {
		out = append(out, b.String())
	}
	return out
}
