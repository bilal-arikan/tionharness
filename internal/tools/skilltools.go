package tools

import "strings"

// skilltools.go marks the SKILL-DISCOVERY surface: the two tools an agent uses to
// find and load a skill's instructions.
//
// They are separated from the work tools for the same reason the coordination
// surface is (see builtin_coordination.go): the system prompt renders the
// "# Available Skills" block for EVERY agent and tells it to load a matching
// skill before acting, so an allowlist-only profile would be instructed to call a
// tool it cannot see. Reading a skill is read-only, so the exemption is safe even
// for read-only profiles.

// SkillToolNames is every tool in the skill-discovery surface — the single source
// of truth behind IsSkillTool.
var SkillToolNames = []string{
	"use_skill",
	"skill_search",
}

// IsSkillTool reports whether name is part of the skill-discovery surface, in its
// bare form or in the namespaced bridge form CLI providers see (the interaction
// MCP bridge prefixes them, e.g. mcp__tionharness_interaction__use_skill).
func IsSkillTool(name string) bool {
	for _, n := range SkillToolNames {
		if name == n || strings.HasSuffix(name, "__"+n) {
			return true
		}
	}
	return false
}
