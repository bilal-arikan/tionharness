package view

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/skills"
)

func TestProjectSkillRendersCatalogEntry(t *testing.T) {
	v, err := ProjectSkill(SkillInput{
		Skill: skills.Skill{
			Slug: "tionharness-build", Name: "Build", Description: "Derler ve test eder.",
			Shared: true, Group: "araçlar", Icon: "hammer",
			Source: skills.SourceWorkspace, Visibility: "summary",
		},
	}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	txt := v.Text()
	for _, want := range []string{
		"SKILL · tionharness-build", "Build", "Derler ve test eder.",
		"kaynak: workspace", "erişim: shared", "görünürlük: summary", "grup: araçlar",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
	// The icon is a UI glyph nothing here can act on; it must not cost a segment.
	if strings.Contains(txt, "ikon") {
		t.Errorf("icon must not be rendered:\n%s", txt)
	}
}

// TestProjectSkillWhenToUseIsFullOnly locks the level discipline: the trigger
// condition is the long half of a catalog entry and only a drill-down pays for it.
func TestProjectSkillWhenToUseIsFullOnly(t *testing.T) {
	in := SkillInput{Skill: skills.Skill{
		Slug: "sk", Source: skills.SourceGlobal, WhenToUse: "Bir derleme kırıldığında",
	}}

	card, err := ProjectSkill(in, LevelCard)
	if err != nil {
		t.Fatalf("card: %v", err)
	}
	if strings.Contains(card.Text(), "ne zaman:") {
		t.Errorf("when-to-use must not reach the card tier:\n%s", card.Text())
	}
	if !strings.Contains(card.Text(), "kaynak: global") {
		t.Errorf("global skills must say which tier they came from:\n%s", card.Text())
	}

	full, err := ProjectSkill(in, LevelFull)
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if !strings.Contains(full.Text(), "ne zaman: Bir derleme kırıldığında") {
		t.Errorf("full tier must carry when-to-use:\n%s", full.Text())
	}
}

func TestProjectSkillRestrictedAccess(t *testing.T) {
	v, err := ProjectSkill(SkillInput{
		Skill: skills.Skill{Slug: "gizli-skill", Description: "yalnız atanmış ajanlar"},
	}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "erişim: kısıtlı") {
		t.Errorf("restricted skill must say so:\n%s", v.Text())
	}
}

func TestProjectSkillRejectsEmptySlug(t *testing.T) {
	if _, err := ProjectSkill(SkillInput{Skill: skills.Skill{}}, LevelCard); err == nil {
		t.Error("a skill with no slug must be an error, not a blank card")
	}
}
