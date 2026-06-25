package market

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writePack writes a pack file into a market dir for test setup.
func writePack(t *testing.T, dir string, p Pack) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, p.ID+packFileSuffix), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// TestStoreInstallPublish covers the package end-to-end against the single global
// tier: seed a pack in the global dir → list → get payload → install a skill →
// publish a new one. There is no bundled/workspace tier any more.
func TestStoreInstallPublish(t *testing.T) {
	global := t.TempDir()
	workspace := t.TempDir() // ledger dir

	body := "---\nname: \"Web Research\"\ndescription: \"x\"\n---\n# Web Research\n"
	writePack(t, global, Pack{
		Schema: SchemaV1, ID: "skill.web-research", Kind: KindSkill,
		Name: "Web Research", Version: "1.0.0",
		Payload: Payload{Skill: &SkillPayload{Slug: "web-research", Body: body}},
	})

	st := New(global, workspace)
	list := st.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 pack, got %d", len(list))
	}
	for _, p := range list {
		if p.Payload.Skill != nil {
			t.Errorf("catalog pack %q should not carry payload", p.ID)
		}
		if p.Source != SourceGlobal {
			t.Errorf("pack source=%q, want global", p.Source)
		}
	}

	// Get loads the payload lazily.
	pack, ok := st.Get("skill.web-research")
	if !ok {
		t.Fatal("get skill.web-research: not found")
	}
	if pack.Payload.Skill == nil || pack.Payload.Skill.Body == "" {
		t.Fatal("payload skill body empty after Get")
	}

	// Install writes SKILL.md under the (separate) workspace skills dir.
	skillsDir := filepath.Join(workspace, "skills")
	res, err := InstallSkill(pack, skillsDir, false)
	if err != nil {
		t.Fatalf("InstallSkill: %v", err)
	}
	if res.Ref != "web-research" {
		t.Errorf("install ref=%q, want web-research", res.Ref)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "web-research", "SKILL.md")); err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}

	// Re-install without overwrite must fail; with overwrite must succeed.
	if _, err := InstallSkill(pack, skillsDir, false); err == nil {
		t.Error("expected conflict on re-install without overwrite")
	}
	if _, err := InstallSkill(pack, skillsDir, true); err != nil {
		t.Errorf("overwrite install failed: %v", err)
	}

	// Publish a fresh skill pack — it lands in the global dir now (no workspace tier).
	np, err := BuildSkillPack("my-skill", "Mine", "x", "", "", "---\nname: \"Mine\"\n---\n# Mine\n", "tester", 0, nil)
	if err != nil {
		t.Fatalf("BuildSkillPack: %v", err)
	}
	if _, err := st.Publish(np); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	got, ok := st.Get("skill.my-skill")
	if !ok {
		t.Fatal("published pack not resolvable")
	}
	if got.Source != SourceGlobal {
		t.Errorf("published pack source=%q, want global", got.Source)
	}
}

// TestInstallLedger covers the per-workspace install ledger + update detection.
func TestInstallLedger(t *testing.T) {
	global := t.TempDir()
	workspace := t.TempDir()
	st := New(global, workspace)

	if st.UpdateAvailable("skill.x", "2.0.0") {
		t.Error("update should not be available before any install")
	}
	st.RecordInstall("skill.x", "1.0.0")
	if got := st.InstalledVersions()["skill.x"]; got != "1.0.0" {
		t.Errorf("ledger version=%q, want 1.0.0", got)
	}
	if !st.UpdateAvailable("skill.x", "2.0.0") {
		t.Error("2.0.0 should be an update over installed 1.0.0")
	}
	if st.UpdateAvailable("skill.x", "1.0.0") {
		t.Error("same version is not an update")
	}
}

// TestCompareVersions covers the semver-lite comparison.
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.9.9", 1},
		{"1.2.0", "1.10.0", -1},
		{"v1.0", "1.0.0", 0},
		{"1.0.0-beta", "1.0.0", 0},
		{"", "0.0.1", -1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
	}
}
