package skills

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/fetch"
)

// ImportResult reports the outcome of importing a Claude Code skill: the resulting
// TionSwarm slug, which fields were carried over, the bundled files copied, and any
// warnings about CC features that do not map onto TionSwarm (stripped on import).
// (SK-IMP)
type ImportResult struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	MappedFields []string `json:"mappedFields"`
	Files        []string `json:"files"`
	Warnings     []string `json:"warnings"`
}

// mapCCSkill parses a Claude Code SKILL.md and renders the equivalent TionSwarm
// SKILL.md, mapping the frontmatter and collecting warnings for unsupported CC
// features. It does not touch disk. sourceURL is recorded as provenance.
//
// Mapping: name/description/when_to_use → same; allowed-tools → always_allow;
// paths → paths (conditional); version/license → same; disable-model-invocation
// → access (shared only when invocation is NOT disabled AND shared is requested);
// user-invocable → user_invocable. Unsupported (context:fork, hooks, agent, model,
// effort, slash-command arguments) are dropped with a warning.
//
// group namespaces the import: when non-empty it is written as the skill's `group`
// frontmatter (overriding any group the source declared), so every skill from one
// import lands under a single collapsible header in the Skills UI and never mixes
// with the user's existing skills. Empty → the source's own `group` is preserved.
func mapCCSkill(raw, sourceURL string, shared bool, group string) (content string, res ImportResult) {
	fm, body := parseFrontmatter(raw)

	name := strings.TrimSpace(fm.scalar("name"))
	desc := oneLine(fm.scalar("description"))
	when := oneLine(fm.scalar("when_to_use", "whentouse", "when"))
	version := strings.TrimSpace(fm.scalar("version"))
	license := strings.TrimSpace(fm.scalar("license"))
	allowed := fm.list("allowed-tools", "allowedtools", "allowed_tools", "alwaysallow", "always_allow")
	paths := fm.list("paths", "path")

	disableInvoke := boolScalar(fm.scalar("disable-model-invocation", "disablemodelinvocation"))
	userInvocable := !boolScalarFalse(fm.scalar("user-invocable", "user_invocable", "userinvocable"))

	// A model-invocation-disabled CC skill must not be auto-advertised/invoked, so
	// it is never shared regardless of the request.
	effectiveShared := shared && !disableInvoke

	res = ImportResult{Name: name}
	add := func(f string) { res.MappedFields = append(res.MappedFields, f) }
	warn := func(w string) { res.Warnings = append(res.Warnings, w) }

	var b strings.Builder
	b.WriteString("---\n")
	writeImportScalar(&b, "name", name, add)
	writeImportScalar(&b, "description", desc, add)
	writeImportScalar(&b, "when_to_use", when, add)
	if effectiveShared {
		b.WriteString("access: shared\n")
		add("access")
	}
	writeImportScalar(&b, "version", version, add)
	writeImportScalar(&b, "license", license, add)
	if sourceURL != "" {
		writeImportScalar(&b, "source_url", sourceURL, add)
	}
	// An explicit import group overrides whatever the source declared; otherwise the
	// source's own group carries over (previously dropped entirely).
	grp := strings.TrimSpace(group)
	if grp == "" {
		grp = strings.TrimSpace(fm.scalar("group", "category"))
	}
	if grp != "" {
		writeImportScalar(&b, "group", grp, add)
	}
	if !userInvocable {
		b.WriteString("user_invocable: false\n")
		add("user_invocable")
	}
	writeImportList(&b, "always_allow", allowed, add)
	writeImportList(&b, "paths", paths, add)
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n")

	// Warn about CC features that do not map onto TionSwarm skills.
	if strings.EqualFold(strings.TrimSpace(fm.scalar("context")), "fork") {
		warn("`context: fork` (isolated subagent) is not supported — the skill loads inline; use run_subagent for isolation.")
	}
	if disableInvoke && shared {
		warn("`disable-model-invocation: true` → imported as restricted (not shared) so the model won't auto-invoke it.")
	}
	for _, k := range []string{"hooks", "agent", "model", "effort"} {
		if fm.scalar(k) != "" || len(fm.list(k)) > 0 {
			warn("`" + k + ":` frontmatter is not supported on TionSwarm skills — dropped.")
		}
	}
	if fm.scalar("argument-hint") != "" || len(fm.list("arguments")) > 0 ||
		strings.Contains(body, "$ARGUMENTS") || hasPositionalArg(body) {
		warn("slash-command arguments ($ARGUMENTS / $1…) won't be substituted — the skill loads as instructions, not a /command.")
	}
	if strings.Contains(body, "!`") || strings.Contains(body, "```!") {
		warn("inline shell injection (!`…`) in the body is NOT executed by TionSwarm — convert to explicit Bash tool steps.")
	}
	return b.String(), res
}

// RenderImportedSkill maps a Claude Code SKILL.md to the equivalent TionSwarm
// SKILL.md (frontmatter remapped, unsupported features stripped with warnings) and
// returns the rendered content plus the mapping result. Exposed for the ingest
// pipeline, which packages the rendered body into a market pack. group namespaces
// the import into a single Skills-UI group ("" = keep the source's own group). (SK-IMP2)
func RenderImportedSkill(raw, sourceURL string, shared bool, group string) (string, ImportResult) {
	return mapCCSkill(raw, sourceURL, shared, group)
}

// SuggestSlug derives a slug from a folder path (its base name), falling back to a
// name, then "skill". Exposed for the ingest pipeline.
func SuggestSlug(relPath, name string) string {
	return suggestSlug(relPath, name)
}

// Slugify converts a free-form string into a lowercase kebab-case slug (Turkish
// letters transliterated). Exposed for the ingest pipeline.
func Slugify(s string) string {
	return slugify(s)
}

// ImportFromSource imports a SINGLE Claude Code skill from a source ("local" reads
// a directory; "github" points at one skill folder) into the workspace tier.
// location is the directory path (local) or the URL (github). For multi-skill
// repos use ImportCollection. (SK-IMP)
func (s *Store) ImportFromSource(source, location, slug string, shared bool) (Skill, ImportResult, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "local"
	}
	var raw string
	var files map[string][]byte
	switch source {
	case "local":
		var err error
		raw, files, err = readLocalSkillDir(location)
		if err != nil {
			return Skill{}, ImportResult{}, err
		}
	case "github":
		tree, prefix, _, ferr := fetch.TreeFrom("github", location)
		if ferr != nil {
			return Skill{}, ImportResult{}, ferr
		}
		found := groupSkills(tree, prefix)
		var pick *discoveredSkill
		for i := range found {
			if found[i].relPath == prefix {
				pick = &found[i]
				break
			}
		}
		if pick == nil && len(found) == 1 {
			pick = &found[0]
		}
		if pick == nil {
			return Skill{}, ImportResult{}, fmt.Errorf("point the URL at a single skill folder (found %d skills here; use collection import for the whole repo)", len(found))
		}
		raw, files = pick.raw, pick.files
	default:
		return Skill{}, ImportResult{}, fmt.Errorf("unsupported source %q (use local|github)", source)
	}
	return s.ImportCCSkill(slug, raw, location, files, shared)
}

// readLocalSkillDir reads a skill directory: SKILL.md (required) plus every other
// file (including nested sub-directories like references/ or scripts/) as a bundled
// resource, keyed by its path relative to the skill folder.
func readLocalSkillDir(dir string) (raw string, files map[string][]byte, err error) {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return "", nil, fmt.Errorf("read SKILL.md in %q: %w", dir, err)
	}
	files = map[string][]byte{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil || strings.EqualFold(rel, "SKILL.md") {
			return nil
		}
		if b, rderr := os.ReadFile(p); rderr == nil {
			files[filepath.ToSlash(rel)] = b
		}
		return nil
	})
	return string(data), files, nil
}

// ImportCCSkill maps a Claude Code SKILL.md to a TionSwarm skill and writes it (plus
// its bundled resource files) into the workspace tier, then reloads the catalog.
// slug defaults to the skill name when empty. Fails if the slug already exists.
// (SK-IMP)
func (s *Store) ImportCCSkill(slug, raw, sourceURL string, files map[string][]byte, shared bool) (Skill, ImportResult, error) {
	// Single-skill import keeps the source's own group ("" = no override).
	content, res := mapCCSkill(raw, sourceURL, shared, "")
	if strings.TrimSpace(res.Name) == "" && slug == "" {
		return Skill{}, res, fmt.Errorf("skill has no name and no slug was provided")
	}
	dir, err := s.workspaceDir()
	if err != nil {
		return Skill{}, res, err
	}
	slug = slugify(slug)
	if slug == "" {
		slug = slugify(res.Name)
	}
	if slug == "" {
		return Skill{}, res, fmt.Errorf("could not derive a slug; provide an explicit slug")
	}
	if _, exists := s.Get(slug); exists {
		return Skill{}, res, fmt.Errorf("a skill with slug %q already exists", slug)
	}
	skillDir := filepath.Join(dir, slug)
	if _, statErr := os.Stat(skillDir); statErr == nil {
		return Skill{}, res, fmt.Errorf("a folder named %q already exists in the workspace skills dir", slug)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return Skill{}, res, fmt.Errorf("create skill folder: %w", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		return Skill{}, res, fmt.Errorf("write SKILL.md: %w", err)
	}
	// Copy bundled resource files (SK-1). SKILL.md is rendered above, never copied
	// raw. Nested sub-directories (references/, evals/, scripts/ …) are preserved so
	// progressive-disclosure resources survive the import; path traversal (absolute
	// paths or a ".." segment) is refused so an import can't escape the skill folder.
	for name, data := range files {
		rel, ok := safeBundledPath(name)
		if !ok {
			continue
		}
		dest := filepath.Join(skillDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return Skill{}, res, fmt.Errorf("create dir for bundled file %q: %w", rel, err)
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return Skill{}, res, fmt.Errorf("write bundled file %q: %w", rel, err)
		}
		res.Files = append(res.Files, filepath.ToSlash(rel))
	}
	s.Reload()
	sk, ok := s.Get(slug)
	if !ok {
		return Skill{}, res, fmt.Errorf("imported skill %q did not resolve after write", slug)
	}
	res.Slug = slug
	return sk, res, nil
}

// --- frontmatter render helpers (import-only) ---

func writeImportScalar(b *strings.Builder, key, val string, add func(string)) {
	if strings.TrimSpace(val) == "" {
		return
	}
	b.WriteString(key + ": " + quoteYAML(val) + "\n")
	add(key)
}

func writeImportList(b *strings.Builder, key string, items []string, add func(string)) {
	clean := make([]string, 0, len(items))
	for _, it := range items {
		if it = strings.TrimSpace(it); it != "" {
			clean = append(clean, it)
		}
	}
	if len(clean) == 0 {
		return
	}
	b.WriteString(key + ":\n")
	for _, it := range clean {
		b.WriteString("  - " + quoteYAML(it) + "\n")
	}
	add(key)
}

// boolScalar reports whether a frontmatter scalar is an explicit true.
func boolScalar(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}

// boolScalarFalse reports whether a frontmatter scalar is an explicit false.
func boolScalarFalse(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "no", "off", "0":
		return true
	}
	return false
}

// safeBundledPath validates a bundled-resource path relative to a skill folder.
// It rejects SKILL.md (rendered separately), absolute paths, and any path that
// escapes the folder via a ".." segment, returning a cleaned forward-slashed
// relative path on success. Nested sub-directories are allowed and preserved.
func safeBundledPath(name string) (string, bool) {
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

// hasPositionalArg reports whether the body uses a $1..$9 slash-command argument.
func hasPositionalArg(body string) bool {
	for d := '1'; d <= '9'; d++ {
		if strings.Contains(body, "$"+string(d)) {
			return true
		}
	}
	return false
}
