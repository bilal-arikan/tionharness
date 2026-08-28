package api

import (
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/settings"
)

// TestUpdateSettings_UnknownFieldRejected verifies PUT /api/settings rejects a
// patch carrying a field the settings.Patch DTO does not declare, rather than
// silently ignoring it.
func TestUpdateSettings_UnknownFieldRejected(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	rec := doJSON(t, h, http.MethodPut, "/api/settings", map[string]any{
		"theme":         "dark",
		"zzzBogusField": 1,
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "zzzBogusField") {
		t.Fatalf("expected error body to name the unknown field, got %s", rec.Body.String())
	}
}

// TestUpdateSettings_ValidPatchApplies verifies a valid patch returns 200 and
// the values are actually persisted into the settings store, not just echoed
// back in the response.
func TestUpdateSettings_ValidPatchApplies(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	rec := doJSON(t, h, http.MethodPut, "/api/settings", map[string]any{
		"theme":  "dark",
		"accent": "#8b5cf6",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}

	got := s.settings.Get()
	if got.Theme != "dark" {
		t.Fatalf("expected theme persisted as %q, got %q", "dark", got.Theme)
	}
	if got.Accent != "#8b5cf6" {
		t.Fatalf("expected accent persisted as %q, got %q", "#8b5cf6", got.Accent)
	}
}

func TestUpdateWorkspaceSettings_WorktreeLifecycleAppliesAndReturns(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	h := s.Routes()

	var got workspaceSettingsDTO
	rec := doJSON(t, h, http.MethodPut, "/api/workspace-settings", map[string]any{
		"worktreeBaseRef": "origin/release",
		"worktreeRootDir": `D:\tion-worktrees`,
	}, &got)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got.WorktreeBaseRef != "origin/release" || got.WorktreeRootDir != `D:\tion-worktrees` {
		t.Fatalf("response worktree settings = base %q root %q", got.WorktreeBaseRef, got.WorktreeRootDir)
	}
	persisted := wsp.Settings()
	if persisted.WorktreeBaseRef != "origin/release" || persisted.WorktreeRootDir != `D:\tion-worktrees` {
		t.Fatalf("persisted worktree settings = base %q root %q", persisted.WorktreeBaseRef, persisted.WorktreeRootDir)
	}
}

// TestTestProvider_UnknownFieldRejected verifies POST /api/settings/test-provider
// rejects a body carrying a field testProviderReq does not declare.
func TestTestProvider_UnknownFieldRejected(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	rec := doJSON(t, h, http.MethodPost, "/api/settings/test-provider", map[string]any{
		"provider":      "anthropic",
		"zzzBogusField": 1,
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d: %s", rec.Code, rec.Body.String())
	}
}

// settingsPatchGoldenFields is the golden list of settings.Patch json tags.
// The frontend's SettingsPatch type (frontend/src/features/settings) must
// carry exactly the same field set. If this test breaks after you add/remove
// a Patch field, update this list AND the frontend type in the same change.
var settingsPatchGoldenFields = []string{
	"theme", "accent", "themePreset", "language", "uiLanguage",
	"defaultPermissionMode", "claudeConfigDir", "claudeCliAuthKind",
	"claudeCliAuthToken", "anthropicKey",
	"extendedPromptCache", "anthropicContextEditing", "anthropicNativeToolSearch",
	"anthropicProgrammaticTools", "anthropicRefusalFallback", "anthropicWebTools",
	"anthropicServerCompaction", "autonomousTaskBudgetTokens",
	"desktopNotifications", "keepAwake",
	"userName", "userTimezone", "userCity", "userCountry", "userNotes",
	"maxContextTokens", "keepRecentMsgs",
	"contextBudgetCeil", "contextBudgetFraction",
	"handoffAuto", "handoffMaxChain", "handoffWriteFile",
	"progressPersist", "progressResume",
	"autonomousAutoContinue", "autonomousAutoContinueMax",
	"fileFreshnessGuard", "autoTagSessions",
	"debugJournalEnabled", "debugJournalCap",
	"reactiveCompact", "maxTokenRetries", "reactiveKeepRecent", "maxProviderRetries",
	"toolGuardWarnings", "toolGuardHardStop",
	"guardExactWarn", "guardExactBlock", "guardSameToolWarn", "guardSameToolHalt",
	"guardNoProgressWarn", "guardNoProgressBlock", "stuckTurnThreshold",
	"lessonReflect", "lessonMaxAgeDays", "maxOutputTokens",
	"autoTitleEnabled",
	"enableShell", "enableCliHooks", "enableCodeMode",
	"claudeResume", "claudePersistentSession", "claudeSysPromptFile",
	"delegationMaxDepth", "delegationMaxCalls",
	"spawnMaxConcurrent", "spawnQueueMax", "spawnMaxPerTurn", "spawnTimeoutMin", "spawnIdleTimeoutMin",
	"chatTurnTimeoutMin", "chatTurnIdleTimeoutMin", "codexStdoutIdleSec",
	"idleResumeMax", "scheduleTimeoutMin", "turnWatchdogMin", "turnIdleWatchdogMin",
	"shellDefaultTimeoutSec", "shellMaxTimeoutSec", "maxToolOutputKB", "agentMessageMaxKB",
	"coordinatorMaxWorkers", "coordinatorMaxTurns", "coordinatorMaxDepth",
	"coordinatorMaxSubtreeSessions", "coordinatorSettleGraceSec",
	"coordinatorStallGuard", "coordinatorStallSweepMin", "coordinatorStallMaxNudges",
	"autonomousConfine", "autonomousBootSeq",
	"backupEnabled", "backupIntervalHours", "backupRetain", "backupDir",
}

// TestSettingsPatch_JSONTagsMatchGoldenList guards against settings.Patch and
// the frontend SettingsPatch type silently drifting apart: it fails the moment
// a field is added to or removed from settings.Patch, so whoever changes the
// backend struct is forced to update this golden list (and, by the same
// change, the frontend type) rather than the two silently diverging.
func TestSettingsPatch_JSONTagsMatchGoldenList(t *testing.T) {
	tags := patchJSONTags(t)

	got := append([]string(nil), tags...)
	want := append([]string(nil), settingsPatchGoldenFields...)
	sort.Strings(got)
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("settings.Patch json tags drifted from the golden list.\ngot:  %v\nwant: %v\n\nUpdate settingsPatchGoldenFields (and the frontend SettingsPatch type) to match.", got, want)
	}
}

func patchJSONTags(t *testing.T) []string {
	t.Helper()
	typ := reflect.TypeOf(settings.Patch{})
	tags := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := f.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		tags = append(tags, name)
	}
	return tags
}
