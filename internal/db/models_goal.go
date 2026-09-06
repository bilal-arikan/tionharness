package db

// Goal is a workspace objective the evolution machinery (_Docs/83) optimizes
// toward. Goals are never written straight from a form: the goal-writer system
// agent turns the user's own words (RawText) into this normalized shape, saves
// it as a DRAFT, and the user reviews, edits and activates it. Every later
// human or agent edit appends a GoalRevision so the goal's own history is
// inspectable before any optimizer acts on it.
type Goal struct {
	ID string `json:"id"` // GOL<n>

	Name    string `json:"name"`              // short title
	Summary string `json:"summary,omitempty"` // one line, what "better" means
	// Description is the agent-normalized statement of the objective; RawText
	// is what the user typed, kept verbatim so intent is never lost in the
	// rewrite (the "loss of user intent" failure mode).
	Description string `json:"description,omitempty"`
	RawText     string `json:"rawText,omitempty"`

	// Status: draft (written, not yet confirmed) | active | paused | archived.
	Status string `json:"status"`
	// Kind: metric (fully measurable) | rubric (open-ended, judged) | mixed.
	Kind string `json:"kind,omitempty"`
	// Priority orders competing goals: 1 (highest) .. 5. 0 = unset (treated as 3).
	Priority int `json:"priority,omitempty"`

	Scope      GoalScope       `json:"scope"`
	Primary    GoalMetric      `json:"primary"`
	Guardrails []GoalGuardrail `json:"guardrails"`
	// Rubric is the plain-language grading rubric for the open-ended part of the
	// goal (Anthropic "Outcomes" pattern); empty for purely metric goals.
	Rubric string     `json:"rubric,omitempty"`
	Policy GoalPolicy `json:"policy"`

	// Questions are the writer's clarifying questions the user should settle
	// before activating; Notes carries the writer's assumptions.
	Questions []string `json:"questions,omitempty"`
	Notes     string   `json:"notes,omitempty"`

	// CreatedBy is provenance only ("user" | "agent:goal-writer").
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	// History is the goal's own change log (newest last).
	History []GoalRevision `json:"history"`
}

// GoalScope narrows which work the goal is measured over. Every list empty =
// the whole workspace.
type GoalScope struct {
	Recipes     []string `json:"recipes,omitempty"`     // recipe slugs
	Agents      []string `json:"agents,omitempty"`      // agent ids
	Automations []string `json:"automations,omitempty"` // automation ids
	Tags        []string `json:"tags,omitempty"`        // session tags
}

// GoalMetric is the primary objective: a catalog metric key and the direction
// that counts as improvement. Target is optional ("good enough" threshold).
type GoalMetric struct {
	Metric    string   `json:"metric"`
	Direction string   `json:"direction"` // min | max
	Target    *float64 `json:"target,omitempty"`
}

// GoalGuardrail is a constraint that must hold while the primary metric is
// pushed (cost may fall, but success rate may not drop under Min).
type GoalGuardrail struct {
	Metric string   `json:"metric"`
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
}

// GoalPolicy says how far the optimizer may go on this goal's behalf.
type GoalPolicy struct {
	// Mode: propose (default; file proposals only) | auto (apply the
	// reversible surfaces listed below) | off (measure only).
	Mode string `json:"mode"`
	// AutoApplySurfaces names the reversible surfaces auto mode may touch
	// (e.g. thinkingLevel, toolVisibility, automationCooldown).
	AutoApplySurfaces []string `json:"autoApplySurfaces,omitempty"`
	// CooldownHours is the minimum time between two changes for this goal.
	CooldownHours int `json:"cooldownHours,omitempty"`
	// MinRuns is how many in-scope runs must accumulate before a pass.
	MinRuns int `json:"minRuns,omitempty"`
}

// GoalRevision is one entry of a goal's change log.
type GoalRevision struct {
	At     int64    `json:"at"`
	By     string   `json:"by"`               // "user" | "agent:goal-writer"
	Note   string   `json:"note,omitempty"`   // human line (what changed / why)
	Fields []string `json:"fields,omitempty"` // changed field names
}

// Goal statuses, kinds, directions and policy modes.
const (
	GoalStatusDraft    = "draft"
	GoalStatusActive   = "active"
	GoalStatusPaused   = "paused"
	GoalStatusArchived = "archived"

	GoalKindMetric = "metric"
	GoalKindRubric = "rubric"
	GoalKindMixed  = "mixed"

	GoalDirectionMin = "min"
	GoalDirectionMax = "max"

	GoalModePropose = "propose"
	GoalModeAuto    = "auto"
	GoalModeOff     = "off"

	GoalByUser   = "user"
	GoalByWriter = "agent:goal-writer"
)
