package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/skills"
)

// writeTestSkill drops a minimal SKILL.md into a skills tier directory.
func writeTestSkill(t *testing.T, dir, slug, frontmatter string) {
	t.Helper()
	d := filepath.Join(dir, slug)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" + frontmatter + "---\nbody of " + slug + "\n"
	if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCountCatalogSkillsOnRealBlock pins the skill count to the REAL renderer
// output rather than a hand-written fixture: if renderCatalog ever changes its
// entry marker, the Session Info "Skill kataloğu" bucket would silently report
// 0 skills, and this test is what catches it.
func TestCountCatalogSkillsOnRealBlock(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "triage", "name: Triage\ndescription: sort issues\nwhen_to_use: on new issues\n")
	writeTestSkill(t, dir, "deploy", "name: Deploy\ndescription: ship a release\n")
	// name_only: listed as a bare slug — still one advertised skill, so it must count.
	writeTestSkill(t, dir, "secret", "name: Secret\ndescription: hidden\nname_only: true\n")

	block := skills.New("", dir).CatalogBlock()
	if block == "" {
		t.Fatal("expected a non-empty catalog block")
	}
	if got := countCatalogSkills(block); got != 3 {
		t.Fatalf("countCatalogSkills = %d, want 3\nblock:\n%s", got, block)
	}

	if got := countCatalogSkills(""); got != 0 {
		t.Fatalf("empty block should count 0 skills, got %d", got)
	}
}

// TestStripBlockLiftsAppendedCatalogs guards the carve mechanism systemFillers
// relies on: buildStaticPrefix appends each catalog with a "\n\n" join and one
// final TrimSpace, so the block must still be findable VERBATIM afterwards.
// If it is not, the bucket silently collapses back into "Sistem promptu" — the
// exact drift this whole change set exists to prevent.
func TestStripBlockLiftsAppendedCatalogs(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(t, dir, "triage", "name: Triage\ndescription: sort issues\n")
	skillsBlock := skills.New("", dir).CatalogBlock()

	base := "You are an agent.\n\n# Workspace Instructions\nBe terse."
	// Mirrors buildStaticPrefix: skills block, then a further block appended after
	// it (the load-on-demand tool catalog occupies that slot in the real prefix).
	lazyBlock := "# Available Tools (load on demand)\n- `spawn_session` — start a session\n"
	prefix := strings.TrimSpace(base + "\n\n" + skillsBlock)
	prefix = strings.TrimSpace(prefix + "\n\n" + lazyBlock)

	// carve() trims the block before searching — the appended copy lost its
	// trailing newline to TrimSpace, so only the trimmed form can match.
	rest, ok := stripBlock(prefix, strings.TrimSpace(skillsBlock))
	if !ok {
		t.Fatalf("skills block not found verbatim in composed prefix:\n%s", prefix)
	}
	if strings.Contains(rest, "`triage`") {
		t.Errorf("skills entry survived the strip:\n%s", rest)
	}
	rest, ok = stripBlock(rest, strings.TrimSpace(lazyBlock))
	if !ok {
		t.Fatalf("lazy-tools block not found verbatim after the skills strip:\n%s", rest)
	}
	if strings.TrimSpace(rest) != base {
		t.Errorf("remaining system prompt = %q, want %q", rest, base)
	}
}
