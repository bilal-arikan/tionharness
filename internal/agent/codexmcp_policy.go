package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// codexServerToolPolicy translates agent namespaces into Codex's exact local
// MCP tool names. Server-wide patterns are handled by mcpServerGate. Codex
// 0.157.1 does not expand globs in enabled_tools/disabled_tools. Refuse a server
// with a narrower glob instead of silently granting tools the policy blocks.
// This avoids launching a second stdio server merely to discover its catalog.
func codexServerToolPolicy(ag db.Agent, key string, exempt bool) (providers.CLIMCPServer, error) {
	var policy providers.CLIMCPServer
	overrides, err := ParseToolOverridesErr(ag)
	if err != nil {
		return policy, err
	}
	var allowed []string
	if strings.TrimSpace(ag.AllowedTools) != "" {
		if err := json.Unmarshal([]byte(ag.AllowedTools), &allowed); err != nil {
			return policy, err
		}
	}
	prefix := key + "__"
	if len(allowed) > 0 && !exempt {
		unrestricted := false
		for _, p := range allowed {
			unrestricted = unrestricted || patternCoversServer(p, key)
		}
		if !unrestricted {
			policy.EnabledTools = []string{}
			for _, p := range allowed {
				if strings.HasPrefix(p, prefix) {
					if strings.HasSuffix(p, "*") {
						return policy, fmt.Errorf("Codex MCP server %q: tool prefix %q requires exact tool names", key, p)
					}
					policy.EnabledTools = append(policy.EnabledTools, strings.TrimPrefix(p, prefix))
				}
			}
		}
	}
	for _, p := range blockedPatterns(overrides) {
		if patternCoversServer(p, key) {
			policy.EnabledTools = []string{}
			return policy, nil
		}
		if strings.HasPrefix(p, prefix) {
			if strings.HasSuffix(p, "*") {
				return policy, fmt.Errorf("Codex MCP server %q: tool prefix %q requires exact tool names", key, p)
			}
			policy.DisabledTools = append(policy.DisabledTools, strings.TrimPrefix(p, prefix))
		}
	}
	return policy, nil
}
