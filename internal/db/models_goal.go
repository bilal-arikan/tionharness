package db

// Goal is a workspace objective the evolution machinery (_Docs/83) optimizes
// toward: one primary metric from the closed catalog, the guardrails that must
// hold while it is pushed, an optional scope and a policy. A goal is created
// either directly by the user (the editor) or from the user's own words by the
// goal-writer system agent, which keeps the statement verbatim in RawText and
// stores the result as a DRAFT for review. Every later edit appends a
// GoalRevision so the goal's own history is inspectable.
type Goal struct {
	ID string `json:"id"` // GOL<n>

	Name        string `json:"name"`                  // short title
	Description string `json:"description,omitempty"` // what "better" means, assumptions included
	// RawText is what the user typed when the goal came through the writer,
	// kept verbatim so intent is never lost in the rewrite. Empty for goals
	// the user entered directly.
	RawText string `json:"rawText,omitempty"`

	// Status: draft (written, not yet confirmed) | active | paused | archived.
	Status string `json:"status"`

	Scope      GoalScope       `json:"scope"`
	Primary    GoalMetric      `json:"primary"`
	Guardrails []GoalGuardrail `json:"guardrails"`
	Policy     GoalPolicy      `json:"policy"`

	// CreatedBy is provenance only ("user" | "agent:goal-writer" | "seed").
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

// GoalPolicy says what the evolver may do on this goal's behalf.
type GoalPolicy struct {
	// Mode: propose (default; file proposals only) | off (measure only).
	Mode string `json:"mode"`
	// CooldownHours is the minimum time between two passes for this goal.
	CooldownHours int `json:"cooldownHours,omitempty"`
	// MinRuns is how many in-scope runs must accumulate before a pass.
	MinRuns int `json:"minRuns,omitempty"`
}

// GoalRevision is one entry of a goal's change log.
type GoalRevision struct {
	At     int64    `json:"at"`
	By     string   `json:"by"`               // "user" | "agent:goal-writer" | "seed"
	Note   string   `json:"note,omitempty"`   // human line (what changed / why)
	Fields []string `json:"fields,omitempty"` // changed field names
}

// Goal statuses, directions, policy modes and provenance markers.
const (
	GoalStatusDraft    = "draft"
	GoalStatusActive   = "active"
	GoalStatusPaused   = "paused"
	GoalStatusArchived = "archived"

	GoalDirectionMin = "min"
	GoalDirectionMax = "max"

	GoalModePropose = "propose"
	GoalModeOff     = "off"

	GoalByUser   = "user"
	GoalByWriter = "agent:goal-writer"
	// GoalBySeed marks a built-in starter goal provisioned into a new workspace
	// (goals.EnsureDefaultGoals), so the UI can tell it from one the user stated.
	GoalBySeed = "seed"
)
