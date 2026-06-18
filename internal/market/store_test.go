package market

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultsAndInstall covers the MVP slice end-to-end at the package level:
// seed bundled packs → list → get payload → install a skill → publish a new one.
func TestDefaultsAndInstall(t *testing.T) {
	global := t.TempDir()
	workspace := t.TempDir()
	if err := EnsureDefaults(global); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	st := New(global, workspace)
	list := st.List()
	if len(list) < 2 {
		t.Fatalf("expected >=2 bundled packs, got %d", len(list))
	}
	for _, p := range list {
		if p.Payload.Skill != nil || p.Payload.Agent != nil {
			t.Errorf("catalog pack %q should not carry payload", p.ID)
		}
	}
	if got := st.ListKind(KindSkill); len(got) != len(list) {
		t.Errorf("ListKind(skill)=%d, want %d (all bundled are skills)", len(got), len(list))
	}

	// Get loads the payload lazily.
	pack, ok := st.Get("skill.web-research")
	if !ok {
		t.Fatal("get skill.web-research: not found")
	}
	if pack.Payload.Skill == nil || pack.Payload.Skill.Body == "" {
		t.Fatal("payload skill body empty after Get")
	}

	// Install writes SKILL.md under the workspace skills dir.
	skillsDir := filepath.Join(workspace, "skills")
	res, err := InstallSkill(pack, skillsDir, false)
	if err != nil {
		t.Fatalf("InstallSkill: %v", err)
	}
	dest := filepath.Join(skillsDir, "web-research", "SKILL.md")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}
	if res.Ref != "web-research" {
		t.Errorf("install ref=%q, want web-research", res.Ref)
	}

	// Re-install without overwrite must fail; with overwrite must succeed.
	if _, err := InstallSkill(pack, skillsDir, false); err == nil {
		t.Error("expected conflict on re-install without overwrite")
	}
	if _, err := InstallSkill(pack, skillsDir, true); err != nil {
		t.Errorf("overwrite install failed: %v", err)
	}

	// Publish a fresh skill pack into the workspace tier, then resolve it.
	body := "---\nname: \"Mine\"\ndescription: \"x\"\n---\n# Mine\n"
	np, err := BuildSkillPack("my-skill", "Mine", "x", "", "", body, "tester", 0)
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
	if got.Source != SourceWorkspace {
		t.Errorf("published pack source=%q, want workspace", got.Source)
	}
}
