package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// configValidateInput is the ask shape for the config_validate tool.
type configValidateInput struct {
	Path string `json:"path"`
}

// ConfigValidateTool validates a TionSwarm JSON config file: that it is well-formed
// JSON and, for recognised file shapes (settings.json, tools-config.json,
// agent/mcp-server records), that the expected top-level fields are present. A
// malformed config file is the classic cause of a silent load failure, so this
// gives an agent a fast pre-write/post-write check. Read-only; rooted at the
// turn's working-directory sandbox.
type ConfigValidateTool struct{ sb Sandbox }

// NewConfigValidateTool constructs the config_validate tool over a sandbox.
func NewConfigValidateTool(sb Sandbox) ConfigValidateTool { return ConfigValidateTool{sb: sb} }

func (ConfigValidateTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "config_validate",
		Description: "Validate a TionSwarm JSON config file (well-formed JSON + expected fields for known " +
			"shapes like settings.json / tools-config.json / agent / mcp-server records). Use before or after " +
			"editing a config to catch malformed JSON that would silently break loading. Read-only.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "Path to the JSON config file (relative to the working directory, or absolute within it)." }
  },
  "required": ["path"],
  "additionalProperties": false
}`),
	}
}

func (t ConfigValidateTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if !t.sb.Ready() {
		return "", fmt.Errorf("config validation is not available (no working directory)")
	}
	in, err := parseInput[configValidateInput]("config_validate", input)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(in.Path) == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := t.sb.Resolve(in.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("INVALID — file not found: %s", in.Path), nil
		}
		return "", fmt.Errorf("read %s: %w", in.Path, err)
	}

	base := strings.ToLower(filepath.Base(abs))
	if !strings.HasSuffix(base, ".json") {
		return fmt.Sprintf("SKIPPED — %s is not a .json file; config_validate only checks JSON configs.", in.Path), nil
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Sprintf("INVALID — %s is not well-formed JSON: %s", in.Path, err.Error()), nil
	}

	var warns []string
	switch {
	case base == "settings.json":
		warns = requireKeys(doc, "defaultProvider")
	case base == "tools-config.json":
		warns = requireKeys(doc, "disabledTools")
	case hasAny(doc, "id", "name") && hasAny(doc, "provider", "model", "soul"):
		// agent record
		warns = requireKeys(doc, "id")
	case hasAny(doc, "transport", "command", "url") && hasAny(doc, "name"):
		// mcp-server record
		warns = requireKeys(doc, "name")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "VALID — %s is well-formed JSON (%d top-level keys).", in.Path, len(doc))
	for _, w := range warns {
		fmt.Fprintf(&b, "\n(warning) %s", w)
	}
	return b.String(), nil
}

func requireKeys(doc map[string]json.RawMessage, keys ...string) []string {
	var warns []string
	for _, k := range keys {
		if _, ok := doc[k]; !ok {
			warns = append(warns, fmt.Sprintf("expected field %q is missing for this config shape", k))
		}
	}
	return warns
}

func hasAny(doc map[string]json.RawMessage, keys ...string) bool {
	for _, k := range keys {
		if _, ok := doc[k]; ok {
			return true
		}
	}
	return false
}
