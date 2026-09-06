package goals

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
)

// Proposal rules (_Docs/83 §4.4, E2). The workspace evolver may only touch a
// closed list of (surface, field) pairs; everything else — permission mode,
// inbound policy, hooks, the goals themselves, their guardrails and rubric,
// locked system agents, the evaluator — is invisible to it. The rules live in
// code so a prompt drift can never widen the reach.

// ProposalRule describes one editable field.
type ProposalRule struct {
	Surface string
	Field   string
	Actions []string // allowed actions
	// MaxValueLen caps free-text values (0 = no text expected / not capped).
	MaxValueLen int
	// Additive marks actions that grow a budgeted list (skills, tools);
	// additions over the budget must name a removal.
	Additive bool
	Hint     string // shown to the model
}

// Proposal surfaces and actions.
const (
	SurfaceAgent      = "agent"
	SurfaceSkill      = "skill"
	SurfaceRecipe     = "recipe"
	SurfaceTools      = "tools"
	SurfaceAutomation = "automation"
	SurfaceSchedule   = "schedule"
	SurfacePrompt     = "prompt"
	SurfaceSettings   = "ws-settings"

	ActionSet    = "set"
	ActionAdd    = "add"
	ActionRemove = "remove"
	ActionPrune  = "prune"
	ActionSwap   = "swap"

	// Growth budgets (additions past these must name a removal).
	MaxSkillsPerAgent = 12
	MaxSoulChars      = 6000
	MaxPromptChars    = 4000

	// MaxProposalsPerPass caps what one evolver pass may file per goal.
	MaxProposalsPerPass = 3
	// DefaultMinRuns / DefaultCooldownHours apply when the goal's policy
	// leaves them at zero.
	DefaultMinRuns       = 5
	DefaultCooldownHours = 72
	// EscalateAfterRepeats is how many passes the same open proposal may
	// return before it is turned into a human escalation and stops recurring.
	EscalateAfterRepeats = 3
)

var proposalRules = []ProposalRule{
	{Surface: SurfaceAgent, Field: "soul", Actions: []string{ActionSet}, MaxValueLen: MaxSoulChars, Hint: "full replacement text of the agent's system prompt; keep it shorter than the current one unless the evidence demands otherwise"},
	{Surface: SurfaceAgent, Field: "identity", Actions: []string{ActionSet}, MaxValueLen: 2000},
	{Surface: SurfaceAgent, Field: "model", Actions: []string{ActionSet}, MaxValueLen: 80, Hint: "a model alias/id the agent's provider serves"},
	{Surface: SurfaceAgent, Field: "thinkingLevel", Actions: []string{ActionSet}, MaxValueLen: 16, Hint: "off|low|medium|high|xhigh|max|ultra"},
	{Surface: SurfaceAgent, Field: "nativeWebSearch", Actions: []string{ActionSet}, MaxValueLen: 8, Hint: "true|false"},
	{Surface: SurfaceAgent, Field: "tools", Actions: []string{ActionSet}, MaxValueLen: 160, Hint: "value \"<tool>=<full|summary|name-only|hidden|blocked>\""},
	{Surface: SurfaceAgent, Field: "skills", Actions: []string{ActionAdd, ActionRemove}, Additive: true, MaxValueLen: 120, Hint: "value = skill slug"},
	{Surface: SurfaceAgent, Field: "coordinatorWorkflow", Actions: []string{ActionSet}, MaxValueLen: 120, Hint: "recipe slug the coordinator follows"},
	{Surface: SurfaceAgent, Field: "coordinatorPrompt", Actions: []string{ActionSet}, MaxValueLen: MaxPromptChars},
	{Surface: SurfaceSkill, Field: "visibility", Actions: []string{ActionSet}, MaxValueLen: 16, Hint: "full|summary|name-only|hidden"},
	{Surface: SurfaceSkill, Field: "autoSummary", Actions: []string{ActionSet}, MaxValueLen: 8, Hint: "true|false"},
	{Surface: SurfaceSkill, Field: "body", Actions: []string{ActionSet}, MaxValueLen: MaxPromptChars, Hint: "a concrete rule/paragraph to add or replace in the skill body"},
	{Surface: SurfaceRecipe, Field: "phase", Actions: []string{ActionPrune, ActionSet}, MaxValueLen: 200, Hint: "prune a phase (value = phase id) or set its profile (value \"<phase>=<profile>\")"},
	{Surface: SurfaceRecipe, Field: "watcher", Actions: []string{ActionPrune}, MaxValueLen: 120, Hint: "value = watcher name"},
	{Surface: SurfaceTools, Field: "visibility", Actions: []string{ActionSet}, MaxValueLen: 160, Hint: "workspace-wide tier: value \"<tool>=<full|summary|name-only|hidden>\""},
	{Surface: SurfaceTools, Field: "disabled", Actions: []string{ActionAdd, ActionRemove}, MaxValueLen: 120, Hint: "value = tool name"},
	{Surface: SurfaceAutomation, Field: "cooldownSec", Actions: []string{ActionSet}, MaxValueLen: 16},
	{Surface: SurfaceAutomation, Field: "maxIterations", Actions: []string{ActionSet}, MaxValueLen: 16},
	{Surface: SurfaceAutomation, Field: "tokenThreshold", Actions: []string{ActionSet}, MaxValueLen: 16},
	{Surface: SurfaceAutomation, Field: "counterInterval", Actions: []string{ActionSet}, MaxValueLen: 16},
	{Surface: SurfaceAutomation, Field: "enabled", Actions: []string{ActionSet}, MaxValueLen: 8, Hint: "true|false"},
	{Surface: SurfaceAutomation, Field: "promptTemplate", Actions: []string{ActionSet}, MaxValueLen: MaxPromptChars},
	{Surface: SurfaceAutomation, Field: "targetAgentId", Actions: []string{ActionSet}, MaxValueLen: 40},
	{Surface: SurfaceSchedule, Field: "cronExpr", Actions: []string{ActionSet}, MaxValueLen: 64},
	{Surface: SurfaceSchedule, Field: "enabled", Actions: []string{ActionSet}, MaxValueLen: 8, Hint: "true|false"},
	{Surface: SurfacePrompt, Field: "override", Actions: []string{ActionSet}, MaxValueLen: MaxPromptChars, Hint: "entity = prompt registry key; value = full override text (must keep the key's {{placeholders}})"},
	{Surface: SurfaceSettings, Field: "terseMode", Actions: []string{ActionSet}, MaxValueLen: 8, Hint: "true|false"},
	{Surface: SurfaceSettings, Field: "instructions", Actions: []string{ActionSet}, MaxValueLen: MaxPromptChars, Hint: "workspace-wide agent instructions text"},
}

// ProposalRules returns the rule list (a copy) in display order.
func ProposalRules() []ProposalRule {
	out := make([]ProposalRule, len(proposalRules))
	copy(out, proposalRules)
	return out
}

// LookupRule finds the rule for a surface/field pair.
func LookupRule(surface, field string) (ProposalRule, bool) {
	for _, r := range proposalRules {
		if r.Surface == surface && r.Field == field {
			return r, true
		}
	}
	return ProposalRule{}, false
}

// RawProposal is the JSON shape the evolver answers with (one entry).
type RawProposal struct {
	Surface        string   `json:"surface"`
	EntityID       string   `json:"entityId"`
	Field          string   `json:"field"`
	Action         string   `json:"action"`
	Value          string   `json:"value"`
	Removes        string   `json:"removes"`
	Title          string   `json:"title"`
	Rationale      string   `json:"rationale"`
	Evidence       string   `json:"evidence"`
	ExpectedMetric string   `json:"expectedMetric"`
	ExpectedDelta  float64  `json:"expectedDelta"`
	SideEffects    []string `json:"sideEffects"`
	Severity       string   `json:"severity"`
}

// ProposalContext is what the checker knows about the workspace.
type ProposalContext struct {
	Goal db.Goal
	// InScope reports whether an entity of a surface is inside the goal's
	// scope (nil = everything is in scope).
	InScope func(surface, entityID string) bool
	// Exists reports whether an entity exists (nil = assume yes).
	Exists func(surface, entityID string) bool
	// SkillCount returns an agent's current skill count (for the budget).
	SkillCount func(agentID string) int
	// OtherGoals are the other ACTIVE goals; a side effect on one of their
	// metrics turns the proposal into a conflict instead of a change.
	OtherGoals []db.Goal
}

// bannedPhrases: a general negative judgement is not evidence (brief §7.4).
var bannedPhrases = []string{
	"işe yaramaz", "güvenilmez", "çalışmıyor", "does not work", "doesn't work", "unreliable", "useless", "never works", "is broken",
}

// CheckProposal validates one raw proposal against the rules and the goal.
// It returns the structured proposal and the reason it was refused ("" =
// accepted). Refusals are counted, never silently ignored.
func CheckProposal(raw RawProposal, cx ProposalContext) (insight.EvolutionProposal, string) {
	p := insight.EvolutionProposal{
		GoalID:  cx.Goal.ID,
		Surface: strings.ToLower(strings.TrimSpace(raw.Surface)),
		Field:   strings.TrimSpace(raw.Field),
		Action:  strings.ToLower(strings.TrimSpace(raw.Action)),
		Value:   strings.TrimSpace(raw.Value),
		Removes: strings.TrimSpace(raw.Removes),
		// EntityID is trimmed only: ids and slugs are case-sensitive.
		EntityID:       strings.TrimSpace(raw.EntityID),
		Evidence:       strings.TrimSpace(raw.Evidence),
		ExpectedMetric: strings.TrimSpace(raw.ExpectedMetric),
		ExpectedDelta:  raw.ExpectedDelta,
		SideEffects:    cleanList(raw.SideEffects),
		Kind:           "change",
	}
	rule, ok := LookupRule(p.Surface, p.Field)
	if !ok {
		return p, fmt.Sprintf("surface/field %s.%s is not editable", p.Surface, p.Field)
	}
	if !contains(rule.Actions, p.Action) {
		return p, fmt.Sprintf("action %q not allowed on %s.%s", p.Action, p.Surface, p.Field)
	}
	if p.Surface != SurfaceSettings && p.EntityID == "" {
		return p, "entity id missing"
	}
	if p.Evidence == "" || !strings.ContainsAny(p.Evidence, "0123456789") {
		return p, "no measured evidence"
	}
	text := strings.ToLower(raw.Title + " " + raw.Rationale)
	for _, b := range bannedPhrases {
		if strings.Contains(text, b) {
			return p, "general negative judgement"
		}
	}
	if p.Action != ActionPrune && p.Action != ActionRemove && p.Value == "" {
		return p, "value missing"
	}
	if rule.MaxValueLen > 0 && len([]rune(p.Value)) > rule.MaxValueLen {
		return p, fmt.Sprintf("value longer than %d characters", rule.MaxValueLen)
	}
	if cx.Exists != nil && p.Surface != SurfaceSettings && !cx.Exists(p.Surface, p.EntityID) {
		return p, fmt.Sprintf("%s %s does not exist", p.Surface, p.EntityID)
	}
	if cx.InScope != nil && !cx.InScope(p.Surface, p.EntityID) {
		return p, fmt.Sprintf("%s %s is outside the goal's scope", p.Surface, p.EntityID)
	}
	if rule.Additive && p.Action == ActionAdd && cx.SkillCount != nil && cx.SkillCount(p.EntityID)+1 > MaxSkillsPerAgent && p.Removes == "" {
		return p, "addition over the growth budget without naming a removal"
	}
	// Expected effect: a metric the goal cares about, moving the right way.
	goalMetrics := map[string]string{cx.Goal.Primary.Metric: cx.Goal.Primary.Direction}
	for _, gr := range cx.Goal.Guardrails {
		if m, ok := Lookup(gr.Metric); ok {
			goalMetrics[gr.Metric] = m.DefaultDirection
		}
	}
	if p.ExpectedMetric == "" {
		return p, "expected metric missing"
	}
	dir, ok := goalMetrics[p.ExpectedMetric]
	if !ok {
		return p, fmt.Sprintf("expected metric %s is not one of the goal's metrics", p.ExpectedMetric)
	}
	if p.ExpectedDelta == 0 || (dir == db.GoalDirectionMin && p.ExpectedDelta > 0) || (dir == db.GoalDirectionMax && p.ExpectedDelta < 0) {
		return p, "expected delta does not improve the metric"
	}
	// Side effects: worsening one of this goal's guardrails is a refusal;
	// touching another active goal's metrics is a conflict to surface.
	for _, m := range p.SideEffects {
		if m == cx.Goal.Primary.Metric {
			return p, "side effect on the goal's own primary metric"
		}
		for _, gr := range cx.Goal.Guardrails {
			if gr.Metric == m {
				return p, "side effect on the goal's own guardrail " + m
			}
		}
	}
	for _, other := range cx.OtherGoals {
		if other.ID == cx.Goal.ID || other.Status != db.GoalStatusActive {
			continue
		}
		if !scopesOverlap(cx.Goal.Scope, other.Scope) {
			continue
		}
		for _, m := range p.SideEffects {
			if m == other.Primary.Metric || hasGuardrail(other, m) {
				p.Kind = "conflict"
				p.Evidence = p.Evidence + " · conflicts with goal " + other.ID + " on " + m
			}
		}
	}
	return p, ""
}

// Signature is the dedupe key of a proposal: one per goal/surface/entity/field/action.
func Signature(p insight.EvolutionProposal) string {
	return strings.ToLower(strings.Join([]string{"evolution", p.GoalID, p.Surface, p.EntityID, p.Field, p.Action}, ":"))
}

// EffectiveMinRuns / EffectiveCooldownHours read the goal's policy with defaults.
func EffectiveMinRuns(g db.Goal) int {
	if g.Policy.MinRuns > 0 {
		return g.Policy.MinRuns
	}
	return DefaultMinRuns
}

func EffectiveCooldownHours(g db.Goal) int {
	if g.Policy.CooldownHours > 0 {
		return g.Policy.CooldownHours
	}
	return DefaultCooldownHours
}

// RulesForPrompt renders the editable surface list for the model.
func RulesForPrompt() string {
	var b strings.Builder
	bySurface := map[string][]ProposalRule{}
	var order []string
	for _, r := range proposalRules {
		if _, ok := bySurface[r.Surface]; !ok {
			order = append(order, r.Surface)
		}
		bySurface[r.Surface] = append(bySurface[r.Surface], r)
	}
	sort.Strings(order)
	for _, s := range order {
		fmt.Fprintf(&b, "- %s:", s)
		for i, r := range bySurface[s] {
			if i > 0 {
				b.WriteString(";")
			}
			fmt.Fprintf(&b, " %s (%s", r.Field, strings.Join(r.Actions, "|"))
			if r.Hint != "" {
				fmt.Fprintf(&b, "; %s", r.Hint)
			}
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func hasGuardrail(g db.Goal, metric string) bool {
	for _, gr := range g.Guardrails {
		if gr.Metric == metric {
			return true
		}
	}
	return false
}

// scopesOverlap: two scopes overlap when either is unscoped on every axis or
// they share an entity on any axis.
func scopesOverlap(a, b db.GoalScope) bool {
	empty := func(s db.GoalScope) bool {
		return len(s.Recipes)+len(s.Agents)+len(s.Automations)+len(s.Tags) == 0
	}
	if empty(a) || empty(b) {
		return true
	}
	for _, pair := range [][2][]string{{a.Recipes, b.Recipes}, {a.Agents, b.Agents}, {a.Automations, b.Automations}, {a.Tags, b.Tags}} {
		set := toSet(pair[0])
		for _, x := range pair[1] {
			if set[x] {
				return true
			}
		}
	}
	return false
}
