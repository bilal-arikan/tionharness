package db

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeAgentToolOverrides(t *testing.T) {
	for _, tt := range []struct {
		name      string
		input     Agent
		overrides string
		blocked   string
	}{
		{"legacy", Agent{BlockedTools: `["Write","Bash","Write"]`}, `{"Bash":"blocked","Write":"blocked"}`, `["Bash","Write"]`},
		{"mixed", Agent{ToolOverrides: `{"Bash":"hidden","Read":"summary"}`, BlockedTools: `["Bash","Write"]`}, `{"Bash":"hidden","Read":"summary","Write":"blocked"}`, `["Write"]`},
		{"empty", Agent{}, `{}`, `[]`},
		{"null", Agent{ToolOverrides: "null", BlockedTools: "null"}, `{}`, `[]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeAgentToolOverrides(tt.input)
			if got.ToolOverrides != tt.overrides || got.BlockedTools != tt.blocked {
				t.Fatalf("normalized = %s / %s, want %s / %s", got.ToolOverrides, got.BlockedTools, tt.overrides, tt.blocked)
			}
			if second := normalizeAgentToolOverrides(got); !reflect.DeepEqual(second, got) {
				t.Fatalf("normalization is not idempotent: %+v", second)
			}
		})
	}
}

func TestNormalizeAgentToolOverridesPreservesMalformedPermissions(t *testing.T) {
	for _, input := range []Agent{
		{ToolOverrides: `{"Read":`, BlockedTools: `["Bash"]`},
		{ToolOverrides: `{"Read":"full"}`, BlockedTools: `["Bash"`},
		{ToolOverrides: `{"Read":"unknown","Write":"hidden"}`, BlockedTools: `["Bash"]`},
	} {
		if got := normalizeAgentToolOverrides(input); !reflect.DeepEqual(got, input) {
			t.Fatalf("corrupt permissions were rewritten: %+v", got)
		}
		if _, err := ParseAgentToolOverrides(input); err == nil {
			t.Fatalf("corrupt permissions lost their error: %+v", input)
		}
	}
}

func TestAgentToolOverridesLegacyLoadAndSave(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, dirAgents, "AGT1.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := Agent{ID: "AGT1", Name: "Legacy", ToolOverrides: `{"Bash":"hidden"}`, BlockedTools: `["Bash","Write"]`}
	body, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	loaded, err := d.GetAgent(ctx, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ToolOverrides != `{"Bash":"hidden","Write":"blocked"}` || loaded.BlockedTools != `["Write"]` {
		t.Fatalf("old permissions were not normalized at load: %+v", loaded)
	}
	afterLoad, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterLoad) != string(body) {
		t.Fatal("opening a store rewrote an existing agent file")
	}
	name := "Renamed"
	if _, err := d.UpdateAgent(ctx, legacy.ID, AgentProfilePatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk Agent
	if err := json.Unmarshal(saved, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.ToolOverrides != loaded.ToolOverrides || onDisk.BlockedTools != loaded.BlockedTools {
		t.Fatalf("explicit save did not persist canonical permissions: %+v", onDisk)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := reopened.GetAgent(ctx, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := ParseAgentToolOverrides(legacy)
	after, parseErr := ParseAgentToolOverrides(got)
	if parseErr != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("permissions changed after save/reopen: %v / %v", after, parseErr)
	}
}

func TestCreateAgentCanonicalizesLegacyTools(t *testing.T) {
	d, ctx := openInheritDB(t)
	t.Cleanup(func() { _ = d.Close() })
	a, err := d.CreateAgent(ctx, Agent{Name: "Imported", BlockedTools: `["Write","Bash"]`})
	if err != nil {
		t.Fatal(err)
	}
	if a.ToolOverrides != `{"Bash":"blocked","Write":"blocked"}` || a.BlockedTools != `["Bash","Write"]` {
		t.Fatalf("new legacy import was not canonicalized: %+v", a)
	}
	if err := d.UpdateAgentTools(ctx, a.ID, true, `{"Write":"full","Bash":"blocked"}`); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetAgent(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ToolOverrides != `{"Bash":"blocked","Write":"full"}` || got.BlockedTools != `["Bash"]` {
		t.Fatalf("tool update did not replace old restrictions: %+v", got)
	}
}

func TestMalformedAgentToolOverridesSurviveStoreRoundTrip(t *testing.T) {
	d, ctx := openInheritDB(t)
	t.Cleanup(func() { _ = d.Close() })
	input := Agent{Name: "Corrupt", ToolOverrides: `{"Read":"unknown","Write":"full"}`, BlockedTools: `["Bash"`}
	a, err := d.CreateAgent(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	name := "Renamed"
	if _, err := d.UpdateAgent(ctx, a.ID, AgentProfilePatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(d.Root())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := reopened.GetAgent(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ToolOverrides != input.ToolOverrides || got.BlockedTools != input.BlockedTools {
		t.Fatalf("store normalization repaired corrupt permissions silently: %+v", got)
	}
	partial, parseErr := ParseAgentToolOverrides(got)
	if parseErr == nil || partial["Write"] != "full" {
		t.Fatalf("partial display results / permission error were not preserved: %v / %v", partial, parseErr)
	}
}
