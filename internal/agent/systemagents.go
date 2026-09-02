package agent

import (
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/prompts"
)

// SystemAgentDefinition is a canonical built-in agent definition.
type SystemAgentDefinition = db.SystemAgentDefinition

var systemAgentDefaults = buildSystemAgentDefaults()

// systemAgentVisuals is the SINGLE source of truth for the canonical look of
// every built-in agent, keyed by SystemKey. Colors are grouped by role so the
// roster reads at a glance:
//
//	analysis agents      violet / slate  (they only summarise and observe)
//	worker, read-only    teal / blue     (explore, planner, reviewer, validator)
//	worker, writing      amber / rust    (coder, config — the ones that mutate)
var systemAgentVisuals = map[string]struct{ Avatar, Color string }{
	// Analysis family.
	"titler":              {"🔖", "#7C6BE8"},
	"overview-summarizer": {"📋", "#9C6BD8"},
	"compaction":          {"📦", "#6B5FA8"},
	"lesson-extractor":    {"🎓", "#B07AD0"},
	"insight":             {"🔮", "#5C6480"},
	"insight-applier":     {"🛠️", "#8A6BC8"},
	"recipe-optimizer":    {"✦", "#6B7FD8"},

	// Worker profiles — read-only.
	"subagent-explore":   {"🔍", "#17A2A2"},
	"subagent-planner":   {"🧭", "#2E86D8"},
	"subagent-reviewer":  {"🧐", "#4FBF8B"},
	"subagent-validator": {"🧪", "#3FB8D8"},

	// Worker profiles — allowed to write.
	"subagent-coder":  {"💻", "#E08A2E"},
	"subagent-config": {"🔧", "#D2683C"},
}

// applySystemAgentVisuals stamps the canonical avatar/color onto every
// definition. A key missing from the table is a programming error: a built-in
// agent with no visual identity is exactly the "random default look" this table
// exists to remove.
func applySystemAgentVisuals(defs []SystemAgentDefinition) []SystemAgentDefinition {
	for i := range defs {
		v, ok := systemAgentVisuals[defs[i].SystemKey]
		if !ok {
			panic("no visual identity for system agent " + defs[i].SystemKey)
		}
		defs[i].Avatar = v.Avatar
		defs[i].Color = v.Color
	}
	return defs
}

func buildSystemAgentDefaults() []SystemAgentDefinition {
	defs := []SystemAgentDefinition{
		{
			SystemKey:      "titler",
			Name:           "Titler",
			Description:    "Generates concise titles for requests and conversations.",
			SystemPrompt:   prompts.Default("title"),
			SuggestedModel: "haiku",
			Provider:       "claude-cli",
			AllowedTools:   "[]",
		},
		{
			SystemKey:      "overview-summarizer",
			Name:           "Overview Summarizer",
			Description:    "Summarizes on-demand board and flow overviews.",
			SystemPrompt:   prompts.Default("summary"),
			SuggestedModel: "haiku",
			Provider:       "claude-cli",
			AllowedTools:   "[]",
		},
		{
			SystemKey:      "compaction",
			Name:           "Compactor",
			Description:    "Compacts conversation history into a structured summary.",
			SystemPrompt:   prompts.Default("compact"),
			SuggestedModel: "haiku",
			Provider:       "claude-cli",
			AllowedTools:   "[]",
		},
		{
			SystemKey:      "lesson-extractor",
			Name:           "Lesson Extractor",
			Description:    "Distills reusable lessons from failed agent turns.",
			SystemPrompt:   prompts.Default("lesson"),
			SuggestedModel: "haiku",
			Provider:       "claude-cli",
			AllowedTools:   "[]",
		},
		{
			SystemKey:      "insight",
			Name:           "Insight Analyzer",
			Description:    "Analyzes session evidence for recurring, actionable findings.",
			SystemPrompt:   prompts.Default("insight-analyzer"),
			SuggestedModel: "haiku",
			Provider:       "claude-cli",
			AllowedTools:   "[]",
			Disabled:       true,
		},
		{
			SystemKey:      "recipe-optimizer",
			Name:           "Recipe Optimizer",
			Description:    "Proposes measured changes to a coordinator recipe from its trajectory statistics (Rota F4).",
			SystemPrompt:   prompts.Default("recipe-optimizer"),
			SuggestedModel: "haiku",
			Provider:       "claude-cli",
			// Suggestion-only: it reads the evidence it is handed and answers with
			// JSON; it never touches an entity, so it needs no tools.
			AllowedTools: "[]",
		},
		{
			SystemKey:      "insight-applier",
			Name:           "Insight Applier",
			Description:    "Applies workspace-opt insight findings to workspace entities.",
			SystemPrompt:   prompts.Default("insight-applier"),
			SuggestedModel: "haiku",
			Provider:       "claude-cli",
			// Deliberately NO group:files and NO group:config: the applier fixes
			// workspace ENTITIES (skills, agents, hooks, automations), so it must not
			// be able to reach Read/Write/Edit/Bash or settings/secret/workspace tools.
			// The allowlist is the enforcement, not the prompt.
			AllowedTools: `["group:automation","group:agents","group:skills-mcp","group:artifacts",` +
				`"insight_list_findings","insight_apply_finding","todo_write"]`,
		},
	}
	workerDescriptions := map[string]string{
		"explore":   "Explores a codebase without modifying it.",
		"planner":   "Produces an executable implementation plan.",
		"coder":     "Implements focused code changes.",
		"reviewer":  "Reviews code for defects and regressions.",
		"validator": "Validates changes with builds and tests.",
		"config":    "Edits workspace configuration within its sandbox.",
	}
	for _, id := range []string{"explore", "planner", "coder", "reviewer", "validator", "config"} {
		allowedTools, err := json.Marshal(defaultSubagentProfiles[id].AllowedTools)
		if err != nil {
			panic(err)
		}
		defs = append(defs, SystemAgentDefinition{
			SystemKey:    "subagent-" + id,
			Name:         "Worker: " + strings.ToUpper(id[:1]) + id[1:],
			Description:  workerDescriptions[id],
			SystemPrompt: prompts.Default("subagent-" + id),
			AllowedTools: string(allowedTools),
		})
	}
	return applySystemAgentVisuals(defs)
}

// SystemAgentDefault returns the canonical definition for key.
func SystemAgentDefault(key string) (SystemAgentDefinition, bool) {
	for _, def := range systemAgentDefaults {
		if def.SystemKey == key {
			return def, true
		}
	}
	return SystemAgentDefinition{}, false
}

// SystemAgentDefaults returns a copy of all canonical definitions.
func SystemAgentDefaults() []SystemAgentDefinition {
	defs := make([]SystemAgentDefinition, len(systemAgentDefaults))
	copy(defs, systemAgentDefaults)
	return defs
}
