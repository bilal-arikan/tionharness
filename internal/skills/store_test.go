package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkill creates <dir>/<slug>/SKILL.md with the given content.
func writeSkill(t *testing.T, dir, slug, content string) {
	t.Helper()
	d := filepath.Join(dir, slug)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseFrontmatter(t *testing.T) {
	fm, body := parseFrontmatter(`---
name: "My Skill"
description: Does a thing
when_to_use: when X happens
alwaysAllow: ["Bash", "Read"]
requiredSources:
  - mcp-gateway
  - linear
---
# Body
Hello world.`)

	if got := fm.scalar("name"); got != "My Skill" {
		t.Errorf("name = %q", got)
	}
	if got := fm.scalar("description"); got != "Does a thing" {
		t.Errorf("description = %q", got)
	}
	if got := fm.scalar("when_to_use", "when"); got != "when X happens" {
		t.Errorf("when_to_use = %q", got)
	}
	if got := fm.list("alwaysallow"); len(got) != 2 || got[0] != "Bash" || got[1] != "Read" {
		t.Errorf("alwaysAllow = %v", got)
	}
	if got := fm.list("requiredsources"); len(got) != 2 || got[1] != "linear" {
		t.Errorf("requiredSources = %v", got)
	}
	if body != "# Body\nHello world." {
		t.Errorf("body = %q", body)
	}
}

func TestStoreTierOverride(t *testing.T) {
	global := t.TempDir()
	ws := t.TempDir()
	proj := t.TempDir()

	writeSkill(t, global, "commit", "---\nname: Global Commit\ndescription: global\n---\nGLOBAL BODY")
	writeSkill(t, global, "only-global", "---\nname: Only Global\ndescription: g\n---\nbody")
	// Same slug in workspace overrides global.
	writeSkill(t, ws, "commit", "---\nname: WS Commit\ndescription: ws\n---\nWS BODY")
	// Same slug in project overrides everything.
	writeSkill(t, proj, "commit", "---\nname: Proj Commit\ndescription: proj\n---\nPROJECT BODY")

	s := New(global, ws, proj)

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("want 2 skills, got %d: %+v", len(list), list)
	}

	sk, ok := s.Get("commit")
	if !ok {
		t.Fatal("commit not found")
	}
	if sk.Source != SourceProject || sk.Name != "Proj Commit" {
		t.Errorf("override failed: source=%s name=%q", sk.Source, sk.Name)
	}
	body, err := s.Body("commit")
	if err != nil {
		t.Fatal(err)
	}
	if body != "PROJECT BODY" {
		t.Errorf("body = %q (want project tier)", body)
	}

	if og, _ := s.Get("only-global"); og.Source != SourceGlobal {
		t.Errorf("only-global source = %s", og.Source)
	}
}

func TestCatalogBlockAndEmpty(t *testing.T) {
	empty := New("", "", t.TempDir())
	if !empty.Empty() {
		t.Error("expected empty store")
	}
	if empty.CatalogBlock() != "" {
		t.Error("expected empty catalog block")
	}

	dir := t.TempDir()
	writeSkill(t, dir, "triage", "---\nname: Triage\ndescription: sort issues\nwhen_to_use: on new issues\n---\nbody")
	s := New("", "", dir)
	block := s.CatalogBlock()
	if block == "" {
		t.Fatal("expected non-empty catalog block")
	}
	for _, want := range []string{"Available Skills", "`triage`", "sort issues", "on new issues", "use_skill"} {
		if !contains(block, want) {
			t.Errorf("catalog block missing %q:\n%s", want, block)
		}
	}
}

func TestCatalogBlockFor(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "alpha", "---\nname: Alpha\ndescription: a\n---\nbody")
	writeSkill(t, dir, "beta", "---\nname: Beta\ndescription: b\n---\nbody")
	writeSkill(t, dir, "gamma", "---\nname: Gamma\ndescription: g\n---\nbody")
	s := New("", "", dir)

	// Only selected slugs, in the given order; unknown/dup skipped.
	block := s.CatalogBlockFor([]string{"gamma", "alpha", "nope", "gamma"})
	ai := indexOf(block, "`alpha`")
	gi := indexOf(block, "`gamma`")
	if gi < 0 || ai < 0 {
		t.Fatalf("expected alpha+gamma in block:\n%s", block)
	}
	if gi > ai {
		t.Errorf("order not preserved: gamma should precede alpha\n%s", block)
	}
	if indexOf(block, "`beta`") >= 0 {
		t.Errorf("beta should not be listed (not selected)\n%s", block)
	}
	if indexOf(block, "nope") >= 0 {
		t.Errorf("unknown slug leaked into block\n%s", block)
	}

	if s.CatalogBlockFor(nil) != "" || s.CatalogBlockFor([]string{"nope"}) != "" {
		t.Error("expected empty block for empty/unknown selection")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
