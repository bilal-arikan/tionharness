package insight

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/seed"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// Lens (a.k.a. scan intent) is a user-editable file describing one scan purpose.
// It lives as markdown under <store>/insight/lenses/<id>.md — frontmatter config
// plus a markdown body that is the LLM analysis instruction — mirroring the skill
// format so users edit lenses the same way they edit skills. Built-in defaults
// are seeded from the embedded tree (defaults.go) and never overwrite user edits.
//
// The frontmatter is parsed FLAT via the skills frontmatter helpers: the nested
// `prefilter:` block in the default files is cosmetic — its sub-keys
// (requiresAny, requiresAll, ...) are read as top-level keys, so no nested-YAML
// parser is needed (kept dependency-free and editable per _Docs/60 §2).
type Lens struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Channel     Channel   `json:"channel"`
	Enabled     bool      `json:"enabled"`
	Model       string    `json:"model,omitempty"` // analysis model hint (e.g. claude-cli)
	Scope       []string  `json:"scope,omitempty"` // surfaces to read: steps|debug|toolCalls|skills|context
	Prefilter   Prefilter `json:"prefilter"`
	Prompt      string    `json:"-"`    // markdown body = analysis instruction (LLM prompt)
	Path        string    `json:"path"` // source file, for editing
	// DefaultState says how this file compares to the lens TionHarness ships:
	// "" = not a shipped lens, "default" = untouched, "tuned" = only enabled/model
	// differ (still auto-refreshes), "edited" = the analysis body was changed, so
	// shipped improvements no longer reach it. Derived (not parsed from the file)
	// and filled by the registry loader; drives both the "restore default" button
	// and the badge that tells the user which lenses stopped updating.
	DefaultState seed.State `json:"defaultState,omitempty"`
}

// Prefilter is a declarative predicate (NO expression language / NO parser,
// _Docs/60 §2) that decides — with cheap structured signals only, before any LLM
// call — whether a session is worth analyzing for this lens. An empty prefilter
// matches everything. Signals are debug-event types and step kinds.
type Prefilter struct {
	RequiresAny []string       `json:"requiresAny,omitempty"` // OR: at least one present
	RequiresAll []string       `json:"requiresAll,omitempty"` // AND: all present
	Excludes    []string       `json:"excludes,omitempty"`    // NOT: skip if any present
	MinCount    map[string]int `json:"minCount,omitempty"`    // signal -> min occurrences
	MinTokens   int            `json:"minTokens,omitempty"`   // session-level token threshold
}

// ParseLens parses a lens definition from raw markdown (frontmatter + body).
// path is recorded for editing and used to derive a fallback id from the file
// name. A lens with no id or an invalid channel is an ERROR (not silently
// defaulted) so malformed lens files surface rather than scan with wrong config.
func ParseLens(raw, path string) (Lens, error) {
	id := strings.TrimSpace(skills.FrontmatterField(raw, "id"))
	if id == "" {
		base := filepath.Base(path)
		id = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if id == "" {
		return Lens{}, fmt.Errorf("lens %s: missing id", path)
	}

	channel := Channel(strings.TrimSpace(skills.FrontmatterField(raw, "channel")))
	if !channel.Valid() {
		return Lens{}, fmt.Errorf("lens %s: invalid channel %q (want %q|%q)",
			id, channel, ChannelAppFix, ChannelWorkspaceOpt)
	}

	l := Lens{
		ID:          id,
		Name:        strings.TrimSpace(skills.FrontmatterField(raw, "name")),
		Description: strings.TrimSpace(skills.FrontmatterField(raw, "description")),
		Channel:     channel,
		Model:       strings.TrimSpace(skills.FrontmatterField(raw, "model")),
		Scope:       skills.FrontmatterList(raw, "scope"),
		Prompt:      skills.FrontmatterBody(raw),
		Path:        path,
		Prefilter:   parsePrefilter(raw),
	}
	// enabled defaults to true; only an explicit "false" disables the lens.
	l.Enabled = !strings.EqualFold(strings.TrimSpace(skills.FrontmatterField(raw, "enabled")), "false")
	if l.Name == "" {
		l.Name = id
	}
	return l, nil
}

// Registry is the set of lenses loaded from a directory. Loading is resilient:
// a malformed lens file is collected as an error but does not prevent the rest
// from loading (one bad file must not blind the whole scanner).
type Registry struct {
	lenses map[string]Lens
}

// LoadRegistry reads every *.md under dir as a lens. It returns the registry plus
// any per-file parse errors (the caller logs them). A missing dir is an empty
// registry with no error.
func LoadRegistry(dir string) (*Registry, []error) {
	r := &Registry{lenses: map[string]Lens{}}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return r, []error{err}
	}
	var errs []error
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			errs = append(errs, readErr)
			continue
		}
		lens, parseErr := ParseLens(string(raw), path)
		if parseErr != nil {
			errs = append(errs, parseErr)
			continue
		}
		lens.DefaultState = DefaultState(dir, lens.ID)
		r.lenses[lens.ID] = lens
	}
	return r, errs
}

// Get returns a lens by id.
func (r *Registry) Get(id string) (Lens, bool) {
	l, ok := r.lenses[id]
	return l, ok
}

// List returns all lenses sorted by id.
func (r *Registry) List() []Lens {
	out := make([]Lens, 0, len(r.lenses))
	for _, l := range r.lenses {
		out = append(out, l)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Enabled returns only the enabled lenses, sorted by id.
func (r *Registry) Enabled() []Lens {
	out := make([]Lens, 0, len(r.lenses))
	for _, l := range r.List() {
		if l.Enabled {
			out = append(out, l)
		}
	}
	return out
}

// SetFrontmatterEnabled returns raw with its frontmatter `enabled:` set to the
// given value. Used by the lens enable/disable toggle so the UI can flip a lens
// without the user hand-editing the file.
func SetFrontmatterEnabled(raw []byte, enabled bool) []byte {
	return SetFrontmatterScalar(raw, "enabled", strconv.FormatBool(enabled))
}

// SetFrontmatterScalar returns raw with frontmatter key set to val — replacing an
// existing line for that key in place (so its position is preserved), or
// inserting one right after the opening `---`. Content with no frontmatter block
// is returned unchanged (nothing safe to edit).
//
// Only TOP-LEVEL keys are matched: an indented line belongs to a nested block
// (the `prefilter:` sub-keys), and rewriting one of those from here would corrupt
// the block it belongs to.
func SetFrontmatterScalar(raw []byte, key, val string) []byte {
	lines := strings.Split(string(raw), "\n")
	// Find the opening and closing frontmatter fences.
	open := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "---" {
			open = i
			break
		}
	}
	if open == -1 {
		return raw // no frontmatter → nothing to toggle
	}
	close := -1
	for i := open + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			close = i
			break
		}
	}
	if close == -1 {
		return raw
	}
	line := key + ": " + val
	prefix := key + ":"
	for i := open + 1; i < close; i++ {
		// Top-level only: an indented line is a nested block's sub-key.
		if lines[i] != strings.TrimLeft(lines[i], " \t") {
			continue
		}
		if strings.HasPrefix(lines[i], prefix) {
			lines[i] = line
			return []byte(strings.Join(lines, "\n"))
		}
	}
	// No existing line for this key → insert just after the opening fence.
	out := append([]string{}, lines[:open+1]...)
	out = append(out, line)
	out = append(out, lines[open+1:]...)
	return []byte(strings.Join(out, "\n"))
}

// atoiSafe parses a base-10 int, returning 0 on any error (missing/blank field).
func atoiSafe(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// parseMinCount parses the inline-map form the flat frontmatter parser leaves as
// a raw scalar — `{ signal: 3, other: 2 }` — into a signal→threshold map. Blank
// or malformed input yields nil (the prefilter then imposes no count threshold).
//
// The pair is split on its LAST colon, not its first: signal names may themselves
// contain one (`"cache_break:ttl-or-server-eviction"` — a cache break narrowed to
// its attributed cause), while the value is always the trailing integer. Splitting
// on the first colon silently produced the key `"cache_break` and a value that
// failed to parse, dropping the threshold — a lens would then match every session.
func parseMinCount(s string) map[string]int {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	out := map[string]int{}
	for _, pair := range strings.Split(s, ",") {
		colon := strings.LastIndex(pair, ":")
		if colon < 0 {
			continue
		}
		key := unquoteYAML(strings.TrimSpace(pair[:colon]))
		n, err := strconv.Atoi(strings.TrimSpace(pair[colon+1:]))
		if key == "" || err != nil {
			continue
		}
		out[key] = n
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// unquoteYAML strips one matching pair of surrounding quotes from a scalar.
func unquoteYAML(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parsePrefilter reads the prefilter keys. They live under a nested
// `prefilter:` mapping in every shipped lens; the frontmatter parser keeps
// nested mappings as raw blocks (skills.FrontmatterNested re-wraps one so its
// keys are readable), and a flat top-level spelling is still honoured for
// hand-written lenses that predate the nesting.
func parsePrefilter(raw string) Prefilter {
	src := skills.FrontmatterNested(raw, "prefilter")
	if src == "" {
		src = raw
	}
	return Prefilter{
		RequiresAny: skills.FrontmatterList(src, "requiresAny"),
		RequiresAll: skills.FrontmatterList(src, "requiresAll"),
		Excludes:    skills.FrontmatterList(src, "excludes"),
		MinCount:    parseMinCount(skills.FrontmatterField(src, "minCount")),
		MinTokens:   atoiSafe(skills.FrontmatterField(src, "minTokens")),
	}
}
