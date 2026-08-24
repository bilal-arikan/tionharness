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
		},
	}, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	txt := v.Text()
	for _, want := range []string{
		"SKILL · tionharness-build", "Build", "Derler ve test eder.",
		"erişim: shared", "grup: araçlar", "ikon: hammer",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

func TestProjectSkillRestrictedAccess(t *testing.T) {
	v, err := ProjectSkill(SkillInput{
		Skill: skills.Skill{Slug: "gizli-skill", Description: "yalnız atanmış ajanlar"},
	}, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "erişim: kısıtlı") {
		t.Errorf("restricted skill must say so:\n%s", v.Text())
	}
}

func TestProjectSkillRejectsEmptySlug(t *testing.T) {
	if _, err := ProjectSkill(SkillInput{Skill: skills.Skill{}}, LevelCard, LensHealth); err == nil {
		t.Error("a skill with no slug must be an error, not a blank card")
	}
}
