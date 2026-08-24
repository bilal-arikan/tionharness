package agent

import (
	"github.com/bilal-arikan/tionharness/internal/prompts"
)

// PromptInfo describes one registered runtime prompt for read-only display in
// the settings UI. Derived from the central registry (internal/prompts) — this
// is purely a viewer payload; editing goes through the workspace config API.
type PromptInfo struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	File   string `json:"file"`   // embedded default source (defaults/<key>.md)
	System string `json:"system"` // the embedded default text
	User   string `json:"user"`   // user-turn template (placeholders shown literally)
	Note   string `json:"note"`   // short usage note
}

// PromptsDir returns the absolute directory holding the embedded prompt default
// sources (internal/prompts/defaults on a locally-built binary), so the UI's
// "open folder" button lands on the actual sources.
func PromptsDir() string { return prompts.SourceDir() }

// Prompts returns the full registry for read-only display.
func Prompts() []PromptInfo {
	specs := prompts.Specs()
	out := make([]PromptInfo, 0, len(specs))
	for _, s := range specs {
		out = append(out, PromptInfo{
			Key:    s.Key,
			Label:  s.Label,
			File:   s.Key + ".md",
			System: prompts.Default(s.Key),
			Note:   s.Hint,
		})
	}
	return out
}
