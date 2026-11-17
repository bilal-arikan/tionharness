package providers

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// This file renders opencode.json in the opencode 2.x schema (verified live on
// 2.2.5 through `opencode debug config`, which shows every loaded document
// after migration).
//
// Why not keep the 1.x document: 2.x still loads it, but its migration turns
// `tools: {x: false}` into deny rules placed BEFORE the `permission` block's
// `"*": "allow"`, and 2.x evaluates rules last-match-wins. Every legacy tool
// switch-off was therefore silently overridden — the subagent launcher and
// every other "disabled" tool stayed on the model's menu (reproduced live: the
// legacy config left `subagent` and `shell` listed and runnable). The native
// `permissions` array is ordered, so it is written broad-first, specific-last.
//
// Tool and permission names in 2.x: bash → shell, task → subagent,
// write/patch fold into edit. A rule whose effect is "deny" for every resource
// removes the tool from the model's menu (verified for shell and subagent).
//
// MCP servers keep opencode's default Code Mode: their tools are reached
// through the native `execute` tool (`tools["<server>"].<tool>(…)`), and the
// parser unwraps a single-call execute back into the bridged tool's name. Code
// Mode is not optional here: with it off (per-server `codemode: false` or
// `execute` denied) MCP tools are exposed directly but are missing from the
// first model step of a fresh `run --standalone` (reproduced live: present from
// step 2 on), so a one-shot turn could not reach the bridge at all.

// opencodeV2Rule is one entry of the ordered `permissions` array.
type opencodeV2Rule struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	Effect   string `json:"effect"`
}

type opencodeV2Doc struct {
	Schema      string                     `json:"$schema"`
	Share       string                     `json:"share"`
	Update      string                     `json:"update"`
	Permissions []opencodeV2Rule           `json:"permissions"`
	Agents      map[string]opencodeV2Agent `json:"agents"`
	MCP         *opencodeV2MCP             `json:"mcp,omitempty"`
}

type opencodeV2Agent struct {
	Mode        string `json:"mode,omitempty"`
	Description string `json:"description,omitempty"`
	System      string `json:"system,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

type opencodeV2MCP struct {
	Servers map[string]opencodeV2MCPServer `json:"servers"`
}

type opencodeV2MCPServer struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Disabled    bool              `json:"disabled"`
}

// opencodeV2SubagentTool is 2.x's native subagent launcher (1.x: "task").
const opencodeV2SubagentTool = "subagent"

func opencodeV2Deny(action string) opencodeV2Rule {
	return opencodeV2Rule{Action: action, Resource: "*", Effect: "deny"}
}

// opencodeV2Permissions renders the ordered rule list for cfg (last match wins).
func opencodeV2Permissions(cfg opencodeConfig) []opencodeV2Rule {
	// Auxiliary calls (title, summary, judge) get an empty menu: the prompt is
	// the whole request. A blanket deny also covers native tools added by
	// future releases, which a per-name list would miss.
	if cfg.DisableAllNativeTools {
		return []opencodeV2Rule{opencodeV2Deny("*")}
	}
	rules := []opencodeV2Rule{{Action: "*", Resource: "*", Effect: "allow"}}
	switch cfg.PermissionMode {
	case "read-only":
		rules = append(rules, opencodeV2Deny("edit"), opencodeV2Deny("shell"), opencodeV2Deny("webfetch"))
	case "ask":
		// `run` auto-rejects every "ask" it cannot show ("This non-interactive run
		// cannot ask the user for permission", verified on 2.2.5) — same honest
		// mapping as 1.x, see opencodePermission.
		rules = append(rules,
			opencodeV2Rule{Action: "edit", Resource: "*", Effect: "ask"},
			opencodeV2Rule{Action: "shell", Resource: "*", Effect: "ask"})
	}
	if cfg.DisableNativeWeb {
		rules = append(rules, opencodeV2Deny("websearch"), opencodeV2Deny("webfetch"))
	}
	// opencode's own subagent launcher runs invisibly to TionHarness; the
	// bridged run_subagent is the supported path. Always off, and last so no
	// earlier allow can re-enable it.
	rules = append(rules, opencodeV2Deny(opencodeV2SubagentTool))
	keys := make([]string, 0, len(cfg.Servers))
	for k := range cfg.Servers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, t := range cfg.Servers[key].DisabledTools {
			rules = append(rules, opencodeV2Deny(opencodeMCPToolID(key, t)))
		}
	}
	return rules
}

var opencodeUnsafeIDChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// opencodeSanitizeID mirrors opencode's own MCP identifier sanitising: every
// character outside [a-zA-Z0-9_-] becomes "_" (server namespaces and tool ids
// alike, on 1.x and 2.x).
func opencodeSanitizeID(s string) string { return opencodeUnsafeIDChars.ReplaceAllString(s, "_") }

// opencodeMCPToolID is the id (and permission action) opencode gives a bridged
// MCP tool: "<server>_<tool>".
func opencodeMCPToolID(server, tool string) string {
	return opencodeSanitizeID(server) + "_" + opencodeSanitizeID(tool)
}

// renderOpenCodeConfigV2 renders cfg as a 2.x opencode.json document. Map keys
// are sorted by encoding/json and the rule list is built deterministically, so
// identical input yields byte-identical output.
func renderOpenCodeConfigV2(cfg opencodeConfig, systemPath string) ([]byte, error) {
	doc := opencodeV2Doc{
		Schema:      "https://opencode.ai/config.json",
		Share:       "disabled",
		Update:      "disable",
		Permissions: opencodeV2Permissions(cfg),
		Agents: map[string]opencodeV2Agent{
			// The built-in subagent persona must not be offered either.
			"general": {Disabled: true},
		},
	}
	agent := opencodeV2Agent{Mode: "primary", Description: "TionHarness bridge agent"}
	if strings.TrimSpace(cfg.SystemPrompt) != "" {
		agent.System = "{file:" + filepath.ToSlash(systemPath) + "}"
	}
	doc.Agents[opencodeAgentName] = agent
	if len(cfg.Servers) > 0 {
		doc.MCP = &opencodeV2MCP{Servers: map[string]opencodeV2MCPServer{}}
		for key, s := range cfg.Servers {
			entry := opencodeV2MCPServer{}
			switch s.Transport {
			case "sse", "http":
				entry.Type = "remote"
				entry.URL = s.URL
				if len(s.Headers) > 0 {
					entry.Headers = s.Headers
				}
			default:
				entry.Type = "local"
				entry.Command = append([]string{s.Command}, s.Args...)
				if len(s.Env) > 0 {
					entry.Environment = s.Env
				}
			}
			doc.MCP.Servers[key] = entry
		}
	}
	return json.MarshalIndent(doc, "", "  ")
}
