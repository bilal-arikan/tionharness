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

// TestNameOnlySkillRendersSlugOnly verifies a name_only skill is listed by slug
// alone (description + when-to-use suppressed) while a normal skill keeps its
// summary — and that SetNameOnly flips the frontmatter and reloads.
func TestNameOnlySkillRendersSlugOnly(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "triage", "---\nname: Triage\ndescription: sort issues\nwhen_to_use: on new issues\nshared: true\n---\nbody")
	writeSkill(t, dir, "deploy", "---\nname: Deploy\ndescription: ship the build\nwhen_to_use: on release\nshared: true\nname_only: true\n---\nbody")
	s := New("", dir)

	// Parsed flag.
	if d, _ := s.Get("deploy"); !d.NameOnly {
		t.Fatal("deploy should parse name_only: true")
	}
	if tr, _ := s.Get("triage"); tr.NameOnly {
		t.Fatal("triage should default to NameOnly=false")
	}

	block := s.CatalogBlock()
	// NameOnly skill: slug present, but its summary text absent.
	if !contains(block, "`deploy`") {
		t.Errorf("name-only skill must still be listed by slug:\n%s", block)
	}
	if contains(block, "ship the build") || contains(block, "on release") {
		t.Errorf("name-only skill must NOT show description/when:\n%s", block)
	}
	// Normal skill keeps its summary.
	if !contains(block, "sort issues") {
		t.Errorf("normal skill must keep its summary:\n%s", block)
	}

	// Toggle off via SetNameOnly → summary returns.
	if _, err := s.SetNameOnly("deploy", false); err != nil {
		t.Fatalf("SetNameOnly off: %v", err)
	}
	if d, _ := s.Get("deploy"); d.NameOnly {
		t.Fatal("deploy NameOnly should be false after toggle off")
	}
	if !contains(s.CatalogBlock(), "ship the build") {
		t.Error("summary must return after NameOnly toggled off")
	}

	// Toggle back on → summary suppressed again.
	if _, err := s.SetNameOnly("deploy", true); err != nil {
		t.Fatalf("SetNameOnly on: %v", err)
	}
	if contains(s.CatalogBlock(), "ship the build") {
		t.Error("summary must be suppressed after NameOnly toggled on")
	}
}

// TestSetVisibilityTiers verifies the 4-way SetVisibility maps onto the
// frontmatter flags and derived Visibility, and that each tier renders as
// expected: full (desc + when), summary (desc only), name-only (slug only),
// hidden (dropped from the catalog). An invalid tier is rejected.
func TestSetVisibilityTiers(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "deploy", "---\nname: Deploy\ndescription: ship the build\nwhen_to_use: on release\nshared: true\n---\nbody")
	s := New("", dir)

	// Default is full.
	if d, _ := s.Get("deploy"); d.Visibility != VisibilityFull {
		t.Fatalf("default visibility = %q, want full", d.Visibility)
	}

	// summary: description present, when-to-use suppressed.
	if _, err := s.SetVisibility("deploy", VisibilitySummary); err != nil {
		t.Fatalf("set summary: %v", err)
	}
	if d, _ := s.Get("deploy"); d.Visibility != VisibilitySummary || !d.SummaryOnly {
		t.Fatalf("summary flags: vis=%q summaryOnly=%v", d.Visibility, d.SummaryOnly)
	}
	block := s.CatalogBlockForAgent(nil)
	if !contains(block, "ship the build") || contains(block, "on release") {
		t.Errorf("summary tier must show desc but not when:\n%s", block)
	}

	// name-only: slug alone.
	if _, err := s.SetVisibility("deploy", VisibilityNameOnly); err != nil {
		t.Fatalf("set name-only: %v", err)
	}
	if d, _ := s.Get("deploy"); d.Visibility != VisibilityNameOnly || !d.NameOnly {
		t.Fatalf("name-only flags: vis=%q nameOnly=%v", d.Visibility, d.NameOnly)
	}
	if block := s.CatalogBlockForAgent(nil); contains(block, "ship the build") {
		t.Errorf("name-only tier must suppress desc:\n%s", block)
	}

	// hidden: dropped from the catalog entirely.
	if _, err := s.SetVisibility("deploy", VisibilityHidden); err != nil {
		t.Fatalf("set hidden: %v", err)
	}
	if d, _ := s.Get("deploy"); d.Visibility != VisibilityHidden || d.AutoSummary {
		t.Fatalf("hidden flags: vis=%q autoSummary=%v", d.Visibility, d.AutoSummary)
	}
	if block := s.CatalogBlockForAgent(nil); contains(block, "`deploy`") {
		t.Errorf("hidden tier must drop the skill from the catalog:\n%s", block)
	}

	// Back to full restores the rich summary.
	if _, err := s.SetVisibility("deploy", VisibilityFull); err != nil {
		t.Fatalf("set full: %v", err)
	}
	if block := s.CatalogBlockForAgent(nil); !contains(block, "ship the build") || !contains(block, "on release") {
		t.Errorf("full tier must restore desc + when:\n%s", block)
	}

	// Invalid tier is rejected.
	if _, err := s.SetVisibility("deploy", "bogus"); err == nil {
		t.Error("invalid tier should error")
	}
}

func TestSetGroup(t *testing.T) {
	dir := t.TempDir()
	// One skill with no group, one with a `category` alias to prove it's dropped.
	writeSkill(t, dir, "a", "---\nname: A\ndescription: d\n---\nbody A")
	writeSkill(t, dir, "b", "---\nname: B\ndescription: d\ncategory: old\n---\nbody B")
	s := New("", dir)

	// Assign both into a group.
	for _, slug := range []string{"a", "b"} {
		sk, err := s.SetGroup(slug, "my pack")
		if err != nil {
			t.Fatalf("set group %s: %v", slug, err)
		}
		if sk.Group != "my pack" {
			t.Errorf("%s group = %q, want 'my pack'", slug, sk.Group)
		}
	}
	// The `category` alias must be gone (only `group` remains) so the two can't disagree.
	data, _ := os.ReadFile(filepath.Join(dir, "b", "SKILL.md"))
	if strings.Contains(string(data), "category:") {
		t.Errorf("category alias not dropped:\n%s", data)
	}
	if !strings.Contains(string(data), "group: ") {
		t.Errorf("group not written:\n%s", data)
	}

	// Empty group ungroups.
	sk, err := s.SetGroup("a", "")
	if err != nil {
		t.Fatalf("ungroup: %v", err)
	}
	if sk.Group != "" {
		t.Errorf("group after ungroup = %q, want empty", sk.Group)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "a", "SKILL.md"))
	if strings.Contains(string(data), "group:") {
		t.Errorf("group marker not removed on ungroup:\n%s", data)
	}
	// Body and other fields survive.
	if !strings.Contains(string(data), "body A") || !strings.Contains(string(data), "name: A") {
		t.Errorf("ungroup damaged the skill:\n%s", data)
	}

	// Missing skill errors.
	if _, err := s.SetGroup("nope", "x"); err == nil {
		t.Error("expected error for missing skill")
	}
}

// TestCatalogBlockForAgentTool verifies the block names the skill tool exactly as
// given (e.g. the namespaced identifier a claude-cli agent must call), instead of
// the bare default.
func TestCatalogBlockForAgentTool(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "triage", "---\nname: Triage\ndescription: sort issues\nshared: true\n---\nbody")
	s := New("", dir)

	const ns = "mcp__tionswarm_interaction__use_skill"
	block := s.CatalogBlockForAgentTool(nil, ns)
	if !contains(block, ns) {
		t.Errorf("block missing namespaced tool %q:\n%s", ns, block)
	}
	// The instruction line must NOT fall back to the bare name when a name is given.
	if contains(block, "`use_skill`") {
		t.Errorf("block should use the namespaced tool, not bare use_skill:\n%s", block)
	}
	// Empty tool name falls back to the default.
	if def := s.CatalogBlockForAgentTool(nil, ""); !contains(def, "`use_skill`") {
		t.Errorf("empty skillTool should fall back to default use_skill:\n%s", def)
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
	if _, err := os.Stat(filepath.Join(dir, "tionswarm-guide", "SKILL.md")); err != nil {
		t.Errorf("tionswarm-guide not seeded: %v", err)
	}
	// Idempotent + non-overwriting: edit a default, re-seed, edit survives.
	guide := filepath.Join(dir, "tionswarm-guide", "SKILL.md")
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

// TestEnsureDefaultsRefreshesPristine covers the version-aware re-seed: an on-disk
// default that is an UNMODIFIED previously-shipped version (its hash is the one
// recorded in the manifest) gets refreshed to the current embedded content, while
// a user-edited one is preserved.
func TestEnsureDefaultsRefreshesPristine(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	guide := filepath.Join(dir, "tionswarm-guide", "SKILL.md")
	embedded, _ := os.ReadFile(guide) // current embedded content (just seeded)

	// Simulate a PRIOR ship: an older on-disk body whose hash is recorded in the
	// manifest as the last-shipped version (i.e. the user never touched it).
	oldBody := []byte("OLD SHIPPED BODY that should be refreshed")
	if err := os.WriteFile(guide, oldBody, 0o644); err != nil {
		t.Fatal(err)
	}
	m := loadShippedManifest(dir)
	m.Files["tionswarm-guide/SKILL.md"] = sha256Hex(oldBody)
	if err := saveShippedManifest(dir, m); err != nil {
		t.Fatal(err)
	}

	// Re-seed: the pristine old copy must be refreshed back to the embedded content.
	if err := EnsureDefaults(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(guide)
	if string(got) != string(embedded) {
		t.Errorf("pristine prior-shipped default was NOT refreshed to embedded content")
	}
	// The manifest must now record the fresh embedded hash.
	if loadShippedManifest(dir).Files["tionswarm-guide/SKILL.md"] != sha256Hex(embedded) {
		t.Errorf("manifest not updated to the refreshed hash")
	}
}

// TestUseSkillBodySK1 covers SK-1: ${SKILL_DIR} expansion + bundled-files footer.
func TestUseSkillBodySK1(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "demo", "---\nname: Demo\ndescription: d\naccess: shared\n---\nSee ${SKILL_DIR}/ref.md for details.")
	if err := os.WriteFile(filepath.Join(dir, "demo", "ref.md"), []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New("", dir)
	body, err := s.UseSkillBody("demo", map[string]bool{"demo": true})
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.ToSlash(filepath.Join(dir, "demo"))
	if !strings.Contains(body, wantDir+"/ref.md") {
		t.Errorf("${SKILL_DIR} not expanded to %q in: %q", wantDir, body)
	}
	if strings.Contains(body, "${SKILL_DIR}") {
		t.Errorf("placeholder left unexpanded: %q", body)
	}
	if !strings.Contains(body, "Bundled files") || !strings.Contains(body, wantDir+"/ref.md") {
		t.Errorf("bundled-files footer missing ref.md: %q", body)
	}
	// The raw Body (edit/detail view) must stay literal — no expansion.
	raw, err := s.Body("demo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "${SKILL_DIR}") {
		t.Errorf("raw Body should keep ${SKILL_DIR} literal: %q", raw)
	}
}

// TestSearchAndConditionalSK2 covers SK-2: conditional (paths) skills stay out of
// the auto-advertised catalog but remain loadable and discoverable via Search.
func TestSearchAndConditionalSK2(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "commit-helper", "---\nname: Commit Helper\ndescription: write git commits\naccess: shared\n---\nbody")
	writeSkill(t, dir, "go-tester", "---\nname: Go Tester\ndescription: run go tests\naccess: shared\npaths:\n  - \"**/*.go\"\n---\nbody")
	s := New("", dir)

	cat := s.CatalogBlockForAgent(nil)
	if !strings.Contains(cat, "commit-helper") {
		t.Errorf("non-conditional shared skill should be advertised: %q", cat)
	}
	if strings.Contains(cat, "go-tester") {
		t.Errorf("conditional skill must NOT be auto-advertised: %q", cat)
	}
	if !s.AllowedFor(nil)["go-tester"] {
		t.Errorf("conditional shared skill should still be loadable")
	}
	found := false
	for _, h := range s.Search("go tests", 10) {
		if h.Slug == "go-tester" {
			found = true
		}
	}
	if !found {
		t.Errorf("Search should find the conditional skill")
	}
	if len(s.Search("zzz-nomatch-term", 10)) != 0 {
		t.Errorf("expected no matches for nonsense query")
	}
}

// TestRichFrontmatterSK4 covers SK-4: provenance fields parse; user-invocable
// defaults true and can be disabled.
func TestRichFrontmatterSK4(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "imported", "---\nname: Imported\ndescription: d\nversion: 1.2.3\nsource_url: https://github.com/x/y\nlicense: MIT\nuser-invocable: false\n---\nbody")
	writeSkill(t, dir, "plain", "---\nname: Plain\ndescription: d\n---\nbody")
	s := New("", dir)

	sk, ok := s.Get("imported")
	if !ok {
		t.Fatal("imported skill not loaded")
	}
	if sk.Version != "1.2.3" {
		t.Errorf("version: got %q", sk.Version)
	}
	if sk.SourceURL != "https://github.com/x/y" {
		t.Errorf("source_url: got %q", sk.SourceURL)
	}
	if sk.License != "MIT" {
		t.Errorf("license: got %q", sk.License)
	}
	if sk.UserInvocable {
		t.Errorf("user-invocable:false should disable")
	}
	pl, _ := s.Get("plain")
	if !pl.UserInvocable {
		t.Errorf("user-invocable should default true when absent")
	}
}
