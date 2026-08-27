package workspace

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// TestDefaultWSSettings pins the always-on defaults (cross-session awareness is
// no longer configurable; codebase-memory defaults on).
func TestDefaultWSSettings(t *testing.T) {
	d := defaultWSSettings()
	if !d.CodebaseMemoryEnabled {
		t.Error("codebase-memory capability should default on")
	}
	if d.DesktopNotifications != nil {
		t.Error("desktopNotifications should default to nil (inherit the global toggle)")
	}
	if d.WorktreeBaseRef != "" || d.WorktreeRootDir != "" {
		t.Fatalf("worktree lifecycle defaults = base %q root %q, want empty fallbacks", d.WorktreeBaseRef, d.WorktreeRootDir)
	}
}

func TestWSSettingsWorktreeLifecycleJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	written := &Workspace{DataDir: dir}
	written.settings.cur = defaultWSSettings()
	written.settings.cur.WorktreeBaseRef = "origin/develop"
	written.settings.cur.WorktreeRootDir = `C:\worktrees\project`
	if err := written.saveSettings(); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	data, err := os.ReadFile(written.settingsPath())
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	for _, field := range []string{`"worktreeBaseRef": "origin/develop"`, `"worktreeRootDir": "C:\\worktrees\\project"`} {
		if !bytes.Contains(data, []byte(field)) {
			t.Fatalf("persisted settings missing %s: %s", field, data)
		}
	}

	loaded := &Workspace{DataDir: dir}
	loaded.loadSettings()
	got := loaded.Settings()
	if got.WorktreeBaseRef != "origin/develop" || got.WorktreeRootDir != `C:\worktrees\project` {
		t.Fatalf("reloaded worktree settings = base %q root %q", got.WorktreeBaseRef, got.WorktreeRootDir)
	}
}

// TestWSSettingsPatchDesktopNotifications verifies the three-state parse of the
// wire "inherit"|"on"|"off" string onto the DesktopNotifications **bool:
//   - absent key      -> patch nil (leave unchanged)
//   - "inherit"       -> non-nil pointer to a nil *bool (clear the override)
//   - "on"/"off"      -> pointer to a *bool holding true/false
//   - unknown value   -> error
func TestWSSettingsPatchDesktopNotifications(t *testing.T) {
	// Absent key: DesktopNotifications must stay nil so the field is left unchanged.
	var absent WSSettingsPatch
	if err := json.Unmarshal([]byte(`{"instructions":"x"}`), &absent); err != nil {
		t.Fatalf("absent-key unmarshal failed: %v", err)
	}
	if absent.DesktopNotifications != nil {
		t.Error("absent desktopNotifications should leave the patch field nil")
	}
	if absent.Instructions == nil || *absent.Instructions != "x" {
		t.Error("sibling fields should still decode via the shadow type")
	}

	// "inherit": non-nil outer pointer, nil inner pointer (clear the override).
	var inherit WSSettingsPatch
	if err := json.Unmarshal([]byte(`{"desktopNotifications":"inherit"}`), &inherit); err != nil {
		t.Fatalf("inherit unmarshal failed: %v", err)
	}
	if inherit.DesktopNotifications == nil {
		t.Fatal(`"inherit" should set a non-nil outer pointer (clear the override)`)
	}
	if *inherit.DesktopNotifications != nil {
		t.Error(`"inherit" should leave the inner *bool nil`)
	}

	// "on"/"off": pointer to a *bool holding true/false.
	for _, tc := range []struct {
		wire string
		want bool
	}{{"on", true}, {"off", false}} {
		var p WSSettingsPatch
		if err := json.Unmarshal([]byte(`{"desktopNotifications":"`+tc.wire+`"}`), &p); err != nil {
			t.Fatalf("%q unmarshal failed: %v", tc.wire, err)
		}
		if p.DesktopNotifications == nil || *p.DesktopNotifications == nil {
			t.Fatalf("%q should yield a non-nil bool override", tc.wire)
		}
		if got := **p.DesktopNotifications; got != tc.want {
			t.Errorf("%q => %v, want %v", tc.wire, got, tc.want)
		}
	}

	// Unknown value must be rejected, not silently ignored.
	var bad WSSettingsPatch
	if err := json.Unmarshal([]byte(`{"desktopNotifications":"maybe"}`), &bad); err == nil {
		t.Error("an unrecognised desktopNotifications value should error")
	}
}
