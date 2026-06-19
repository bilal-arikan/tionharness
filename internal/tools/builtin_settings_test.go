package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeSettingsBridge is an in-memory SettingsBridge for testing the tools
// without the api/settings layer.
type fakeSettingsBridge struct {
	path  string
	state map[string]any
}

func (f *fakeSettingsBridge) Path() string { return f.path }

func (f *fakeSettingsBridge) Snapshot() (string, error) {
	data, err := json.MarshalIndent(f.state, "", "  ")
	return string(data), err
}

func (f *fakeSettingsBridge) Apply(patchJSON string) (string, error) {
	var patch map[string]any
	if err := json.Unmarshal([]byte(patchJSON), &patch); err != nil {
		return "", err
	}
	for k, v := range patch {
		f.state[k] = v
	}
	return f.Snapshot()
}

func TestGetSettingsReportsPath(t *testing.T) {
	b := &fakeSettingsBridge{path: "/data/settings.json", state: map[string]any{"theme": "dark"}}
	out, err := NewGetSettingsTool(b).Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("get_settings: %v", err)
	}
	if !strings.Contains(out, "/data/settings.json") {
		t.Fatalf("expected file path in output, got: %s", out)
	}
	if !strings.Contains(out, `"theme": "dark"`) {
		t.Fatalf("expected current settings in output, got: %s", out)
	}
}

func TestUpdateSettingsAppliesPatch(t *testing.T) {
	b := &fakeSettingsBridge{path: "/data/settings.json", state: map[string]any{"theme": "dark", "pauseAutonomy": false}}
	out, err := NewUpdateSettingsTool(b).Call(context.Background(), json.RawMessage(`{"patch":{"theme":"light","pauseAutonomy":true}}`))
	if err != nil {
		t.Fatalf("update_settings: %v", err)
	}
	if b.state["theme"] != "light" {
		t.Fatalf("theme not applied: %v", b.state["theme"])
	}
	if b.state["pauseAutonomy"] != true {
		t.Fatalf("pauseAutonomy not applied: %v", b.state["pauseAutonomy"])
	}
	if !strings.Contains(out, "activated") {
		t.Fatalf("expected confirmation in output, got: %s", out)
	}
}

func TestUpdateSettingsRejectsEmptyPatch(t *testing.T) {
	b := &fakeSettingsBridge{path: "/data/settings.json", state: map[string]any{}}
	if _, err := NewUpdateSettingsTool(b).Call(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for missing patch")
	}
}
