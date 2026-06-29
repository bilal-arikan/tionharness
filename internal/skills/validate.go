package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SkillValidation is the structured result of validating one skill's SKILL.md.
// Valid is true only when there are no Errors (Warnings do not fail validation).
type SkillValidation struct {
	Slug     string   `json:"slug"`
	Found    bool     `json:"found"`
	Tier     string   `json:"tier,omitempty"` // "workspace" | "global"
	Path     string   `json:"path,omitempty"`
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// slugPattern is the accepted skill slug shape: lowercase kebab-case.
var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ValidateSkill checks a skill's SKILL.md for the problems that break loading or
// advertising: missing file, malformed/empty frontmatter, missing name/description,
// empty body, and slug hygiene. It reads the file directly from the tier dirs
// (workspace preferred) so it can validate even skills the catalog skipped because
// they failed to parse. This is the single source of truth shared by the
// skill_validate tool and any UI validation.
func (s *Store) ValidateSkill(slug string) SkillValidation {
	res := SkillValidation{Slug: slug}

	if strings.TrimSpace(slug) == "" {
		res.Errors = append(res.Errors, "slug is empty")
		return res
	}
	if !slugPattern.MatchString(slug) {
		res.Errors = append(res.Errors, "slug must be lowercase kebab-case (a-z, 0-9, hyphens), e.g. \"my-skill\"")
	}

	// Locate SKILL.md, preferring the workspace tier (highest priority). Scan in
	// reverse so workspace (appended last) wins over global.
	var path, tier string
	for i := len(s.tiers) - 1; i >= 0; i-- {
		candidate := filepath.Join(s.tiers[i].dir, slug, "SKILL.md")
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			path = candidate
			tier = string(s.tiers[i].source)
			break
		}
	}
	if path == "" {
		res.Errors = append(res.Errors, "SKILL.md not found in any skills tier (expected <tier>/"+slug+"/SKILL.md)")
		return res
	}
	res.Found = true
	res.Tier = tier
	res.Path = path

	data, err := os.ReadFile(path)
	if err != nil {
		res.Errors = append(res.Errors, "cannot read SKILL.md: "+err.Error())
		return res
	}
	content := string(data)

	fmText, body := splitFrontmatter(content)
	if strings.TrimSpace(fmText) == "" {
		res.Errors = append(res.Errors, "missing YAML frontmatter block (file must start with a \"---\" fenced header)")
	} else {
		if FrontmatterField(content, "name") == "" {
			res.Errors = append(res.Errors, "frontmatter is missing required field: name")
		}
		desc := FrontmatterField(content, "description")
		if desc == "" {
			res.Errors = append(res.Errors, "frontmatter is missing required field: description")
		} else if len(desc) < 16 {
			res.Warnings = append(res.Warnings, "description is very short (<16 chars); a fuller description improves skill discovery")
		}
	}

	if strings.TrimSpace(body) == "" {
		res.Errors = append(res.Errors, "skill body is empty (nothing after the frontmatter); add the instructions the skill should teach")
	}

	res.Valid = len(res.Errors) == 0
	return res
}
