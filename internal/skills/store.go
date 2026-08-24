package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// tier pairs a directory with the source label skills loaded from it carry.
type tier struct {
	dir    string
	source Source
}

// Store resolves and caches skills from the two tiers (global + workspace). It
// is safe for concurrent use. The cache holds only frontmatter metadata; bodies
// are read from disk on demand so edits are always picked up and the context stays lean.
type Store struct {
	tiers []tier

	mu     sync.RWMutex
	loaded bool
	bySlug map[string]Skill // slug -> resolved (highest-priority) skill
	order  []string         // slugs, display order (sorted by name)
}

// New builds a store over the two skill tiers. Any dir may be empty/missing;
// missing dirs are simply skipped. Priority is workspace > global, so they are
// scanned in ascending priority and the workspace tier overrides the global one.
func New(globalDir, workspaceDir string) *Store {
	var tiers []tier
	if globalDir != "" {
		tiers = append(tiers, tier{globalDir, SourceGlobal})
	}
	if workspaceDir != "" {
		tiers = append(tiers, tier{workspaceDir, SourceWorkspace})
	}
	return &Store{tiers: tiers, bySlug: map[string]Skill{}}
}

// ensure lazily loads the catalog on first use.
func (s *Store) ensure() {
	s.mu.RLock()
	done := s.loaded
	s.mu.RUnlock()
	if done {
		return
	}
	s.Reload()
}

// Reload re-scans every tier, rebuilding the catalog. Cheap: it reads only each
// SKILL.md's frontmatter, not its body.
func (s *Store) Reload() {
	bySlug := map[string]Skill{}
	for _, t := range s.tiers { // ascending priority: later overrides earlier
		for _, sk := range scanDir(t) {
			bySlug[sk.Slug] = sk
		}
	}
	order := make([]string, 0, len(bySlug))
	for slug := range bySlug {
		order = append(order, slug)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := bySlug[order[i]], bySlug[order[j]]
		if strings.EqualFold(a.Name, b.Name) {
			return a.Slug < b.Slug
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})

	s.mu.Lock()
	s.bySlug = bySlug
	s.order = order
	s.loaded = true
	s.mu.Unlock()
}

// scanDir reads every <dir>/<slug>/SKILL.md and parses its frontmatter.
func scanDir(t tier) []Skill {
	entries, err := os.ReadDir(t.dir)
	if err != nil {
		return nil // missing/inaccessible dir → no skills
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		if strings.HasPrefix(slug, ".") {
			continue
		}
		path := filepath.Join(t.dir, slug, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue // no SKILL.md → not a skill dir
		}
		fm, _ := parseFrontmatter(string(data))
		name := fm.scalar("name")
		if name == "" {
			name = slug
		}
		sk := Skill{
			Slug:            slug,
			Name:            name,
			Description:     fm.scalar("description"),
			WhenToUse:       fm.scalar("when_to_use", "whentouse", "when"),
			Icon:            fm.scalar("icon"),
			Color:           fm.scalar("color"),
			Group:           fm.scalar("group", "category"),
			AlwaysAllow:     fm.list("alwaysallow", "always_allow"),
			RequiredSources: fm.list("requiredsources", "required_sources"),
			SubSkills:       fm.list("subskills", "sub_skills", "related"),
			Paths:           fm.list("paths", "path"),
			Version:         fm.scalar("version"),
			SourceURL:       fm.scalar("source_url", "sourceurl", "repo", "homepage"),
			License:         fm.scalar("license"),
			Kind:            fm.scalar("kind"),
			Pattern:         fm.scalar("pattern"),
			WorkerTargets:   fm.list("worker_targets", "workertargets", "targets"),
			StopCondition:   fm.scalar("stop_condition", "stopcondition"),
			MaxTurns:        parseIntFrontmatter(fm.scalar("max_turns", "maxturns")),
			UserInvocable:   isUserInvocable(fm),
			Shared:          isShared(fm),
			AutoSummary:     isAutoSummary(fm),
			NameOnly:        isNameOnly(fm),
			SummaryOnly:     isSummaryOnly(fm),
			Source:          t.source,
			Path:            path,
		}
		// Stamp the SKILL.md last-modified time (Unix seconds) so the UI can show
		// a "last edited" date and sort by recency. Best-effort: 0 if stat fails.
		if info, statErr := os.Stat(path); statErr == nil {
			sk.ModifiedAt = info.ModTime().Unix()
		}
		// How this file compares to the skill TionHarness ships — only meaningful in
		// the GLOBAL tier, where the defaults are seeded. A workspace-tier skill of
		// the same slug is a deliberate override living in a different file, so it
		// has no shipped default to be measured against or restored from.
		if t.source == SourceGlobal {
			sk.DefaultState = DefaultState(t.dir, slug)
		}
		sk.Visibility = skillVisibility(sk)
		out = append(out, sk)
	}
	return out
}

// skillVisibility derives a skill's 4-way visibility tier from its underlying
// frontmatter flags (the inverse of Store.SetVisibility). NameOnly beats
// SummaryOnly when both are somehow set. A skill whose summary is not
// auto-injected reads as hidden. Paths (conditional skills) are a separate
// discovery mechanism and do NOT change the displayed tier.
func skillVisibility(sk Skill) string {
	if !sk.AutoSummary {
		return VisibilityHidden
	}
	if sk.NameOnly {
		return VisibilityNameOnly
	}
	if sk.SummaryOnly {
		return VisibilitySummary
	}
	return VisibilityFull
}

// parseIntFrontmatter parses an optional integer frontmatter scalar. An empty
// value yields 0 (meaning "unset → use the default"); a present-but-malformed
// value also yields 0 rather than failing the whole scan, since these fields are
// optional overrides. Hard validation of required fields (e.g. pattern) happens
// at apply time, not during the lazy catalog scan.
func parseIntFrontmatter(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// isShared reports whether a skill's frontmatter marks it as on-demand/shared
// (visible to every agent). Accepts `access: shared` (or "auto"/"on-demand") and
// the boolean `shared: true`. Anything else (incl. absent) means restricted.
func isShared(fm frontmatter) bool {
	switch strings.ToLower(strings.TrimSpace(fm.scalar("access"))) {
	case "shared", "auto", "on-demand", "ondemand", "public":
		return true
	}
	return strings.EqualFold(strings.TrimSpace(fm.scalar("shared")), "true")
}

// isAutoSummary reports whether a skill's one-line summary should be auto-injected
// into every agent's prompt. Defaults to TRUE (absent key → enabled); only an
// explicit `auto_summary: false` (or no/off/0) disables it.
func isAutoSummary(fm frontmatter) bool {
	switch strings.ToLower(strings.TrimSpace(fm.scalar("auto_summary", "autosummary", "auto_include", "autoinclude"))) {
	case "false", "no", "off", "0":
		return false
	}
	return true
}

// isNameOnly reports whether a skill should be advertised as SLUG ONLY in the
// Available Skills block (description + when-to-use suppressed). Defaults to FALSE
// (absent key → full summary); only an explicit `name_only: true` (or yes/on/1)
// enables it. See Skill.NameOnly.
func isNameOnly(fm frontmatter) bool {
	switch strings.ToLower(strings.TrimSpace(fm.scalar("name_only", "nameonly"))) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}

// isSummaryOnly reports whether a skill should be advertised as slug +
// description only (when-to-use suppressed) in the Available Skills block.
// Defaults to FALSE; only an explicit `summary_only: true` (or yes/on/1) enables
// it. See Skill.SummaryOnly.
func isSummaryOnly(fm frontmatter) bool {
	switch strings.ToLower(strings.TrimSpace(fm.scalar("summary_only", "summaryonly"))) {
	case "true", "yes", "on", "1":
		return true
	}
	return false
}

// isUserInvocable mirrors Claude Code's user-invocable (default TRUE). Only an
// explicit false/no/off/0 disables it. (SK-4)
func isUserInvocable(fm frontmatter) bool {
	switch strings.ToLower(strings.TrimSpace(fm.scalar("user_invocable", "user-invocable", "userinvocable"))) {
	case "false", "no", "off", "0":
		return false
	}
	return true
}

// List returns the resolved skills in display order.
func (s *Store) List() []Skill {
	s.ensure()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Skill, 0, len(s.order))
	for _, slug := range s.order {
		out = append(out, s.bySlug[slug])
	}
	return out
}

// Get returns the resolved skill for a slug.
func (s *Store) Get(slug string) (Skill, bool) {
	s.ensure()
	s.mu.RLock()
	defer s.mu.RUnlock()
	sk, ok := s.bySlug[slug]
	return sk, ok
}

// Empty reports whether no skills are available.
func (s *Store) Empty() bool {
	s.ensure()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.bySlug) == 0
}

// Body reads and returns a skill's markdown instructions (frontmatter stripped),
// reading from disk so edits are always reflected. This is the lazy half of the
// design: the body is fetched only when a skill is actually invoked.
func (s *Store) Body(slug string) (string, error) {
	sk, ok := s.Get(slug)
	if !ok {
		return "", fmt.Errorf("skill %q not found", slug)
	}
	data, err := os.ReadFile(sk.Path)
	if err != nil {
		if os.IsNotExist(err) {
			// The file was deleted/moved out-of-band (the catalog held a stale
			// path). Refresh so the missing skill drops, and report it clearly.
			s.Reload()
			return "", fmt.Errorf("skill %q is no longer available (its file was moved or deleted); catalog refreshed", slug)
		}
		return "", fmt.Errorf("read skill %q: %w", slug, err)
	}
	_, body := parseFrontmatter(string(data))
	return strings.TrimSpace(body), nil
}

// UseSkillBody returns a skill's body for the use_skill tool: its markdown
// instructions plus a footer advertising any sub-skills the model may load next
// (progressive disclosure). When allow is non-nil, sub-skills outside that set
// are omitted (the agent could not load them anyway); a nil allow lists all
// known sub-skills. The plain Body method is left untouched for the raw detail
// view — only the tool path appends the footer.
func (s *Store) UseSkillBody(slug string, allow map[string]bool) (string, error) {
	body, err := s.Body(slug)
	if err != nil {
		return "", err
	}
	sk, _ := s.Get(slug)
	// SK-1: expand ${SKILL_DIR} so the body can point at bundled resources, then
	// advertise any sibling files + sub-skills as on-demand footers.
	body = substituteSkillVars(body, sk)
	var footers []string
	if f := bundledFilesFooter(sk); f != "" {
		footers = append(footers, f)
	}
	if f := s.subskillFooter(sk, allow); f != "" {
		footers = append(footers, f)
	}
	if len(footers) > 0 {
		body = strings.TrimSpace(body) + "\n\n" + strings.Join(footers, "\n\n")
	}
	return body, nil
}

// substituteSkillVars expands ${SKILL_DIR} (and the Claude Code-compatible
// ${CLAUDE_SKILL_DIR} alias) in a skill body to the absolute directory holding the
// skill's SKILL.md, so the body can reference bundled resource files (reference
// docs, templates, scripts) the agent then reads with the fs tools. The path is
// forward-slashed so it is safe to embed in shell/markdown on Windows. Mirrors
// Claude Code's createSkillCommand baseDir substitution.
func substituteSkillVars(body string, sk Skill) string {
	if sk.Path == "" {
		return body
	}
	dir := filepath.ToSlash(filepath.Dir(sk.Path))
	return strings.NewReplacer("${SKILL_DIR}", dir, "${CLAUDE_SKILL_DIR}", dir).Replace(body)
}

// bundledFilesFooter lists the resource files shipped alongside a skill's SKILL.md
// (templates, reference docs, scripts) so the agent knows they exist and can load
// them on demand with the Read tool — the multi-file half of progressive
// disclosure. The SKILL.md itself is excluded; nested dirs are listed as a path.
// Returns "" for a lone-SKILL.md skill.
func bundledFilesFooter(sk Skill) string {
	if sk.Path == "" {
		return ""
	}
	dir := filepath.Dir(sk.Path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var items []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			items = append(items, fmt.Sprintf("- `%s/` (directory)", filepath.ToSlash(filepath.Join(dir, name))))
			continue
		}
		if strings.EqualFold(name, "SKILL.md") {
			continue
		}
		items = append(items, fmt.Sprintf("- `%s`", filepath.ToSlash(filepath.Join(dir, name))))
	}
	if len(items) == 0 {
		return ""
	}
	return "---\n## Bundled files\n" +
		"This skill ships with extra resource files. Read them with the `Read` tool " +
		"only when the task needs them:\n" + strings.Join(items, "\n")
}

// subskillFooter renders the "Related skills" block for a skill's declared
// sub-skills, skipping unknown, self-referential and (when allow is set)
// disallowed slugs. Returns "" when nothing remains to advertise.
func (s *Store) subskillFooter(sk Skill, allow map[string]bool) string {
	if len(sk.SubSkills) == 0 {
		return ""
	}
	var items []string
	seen := map[string]bool{sk.Slug: true}
	for _, sub := range sk.SubSkills {
		sub = strings.TrimSpace(sub)
		if sub == "" || seen[sub] {
			continue
		}
		seen[sub] = true
		if allow != nil && !allow[sub] {
			continue
		}
		child, ok := s.Get(sub)
		if !ok {
			continue
		}
		if child.Description != "" {
			items = append(items, fmt.Sprintf("- `%s` — %s", sub, child.Description))
		} else {
			items = append(items, fmt.Sprintf("- `%s`", sub))
		}
	}
	if len(items) == 0 {
		return ""
	}
	return "---\n## Related skills\n" +
		"This skill builds on more detailed skills. When the task needs them, call " +
		"`use_skill` with the slug to load their full instructions:\n" +
		strings.Join(items, "\n")
}

// SetAccess flips a skill's access mode by rewriting its SKILL.md frontmatter,
// then reloads the catalog. Returns the updated skill.
func (s *Store) SetAccess(slug string, shared bool) (Skill, error) {
	sk, ok := s.Get(slug)
	if !ok {
		return Skill{}, fmt.Errorf("skill %q not found", slug)
	}
	data, err := os.ReadFile(sk.Path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Reload()
			return Skill{}, fmt.Errorf("skill %q is no longer available (its file was moved or deleted); catalog refreshed", slug)
		}
		return Skill{}, fmt.Errorf("read skill %q: %w", slug, err)
	}
	updated := setFrontmatterAccess(string(data), shared)
	if err := os.WriteFile(sk.Path, []byte(updated), 0o644); err != nil {
		return Skill{}, fmt.Errorf("write skill %q: %w", slug, err)
	}
	s.Reload()
	out, _ := s.Get(slug)
	return out, nil
}

// SetAutoSummary toggles whether a skill's summary is auto-injected into every
// agent's prompt, by rewriting its SKILL.md frontmatter, then reloads the
// catalog. Returns the updated skill.
func (s *Store) SetAutoSummary(slug string, on bool) (Skill, error) {
	sk, ok := s.Get(slug)
	if !ok {
		return Skill{}, fmt.Errorf("skill %q not found", slug)
	}
	data, err := os.ReadFile(sk.Path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Reload()
			return Skill{}, fmt.Errorf("skill %q is no longer available (its file was moved or deleted); catalog refreshed", slug)
		}
		return Skill{}, fmt.Errorf("read skill %q: %w", slug, err)
	}
	updated := setFrontmatterAutoSummary(string(data), on)
	if err := os.WriteFile(sk.Path, []byte(updated), 0o644); err != nil {
		return Skill{}, fmt.Errorf("write skill %q: %w", slug, err)
	}
	s.Reload()
	out, _ := s.Get(slug)
	return out, nil
}

// SetNameOnly toggles whether a skill is advertised as slug-only (description +
// when-to-use suppressed) in the Available Skills block, by rewriting its SKILL.md
// frontmatter, then reloads the catalog. Returns the updated skill.
func (s *Store) SetNameOnly(slug string, on bool) (Skill, error) {
	sk, ok := s.Get(slug)
	if !ok {
		return Skill{}, fmt.Errorf("skill %q not found", slug)
	}
	data, err := os.ReadFile(sk.Path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Reload()
			return Skill{}, fmt.Errorf("skill %q is no longer available (its file was moved or deleted); catalog refreshed", slug)
		}
		return Skill{}, fmt.Errorf("read skill %q: %w", slug, err)
	}
	updated := setFrontmatterNameOnly(string(data), on)
	if err := os.WriteFile(sk.Path, []byte(updated), 0o644); err != nil {
		return Skill{}, fmt.Errorf("write skill %q: %w", slug, err)
	}
	s.Reload()
	out, _ := s.Get(slug)
	return out, nil
}

// SetVisibility forces a skill into exactly one of the four visibility tiers
// (full | summary | name-only | hidden) by rewriting its SKILL.md frontmatter
// flags together in a single write, then reloads the catalog. This is the skill
// analogue of tools.Registry.SetVisibility and the single entry point the Skills
// screen's 4-way selector drives. Returns the updated skill; an invalid tier is
// rejected so a typo can't silently leave a skill at its old tier.
func (s *Store) SetVisibility(slug, tier string) (Skill, error) {
	var autoSummary, nameOnly, summaryOnly bool
	switch tier {
	case VisibilityFull:
		autoSummary = true
	case VisibilitySummary:
		autoSummary, summaryOnly = true, true
	case VisibilityNameOnly:
		autoSummary, nameOnly = true, true
	case VisibilityHidden:
		// all false: summary not auto-injected → folded out of the catalog.
	default:
		return Skill{}, fmt.Errorf("invalid visibility tier %q (want full|summary|name-only|hidden)", tier)
	}
	sk, ok := s.Get(slug)
	if !ok {
		return Skill{}, fmt.Errorf("skill %q not found", slug)
	}
	data, err := os.ReadFile(sk.Path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Reload()
			return Skill{}, fmt.Errorf("skill %q is no longer available (its file was moved or deleted); catalog refreshed", slug)
		}
		return Skill{}, fmt.Errorf("read skill %q: %w", slug, err)
	}
	updated := setFrontmatterAutoSummary(string(data), autoSummary)
	updated = setFrontmatterNameOnly(updated, nameOnly)
	updated = setFrontmatterSummaryOnly(updated, summaryOnly)
	if err := os.WriteFile(sk.Path, []byte(updated), 0o644); err != nil {
		return Skill{}, fmt.Errorf("write skill %q: %w", slug, err)
	}
	s.Reload()
	out, _ := s.Get(slug)
	return out, nil
}

// SetGroup rewrites a skill's `group` frontmatter (dropping the `category` alias)
// without touching any other field or its body, then reloads the catalog. An empty
// group removes the marker (skill becomes ungrouped). This is the single-skill entry
// point the Skills screen's bulk "set group" action drives per selected skill.
func (s *Store) SetGroup(slug, group string) (Skill, error) {
	sk, ok := s.Get(slug)
	if !ok {
		return Skill{}, fmt.Errorf("skill %q not found", slug)
	}
	data, err := os.ReadFile(sk.Path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Reload()
			return Skill{}, fmt.Errorf("skill %q is no longer available (its file was moved or deleted); catalog refreshed", slug)
		}
		return Skill{}, fmt.Errorf("read skill %q: %w", slug, err)
	}
	// Drop the `category` alias so the two can't disagree, then upsert/remove `group`.
	updated := setFrontmatterFields(string(data), []fmField{
		{Key: "category", Val: ""},
		{Key: "group", Val: strings.TrimSpace(group)},
	}, nil)
	if err := os.WriteFile(sk.Path, []byte(updated), 0o644); err != nil {
		return Skill{}, fmt.Errorf("write skill %q: %w", slug, err)
	}
	s.Reload()
	out, _ := s.Get(slug)
	return out, nil
}

// SkillInput carries the editable fields used to create or update a skill. It
// maps onto the SKILL.md frontmatter (plus the markdown body).
type SkillInput struct {
	Name        string
	Description string
	WhenToUse   string
	Icon        string
	Color       string
	Group       string
	Shared      bool
	Body        string
}

// fields renders the input's frontmatter as an ordered scalar list. Descriptions
// and when-to-use are collapsed to a single line (the parser is line-based).
func (in SkillInput) fields() []fmField {
	access := ""
	if in.Shared {
		access = "shared"
	}
	return []fmField{
		{"name", strings.TrimSpace(in.Name)},
		{"description", oneLine(in.Description)},
		{"when_to_use", oneLine(in.WhenToUse)},
		{"icon", strings.TrimSpace(in.Icon)},
		{"color", strings.TrimSpace(in.Color)},
		{"group", strings.TrimSpace(in.Group)},
		{"access", access},
	}
}

// workspaceDir returns the workspace tier directory (where new skills are
// created). Errors when this store has no workspace tier (e.g. unknown workdir).
func (s *Store) workspaceDir() (string, error) {
	for _, t := range s.tiers {
		if t.source == SourceWorkspace && t.dir != "" {
			return t.dir, nil
		}
	}
	return "", fmt.Errorf("no workspace skills directory is configured")
}

// Create writes a new skill into the workspace tier and reloads the catalog.
// When slug is empty it is derived from the name. Fails if the slug already
// resolves (in any tier) or its folder exists.
func (s *Store) Create(slug string, in SkillInput) (Skill, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Skill{}, fmt.Errorf("skill name is required")
	}
	dir, err := s.workspaceDir()
	if err != nil {
		return Skill{}, err
	}
	slug = slugify(slug)
	if slug == "" {
		slug = slugify(in.Name)
	}
	if slug == "" {
		return Skill{}, fmt.Errorf("could not derive a slug from the name; provide an explicit slug")
	}
	if _, exists := s.Get(slug); exists {
		return Skill{}, fmt.Errorf("a skill with slug %q already exists", slug)
	}
	skillDir := filepath.Join(dir, slug)
	if _, statErr := os.Stat(skillDir); statErr == nil {
		return Skill{}, fmt.Errorf("a folder named %q already exists in the workspace skills dir", slug)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return Skill{}, fmt.Errorf("create skill folder: %w", err)
	}
	body := in.Body
	if strings.TrimSpace(body) == "" {
		body = "# " + strings.TrimSpace(in.Name) + "\n\nWrite the skill instructions here."
	}
	content := setFrontmatterFields("", in.fields(), &body)
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Skill{}, fmt.Errorf("write SKILL.md: %w", err)
	}
	s.Reload()
	out, _ := s.Get(slug)
	return out, nil
}

// Update rewrites an existing skill's SKILL.md (frontmatter + body) in place,
// preserving any unmanaged frontmatter keys, then reloads the catalog. The slug
// (folder name) is immutable.
func (s *Store) Update(slug string, in SkillInput) (Skill, error) {
	if strings.TrimSpace(in.Name) == "" {
		return Skill{}, fmt.Errorf("skill name is required")
	}
	sk, ok := s.Get(slug)
	if !ok {
		return Skill{}, fmt.Errorf("skill %q not found", slug)
	}
	data, err := os.ReadFile(sk.Path)
	if err != nil {
		if os.IsNotExist(err) {
			s.Reload()
			return Skill{}, fmt.Errorf("skill %q is no longer available (its file was moved or deleted); catalog refreshed", slug)
		}
		return Skill{}, fmt.Errorf("read skill %q: %w", slug, err)
	}
	body := in.Body
	content := setFrontmatterFields(string(data), in.fields(), &body)
	if err := os.WriteFile(sk.Path, []byte(content), 0o644); err != nil {
		return Skill{}, fmt.Errorf("write skill %q: %w", slug, err)
	}
	s.Reload()
	out, _ := s.Get(slug)
	return out, nil
}

// Delete removes a skill's backing folder (and SKILL.md) from disk, then reloads
// the catalog. Guarded so it only ever removes a direct child of a known tier
// directory — never a tier root or anything outside it.
func (s *Store) Delete(slug string) error {
	sk, ok := s.Get(slug)
	if !ok {
		return fmt.Errorf("skill %q not found", slug)
	}
	if sk.Path == "" {
		return fmt.Errorf("skill %q has no backing file", slug)
	}
	skillDir := filepath.Dir(sk.Path)
	parent := filepath.Dir(skillDir)
	safe := false
	for _, t := range s.tiers {
		if t.dir != "" && filepath.Clean(parent) == filepath.Clean(t.dir) {
			safe = true
			break
		}
	}
	if !safe || filepath.Base(skillDir) == "" {
		return fmt.Errorf("refusing to delete %q: not inside a known skills directory", slug)
	}
	if err := os.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("delete skill %q: %w", slug, err)
	}
	s.Reload()
	return nil
}

// slugify converts a free-form name into a lowercase kebab-case slug. Common
// Turkish letters are transliterated; other non-ASCII characters are dropped.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer(
		"ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u", "İ", "i",
	)
	s = repl.Replace(s)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.' || r == '/':
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// oneLine collapses internal newlines/tabs to single spaces and trims — keeping
// a scalar frontmatter value on one line.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.Join(strings.Fields(s), " ")
}

// DefaultSkillTool is the identifier the native tool loop registers for the
// skill-loading tool. claude-cli agents reach it namespaced through the
// Interaction MCP bridge, so callers there pass the namespaced name instead.
const DefaultSkillTool = "use_skill"

// CatalogBlock renders the prompt section advertising EVERY available skill.
func (s *Store) CatalogBlock() string {
	return renderCatalog(s.List(), DefaultSkillTool)
}

// CatalogBlockFor renders the prompt section for a specific ordered selection of
// slugs (an agent's chosen skills). Unknown/blank/duplicate slugs are skipped;
// the given order is preserved. Returns "" when no known skill remains.
func (s *Store) CatalogBlockFor(slugs []string) string {
	if len(slugs) == 0 {
		return ""
	}
	picked := make([]Skill, 0, len(slugs))
	seen := map[string]bool{}
	for _, slug := range slugs {
		slug = strings.TrimSpace(slug)
		if slug == "" || seen[slug] {
			continue
		}
		if sk, ok := s.Get(slug); ok {
			picked = append(picked, sk)
			seen[slug] = true
		}
	}
	return renderCatalog(picked, DefaultSkillTool)
}

// SharedList returns the shared (on-demand) skills in display order.
func (s *Store) SharedList() []Skill {
	out := []Skill{}
	for _, sk := range s.List() {
		if sk.Shared {
			out = append(out, sk)
		}
	}
	return out
}

// Search returns skills whose slug/name/description/when-to-use contains EVERY
// whitespace-separated term of the query (case-insensitive), in display order,
// capped at limit (<=0 → uncapped). Powers the skill_search tool so an agent can
// discover on-demand and conditional (paths-gated) skills that are deliberately
// kept out of the per-turn catalog. An empty query returns the full list. (SK-2)
func (s *Store) Search(query string, limit int) []Skill {
	terms := strings.Fields(strings.ToLower(query))
	all := s.List()
	out := make([]Skill, 0, len(all))
	for _, sk := range all {
		hay := strings.ToLower(sk.Slug + " " + sk.Name + " " + sk.Description + " " + sk.WhenToUse)
		match := true
		for _, t := range terms {
			if !strings.Contains(hay, t) {
				match = false
				break
			}
		}
		if match {
			out = append(out, sk)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

// effectiveFor returns the skills visible to an agent: its assigned skills first
// (in the given order, known + de-duplicated), then any shared skills it has not
// already assigned. This is the set advertised in the agent's prompt.
func (s *Store) effectiveFor(assigned []string) []Skill {
	out := []Skill{}
	seen := map[string]bool{}
	for _, slug := range assigned {
		slug = strings.TrimSpace(slug)
		if slug == "" || seen[slug] {
			continue
		}
		if sk, ok := s.Get(slug); ok {
			out = append(out, sk)
			seen[slug] = true
		}
	}
	for _, sk := range s.SharedList() {
		// A shared skill with auto-summary turned off is NOT advertised
		// automatically — it only reaches an agent via explicit assignment
		// (handled by the assigned loop above, which already ran).
		if !sk.AutoSummary {
			continue
		}
		// SK-2: a CONDITIONAL skill (non-empty paths) is never auto-advertised —
		// it stays out of the prompt and is reached via skill_search or explicit
		// assignment. This is what keeps the catalog lean as skill count grows.
		if len(sk.Paths) > 0 {
			continue
		}
		if !seen[sk.Slug] {
			out = append(out, sk)
			seen[sk.Slug] = true
		}
	}
	return out
}

// CatalogBlockForAgent renders the Available Skills block an agent sees: its
// assigned skills (in order) plus all shared (on-demand) skills. "" when neither.
// Uses the default (bare) skill-tool name; see CatalogBlockForAgentTool to render
// the namespaced name a claude-cli agent must use.
func (s *Store) CatalogBlockForAgent(assigned []string) string {
	return renderCatalog(s.effectiveFor(assigned), DefaultSkillTool)
}

// CatalogBlockForAgentTool is CatalogBlockForAgent with an explicit skill-tool
// identifier, so the block names the tool exactly as the target agent will see it
// (bare for native providers, namespaced for claude-cli's MCP bridge). An empty
// skillTool falls back to the default.
func (s *Store) CatalogBlockForAgentTool(assigned []string, skillTool string) string {
	return renderCatalog(s.effectiveFor(assigned), skillTool)
}

// AllowedFor returns the set of skill slugs an agent may load via use_skill: its
// assigned skills plus every shared skill.
func (s *Store) AllowedFor(assigned []string) map[string]bool {
	allow := map[string]bool{}
	for _, slug := range assigned {
		if slug = strings.TrimSpace(slug); slug != "" {
			allow[slug] = true
		}
	}
	for _, sk := range s.SharedList() {
		allow[sk.Slug] = true
	}
	return allow
}

// Catalog line caps. A SKILL.md's frontmatter is USER-AUTHORED free text, but the
// "# Available Skills" block rides every turn's cached prefix — so one verbose
// description taxes every agent, every turn, forever. Cap each field the way the
// tool catalog already caps a lazy tool's summary (tools.lazyCatalogDescMaxChars =
// 200): keep the identifying first sentence, drop the essay. Nothing is lost —
// skill_search returns the full frontmatter, and use_skill loads the real body.
const (
	catalogDescMaxChars = 200
	catalogWhenMaxChars = 160
)

// catalogLine reduces a (possibly multi-paragraph) frontmatter field to a single
// catalog line: the first non-empty line, hard-capped on a UTF-8 rune boundary.
func catalogLine(s string, max int) string {
	line := ""
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			line = t
			break
		}
	}
	if utf8.RuneCountInString(line) <= max {
		return line
	}
	return strings.TrimSpace(string([]rune(line)[:max])) + "…"
}

// renderCatalog builds the "# Available Skills" block from a resolved skill list
// (already in the desired order). Lists only slug + description + when-to-use
// (frontmatter), and instructs the model to call use_skill to load the body.
// Returns "" when the list is empty.
func renderCatalog(list []Skill, skillTool string) string {
	if len(list) == 0 {
		return ""
	}
	if strings.TrimSpace(skillTool) == "" {
		skillTool = DefaultSkillTool
	}
	// Sibling skill_search tool name + deferred-activation guidance. A claude-cli
	// agent reaches these through the Interaction MCP bridge, where the names are
	// namespaced (mcp__tionharness_interaction__use_skill) AND may be DEFERRED by the CLI
	// when many MCP tools are present (e.g. a large gateway). When the tool is
	// namespaced, point the model at ToolSearch up front so it does not waste its
	// first call on a rejected/unloaded name. We deliberately keep this to a single
	// short clause: the full deferred-loading mechanism (what "DEFERRED" means, that
	// an unloaded name returns "No such tool available") is explained ONCE in the
	// "Available Tools (load on demand)" block and not repeated here. Native (bare)
	// agents get the schema eagerly, so no note is needed there.
	searchTool := "skill_search"
	var deferNote string
	if i := strings.LastIndex(skillTool, "__"); i > 0 && strings.HasPrefix(skillTool, "mcp__") {
		searchTool = skillTool[:i+2] + searchTool // share the namespace prefix
		deferNote = fmt.Sprintf("\nThese may be DEFERRED MCP tools: run `ToolSearch` with "+
			"`select:%s,%s` to load them before your first call (see the Available Tools "+
			"note for how deferred loading works).", skillTool, searchTool)
	}
	var b strings.Builder
	b.WriteString("# Available Skills\n")
	fmt.Fprintf(&b, "Reusable instruction sets, listed as slug + summary. When a task matches one, "+
		"call the `%s` tool with its slug to load the full instructions BEFORE acting — "+
		"don't guess from the summary.%s\n", skillTool, deferNote)
	for _, sk := range list {
		// NameOnly skills are listed by slug alone (description + when-to-use
		// suppressed) — the model sees the skill exists and uses skill_search to
		// learn what it does before use_skill. Mirrors a tool's NameOnly tier.
		if sk.NameOnly {
			fmt.Fprintf(&b, "- `%s`\n", sk.Slug)
			continue
		}
		// SummaryOnly skills show slug + description but NOT their when-to-use —
		// the leaner "summary" tier between full and name-only.
		fmt.Fprintf(&b, "- `%s` — %s", sk.Slug, catalogLine(sk.Description, catalogDescMaxChars))
		if sk.WhenToUse != "" && !sk.SummaryOnly {
			fmt.Fprintf(&b, " (when: %s)", catalogLine(sk.WhenToUse, catalogWhenMaxChars))
		}
		b.WriteString("\n")
	}
	// SK-2: not every skill is listed here — on-demand/conditional skills are kept
	// out to save context. Some listed skills show their slug ALONE (summary
	// suppressed). For either case, skill_search reveals what a skill does.
	fmt.Fprintf(&b, "Entries shown as a slug alone (no summary) and skills not listed at all "+
		"(on-demand/conditional) are discoverable with `%s`: call it with keywords to see "+
		"what a skill does before loading it.", searchTool)
	return strings.TrimSpace(b.String())
}
