package market

import "fmt"

// BuildSkillPack assembles a skill Pack from a skill's metadata and full SKILL.md
// text. The caller (API layer) pulls these from the skills store, keeping the
// market package free of a skills dependency. The body is carried verbatim
// (frontmatter + markdown) so the install is lossless. files carries the skill's
// bundled resources (relpath -> content) so nested docs/templates survive publish.
func BuildSkillPack(slug, name, description, icon, color, body, author string, createdAt int64, files map[string][]byte) (Pack, error) {
	if slug == "" {
		return Pack{}, fmt.Errorf("slug is required")
	}
	if body == "" {
		return Pack{}, fmt.Errorf("skill body is empty")
	}
	if name == "" {
		name = slug
	}
	if len(files) == 0 {
		files = nil
	}
	return Pack{
		Schema:      SchemaV1,
		ID:          KindSkill + "." + slug,
		Kind:        KindSkill,
		Name:        name,
		Description: description,
		Version:     "1.0.0",
		Author:      author,
		Icon:        icon,
		Color:       color,
		CreatedAt:   createdAt,
		Payload:     Payload{Skill: &SkillPayload{Slug: slug, Body: body}},
		Files:       files,
	}, nil
}
