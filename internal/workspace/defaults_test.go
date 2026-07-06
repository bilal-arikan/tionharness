package workspace

import (
	"strings"
	"testing"
)

// TestDefaultInstructionsAreTionSwarmNative guards the embedded workspace prompt
// against regressing back into a copy of the external agent project's system prompt. The seed
// must describe TionSwarm's real surface and must not teach capabilities TionSwarm
// does not have (datatable/spreadsheet rendering, call_llm, the external agent project doc paths,
// MCP tool metadata requirements).
//
// NOTE: `html-preview` and `render_template` were once forbidden as the external agent project-isms
// but are now genuine TionSwarm-native features (inline sandboxed HTML render +
// the branded-template tool, _Docs/53), so the prompt IS allowed to teach them.
// The still-forbidden entries remain capabilities TionSwarm does not implement.
func TestDefaultInstructionsAreTionSwarmNative(t *testing.T) {
	forbidden := []string{
		"datatable",
		"spreadsheet",
		"call_llm",
		"~/.external-agent/docs",
		"_displayName",
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
