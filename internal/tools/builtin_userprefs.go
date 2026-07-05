package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// UpdateUserPreferencesTool lets the agent persist durable facts about the USER
// into the app-wide user profile (Settings ▸ Profile): name, timezone, city,
// country and the free-form preference notes. That profile is injected into
// every turn's context ("About the user" block), so a saved preference reaches
// every agent in every workspace from the next turn on.
//
// It is a narrow, purpose-named wrapper over the same SettingsBridge that backs
// update_settings — the agent doesn't need to know the settings.json field names,
// and the tool only ever touches the five profile fields (it cannot flip an
// unrelated app setting by accident).
type UpdateUserPreferencesTool struct{ bridge SettingsBridge }

// NewUpdateUserPreferencesTool builds update_user_preferences over the bridge.
func NewUpdateUserPreferencesTool(b SettingsBridge) UpdateUserPreferencesTool {
	return UpdateUserPreferencesTool{bridge: b}
}

func (UpdateUserPreferencesTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "update_user_preferences",
		Description: "Save durable facts about the USER into their profile (the same one on the " +
			"Settings ▸ Profile screen), injected into every agent's context from the next turn on. " +
			"Use it when the user tells you their name, location, timezone, or a lasting preference " +
			"about how they want you to work. Pass only the fields to change. `notes` REPLACES the " +
			"free-form preference notes; `notes_append` adds a line to them instead (preferred for " +
			"incremental facts). Do not store secrets or transient task details here.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "name":         { "type": "string", "description": "The user's name." },
    "timezone":     { "type": "string", "description": "IANA timezone, e.g. Europe/Istanbul." },
    "city":         { "type": "string", "description": "The user's city." },
    "country":      { "type": "string", "description": "The user's country." },
    "notes":        { "type": "string", "description": "REPLACE the free-form preference notes with this text." },
    "notes_append": { "type": "string", "description": "Append this line to the existing preference notes (kept as-is)." }
  },
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"name":"Bilal","country":"Turkey"}`),
			json.RawMessage(`{"notes_append":"Prefers PowerShell syntax for terminal commands."}`),
		},
	}
}

func (t UpdateUserPreferencesTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.bridge == nil {
		return "", fmt.Errorf("settings bridge not configured")
	}
	var in struct {
		Name        *string `json:"name"`
		Timezone    *string `json:"timezone"`
		City        *string `json:"city"`
		Country     *string `json:"country"`
		Notes       *string `json:"notes"`
		NotesAppend *string `json:"notes_append"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("update_user_preferences", err)
	}
	if in.Notes != nil && in.NotesAppend != nil {
		return "", fmt.Errorf("pass either notes (replace) or notes_append (add a line), not both")
	}

	patch := map[string]string{}
	set := func(key string, v *string) {
		if v != nil {
			patch[key] = strings.TrimSpace(*v)
		}
	}
	set("userName", in.Name)
	set("userTimezone", in.Timezone)
	set("userCity", in.City)
	set("userCountry", in.Country)
	set("userNotes", in.Notes)
	if in.NotesAppend != nil {
		add := strings.TrimSpace(*in.NotesAppend)
		if add == "" {
			return "", fmt.Errorf("notes_append is empty")
		}
		cur, err := t.currentNotes()
		if err != nil {
			return "", err
		}
		if cur == "" {
			patch["userNotes"] = add
		} else {
			patch["userNotes"] = cur + "\n" + add
		}
	}
	if len(patch) == 0 {
		return "", fmt.Errorf("nothing to update: pass at least one of name/timezone/city/country/notes/notes_append")
	}

	raw, err := json.Marshal(patch)
	if err != nil {
		return "", err
	}
	if _, err := t.bridge.Apply(string(raw)); err != nil {
		return "", err
	}
	keys := make([]string, 0, len(patch))
	for k := range patch {
		keys = append(keys, k)
	}
	return "user preferences saved (" + strings.Join(keys, ", ") + ") — active for every agent from the next turn", nil
}

// currentNotes reads the existing userNotes out of the masked settings snapshot
// so notes_append can extend rather than clobber them.
func (t UpdateUserPreferencesTool) currentNotes() (string, error) {
	snap, err := t.bridge.Snapshot()
	if err != nil {
		return "", err
	}
	var view struct {
		UserNotes string `json:"userNotes"`
	}
	if err := json.Unmarshal([]byte(snap), &view); err != nil {
		return "", fmt.Errorf("cannot parse settings snapshot: %w", err)
	}
	return strings.TrimSpace(view.UserNotes), nil
}
