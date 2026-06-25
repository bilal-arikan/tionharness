package market

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallSkillWritesNestedFiles(t *testing.T) {
	dir := t.TempDir()
	p := Pack{
		Schema: SchemaV1, ID: "skill.demo", Kind: KindSkill,
		Payload: Payload{Skill: &SkillPayload{Slug: "demo", Body: "---\nname: Demo\n---\nbody"}},
		Files: map[string][]byte{
			"references/guide.md": []byte("ref"),
			"scripts/run.sh":      []byte("echo"),
			"../escape.txt":       []byte("nope"), // traversal must be refused
			"SKILL.md":            []byte("ignored"),
		},
	}
	res, err := InstallSkill(p, dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Ref != "demo" {
		t.Errorf("ref = %q", res.Ref)
	}
	for _, rel := range []string{"SKILL.md", "references/guide.md", "scripts/run.sh"} {
		if _, err := os.Stat(filepath.Join(dir, "demo", filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %q written: %v", rel, err)
		}
	}
	// Traversal target must NOT exist outside the skill folder.
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); err == nil {
		t.Errorf("path traversal was not refused")
	}
}

func TestBuildSkillPackCarriesFiles(t *testing.T) {
	files := map[string][]byte{"references/r.md": []byte("x")}
	p, err := BuildSkillPack("s", "S", "d", "", "", "---\nname: S\n---\nb", "me", 0, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Files) != 1 || string(p.Files["references/r.md"]) != "x" {
		t.Errorf("files not carried: %v", p.Files)
	}
	// Empty files map normalises to nil (omitempty in JSON).
	p2, _ := BuildSkillPack("s", "S", "d", "", "", "body", "me", 0, map[string][]byte{})
	if p2.Files != nil {
		t.Errorf("empty files should be nil, got %v", p2.Files)
	}
}
