package workspace

import (
	"strings"
	"testing"
)

// TestDefaultInstructionsAreTionSwarmNative guards the embedded workspace prompt
// against regressing back into a copy of the external agent project's system prompt. The seed
// must describe TionSwarm's real surface and must not teach capabilities TionSwarm
// does not have (datatable/spreadsheet rendering, call_llm, render_template,
// the external agent project doc paths, MCP tool metadata requirements).
func TestDefaultInstructionsAreTionSwarmNative(t *testing.T) {
	forbidden := []string{
		"datatable",
		"spreadsheet",
		"render_template",
		"call_llm",
		"~/.external-agent/docs",
		"_displayName",
		"html-preview",
		"pdf-preview",
		"markdown-preview",
		"the external agent project",
	}
	for _, s := range forbidden {
		if strings.Contains(defaultInstructions, s) {
			t.Errorf("default instructions still contain the external agent project-ism %q", s)
		}
	}

	required := []string{
		"TionSwarm",
		"run_subagent",
		"use_skill",
		"update_session",
		"mermaid",
	}
	for _, s := range required {
		if !strings.Contains(defaultInstructions, s) {
			t.Errorf("default instructions missing required TionSwarm term %q", s)
		}
	}
}
