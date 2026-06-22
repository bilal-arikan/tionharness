package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// Config tools let an agent read and edit its OWN workspace configuration —
// the editable files under <workspace>/config/ (runtime prompts, instructions,
// README) — separately from the regular workspace files. They are bound to a
// sandbox rooted at the config directory, so paths can never escape it.
//
// Editing config/prompts/{summary,reflect,title}.md changes how the runtime
// summarizes / reflects / titles for this workspace on the next call; editing
// instructions.md takes effect on the next workspace load (the file is the
// source of truth there).

const cfgReadMaxBytes = 256 * 1024

// ConfigReadTool reads a file from the workspace config directory.
type ConfigReadTool struct{ sb Sandbox }

// NewConfigReadTool binds read_config to a config-rooted sandbox.
func NewConfigReadTool(sb Sandbox) ConfigReadTool { return ConfigReadTool{sb: sb} }

func (ConfigReadTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "read_config",
		Description: "Read a workspace config file (under config/: prompts/summary.md, prompts/reflect.md, prompts/title.md, instructions.md, README.md). Paths are relative to the config folder.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"path":{"type":"string","description":"File path relative to the config folder, e.g. prompts/summary.md"}},
			"required":["path"],
			"additionalProperties":false
		}`),
	}
}

func (t ConfigReadTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	abs, err := t.sb.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory; use list_config", args.Path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if len(data) > cfgReadMaxBytes {
		return string(data[:cfgReadMaxBytes]) + "\n\n[truncated at 256KB]", nil
	}
	return string(data), nil
}

// ConfigWriteTool creates or overwrites a file in the workspace config directory.
type ConfigWriteTool struct{ sb Sandbox }

// NewConfigWriteTool binds write_config to a config-rooted sandbox.
func NewConfigWriteTool(sb Sandbox) ConfigWriteTool { return ConfigWriteTool{sb: sb} }

func (ConfigWriteTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "write_config",
		Description: "Create or overwrite a workspace config file (under config/). Use this to edit your own runtime prompts (prompts/summary.md, prompts/reflect.md, prompts/title.md), the workspace instructions (instructions.md) or notes (README.md). A blank prompt file makes the runtime use its built-in default. Paths are relative to the config folder.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"path":{"type":"string","description":"File path relative to the config folder, e.g. prompts/title.md"},
				"content":{"type":"string","description":"Full file content to write"}
			},
			"required":["path","content"],
			"additionalProperties":false
		}`),
	}
}

func (t ConfigWriteTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	abs, err := t.sb.Resolve(args.Path)
	if err != nil {
		return "", err
	}
	old, statErr := os.ReadFile(abs)
	created := statErr != nil
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(args.Content), 0o644); err != nil {
		return "", err
	}
	// Surface a diff card like the regular file tools.
	added, removed, patch := lineDiff(string(old), args.Content)
	recordDiff(ctx, FileDiff{Path: "config/" + t.sb.Rel(abs), Added: added, Removed: removed, Patch: patch, Created: created})
	return fmt.Sprintf("Wrote %d bytes to config/%s", len(args.Content), t.sb.Rel(abs)), nil
}

// ConfigListTool lists the files in the workspace config directory.
type ConfigListTool struct{ sb Sandbox }

// NewConfigListTool binds list_config to a config-rooted sandbox.
func NewConfigListTool(sb Sandbox) ConfigListTool { return ConfigListTool{sb: sb} }

func (ConfigListTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "list_config",
		Description: "List the editable files in this workspace's config/ folder (prompts, instructions, README).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ConfigListTool) Call(_ context.Context, _ json.RawMessage) (string, error) {
	root, err := t.sb.Resolve(".")
	if err != nil {
		return "", err
	}
	var files []string
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			rel, rErr := filepath.Rel(root, p)
			if rErr == nil {
				files = append(files, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if len(files) == 0 {
		return "(config folder is empty)", nil
	}
	sort.Strings(files)
	return strings.Join(files, "\n"), nil
}
