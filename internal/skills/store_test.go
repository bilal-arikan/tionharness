package skills

import (
	"os"
	"path/filepath"
	"strings"
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

	writeSkill(t, global, "commit", "---\nname: Global Commit\ndescription: global\n---\nGLOBAL BODY")
	writeSkill(t, global, "only-global", "---\nname: Only Global\ndescription: g\n---\nbody")
	// Same slug in workspace overrides global.
	writeSkill(t, ws, "commit", "---\nname: WS Commit\ndescription: ws\n---\nWS BODY")

	s := New(global, ws)

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("want 2 skills, got %d: %+v", len(list), list)
	}

	sk, ok := s.Get("commit")
	if !ok {
		t.Fatal("commit not found")
	}
	if sk.Source != SourceWorkspace || sk.Name != "WS Commit" {
		t.Errorf("override failed: source=%s name=%q", sk.Source, sk.Name)
	}
	body, err := s.Body("commit")
	if err != nil {
		t.Fatal(err)
	}
	if body != "WS BODY" {
		t.Errorf("body = %q (want workspace tier)", body)
	}

	if og, _ := s.Get("only-global"); og.Source != SourceGlobal {
		t.Errorf("only-global source = %s", og.Source)
	}
}

func TestCatalogBlockAndEmpty(t *testing.T) {
	empty := New("", t.TempDir())
	if !empty.Empty() {
		t.Error("expected empty store")
	}
	if empty.CatalogBlock() != "" {
		t.Error("expected empty catalog block")
	}

	dir := t.TempDir()
	writeSkill(t, dir, "triage", "---\nname: Triage\ndescription: sort issues\nwhen_to_use: on new issues\n---\nbody")
	s := New("", dir)
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
	s := New("", dir)

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

func TestSharedSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "review", "---\nname: Review\ndescription: r\naccess: shared\n---\nbody")
	writeSkill(t, dir, "deploy", "---\nname: Deploy\ndescription: d\n---\nbody") // restricted (default)
	writeSkill(t, dir, "notes", "---\nname: Notes\ndescription: n\nshared: true\n---\nbody")
	s := New("", dir)

	if sh := s.SharedList(); len(sh) != 2 {
		t.Fatalf("want 2 shared, got %d: %+v", len(sh), sh)
	}

	// An agent with no assignment still sees shared skills + can load them.
	allow := s.AllowedFor(nil)
	if !allow["review"] || !allow["notes"] || allow["deploy"] {
		t.Errorf("AllowedFor(nil) = %v (want review+notes, not deploy)", allow)
	}
	block := s.CatalogBlockForAgent(nil)
	if indexOf(block, "`review`") < 0 || indexOf(block, "`notes`") < 0 {
		t.Errorf("shared skills missing from no-assignment catalog:\n%s", block)
	}
	if indexOf(block, "`deploy`") >= 0 {
		t.Errorf("restricted skill leaked into no-assignment catalog:\n%s", block)
	}

	// Assigning the restricted skill makes it visible/usable, and it comes first.
	allow2 := s.AllowedFor([]string{"deploy"})
	if !allow2["deploy"] || !allow2["review"] {
		t.Errorf("AllowedFor([deploy]) = %v (want deploy+shared)", allow2)
	}
	b2 := s.CatalogBlockForAgent([]string{"deploy"})
	di, ri := indexOf(b2, "`deploy`"), indexOf(b2, "`review`")
	if di < 0 || ri < 0 || di > ri {
		t.Errorf("assigned skill should precede shared:\n%s", b2)
	}
}

func TestSetFrontmatterAccess(t *testing.T) {
	// Add access to a block that lacks it; preserve other fields + body.
	in := "---\nname: X\ndescription: d\n---\n\n# Body\ntext"
	out := setFrontmatterAccess(in, true)
	if !contains(out, "access: shared") || !contains(out, "name: X") || !contains(out, "# Body") {
		t.Fatalf("shared rewrite lost content:\n%s", out)
	}
	// Toggling back to restricted drops the access line, keeps the rest.
	back := setFrontmatterAccess(out, false)
	if contains(back, "access:") {
		t.Errorf("restricted should drop access line:\n%s", back)
	}
	if !contains(back, "name: X") || !contains(back, "# Body") {
		t.Errorf("restricted rewrite lost content:\n%s", back)
	}
	// Re-parsing the rewritten file reflects the new mode.
	if fm, _ := parseFrontmatter(out); !isShared(fm) {
		t.Error("rewritten shared file should parse as shared")
	}
	if fm, _ := parseFrontmatter(back); isShared(fm) {
		t.Error("rewritten restricted file should parse as restricted")
	}
}

func TestStoreSetAccess(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "doc", "---\nname: Doc\ndescription: d\n---\nbody")
	s := New("", dir)
	if sk, _ := s.Get("doc"); sk.Shared {
		t.Fatal("doc should start restricted")
	}
	sk, err := s.SetAccess("doc", true)
	if err != nil || !sk.Shared {
		t.Fatalf("SetAccess(true) = %+v, %v", sk, err)
	}
	if got, _ := s.Get("doc"); !got.Shared {
		t.Error("store not reloaded after SetAccess")
	}
	if body, _ := s.Body("doc"); body != "body" {
		t.Errorf("body changed by access rewrite: %q", body)
	}
}

func TestBodyMissingFileSelfHeals(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "ghost", "---\nname: Ghost\ndescription: g\n---\nbody")
	s := New("", dir)
	if _, ok := s.Get("ghost"); !ok {
		t.Fatal("ghost should load")
	}
	// Delete the file out-of-band, then read: clear error + catalog self-heals.
	if err := os.RemoveAll(filepath.Join(dir, "ghost")); err != nil {
		t.Fatal(err)
	}
	_, err := s.Body("ghost")
	if err == nil || !contains(err.Error(), "no longer available") {
		t.Fatalf("want self-heal error, got %v", err)
	}
	if _, ok := s.Get("ghost"); ok {
		t.Error("ghost should be dropped after self-heal reload")
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

func TestUseSkillBodySubskillFooter(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "overview", `---
name: Overview
description: high-level
subskills: [deep, missing, overview]
---
# Overview
Body text.`)
	writeSkill(t, dir, "deep", `---
name: Deep
description: detailed steps
---
# Deep`)

	s := New("", dir)

	// allow=nil → known sub-skills surface, unknown/self are dropped.
	body, err := s.UseSkillBody("overview", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "Body text.") {
		t.Errorf("missing original body: %q", body)
	}
	if !strings.Contains(body, "Related skills") || !strings.Contains(body, "`deep` — detailed steps") {
		t.Errorf("missing sub-skill footer: %q", body)
	}
	if strings.Contains(body, "missing") || strings.Contains(body, "`overview`") {
		t.Errorf("footer should drop unknown/self slugs: %q", body)
	}

	// allow set excluding "deep" → no footer at all.
	body2, _ := s.UseSkillBody("overview", map[string]bool{"overview": true})
	if strings.Contains(body2, "Related skills") {
		t.Errorf("disallowed sub-skill should be hidden: %q", body2)
	}
}

func TestStoreCreateUpdateDelete(t *testing.T) {
	global := t.TempDir()
	wsDir := t.TempDir()
	writeSkill(t, global, "g", "---\nname: G\ndescription: g\n---\nbody")
	s := New(global, wsDir)

	// Create derives a kebab slug from the (Turkish) name and writes to the
	// workspace tier.
	sk, err := s.Create("", SkillInput{
		Name:        "Görev Planlayıcı",
		Description: "Plans tasks: end to end",
		Icon:        "🗂️",
		Shared:      true,
		Body:        "# Planner\nDo the thing.",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sk.Slug != "gorev-planlayici" {
		t.Errorf("slug = %q, want gorev-planlayici", sk.Slug)
	}
	if sk.Source != SourceWorkspace || sk.Icon != "🗂️" || !sk.Shared {
		t.Errorf("created skill = %+v", sk)
	}
	if body, _ := s.Body(sk.Slug); !strings.Contains(body, "Do the thing.") {
		t.Errorf("body = %q", body)
	}
	// Description with a colon must round-trip (quoting).
	if got, _ := s.Get(sk.Slug); got.Description != "Plans tasks: end to end" {
		t.Errorf("description = %q", got.Description)
	}

	// Duplicate slug rejected.
	if _, err := s.Create("gorev-planlayici", SkillInput{Name: "Dup"}); err == nil {
		t.Error("expected duplicate-slug error")
	}
	// Empty name rejected.
	if _, err := s.Create("", SkillInput{Name: "  "}); err == nil {
		t.Error("expected empty-name error")
	}

	// Update changes metadata + body, flips access off, preserves the slug.
	up, err := s.Update("gorev-planlayici", SkillInput{
		Name:        "Planner v2",
		Description: "updated",
		Icon:        "📋",
		Shared:      false,
		Body:        "# v2\nNew body.",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if up.Name != "Planner v2" || up.Icon != "📋" || up.Shared {
		t.Errorf("updated skill = %+v", up)
	}
	if body, _ := s.Body("gorev-planlayici"); !strings.Contains(body, "New body.") || strings.Contains(body, "Do the thing.") {
		t.Errorf("body not replaced: %q", body)
	}

	// Update cannot target the global tier's slug into creating; missing slug errors.
	if _, err := s.Update("nope", SkillInput{Name: "x"}); err == nil {
		t.Error("expected not-found error on Update")
	}

	// Delete removes the folder and drops it from the catalog.
	if err := s.Delete("gorev-planlayici"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := s.Get("gorev-planlayici"); ok {
		t.Error("skill still present after delete")
	}
	if _, err := os.Stat(filepath.Join(wsDir, "gorev-planlayici")); !os.IsNotExist(err) {
		t.Errorf("folder not removed: %v", err)
	}
}

func TestCreateWithoutWorkspaceTier(t *testing.T) {
	s := New(t.TempDir(), "") // global only
	if _, err := s.Create("x", SkillInput{Name: "X"}); err == nil {
		t.Error("expected error creating without a workspace tier")
	}
}

func TestSetFrontmatterFields(t *testing.T) {
	in := "---\nname: Old\ndescription: old\nsubskills:\n  - a\n  - b\n---\n\n# Body\ntext"
	body := "# New body"
	out := setFrontmatterFields(in, []fmField{
		{"name", "New"},
		{"description", "has: colon"},
		{"icon", "🎯"},
	}, &body)

	fm, gotBody := parseFrontmatter(out)
	if fm.scalar("name") != "New" {
		t.Errorf("name = %q", fm.scalar("name"))
	}
	if fm.scalar("description") != "has: colon" {
		t.Errorf("description = %q (quoting failed)", fm.scalar("description"))
	}
	if fm.scalar("icon") != "🎯" {
		t.Errorf("icon = %q", fm.scalar("icon"))
	}
	// Unmanaged block list preserved.
	if got := fm.list("subskills"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("subskills not preserved: %v", got)
	}
	if strings.TrimSpace(gotBody) != "# New body" {
		t.Errorf("body = %q", gotBody)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Görev Planlayıcı": "gorev-planlayici",
		"  Hello World!  ":  "hello-world",
		"a/b.c_d":           "a-b-c-d",
		"---":               "",
		"ÇÖŞ":               "cos",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnsureDefaultsSeeds(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "swarmgo-guide", "SKILL.md")); err != nil {
		t.Errorf("swarmgo-guide not seeded: %v", err)
	}
	// Idempotent + non-overwriting: edit a default, re-seed, edit survives.
	guide := filepath.Join(dir, "swarmgo-guide", "SKILL.md")
	if err := os.WriteFile(guide, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(guide)
	if string(got) != "edited" {
		t.Errorf("EnsureDefaults overwrote a user edit: %q", got)
	}
}
