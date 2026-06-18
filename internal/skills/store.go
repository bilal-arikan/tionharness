package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// tier pairs a directory with the source label skills loaded from it carry.
type tier struct {
	dir    string
	source Source
}

// Store resolves and caches skills from the three tiers. It is safe for
// concurrent use. The cache holds only frontmatter metadata; bodies are read
// from disk on demand so edits are always picked up and the context stays lean.
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
		out = append(out, Skill{
			Slug:            slug,
			Name:            name,
			Description:     fm.scalar("description"),
			WhenToUse:       fm.scalar("when_to_use", "whentouse", "when"),
			Icon:            fm.scalar("icon"),
			Color:           fm.scalar("color"),
			AlwaysAllow:     fm.list("alwaysallow", "always_allow"),
			RequiredSources: fm.list("requiredsources", "required_sources"),
			Shared:          isShared(fm),
			Source:          t.source,
			Path:            path,
		})
	}
	return out
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

// CatalogBlock renders the prompt section advertising EVERY available skill.
func (s *Store) CatalogBlock() string {
	return renderCatalog(s.List())
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
	return renderCatalog(picked)
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
		if !seen[sk.Slug] {
			out = append(out, sk)
			seen[sk.Slug] = true
		}
	}
	return out
}

// CatalogBlockForAgent renders the Available Skills block an agent sees: its
// assigned skills (in order) plus all shared (on-demand) skills. "" when neither.
func (s *Store) CatalogBlockForAgent(assigned []string) string {
	return renderCatalog(s.effectiveFor(assigned))
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

// renderCatalog builds the "# Available Skills" block from a resolved skill list
// (already in the desired order). Lists only slug + description + when-to-use
// (frontmatter), and instructs the model to call use_skill to load the body.
// Returns "" when the list is empty.
func renderCatalog(list []Skill) string {
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Available Skills\n")
	b.WriteString("You have reusable skills — predefined instruction sets for specific tasks. " +
		"Each entry below shows only a slug and a short summary. When a task matches a skill, " +
		"call the `use_skill` tool with that slug to load its full instructions BEFORE acting. " +
		"Do not guess a skill's contents from its summary.\n")
	for _, sk := range list {
		fmt.Fprintf(&b, "- `%s` — %s", sk.Slug, sk.Description)
		if sk.WhenToUse != "" {
			fmt.Fprintf(&b, " (when: %s)", sk.WhenToUse)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
