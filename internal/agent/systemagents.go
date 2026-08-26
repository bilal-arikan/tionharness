package agent

import (
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/prompts"
)

// SystemAgentDefinition is a canonical built-in agent definition.
type SystemAgentDefinition = db.SystemAgentDefinition

var systemAgentDefaults = []SystemAgentDefinition{
	{
		SystemKey:      "titler",
		Name:           "Titler",
		Description:    "Generates concise titles for requests and conversations.",
		SystemPrompt:   prompts.Default("title"),
		SuggestedModel: "haiku",
		AllowedTools:   "[]",
	},
	{
		SystemKey:      "compactor",
		Name:           "Compactor",
		Description:    "Summarizes structured workspace data.",
		SystemPrompt:   prompts.Default("summary"),
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
