package providers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// This file renders the per-turn `opencode.json` the opencode-cli transport
// hands to its subprocess through OPENCODE_CONFIG. Unlike codex's config.toml it
// is plain JSON, so encoding/json does the escaping; the shape is the subset of
// opencode's documented config the bridge needs (agent prompt, permission,
// tools on/off, MCP servers). Verified live on opencode 1.19.0.

// opencodeAgentName is the primary agent every TionHarness turn runs as. Its
// prompt REPLACES opencode's built-in "build" persona; opencode still appends
// its own environment block and any AGENTS.md it finds in the working directory.
const opencodeAgentName = "tionharness"

// opencodeSystemFile is the prompt file the agent definition points at
// ({file:...}): the system prompt is far too large for a command line or an
// environment variable (Windows caps both around 32 KB).
const opencodeSystemFile = "system.md"

// opencodeNativeTools lists every opencode built-in tool the config can switch
// off through the `tools` block. Kept explicit so an auxiliary (tool-less) call
// can disable all of them and a read-only turn its mutating subset.
var opencodeNativeTools = []string{
	"bash", "edit", "write", "patch", "read", "glob", "grep", "list",
	"webfetch", "websearch", "todowrite", "todoread", "task", "skill", "question",
}

// opencodeMutatingTools is the subset a read-only turn must not run.
var opencodeMutatingTools = []string{"bash", "edit", "write", "patch"}

// opencodeConfig is one turn's rendered configuration.
type opencodeConfig struct {
	// SystemPrompt becomes the tionharness agent's prompt file. Empty renders
	// no prompt key (opencode's default persona applies).
	SystemPrompt string
	// PermissionMode is TionHarness's "" | "auto" | "ask" | "read-only".
	PermissionMode string
	// DisableNativeShell switches opencode's bash tool off so shell execution
	// goes through the permissioned TionHarness bridge instead.
	DisableNativeShell bool
	// DisableNativeWeb switches websearch/webfetch off (Request.NativeWebSearch
	// false) so the bridged TionHarness web tools are the only path.
	DisableNativeWeb bool
	// DisableAllNativeTools empties the built-in menu (auxiliary calls: title,
	// summary, judge) — the prompt is the whole request.
	DisableAllNativeTools bool
	// Servers renders the `mcp` block, keyed by server key.
	Servers map[string]CLIMCPServer
}

// opencodeConfigDoc is the JSON shape written to disk.
type opencodeConfigDoc struct {
	Schema     string                          `json:"$schema"`
	Share      string                          `json:"share"`
	Autoupdate bool                            `json:"autoupdate"`
	Permission map[string]string               `json:"permission,omitempty"`
	Tools      map[string]bool                 `json:"tools,omitempty"`
	Agent      map[string]opencodeAgentDoc     `json:"agent"`
	MCP        map[string]opencodeMCPServerDoc `json:"mcp,omitempty"`
}

type opencodeAgentDoc struct {
	Mode        string `json:"mode,omitempty"`
	Description string `json:"description,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	Disable     bool   `json:"disable,omitempty"`
}

type opencodeMCPServerDoc struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     bool              `json:"enabled"`
}

// opencodePermission maps TionHarness's permission mode onto opencode's
// `permission` block.
//
// WARNING — "ask" is NOT per-tool approval here: `opencode run` auto-rejects
// every permission request it cannot show ("permission requested ...;
// auto-rejecting", verified on 1.19.0). The honest mapping is therefore: auto
// allows everything, read-only denies the mutating natives, and ask keeps
// opencode's own ask so a native edit/bash attempt is rejected with a visible
// message while the bridged TionHarness tools (which carry real approval
// through the Interaction server) stay usable. The UI must not present it as
// native approval.
func opencodePermission(mode string) map[string]string {
	switch mode {
	case "read-only":
		return map[string]string{"*": "allow", "edit": "deny", "bash": "deny", "webfetch": "deny"}
	case "ask":
		return map[string]string{"*": "allow", "edit": "ask", "bash": "ask"}
	default: // "auto", "" and any unknown value
		return map[string]string{"*": "allow"}
	}
}

// opencodeToolsBlock renders the `tools` on/off map for cfg. Only switched-off
// tools are listed (opencode's default is on), in sorted order so the rendered
// file is deterministic.
func opencodeToolsBlock(cfg opencodeConfig) map[string]bool {
	off := map[string]bool{}
	// opencode's own subagent launcher runs invisibly to TionHarness; the
	// bridged run_subagent is the supported path. Always off.
	off["task"] = false
	if cfg.DisableAllNativeTools {
		for _, t := range opencodeNativeTools {
			off[t] = false
		}
		return off
	}
	if cfg.DisableNativeShell {
		off["bash"] = false
	}
	if cfg.DisableNativeWeb {
		off["websearch"] = false
		off["webfetch"] = false
	}
	if cfg.PermissionMode == "read-only" {
		for _, t := range opencodeMutatingTools {
			off[t] = false
		}
	}
	return off
}

// renderOpenCodeConfig renders cfg as the opencode.json document. systemPath is
// the absolute path of the prompt file the agent references (ignored when the
// prompt is empty). Map iteration order is irrelevant: encoding/json sorts map
// keys, so identical input yields byte-identical output.
func renderOpenCodeConfig(cfg opencodeConfig, systemPath string) ([]byte, error) {
	doc := opencodeConfigDoc{
		Schema:     "https://opencode.ai/config.json",
		Share:      "disabled",
		Autoupdate: false,
		Permission: opencodePermission(cfg.PermissionMode),
		Tools:      opencodeToolsBlock(cfg),
		Agent: map[string]opencodeAgentDoc{
			// The built-in subagent must not be offered to the model.
			"general": {Disable: true},
		},
	}
	agent := opencodeAgentDoc{Mode: "primary", Description: "TionHarness bridge agent"}
	if strings.TrimSpace(cfg.SystemPrompt) != "" {
		agent.Prompt = "{file:" + filepath.ToSlash(systemPath) + "}"
	}
	doc.Agent[opencodeAgentName] = agent
	if len(cfg.Servers) > 0 {
		doc.MCP = map[string]opencodeMCPServerDoc{}
		keys := make([]string, 0, len(cfg.Servers))
		for k := range cfg.Servers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			s := cfg.Servers[key]
			entry := opencodeMCPServerDoc{Enabled: true}
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
			doc.MCP[key] = entry
			// opencode names MCP tools "<server>_<tool>"; a per-tool disable list
			// maps onto the same `tools` block the natives use. An enabled-only
			// list cannot be expressed without enumerating the server (opencode has
			// no allow-only switch), so it is left to the bridge's own tier filter.
			for _, t := range s.DisabledTools {
				doc.Tools[key+"_"+t] = false
			}
		}
	}
	return json.MarshalIndent(doc, "", "  ")
}

// writeOpenCodeConfig writes the turn's opencode.json (and the system prompt
// file it references) into dir and returns the config path.
func writeOpenCodeConfig(dir string, cfg opencodeConfig) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	systemPath := filepath.Join(dir, opencodeSystemFile)
	if strings.TrimSpace(cfg.SystemPrompt) != "" {
		if err := os.WriteFile(systemPath, []byte(cfg.SystemPrompt), 0o600); err != nil {
			return "", fmt.Errorf("write opencode system prompt: %w", err)
		}
	}
	data, err := renderOpenCodeConfig(cfg, systemPath)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write opencode config: %w", err)
	}
	return path, nil
}
