package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// Settings tools let an agent inspect and change the APPLICATION-WIDE settings —
// the single settings.json document that backs the Settings screen (theme,
// providers, context/memory budgets, autonomy, gated tool capabilities, ...).
//
// Unlike read_config/write_config (which edit per-workspace config FILES), these
// tools go through the live settings store: a change is persisted to the file
// AND pushed into every running subsystem immediately (no restart). Secret keys
// are never returned — the snapshot is the masked client view.

// SettingsBridge is the seam between the agent tools and the application's live
// settings store. It is implemented in the api layer (where the store and the
// "apply to live subsystems" hook both live) and injected into the runtime.
type SettingsBridge interface {
	// Path returns the settings.json file path on disk.
	Path() string
	// Snapshot returns the current settings as masked, pretty-printed JSON
	// (the same shape accepted by Apply's patch).
	Snapshot() (string, error)
	// Apply merges a partial JSON patch into the settings, persists it, and
	// re-applies the result to every live subsystem. It returns the new masked
	// snapshot. Only the fields present in the patch are changed.
	Apply(patchJSON string) (string, error)
}

// GetSettingsTool reads the live application settings.
type GetSettingsTool struct{ bridge SettingsBridge }

// NewGetSettingsTool builds get_settings over the given bridge.
func NewGetSettingsTool(b SettingsBridge) GetSettingsTool { return GetSettingsTool{bridge: b} }

func (GetSettingsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "get_settings",
		Description: "Read the live application-wide settings (the settings.json document behind the Settings screen): theme/appearance, providers & default model, context/memory budgets, autonomy, tool-output compaction and the gated tool capabilities (shell / self-management / delegation / spawn). Secret API keys are masked (you only see whether a key is set). Returns the settings file path plus the current values as JSON — use the field names here as the keys for update_settings.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t GetSettingsTool) Call(_ context.Context, _ json.RawMessage) (string, error) {
	if t.bridge == nil {
		return "", fmt.Errorf("settings bridge not configured")
	}
	snap, err := t.bridge.Snapshot()
	if err != nil {
		return "", err
	}
	return "Settings file: " + t.bridge.Path() + "\n\n" + snap, nil
}

// UpdateSettingsTool applies a partial patch to the live application settings.
type UpdateSettingsTool struct{ bridge SettingsBridge }

// NewUpdateSettingsTool builds update_settings over the given bridge.
func NewUpdateSettingsTool(b SettingsBridge) UpdateSettingsTool { return UpdateSettingsTool{bridge: b} }

func (UpdateSettingsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "update_settings",
		Description: "Change one or more application-wide settings and ACTIVATE them immediately (persisted to settings.json AND pushed into the live runtime — no restart). Pass a `patch` object containing only the fields you want to change, using the exact field names from get_settings (e.g. {\"theme\":\"light\"}, {\"autoTitleEnabled\":false}, {\"enableShell\":true}, {\"defaultModel\":\"claude-sonnet-4-6\"}). Numeric fields are clamped to safe ranges. Write-only secret fields are accepted: `anthropicKey` / `minimaxKey` (\"\" clears). Always read get_settings first so you patch the right keys. Returns the new masked settings.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"patch":{"type":"object","description":"Partial settings object: only the fields to change, keyed by their get_settings names.","additionalProperties":true}
			},
			"required":["patch"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"patch":{"theme":"light"}}`),
			json.RawMessage(`{"patch":{"defaultModel":"claude-sonnet-4-6","autoTitleEnabled":false}}`),
			json.RawMessage(`{"patch":{"enableShell":true,"enableCliHooks":true}}`),
		},
	}
}

func (t UpdateSettingsTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.bridge == nil {
		return "", fmt.Errorf("settings bridge not configured")
	}
	var args struct {
		Patch json.RawMessage `json:"patch"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if len(strings.TrimSpace(string(args.Patch))) == 0 || string(args.Patch) == "null" {
		return "", fmt.Errorf("patch is required (an object of fields to change)")
	}
	snap, err := t.bridge.Apply(string(args.Patch))
	if err != nil {
		return "", err
	}
	return "Settings updated and activated. New settings:\n\n" + snap, nil
}
