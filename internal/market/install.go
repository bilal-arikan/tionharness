package market

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// InstallResult describes what an install produced, for the UI to report and to
// refresh the right panel afterwards.
type InstallResult struct {
	Kind string `json:"kind"`
	// Ref is the kind-specific identifier of the installed entity: a skill slug,
	// a created agent/flow id, or a custom-provider id.
	Ref     string `json:"ref"`
	Message string `json:"message"`
}

// InstallSkill writes a skill pack's SKILL.md into the workspace skills dir under
// its slug. skillsDir is the workspace tier dir (<workspace>/skills). When
// overwrite is false and the skill already exists, it errors instead of clobbering.
//
// The skill registry should be Reload()ed by the caller after a successful
// install so the new skill appears immediately.
func InstallSkill(p Pack, skillsDir string, overwrite bool) (InstallResult, error) {
	if p.Kind != KindSkill || p.Payload.Skill == nil {
		return InstallResult{}, fmt.Errorf("pack %q is not a skill", p.ID)
	}
	if skillsDir == "" {
		return InstallResult{}, fmt.Errorf("no workspace skills directory")
	}
	sp := p.Payload.Skill
	slug := safeFileName(sp.Slug)
	if slug == "" {
		return InstallResult{}, fmt.Errorf("skill pack has no slug")
	}
	if sp.Body == "" {
		return InstallResult{}, fmt.Errorf("skill pack has no body")
	}
	dest := filepath.Join(skillsDir, slug, "SKILL.md")
	if !overwrite {
		if _, err := os.Stat(dest); err == nil {
			return InstallResult{}, fmt.Errorf("skill %q already exists (enable overwrite to replace)", slug)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create skill dir: %w", err)
	}
	if err := os.WriteFile(dest, []byte(sp.Body), 0o644); err != nil {
		return InstallResult{}, fmt.Errorf("write skill: %w", err)
	}
	// Write bundled resource files (nested dirs preserved). Path traversal is
	// refused so a pack can't escape the skill folder; SKILL.md is rendered above.
	skillDir := filepath.Join(skillsDir, slug)
	for name, data := range p.Files {
		rel, ok := safeRelPath(name)
		if !ok {
			continue
		}
		fp := filepath.Join(skillDir, rel)
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			return InstallResult{}, fmt.Errorf("create dir for %q: %w", rel, err)
		}
		if err := os.WriteFile(fp, data, 0o644); err != nil {
			return InstallResult{}, fmt.Errorf("write bundled file %q: %w", rel, err)
		}
	}
	return InstallResult{
		Kind:    KindSkill,
		Ref:     slug,
		Message: fmt.Sprintf("Skill %q installed", slug),
	}, nil
}

// safeRelPath validates a bundled-file path relative to an entity folder: it rejects
// SKILL.md (rendered separately), absolute paths and any ".." escape, returning a
// cleaned OS-native relative path. Nested sub-directories are allowed and preserved.
func safeRelPath(name string) (string, bool) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimPrefix(name, "./")
	if name == "" || name == "." || strings.EqualFold(name, "SKILL.md") {
		return "", false
	}
	if path.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", false
	}
	clean := path.Clean(name)
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", false
	}
	return filepath.FromSlash(clean), true
}
