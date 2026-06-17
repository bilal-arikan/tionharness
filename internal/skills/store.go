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

// New builds a store over the three skill tiers. Any dir may be empty/missing;
// missing dirs are simply skipped. Priority is project > workspace > global, so
// they are scanned in ascending priority and later tiers overwrite earlier ones.
func New(globalDir, workspaceDir, projectDir string) *Store {
	var tiers []tier
	if globalDir != "" {
		tiers = append(tiers, tier{globalDir, SourceGlobal})
	}
	if workspaceDir != "" {
		tiers = append(tiers, tier{workspaceDir, SourceWorkspace})
	}
	if projectDir != "" {
		tiers = append(tiers, tier{projectDir, SourceProject})
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
			Source:          t.source,
			Path:            path,
		})
	}
	return out
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
		return "", fmt.Errorf("read skill %q: %w", slug, err)
	}
	_, body := parseFrontmatter(string(data))
	return strings.TrimSpace(body), nil
}

// CatalogBlock renders the system-prompt section advertising the available
// skills. It lists only slug + description + when-to-use (frontmatter only), and
// instructs the model to call use_skill to load a skill's full instructions.
// Returns "" when there are no skills.
func (s *Store) CatalogBlock() string {
	list := s.List()
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
