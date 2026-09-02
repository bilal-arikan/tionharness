package skills

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Recipe editing (Rota F4-v2). ApplyRecipeProposal rewrites ONLY the
// structured part of a coordinator recipe — the `phases:` block, the
// recipe-wide `watchers:` and the `version:` — from a parsed spec, leaving
// every other frontmatter line and the prose body byte-for-byte as they were.
// The result is re-parsed before it is written: an edit that would not load
// is refused, never half-written.
//
// Actions are the optimizer's pruning / change class; additions that need a
// human (split / merge / rollback) are refused here and stay proposals.

// Recipe actions ApplyRecipeProposal accepts.
const (
	RecipeActionPruneWatcher  = "prune_watcher"
	RecipeActionPrunePhase    = "prune_phase"
	RecipeActionMakeOptional  = "make_optional"
	RecipeActionChangeProfile = "change_profile"
	RecipeActionAddGate       = "add_gate"
	RecipeActionBindWatcher   = "bind_watcher"
)

// PruneActions is the class a recipe may opt into applying automatically
// (`auto_prune: true` in its frontmatter).
var PruneActions = map[string]bool{
	RecipeActionPruneWatcher: true, RecipeActionPrunePhase: true, RecipeActionMakeOptional: true,
}

// ApplyRecipeProposal applies one action to the recipe file and returns the
// new version. value is action-specific: the new profile for change_profile,
// "kind value" for add_gate, the watcher name for bind_watcher (target = the
// phase, "" = recipe-wide).
func ApplyRecipeProposal(store *Store, slug, action, target, value string) (newVersion string, err error) {
	if store == nil {
		return "", fmt.Errorf("skills store unavailable")
	}
	sk, ok := store.Get(slug)
	if !ok {
		return "", fmt.Errorf("skill %q not found", slug)
	}
	if !sk.IsCoordinatorWorkflow() {
		return "", fmt.Errorf("skill %q is not a coordinator-workflow", slug)
	}
	data, err := os.ReadFile(sk.Path)
	if err != nil {
		return "", fmt.Errorf("read skill %q: %w", slug, err)
	}
	fmText, body := splitFrontmatter(string(data))
	if fmText == "" {
		return "", fmt.Errorf("skill %q has no frontmatter", slug)
	}
	fm, _ := parseFrontmatter(string(data))
	spec, perr := parseRecipeSpec(fm, fm.scalar("version"))
	if perr != nil {
		return "", fmt.Errorf("recipe %q is invalid, refusing to edit: %w", slug, perr)
	}
	if spec == nil {
		spec = &RecipeSpec{}
	}
	if err := applyRecipeAction(spec, action, target, value); err != nil {
		return "", err
	}
	newVersion = bumpVersion(fm.scalar("version"))
	spec.Version = newVersion
	if err := spec.Validate(); err != nil {
		return "", fmt.Errorf("edited recipe would not load: %w", err)
	}
	newFM := rewriteRecipeFrontmatter(fmText, spec, newVersion)
	content := "---\n" + strings.TrimRight(newFM, "\n") + "\n---\n" + body
	// Round-trip: what we wrote must parse to what we meant.
	check, _ := parseFrontmatter(content)
	if _, cerr := parseRecipeSpec(check, check.scalar("version")); cerr != nil {
		return "", fmt.Errorf("edited recipe does not round-trip: %w", cerr)
	}
	if err := os.WriteFile(sk.Path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write skill %q: %w", slug, err)
	}
	store.Reload()
	return newVersion, nil
}

// applyRecipeAction mutates the spec.
func applyRecipeAction(spec *RecipeSpec, action, target, value string) error {
	target = strings.TrimSpace(target)
	value = strings.TrimSpace(value)
	phaseIdx := func(id string) int {
		for i, p := range spec.Phases {
			if strings.EqualFold(p.ID, id) {
				return i
			}
		}
		return -1
	}
	switch action {
	case RecipeActionPruneWatcher:
		if target == "" {
			return fmt.Errorf("prune_watcher needs the watcher name")
		}
		found := false
		spec.Watchers, found = removeString(spec.Watchers, target, found)
		for i := range spec.Phases {
			spec.Phases[i].Watchers, found = removeString(spec.Phases[i].Watchers, target, found)
		}
		if !found {
			return fmt.Errorf("watcher %q is not bound in the recipe", target)
		}
	case RecipeActionPrunePhase:
		i := phaseIdx(target)
		if i < 0 {
			return fmt.Errorf("phase %q is not declared", target)
		}
		spec.Phases = append(spec.Phases[:i], spec.Phases[i+1:]...)
		if len(spec.Phases) == 0 {
			return fmt.Errorf("refusing to remove the last phase")
		}
	case RecipeActionMakeOptional:
		i := phaseIdx(target)
		if i < 0 {
			return fmt.Errorf("phase %q is not declared", target)
		}
		spec.Phases[i].Optional = true
	case RecipeActionChangeProfile:
		i := phaseIdx(target)
		if i < 0 {
			return fmt.Errorf("phase %q is not declared", target)
		}
		if value == "" {
			return fmt.Errorf("change_profile needs the new profile")
		}
		spec.Phases[i].Profile = value
	case RecipeActionAddGate:
		i := phaseIdx(target)
		if i < 0 {
			return fmt.Errorf("phase %q is not declared", target)
		}
		kind, gv, _ := strings.Cut(value, " ")
		spec.Phases[i].Gate = &GateSpec{Kind: strings.TrimSpace(kind), Value: strings.TrimSpace(gv)}
	case RecipeActionBindWatcher:
		if value == "" {
			return fmt.Errorf("bind_watcher needs the watcher name")
		}
		if target == "" {
			spec.Watchers = append(spec.Watchers, value)
		} else {
			i := phaseIdx(target)
			if i < 0 {
				return fmt.Errorf("phase %q is not declared", target)
			}
			spec.Phases[i].Watchers = append(spec.Phases[i].Watchers, value)
		}
	default:
		return fmt.Errorf("action %q cannot be applied automatically", action)
	}
	return nil
}

func removeString(list []string, s string, found bool) ([]string, bool) {
	out := list[:0:0]
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), s) {
			found = true
			continue
		}
		out = append(out, x)
	}
	return out, found
}

// bumpVersion increments a numeric version, else starts a numbered line.
func bumpVersion(v string) string {
	v = strings.TrimSpace(v)
	if n, err := strconv.Atoi(v); err == nil {
		return strconv.Itoa(n + 1)
	}
	if v == "" {
		return "2"
	}
	return v + ".1"
}

// rewriteRecipeFrontmatter drops the old phases / watchers / version /
// optimizer lines and appends the rendered ones, keeping everything else.
func rewriteRecipeFrontmatter(fmText string, spec *RecipeSpec, version string) string {
	lines := strings.Split(strings.ReplaceAll(fmText, "\r\n", "\n"), "\n")
	var kept []string
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		trimmed := strings.TrimSpace(ln)
		key := ""
		if colon := strings.Index(ln, ":"); colon >= 0 && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "#") {
			key = strings.ToLower(strings.TrimSpace(ln[:colon]))
		}
		switch key {
		case "phases", "watchers", "version", "optimizer":
			val := strings.TrimSpace(ln[strings.Index(ln, ":")+1:])
			if val == "" {
				// Block form: skip its indented lines too.
				if _, _, consumed := collectRawBlock(lines[i+1:]); consumed > 0 {
					i += consumed
				}
			}
			continue
		}
		kept = append(kept, ln)
	}
	out := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	out += "\nversion: " + version
	if len(spec.Phases) > 0 {
		out += "\n" + renderPhaseBlock(spec.Phases)
	}
	if len(spec.Watchers) > 0 {
		out += "\nwatchers: " + inlineArray(spec.Watchers)
	}
	if strings.TrimSpace(spec.Optimizer) != "" {
		out += "\noptimizer: " + spec.Optimizer
	}
	return out
}

// renderPhaseBlock emits the `phases:` block in the form the parser reads.
func renderPhaseBlock(phases []PhaseSpec) string {
	var b strings.Builder
	b.WriteString("phases:")
	for _, p := range phases {
		fmt.Fprintf(&b, "\n  - id: %s", p.ID)
		if p.Label != "" {
			fmt.Fprintf(&b, "\n    label: %s", quoteIfNeeded(p.Label))
		}
		if p.Profile != "" {
			fmt.Fprintf(&b, "\n    profile: %s", p.Profile)
		}
		if p.Gate != nil {
			if p.Gate.Value != "" {
				fmt.Fprintf(&b, "\n    gate: { kind: %s, value: %s }", p.Gate.Kind, quoteIfNeeded(p.Gate.Value))
			} else {
				fmt.Fprintf(&b, "\n    gate: { kind: %s }", p.Gate.Kind)
			}
		}
		if len(p.Watchers) > 0 {
			fmt.Fprintf(&b, "\n    watchers: %s", inlineArray(p.Watchers))
		}
		if p.MaxRounds > 0 {
			fmt.Fprintf(&b, "\n    max_rounds: %d", p.MaxRounds)
		}
		if p.Optional {
			b.WriteString("\n    optional: true")
		}
	}
	return b.String()
}

func inlineArray(items []string) string {
	q := make([]string, 0, len(items))
	for _, it := range items {
		q = append(q, quoteIfNeeded(strings.TrimSpace(it)))
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// quoteIfNeeded wraps a value in double quotes when it carries characters the
// line parser would misread (colons, commas, brackets, leading/trailing space).
func quoteIfNeeded(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":,[]{}#\"") || s != strings.TrimSpace(s) {
		return strconv.Quote(s)
	}
	return s
}
