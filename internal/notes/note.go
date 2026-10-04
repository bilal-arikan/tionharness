// Package notes is the workspace's durable, human-readable memory: Markdown
// notes with a frontmatter block, stored under <store>/notes/<id>.md.
//
// The design follows the vault-first memory contract (_Docs/94): a note
// declares its REACH when it is written (scope + agents/projects) and a reader
// never widens it; every note carries a confidence tag; a correction never
// overwrites — it supersedes, and a superseded note is served only together with
// its correction. Notes are never deleted by agents; they are archived.
//
// The package is a leaf: it imports nothing from the application besides the
// shared archive vocabulary, so the store (db), the view layer, the tools and
// the awareness layer can all depend on it.
package notes

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Kind classifies what a note records.
type Kind string

const (
	// KindLesson is a reusable rule distilled from a failure or a hard-won fact.
	KindLesson Kind = "lesson"
	// KindDecision records a choice, its reasoning and the alternatives rejected.
	KindDecision Kind = "decision"
	// KindWork files what happened in a session: changes, verification, open items.
	KindWork Kind = "work"
	// KindGotcha is a trap that cost time and how to avoid it.
	KindGotcha Kind = "gotcha"
	// KindPattern is a recurring observation across sessions.
	KindPattern Kind = "pattern"
	// KindProfile describes a person, team, project or system the agents work with.
	KindProfile Kind = "profile"
	// KindReference points at an external resource with the context it matters in.
	KindReference Kind = "reference"
)

// Kinds lists every valid kind in display order.
var Kinds = []Kind{KindLesson, KindDecision, KindWork, KindGotcha, KindPattern, KindProfile, KindReference}

// Scope is the reach a note declares at write time.
type Scope string

const (
	// ScopeAgent: only the agents listed in Agents see it.
	ScopeAgent Scope = "agent"
	// ScopeProject: only sessions whose working directory is under one of Projects.
	ScopeProject Scope = "project"
	// ScopeWorkspace: every session in this workspace. The widest reach there is;
	// workspaces are isolated, so there is deliberately no wider scope.
	ScopeWorkspace Scope = "workspace"
)

// Scopes lists every valid scope.
var Scopes = []Scope{ScopeAgent, ScopeProject, ScopeWorkspace}

// Confidence is the epistemic tag every note carries.
type Confidence string

const (
	// ConfidenceVerified: the writer checked it and says how (Verification).
	ConfidenceVerified Confidence = "verified"
	// ConfidenceInferred: a reasonable conclusion that was not tested.
	ConfidenceInferred Confidence = "inferred"
	// ConfidenceUnverified: a hunch or hearsay, recorded so it is not lost.
	ConfidenceUnverified Confidence = "unverified"
)

// Confidences lists every valid confidence tag.
var Confidences = []Confidence{ConfidenceVerified, ConfidenceInferred, ConfidenceUnverified}

// Sources name who wrote a note. Free-form, but these are the ones the
// application itself uses.
const (
	SourceAgent           = "agent"            // the remember tool
	SourceWork            = "work"             // the record_work tool
	SourceUser            = "user"             // the Notes screen
	SourceLessonExtractor = "lesson-extractor" // the failure→lesson reflection
	SourceInsight         = "insight"          // promoted retrospective findings
	SourceCorrection      = "correction"       // note_correct (the replacement)
)

// Limits the validator enforces. A note past MaxBodyBytes is refused, not
// trimmed: the writer must split it, because a silently cut note misleads
// every later reader.
const (
	MaxTitleRunes = 200
	MaxBodyBytes  = 20000
	MaxTags       = 20
)

// Note is one memory. Links is derived from Body on load and never stored.
type Note struct {
	ID         string     `json:"id"`
	Kind       Kind       `json:"kind"`
	Title      string     `json:"title"`
	Scope      Scope      `json:"scope"`
	Agents     []string   `json:"agents,omitempty"`
	Projects   []string   `json:"projects,omitempty"`
	Confidence Confidence `json:"confidence"`
	// Verification says HOW a verified note was checked. Required for verified.
	Verification string `json:"verification,omitempty"`
	// Supersedes / SupersededBy form the correction chain. A note with
	// SupersededBy set is retired: served only next to its replacement.
	Supersedes   string `json:"supersedes,omitempty"`
	SupersededBy string `json:"supersededBy,omitempty"`
	Created      int64  `json:"created"`
	Updated      int64  `json:"updated"`
	// SourceSession / SourceAgent are provenance stamped by the runtime, never
	// claimed by the writer.
	SourceSession string `json:"sourceSession,omitempty"`
	SourceAgent   string `json:"sourceAgent,omitempty"`
	Source        string `json:"source,omitempty"`
	// Signature deduplicates machine-written notes (a lesson's failure shape): a
	// repeat bumps Occurrences instead of adding a near-duplicate.
	Signature   string   `json:"signature,omitempty"`
	Occurrences int      `json:"occurrences,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	// Private notes are the user's own: never served to an agent.
	Private  bool   `json:"private,omitempty"`
	Archived bool   `json:"archived,omitempty"`
	Body     string `json:"body"`
	// Links are the [[wikilink]] targets found in Body (raw text, unresolved).
	Links []string `json:"links,omitempty"`
}

// Retired reports whether the note has been superseded by a correction.
func (n Note) Retired() bool { return strings.TrimSpace(n.SupersededBy) != "" }

// Active reports whether the note is a current, servable memory: not archived,
// not retired and not private.
func (n Note) Active() bool { return !n.Archived && !n.Retired() && !n.Private }

// Reaches reports whether the note's declared scope covers a reader identified
// by its agent id and its project (working directory). This is a relevance
// rule, not access control: it answers "does this lesson bear on the reader?".
// A workspace note reaches everyone; an agent note reaches the listed agents;
// a project note reaches readers whose project is one of (or under one of) the
// listed roots.
func (n Note) Reaches(agentID, project string) bool {
	switch n.Scope {
	case ScopeWorkspace:
		return true
	case ScopeAgent:
		for _, a := range n.Agents {
			if a != "" && a == agentID {
				return true
			}
		}
		return false
	case ScopeProject:
		p := normalizeProject(project)
		if p == "" {
			return false
		}
		for _, root := range n.Projects {
			r := normalizeProject(root)
			if r == "" {
				continue
			}
			if p == r || strings.HasPrefix(p, r+"/") {
				return true
			}
		}
		return false
	}
	return false
}

// normalizeProject makes two spellings of one directory compare equal: forward
// slashes, no trailing slash, case kept (paths are case-sensitive on most of the
// platforms the app runs on; a Windows user gets the same spelling back from the
// session record every time).
func normalizeProject(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, `\`, "/"))
	for len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// wikilinkRe matches [[target]] and [[target|alias]]; the target may be an id
// or a title. A trailing "#section" is dropped from the target.
var wikilinkRe = regexp.MustCompile(`\[\[([^\[\]|#]+)(?:#[^\[\]|]*)?(?:\|[^\[\]]*)?\]\]`)

// ParseWikilinks returns the distinct link targets in body, in order of first
// appearance, trimmed.
func ParseWikilinks(body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range wikilinkRe.FindAllStringSubmatch(body, -1) {
		t := strings.TrimSpace(m[1])
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return out
}

// ValidationError is returned by Validate; Field names the offending field so a
// tool can tell the model exactly what to fix.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Msg: fmt.Sprintf(format, args...)}
}

// Normalize trims and lowercases the enumerated fields, dedupes lists and
// derives Links. It does not validate; Validate does.
func (n *Note) Normalize() {
	n.ID = strings.TrimSpace(n.ID)
	n.Kind = Kind(strings.ToLower(strings.TrimSpace(string(n.Kind))))
	n.Scope = Scope(strings.ToLower(strings.TrimSpace(string(n.Scope))))
	n.Confidence = Confidence(strings.ToLower(strings.TrimSpace(string(n.Confidence))))
	n.Title = strings.Join(strings.Fields(n.Title), " ")
	n.Verification = strings.TrimSpace(n.Verification)
	n.Supersedes = strings.TrimSpace(n.Supersedes)
	n.SupersededBy = strings.TrimSpace(n.SupersededBy)
	n.SourceSession = strings.TrimSpace(n.SourceSession)
	n.SourceAgent = strings.TrimSpace(n.SourceAgent)
	n.Source = strings.TrimSpace(n.Source)
	n.Signature = strings.TrimSpace(n.Signature)
	n.Agents = dedupeStrings(n.Agents, false)
	n.Projects = dedupeStrings(n.Projects, false)
	for i, p := range n.Projects {
		n.Projects[i] = normalizeProject(p)
	}
	n.Tags = dedupeStrings(n.Tags, true)
	n.Body = strings.TrimRight(strings.ReplaceAll(n.Body, "\r\n", "\n"), " \t\n")
	n.Links = ParseWikilinks(n.Body)
}

// Validate checks the contract: enumerations, required fields, the scope's
// required list, verification for a verified note, and the size limits. It
// normalizes first so a caller may pass a raw note.
func (n *Note) Validate() error {
	n.Normalize()
	if !kindValid(n.Kind) {
		return invalid("kind", "must be one of %s", joinKinds())
	}
	if !scopeValid(n.Scope) {
		return invalid("scope", "must be one of agent, project, workspace")
	}
	if !confidenceValid(n.Confidence) {
		return invalid("confidence", "must be one of verified, inferred, unverified")
	}
	if n.Title == "" {
		return invalid("title", "required")
	}
	if utf8.RuneCountInString(n.Title) > MaxTitleRunes {
		return invalid("title", "longer than %d characters", MaxTitleRunes)
	}
	if strings.TrimSpace(n.Body) == "" {
		return invalid("body", "required")
	}
	if len(n.Body) > MaxBodyBytes {
		return invalid("body", "%d bytes is over the %d-byte limit; split the note instead of trimming it", len(n.Body), MaxBodyBytes)
	}
	if n.Scope == ScopeAgent && len(n.Agents) == 0 {
		return invalid("agents", "scope \"agent\" needs at least one agent id")
	}
	if n.Scope == ScopeProject && len(n.Projects) == 0 {
		return invalid("projects", "scope \"project\" needs at least one project root (an absolute working directory)")
	}
	if n.Confidence == ConfidenceVerified && n.Verification == "" {
		return invalid("verification", "a \"verified\" note must say how it was verified")
	}
	if len(n.Tags) > MaxTags {
		return invalid("tags", "more than %d tags", MaxTags)
	}
	if strings.ContainsAny(n.Title, "\n[]") {
		return invalid("title", "may not contain newlines or square brackets (it is a wikilink target)")
	}
	return nil
}

func kindValid(k Kind) bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

func scopeValid(s Scope) bool {
	for _, x := range Scopes {
		if x == s {
			return true
		}
	}
	return false
}

func confidenceValid(c Confidence) bool {
	for _, x := range Confidences {
		if x == c {
			return true
		}
	}
	return false
}

func joinKinds() string {
	parts := make([]string, 0, len(Kinds))
	for _, k := range Kinds {
		parts = append(parts, string(k))
	}
	return strings.Join(parts, ", ")
}

func dedupeStrings(in []string, lower bool) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if lower {
			s = strings.ToLower(s)
		}
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Line renders the one-line, agent-facing form of a note: kind, confidence,
// title, the first sentence of the body and the id as the handle. now is used
// for the age stamp.
func (n Note) Line(now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- [%s·%s] %s", n.Kind, n.Confidence, n.Title)
	if first := FirstSentence(n.Body, 160); first != "" && !strings.EqualFold(first, n.Title) {
		b.WriteString(" — " + first)
	}
	fmt.Fprintf(&b, " (%s", n.ID)
	if n.Occurrences > 1 {
		fmt.Fprintf(&b, ", seen %d×", n.Occurrences)
	}
	if n.Updated > 0 && !now.IsZero() {
		b.WriteString(", " + Age(now.Unix()-n.Updated))
	}
	b.WriteString(")")
	return b.String()
}

// FirstSentence returns the first sentence of text clipped to max runes, with
// Markdown list markers and heading hashes stripped so the line reads plainly.
func FirstSentence(text string, max int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	// First non-empty line.
	// First non-empty, non-heading line: a heading names a section, it is
	// not the note's first statement.
	line := ""
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		l = strings.TrimLeft(l, ">-* \t")
		if l != "" {
			line = l
			break
		}
	}
	line = strings.Join(strings.Fields(line), " ")
	// Cut at the first sentence terminator followed by a space (so "v1.2" or a
	// path stays whole).
	for i := 0; i+1 < len(line); i++ {
		if (line[i] == '.' || line[i] == '!' || line[i] == '?') && line[i+1] == ' ' {
			line = line[:i+1]
			break
		}
	}
	return clipRunes(line, max)
}

func clipRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

// Age renders an age in seconds the shortest unambiguous way: "now", "5m",
// "3h", "2d", "6w".
func Age(sec int64) string {
	switch {
	case sec < 60:
		return "now"
	case sec < 3600:
		return fmt.Sprintf("%dm", sec/60)
	case sec < 86400:
		return fmt.Sprintf("%dh", sec/3600)
	case sec < 7*86400:
		return fmt.Sprintf("%dd", sec/86400)
	default:
		return fmt.Sprintf("%dw", sec/(7*86400))
	}
}

// sortByUpdated orders newest first, ties by id for determinism.
func sortByUpdated(ns []Note) {
	sort.SliceStable(ns, func(i, j int) bool {
		if ns[i].Updated != ns[j].Updated {
			return ns[i].Updated > ns[j].Updated
		}
		return ns[i].ID < ns[j].ID
	})
}
