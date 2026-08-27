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
	meta = append(meta, "erişim: "+access)
	if sk.Group != "" {
		meta = append(meta, "grup: "+sk.Group)
	}
	if sk.Icon != "" {
		meta = append(meta, "ikon: "+sk.Icon)
	}
	l.add("%s", strings.Join(meta, " · "))

	v.Body = l.String()
	v.finalize()
	return v, nil
}
