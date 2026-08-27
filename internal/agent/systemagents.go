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

func buildSystemAgentDefaults() []SystemAgentDefinition {
	defs := []SystemAgentDefinition{
		{
			SystemKey:      "titler",
			Name:           "Titler",
			Description:    "Generates concise titles for requests and conversations.",
			SystemPrompt:   prompts.Default("title"),
			SuggestedModel: "haiku",
			AllowedTools:   "[]",
		},
		{
			SystemKey:      "overview-summarizer",
			Name:           "Overview Summarizer",
			Description:    "Summarizes on-demand board and flow overviews.",
			SystemPrompt:   prompts.Default("summary"),
			SuggestedModel: "haiku",
			AllowedTools:   "[]",
		},
		{
			SystemKey:      "compaction",
			Name:           "Compactor",
			Description:    "Compacts conversation history into a structured summary.",
			SystemPrompt:   prompts.Default("compact"),
			SuggestedModel: "haiku",
			AllowedTools:   "[]",
		},
		{
			SystemKey:      "lesson-extractor",
			Name:           "Lesson Extractor",
			Description:    "Distills reusable lessons from failed agent turns.",
			SystemPrompt:   prompts.Default("lesson"),
			SuggestedModel: "haiku",
			AllowedTools:   "[]",
		},
		{
			SystemKey:      "insight",
			Name:           "Insight Analyzer",
			Description:    "Analyzes session evidence for recurring, actionable findings.",
			SystemPrompt:   prompts.Default("insight-analyzer"),
			SuggestedModel: "haiku",
			AllowedTools:   "[]",
			Disabled:       true,
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
	return defs
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
