package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/skills"
)

// SkillInput is one skill catalog entry. The projection renders the catalog
// metadata (slug, name, description, access tier, group) — never the body. The
// body is the skill's instructions and stays behind use_skill; the map only
// advertises what the skill IS so the reader can decide whether to load it.
type SkillInput struct {
	Skill skills.Skill
	// Now is the clock used for the asOf stamp. Zero means time.Now().
	Now time.Time
}

// ProjectSkill renders one skill's catalog entry.
func ProjectSkill(in SkillInput, level Level) (View, error) {
	sk := in.Skill
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	if sk.Slug == "" {
		return View{}, fmt.Errorf("view: skill has no slug")
	}

	v := View{
		Ref:    Ref{Kind: KindSkill, ID: sk.Slug},
		Level:  level,
		AsOf:   now,
		Source: sk.Slug,
	}
	v.Header = fmt.Sprintf("SKILL · %s · asOf %s", sk.Slug, hhmmss(now))

	if level == LevelTiny {
		v.finalize()
		return v, nil
	}

	var l lines
	if sk.Name != "" && sk.Name != sk.Slug {
		l.add("%s", clip(sk.Name, 80))
	}
	if sk.Description != "" {
		l.add("%s", clip(sk.Description, 120))
	}
	access := "kısıtlı"
	if sk.Shared {
		access = "shared"
	}
	var meta []string
	// The tier decides whether editing this skill is even possible here: a global
	// skill lives in the shared data dir and is read-only to workspace tooling,
	// while a workspace one is the workspace's own file.
	if sk.Source != "" {
		meta = append(meta, "kaynak: "+string(sk.Source))
	}
	meta = append(meta, "erişim: "+access)
	// How the skill is ADVERTISED in the per-turn catalog. A hidden or name-only
	// skill still resolves through use_skill but will not reach an agent that never
	// learns it exists — the difference between "not offered" and "not there".
	if sk.Visibility != "" {
		meta = append(meta, "görünürlük: "+sk.Visibility)
	}
	if sk.Group != "" {
		meta = append(meta, "grup: "+sk.Group)
	}
	// The icon is a UI glyph: nothing a reader of this projection can act on, and
	// it cost a segment on every skill card.
	l.add("%s", strings.Join(meta, " · "))

	// When-to-use is the trigger condition — the long half of the catalog entry,
	// and the part a reader only needs once they are deciding to load the skill.
	if level == LevelFull && sk.WhenToUse != "" {
		l.add("ne zaman: %s", clip(sk.WhenToUse, 200))
	}

	v.Body = l.String()
	v.finalize()
	return v, nil
}
