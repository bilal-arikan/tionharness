package db

// Automation is an event-driven rule that starts a NEW session whenever a
// session carrying TriggerTag finishes a turn (StopReason end_turn). It takes
// the finishing session's final reply as the "result", renders it into
// PromptTemplate, and spawns TargetAgentID with the composed prompt. When the
// spawned session itself carries TriggerTag (the default — see SpawnTags) each
// completion re-fires the rule, forming a self-continuing loop.
//
// Automations are the tag-triggered complement to cron Schedules: a Schedule
// fires on a clock, an Automation fires on a tagged session completing. They are
// surfaced in the same Schedules screen (a separate "Automations" section) but
// kept a distinct entity so the cron Schedule model stays clean.
//
// Guardrails bound the loop: MaxIterations caps total fires, CooldownSec spaces
// them out, and Enabled is the kill switch. IterationCount accumulates across the
// whole chain (every spawned session shares TriggerTag → the same rule) so a
// runaway loop stops at MaxIterations.
type Automation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// TriggerTag is the session tag this rule watches. A finishing session whose
	// Tags contain TriggerTag fires the rule.
	TriggerTag string `json:"triggerTag"`
	// TargetAgentID is the agent that runs the spawned session.
	TargetAgentID string `json:"targetAgentId"`
	// PromptTemplate is the prompt delivered to the spawned session. Placeholders:
	// {{result}} (the finishing session's final reply), {{title}} (its title),
	// {{tag}} (TriggerTag), {{sessionId}} (the finishing session's id).
	PromptTemplate string `json:"promptTemplate"`
	// SpawnTags are the tags applied to the spawned session. When nil it defaults
	// to [TriggerTag], so the spawned session re-triggers this rule (the loop). Set
	// it to an empty non-nil slice ([]) or different tags to break/redirect the loop.
	SpawnTags []string `json:"spawnTags,omitempty"`
	// Enabled is the kill switch. A disabled automation never fires.
	Enabled bool `json:"enabled"`
	// MaxIterations caps the total number of fires (0 = unlimited — dangerous, an
	// unbounded loop). Default 50 in the create paths.
	MaxIterations int `json:"maxIterations"`
	// CooldownSec is the minimum number of seconds between two fires of this rule
	// (0 = no cooldown). Bounds burst re-triggering.
	CooldownSec int `json:"cooldownSec"`
	// ExpiresAt is an optional end date (unix seconds). When > 0 the automation
	// stops firing once the time passes and is auto-disabled on the next attempt.
	// 0 means "no end date" (runs until maxIterations / manual disable).
	ExpiresAt int64 `json:"expiresAt,omitempty"`

	// --- runtime bookkeeping (updated by RecordAutomationFire) ---
	IterationCount int    `json:"iterationCount"`
	LastFiredAt    int64  `json:"lastFiredAt,omitempty"`
	LastSessionID  string `json:"lastSessionId,omitempty"`
	LastError      string `json:"lastError,omitempty"`

	// CreatedBy is the ID of the agent that created this automation via a
	// self-management tool ("" = created by the user). Agents may only edit/delete
	// agent-created automations.
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}
