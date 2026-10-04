package notes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Store is the in-memory index over <dir>/*.md with write-through atomic
// persistence (write <id>.md.tmp → rename), the same discipline as the entity
// store. One Store per workspace; it is safe for concurrent use.
type Store struct {
	dir string

	mu      sync.RWMutex
	notes   map[string]*Note
	byTitle map[string]string // lower(title) → id (active notes win over retired)
	counter int
	now     func() time.Time
	// quarantined lists files Open could not parse (left on disk, skipped).
	quarantined []string
}

// ErrNotFound is returned when an id (or title) resolves to nothing.
var ErrNotFound = errors.New("note not found")

// idPrefix is the human-readable id prefix, matching the store's convention
// (TSK7, SES12 …).
const idPrefix = "NOTE"

// Open loads every note under dir (creating it when missing).
func Open(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("notes: empty dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("notes: mkdir: %w", err)
	}
	s := &Store{dir: dir, notes: map[string]*Note{}, byTitle: map[string]string{}, now: time.Now}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("notes: read dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("notes: read %s: %w", e.Name(), err)
		}
		n, err := Unmarshal(data)
		if err != nil {
			// A broken file must not take the whole store down: skip it and let
			// the user see it on disk. Logged by the caller through Quarantined.
			s.quarantined = append(s.quarantined, e.Name())
			continue
		}
		if n.ID == "" {
			n.ID = strings.TrimSuffix(e.Name(), ".md")
		}
		s.index(&n)
	}
	return s, nil
}

// Quarantined returns the names of files Open could not parse.
func (s *Store) Quarantined() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.quarantined...)
}

// Dir returns the directory the store persists to.
func (s *Store) Dir() string { return s.dir }

// index registers a note in the maps and advances the id counter.
func (s *Store) index(n *Note) {
	s.notes[n.ID] = n
	s.reindexTitle(n)
	if num, ok := strings.CutPrefix(n.ID, idPrefix); ok {
		if i, err := strconv.Atoi(num); err == nil && i > s.counter {
			s.counter = i
		}
	}
}

// reindexTitle maps the title to this note unless an ACTIVE note already owns
// the title (a retired note must not shadow its correction).
func (s *Store) reindexTitle(n *Note) {
	key := strings.ToLower(n.Title)
	if key == "" {
		return
	}
	if cur, ok := s.byTitle[key]; ok && cur != n.ID {
		if c := s.notes[cur]; c != nil && c.Active() && !n.Active() {
			return
		}
	}
	s.byTitle[key] = n.ID
}

func (s *Store) nextID() string {
	s.counter++
	return idPrefix + strconv.Itoa(s.counter)
}

func (s *Store) path(id string) string { return filepath.Join(s.dir, id+".md") }

// persist writes one note atomically. Caller holds the write lock.
func (s *Store) persist(n *Note) error {
	final := s.path(n.ID)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, Marshal(*n), 0o644); err != nil {
		return fmt.Errorf("notes: write %s: %w", n.ID, err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("notes: rename %s: %w", n.ID, err)
	}
	return nil
}

// Put creates a note (empty ID) or replaces an existing one by ID. On create,
// a non-empty Signature that matches an existing non-archived note of the same
// kind updates THAT note instead (Occurrences++, newest wording wins), which is
// how machine-written lessons dedupe. Created/Updated are stamped here; a
// caller's Created is kept on update.
func (s *Store) Put(n Note) (Note, error) {
	if err := n.Validate(); err != nil {
		return Note{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().Unix()
	if n.ID == "" {
		if n.Signature != "" {
			if existing := s.findBySignature(n.Kind, n.Signature); existing != nil {
				existing.Occurrences++
				existing.Updated = now
				existing.Title = n.Title
				existing.Body = n.Body
				existing.Links = n.Links
				existing.Confidence = n.Confidence
				existing.Verification = n.Verification
				if n.SourceSession != "" {
					existing.SourceSession = n.SourceSession
				}
				if n.SourceAgent != "" {
					existing.SourceAgent = n.SourceAgent
				}
				existing.Tags = mergeTags(existing.Tags, n.Tags)
				s.reindexTitle(existing)
				if err := s.persist(existing); err != nil {
					return Note{}, err
				}
				return *existing, nil
			}
		}
		n.ID = s.nextID()
		n.Created = now
		if n.Occurrences == 0 {
			n.Occurrences = 1
		}
	} else {
		existing, ok := s.notes[n.ID]
		if !ok {
			return Note{}, fmt.Errorf("%w: %s", ErrNotFound, n.ID)
		}
		if n.Created == 0 {
			n.Created = existing.Created
		}
		if n.Occurrences == 0 {
			n.Occurrences = existing.Occurrences
		}
		if existing.Title != n.Title {
			delete(s.byTitle, strings.ToLower(existing.Title))
		}
	}
	n.Updated = now
	cp := n
	s.notes[cp.ID] = &cp
	s.reindexTitle(&cp)
	if num, ok := strings.CutPrefix(cp.ID, idPrefix); ok {
		if i, err := strconv.Atoi(num); err == nil && i > s.counter {
			s.counter = i
		}
	}
	if err := s.persist(&cp); err != nil {
		return Note{}, err
	}
	return cp, nil
}

func (s *Store) findBySignature(kind Kind, sig string) *Note {
	var best *Note
	for _, n := range s.notes {
		if n.Signature == sig && n.Kind == kind && !n.Archived && !n.Retired() {
			if best == nil || n.Updated > best.Updated {
				best = n
			}
		}
	}
	return best
}

func mergeTags(a, b []string) []string {
	return dedupeStrings(append(append([]string(nil), a...), b...), true)
}

// Get returns a note by id.
func (s *Store) Get(id string) (Note, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.notes[strings.TrimSpace(id)]
	if !ok {
		return Note{}, false
	}
	return *n, true
}

// Resolve finds a note by id or by title (case-insensitive) — the two forms a
// wikilink target takes.
func (s *Store) Resolve(ref string) (Note, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Note{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n, ok := s.notes[ref]; ok {
		return *n, true
	}
	if id, ok := s.byTitle[strings.ToLower(ref)]; ok {
		if n, ok := s.notes[id]; ok {
			return *n, true
		}
	}
	return Note{}, false
}

// Filter narrows List. Zero values mean "no constraint". Archived, retired
// (superseded) and private notes are excluded unless their Include flag is set,
// because that is what every agent-facing reader wants; the UI sets the flags.
type Filter struct {
	Kinds           []Kind
	Scope           Scope
	Source          string
	Tag             string
	SourceSession   string
	SourceAgent     string
	SignaturePrefix string
	Since           int64 // Updated >= Since
	IncludeArchived bool
	IncludeRetired  bool
	IncludePrivate  bool
	// Reader restricts to notes whose declared reach covers this reader. Nil =
	// no reach filter (the UI sees everything).
	Reader *Reader
}

// Reader identifies who is asking, for the reach rule.
type Reader struct {
	AgentID string
	Project string
}

func (f Filter) match(n *Note) bool {
	if n.Archived && !f.IncludeArchived {
		return false
	}
	if n.Retired() && !f.IncludeRetired {
		return false
	}
	if n.Private && !f.IncludePrivate {
		return false
	}
	if len(f.Kinds) > 0 {
		ok := false
		for _, k := range f.Kinds {
			if k == n.Kind {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if f.Scope != "" && n.Scope != f.Scope {
		return false
	}
	if f.Source != "" && n.Source != f.Source {
		return false
	}
	if f.SourceSession != "" && n.SourceSession != f.SourceSession {
		return false
	}
	if f.SourceAgent != "" && n.SourceAgent != f.SourceAgent {
		return false
	}
	if f.SignaturePrefix != "" && !strings.HasPrefix(n.Signature, f.SignaturePrefix) {
		return false
	}
	if f.Since > 0 && n.Updated < f.Since {
		return false
	}
	if f.Tag != "" {
		ok := false
		for _, t := range n.Tags {
			if strings.EqualFold(t, f.Tag) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if f.Reader != nil && !n.Reaches(f.Reader.AgentID, f.Reader.Project) {
		return false
	}
	return true
}

// List returns matching notes newest-first.
func (s *Store) List(f Filter) []Note {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Note, 0, len(s.notes))
	for _, n := range s.notes {
		if f.match(n) {
			out = append(out, *n)
		}
	}
	sortByUpdated(out)
	return out
}

// Count returns how many notes match.
func (s *Store) Count(f Filter) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := 0
	for _, n := range s.notes {
		if f.match(n) {
			c++
		}
	}
	return c
}

// Hit is one search result.
type Hit struct {
	Note    Note    `json:"note"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

// Search is the lexical baseline every installation has: every query token
// must appear (AND) in the title, body or tags, case-insensitively; the score
// weights title hits, counts body hits and nudges recent notes up. A semantic
// layer may re-rank on top of it; nothing disappears without one.
func (s *Store) Search(query string, f Filter, limit int) []Hit {
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := s.now().Unix()
	var hits []Hit
	for _, n := range s.notes {
		if !f.match(n) {
			continue
		}
		title := strings.ToLower(n.Title)
		body := strings.ToLower(n.Body)
		tags := strings.ToLower(strings.Join(n.Tags, " "))
		score := 0.0
		ok := true
		for _, t := range tokens {
			inTitle := strings.Contains(title, t)
			inBody := strings.Count(body, t)
			inTags := strings.Contains(tags, t)
			if !inTitle && inBody == 0 && !inTags {
				ok = false
				break
			}
			if inTitle {
				score += 3
			}
			if inTags {
				score += 2
			}
			score += float64(min(inBody, 5))
		}
		if !ok {
			continue
		}
		// Recency nudge: a note touched this week beats an identical one from
		// months ago, but never beats one more term match.
		if age := now - n.Updated; age < 7*86400 {
			score += 0.5
		}
		if n.Confidence == ConfidenceVerified {
			score += 0.25
		}
		hits = append(hits, Hit{Note: *n, Score: score, Snippet: snippet(n.Body, tokens[0])})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Note.Updated != hits[j].Note.Updated {
			return hits[i].Note.Updated > hits[j].Note.Updated
		}
		return hits[i].Note.ID < hits[j].Note.ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func tokenize(q string) []string {
	var out []string
	for _, f := range strings.Fields(strings.ToLower(q)) {
		f = strings.Trim(f, `"'.,;:!?()[]{}`)
		if len(f) >= 2 {
			out = append(out, f)
		}
	}
	return out
}

// snippet returns the line of body that contains tok (or the first line),
// clipped.
func snippet(body, tok string) string {
	for _, l := range strings.Split(body, "\n") {
		if strings.Contains(strings.ToLower(l), tok) {
			return clipRunes(strings.Join(strings.Fields(l), " "), 160)
		}
	}
	return FirstSentence(body, 160)
}

// Correct files replacement as the correction of oldID: the replacement is
// created with Supersedes=oldID, the old note is marked SupersededBy and keeps
// its text (a dated record is preserved, never rewritten). A note that is
// already retired cannot be corrected again — correct its successor.
func (s *Store) Correct(oldID string, replacement Note) (Note, error) {
	replacement.ID = ""
	replacement.Supersedes = strings.TrimSpace(oldID)
	replacement.SupersededBy = ""
	replacement.Signature = "" // a correction never merges into the shape it corrects
	if err := replacement.Validate(); err != nil {
		return Note{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.notes[replacement.Supersedes]
	if !ok {
		return Note{}, fmt.Errorf("%w: %s", ErrNotFound, replacement.Supersedes)
	}
	if old.Retired() {
		return Note{}, fmt.Errorf("notes: %s is already superseded by %s; correct that one instead", old.ID, old.SupersededBy)
	}
	now := s.now().Unix()
	replacement.ID = s.nextID()
	replacement.Created = now
	replacement.Updated = now
	if replacement.Occurrences == 0 {
		replacement.Occurrences = 1
	}
	cp := replacement
	s.notes[cp.ID] = &cp
	s.reindexTitle(&cp)
	if err := s.persist(&cp); err != nil {
		delete(s.notes, cp.ID)
		return Note{}, err
	}
	old.SupersededBy = cp.ID
	old.Updated = now
	s.reindexTitle(old)
	if err := s.persist(old); err != nil {
		return Note{}, err
	}
	return cp, nil
}

// SetArchived puts a note away (or restores it). Archiving is the only way a
// note leaves the servable set without a correction.
func (s *Store) SetArchived(id string, archived bool) (Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notes[strings.TrimSpace(id)]
	if !ok {
		return Note{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if n.Archived == archived {
		return *n, nil
	}
	n.Archived = archived
	n.Updated = s.now().Unix()
	s.reindexTitle(n)
	if err := s.persist(n); err != nil {
		return Note{}, err
	}
	return *n, nil
}

// Delete removes a note's file for good. Reserved for the human (the Notes
// screen); no agent tool calls it. A note that others link to or that is part
// of a correction chain is refused so the chain stays walkable.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	n, ok := s.notes[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if n.SupersededBy != "" || n.Supersedes != "" {
		return fmt.Errorf("notes: %s is part of a correction chain; archive it instead", id)
	}
	for _, other := range s.notes {
		if other.ID == id {
			continue
		}
		for _, l := range other.Links {
			if l == id || strings.EqualFold(l, n.Title) {
				return fmt.Errorf("notes: %s is linked from %s; archive it instead", id, other.ID)
			}
		}
	}
	if err := os.Remove(s.path(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("notes: delete %s: %w", id, err)
	}
	delete(s.notes, id)
	if cur, ok := s.byTitle[strings.ToLower(n.Title)]; ok && cur == id {
		delete(s.byTitle, strings.ToLower(n.Title))
	}
	return nil
}

// Expansion is a note's neighbourhood: what it links to, what links to it, and
// its correction chain.
type Expansion struct {
	Note       Note     `json:"note"`
	Links      []Note   `json:"links"`
	Unresolved []string `json:"unresolved,omitempty"`
	Backlinks  []Note   `json:"backlinks"`
	// Predecessors walk Supersedes backwards (oldest last); Successors walk
	// SupersededBy forwards (newest last).
	Predecessors []Note `json:"predecessors,omitempty"`
	Successors   []Note `json:"successors,omitempty"`
}

// Expand resolves the neighbourhood of id (or title).
func (s *Store) Expand(ref string) (Expansion, error) {
	n, ok := s.Resolve(ref)
	if !ok {
		return Expansion{}, fmt.Errorf("%w: %s", ErrNotFound, ref)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Empty neighbourhoods serialise as [] rather than null: every consumer
	// (the Notes screen, the map, the tools) iterates these without a guard.
	ex := Expansion{Note: n, Links: []Note{}, Backlinks: []Note{}}
	for _, l := range n.Links {
		if t, ok := s.resolveLocked(l); ok {
			ex.Links = append(ex.Links, *t)
		} else {
			ex.Unresolved = append(ex.Unresolved, l)
		}
	}
	for _, other := range s.notes {
		if other.ID == n.ID {
			continue
		}
		for _, l := range other.Links {
			if l == n.ID || strings.EqualFold(l, n.Title) {
				ex.Backlinks = append(ex.Backlinks, *other)
				break
			}
		}
	}
	sortByUpdated(ex.Backlinks)
	seen := map[string]bool{n.ID: true}
	for cur := n.Supersedes; cur != "" && !seen[cur]; {
		seen[cur] = true
		p, ok := s.notes[cur]
		if !ok {
			break
		}
		ex.Predecessors = append(ex.Predecessors, *p)
		cur = p.Supersedes
	}
	for cur := n.SupersededBy; cur != "" && !seen[cur]; {
		seen[cur] = true
		p, ok := s.notes[cur]
		if !ok {
			break
		}
		ex.Successors = append(ex.Successors, *p)
		cur = p.SupersededBy
	}
	return ex, nil
}

func (s *Store) resolveLocked(ref string) (*Note, bool) {
	if n, ok := s.notes[ref]; ok {
		return n, true
	}
	if id, ok := s.byTitle[strings.ToLower(ref)]; ok {
		if n, ok := s.notes[id]; ok {
			return n, true
		}
	}
	return nil, false
}

// Current follows the correction chain to the newest note, so a reader who
// holds a retired id lands on the version that is in force.
func (s *Store) Current(id string) (Note, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.notes[strings.TrimSpace(id)]
	if !ok {
		return Note{}, false
	}
	seen := map[string]bool{}
	for n.SupersededBy != "" && !seen[n.ID] {
		seen[n.ID] = true
		next, ok := s.notes[n.SupersededBy]
		if !ok {
			break
		}
		n = next
	}
	return *n, true
}

// LinkWarnings lists the wikilinks in n that resolve to nothing. Used by the
// writing tools to warn (not refuse): a forward link to a note that does not
// exist yet is allowed, as in any wiki.
func (s *Store) LinkWarnings(n Note) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []string
	for _, l := range ParseWikilinks(n.Body) {
		if _, ok := s.resolveLocked(l); !ok {
			out = append(out, l)
		}
	}
	return out
}

// Stats is the roll-up the workspace projection and the Notes screen show.
type Stats struct {
	Total    int            `json:"total"`
	Active   int            `json:"active"`
	Retired  int            `json:"retired"`
	Archived int            `json:"archived"`
	Private  int            `json:"private"`
	ByKind   map[Kind]int   `json:"byKind"`
	ByScope  map[Scope]int  `json:"byScope"`
	Newest   int64          `json:"newest"`
	Sources  map[string]int `json:"sources"`
}

// Stats counts the store.
func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := Stats{ByKind: map[Kind]int{}, ByScope: map[Scope]int{}, Sources: map[string]int{}}
	for _, n := range s.notes {
		st.Total++
		switch {
		case n.Archived:
			st.Archived++
		case n.Retired():
			st.Retired++
		case n.Private:
			st.Private++
		default:
			st.Active++
			st.ByKind[n.Kind]++
			st.ByScope[n.Scope]++
		}
		if n.Source != "" {
			st.Sources[n.Source]++
		}
		if n.Updated > st.Newest {
			st.Newest = n.Updated
		}
	}
	return st
}

// SetClock overrides the clock (tests).
func (s *Store) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now != nil {
		s.now = now
	}
}
