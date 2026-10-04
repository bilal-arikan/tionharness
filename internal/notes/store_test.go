package notes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "notes"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	clock := time.Unix(1_700_000_000, 0)
	s.SetClock(func() time.Time { clock = clock.Add(time.Second); return clock })
	return s
}

func lesson(title, body string) Note {
	return Note{Kind: KindLesson, Title: title, Body: body, Scope: ScopeWorkspace, Confidence: ConfidenceInferred, Source: SourceAgent}
}

func TestPutAssignsIDAndRoundTripsThroughDisk(t *testing.T) {
	s := openTestStore(t)
	n, err := s.Put(Note{
		Kind: KindDecision, Title: "Use JSONL: not SQLite", Scope: ScopeProject, Projects: []string{`C:\repo\`},
		Confidence: ConfidenceVerified, Verification: "measured on 3 machines", Tags: []string{"Storage", "storage", " "},
		Body: "We keep JSONL.\n\nSee [[NOTE99]] and [[Older note|alias]].",
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if n.ID != "NOTE1" || n.Created == 0 || n.Updated == 0 || n.Occurrences != 1 {
		t.Fatalf("unexpected stamps: %+v", n)
	}
	if got := n.Projects; len(got) != 1 || got[0] != "C:/repo" {
		t.Fatalf("project not normalized: %v", got)
	}
	if got := n.Tags; len(got) != 1 || got[0] != "storage" {
		t.Fatalf("tags not deduped: %v", got)
	}
	if got := n.Links; len(got) != 2 || got[0] != "NOTE99" || got[1] != "Older note" {
		t.Fatalf("links: %v", got)
	}
	// Reopen from disk: identical.
	re, err := Open(s.Dir())
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	back, ok := re.Get("NOTE1")
	if !ok {
		t.Fatal("note missing after reopen")
	}
	if back.Title != n.Title || back.Body != n.Body || back.Verification != n.Verification || back.Created != n.Created || len(back.Links) != 2 {
		t.Fatalf("round trip changed the note:\n%+v\n%+v", n, back)
	}
	// The counter continues after reopen.
	n2, err := re.Put(lesson("second", "body"))
	if err != nil || n2.ID != "NOTE2" {
		t.Fatalf("counter after reopen: %v %v", n2.ID, err)
	}
}

func TestValidateRefusesTheContractBreaks(t *testing.T) {
	s := openTestStore(t)
	cases := []struct {
		name string
		n    Note
		want string
	}{
		{"kind", Note{Kind: "idea", Title: "t", Body: "b", Scope: ScopeWorkspace, Confidence: ConfidenceInferred}, "kind"},
		{"scope", Note{Kind: KindLesson, Title: "t", Body: "b", Scope: "global", Confidence: ConfidenceInferred}, "scope"},
		{"agent scope needs agents", Note{Kind: KindLesson, Title: "t", Body: "b", Scope: ScopeAgent, Confidence: ConfidenceInferred}, "agents"},
		{"project scope needs projects", Note{Kind: KindLesson, Title: "t", Body: "b", Scope: ScopeProject, Confidence: ConfidenceInferred}, "projects"},
		{"verified needs verification", Note{Kind: KindLesson, Title: "t", Body: "b", Scope: ScopeWorkspace, Confidence: ConfidenceVerified}, "verification"},
		{"empty body", Note{Kind: KindLesson, Title: "t", Body: "  ", Scope: ScopeWorkspace, Confidence: ConfidenceInferred}, "body"},
		{"huge body", Note{Kind: KindLesson, Title: "t", Body: strings.Repeat("x", MaxBodyBytes+1), Scope: ScopeWorkspace, Confidence: ConfidenceInferred}, "body"},
		{"bracket title", Note{Kind: KindLesson, Title: "t [x]", Body: "b", Scope: ScopeWorkspace, Confidence: ConfidenceInferred}, "title"},
	}
	for _, c := range cases {
		_, err := s.Put(c.n)
		if err == nil {
			t.Fatalf("%s: expected an error", c.name)
		}
		ve, ok := err.(*ValidationError)
		if !ok || ve.Field != c.want {
			t.Fatalf("%s: want field %q, got %v", c.name, c.want, err)
		}
	}
	if s.Count(Filter{}) != 0 {
		t.Fatal("a refused note must not be stored")
	}
}

func TestSignatureDedupeBumpsOccurrencesAndKeepsNewestWording(t *testing.T) {
	s := openTestStore(t)
	a := lesson("Quote paths", "Paths with spaces fail.")
	a.Signature = "Bash:abcd"
	a.Source = SourceLessonExtractor
	first, _ := s.Put(a)
	b := a
	b.Title = "Quote paths with spaces"
	b.Body = "Always quote paths."
	second, err := s.Put(b)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if second.ID != first.ID || second.Occurrences != 2 || second.Body != "Always quote paths." || second.Title != b.Title {
		t.Fatalf("dedupe: %+v", second)
	}
	if s.Count(Filter{}) != 1 {
		t.Fatal("dedupe must not add a row")
	}
	// A different kind with the same signature is a different note.
	c := a
	c.Kind = KindGotcha
	third, _ := s.Put(c)
	if third.ID == first.ID {
		t.Fatal("signature dedupe must be per kind")
	}
}

func TestCorrectSupersedesAndServesOnlyTheCorrection(t *testing.T) {
	s := openTestStore(t)
	old, _ := s.Put(lesson("Redis is fine", "Use Redis for the cache."))
	fixed, err := s.Correct(old.ID, lesson("Redis is fine", "Redis was dropped; use the in-process cache."))
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if fixed.Supersedes != old.ID {
		t.Fatalf("supersedes: %+v", fixed)
	}
	back, _ := s.Get(old.ID)
	if back.SupersededBy != fixed.ID || back.Body != "Use Redis for the cache." {
		t.Fatalf("old note must be retired, not rewritten: %+v", back)
	}
	active := s.List(Filter{})
	if len(active) != 1 || active[0].ID != fixed.ID {
		t.Fatalf("default listing must serve only the correction: %+v", active)
	}
	all := s.List(Filter{IncludeRetired: true})
	if len(all) != 2 {
		t.Fatalf("retired listing: %d", len(all))
	}
	// The title now resolves to the correction, not the retired note.
	if r, ok := s.Resolve("redis is fine"); !ok || r.ID != fixed.ID {
		t.Fatalf("title must resolve to the active note: %+v %v", r, ok)
	}
	// Current follows the chain.
	if cur, ok := s.Current(old.ID); !ok || cur.ID != fixed.ID {
		t.Fatalf("current: %+v", cur)
	}
	// Correcting a retired note is refused.
	if _, err := s.Correct(old.ID, lesson("x", "y")); err == nil {
		t.Fatal("correcting a retired note must fail")
	}
	// The chain is walkable from both ends.
	ex, err := s.Expand(fixed.ID)
	if err != nil || len(ex.Predecessors) != 1 || ex.Predecessors[0].ID != old.ID {
		t.Fatalf("expand predecessors: %+v %v", ex, err)
	}
	ex, _ = s.Expand(old.ID)
	if len(ex.Successors) != 1 || ex.Successors[0].ID != fixed.ID {
		t.Fatalf("expand successors: %+v", ex)
	}
	// Chain members cannot be deleted.
	if err := s.Delete(old.ID); err == nil {
		t.Fatal("delete of a chain member must be refused")
	}
}

func TestReachIsDeclaredAtWriteTime(t *testing.T) {
	ws := Note{Scope: ScopeWorkspace}
	ag := Note{Scope: ScopeAgent, Agents: []string{"AGT1"}}
	pr := Note{Scope: ScopeProject, Projects: []string{"/home/me/repo"}}
	if !ws.Reaches("AGT9", "") || !ag.Reaches("AGT1", "") || ag.Reaches("AGT2", "") {
		t.Fatal("workspace/agent reach")
	}
	if !pr.Reaches("", "/home/me/repo") || !pr.Reaches("", "/home/me/repo/sub/dir") || pr.Reaches("", "/home/me/repo2") || pr.Reaches("", "") {
		t.Fatal("project reach")
	}
	if !pr.Reaches("", `\home\me\repo\`) {
		t.Fatal("project reach must normalize separators and trailing slash")
	}
	s := openTestStore(t)
	s.Put(Note{Kind: KindLesson, Title: "mine", Body: "b", Scope: ScopeAgent, Agents: []string{"AGT1"}, Confidence: ConfidenceInferred})
	s.Put(Note{Kind: KindLesson, Title: "theirs", Body: "b", Scope: ScopeAgent, Agents: []string{"AGT2"}, Confidence: ConfidenceInferred})
	s.Put(Note{Kind: KindLesson, Title: "all", Body: "b", Scope: ScopeWorkspace, Confidence: ConfidenceInferred})
	got := s.List(Filter{Reader: &Reader{AgentID: "AGT1"}})
	if len(got) != 2 {
		t.Fatalf("reader filter: %+v", got)
	}
	for _, n := range got {
		if n.Title == "theirs" {
			t.Fatal("another agent's note must not reach AGT1")
		}
	}
}

func TestSearchIsANDMatchedAndRanksTitleHits(t *testing.T) {
	s := openTestStore(t)
	s.Put(lesson("Windows path quoting", "Quote every path on Windows shells."))
	s.Put(lesson("Unrelated", "Nothing about quoting here. Windows is mentioned."))
	s.Put(lesson("Also quoting", "Linux path quoting is forgiving."))
	hits := s.Search("windows quoting", Filter{}, 10)
	if len(hits) != 2 {
		t.Fatalf("AND match: %d hits", len(hits))
	}
	if hits[0].Note.Title != "Windows path quoting" {
		t.Fatalf("title hit must rank first: %+v", hits[0].Note.Title)
	}
	if s.Search("", Filter{}, 10) != nil {
		t.Fatal("empty query returns nothing")
	}
	private := lesson("secret", "windows quoting secret")
	private.Private = true
	s.Put(private)
	if len(s.Search("secret", Filter{}, 10)) != 0 {
		t.Fatal("private notes are never served by default")
	}
	if len(s.Search("secret", Filter{IncludePrivate: true}, 10)) != 1 {
		t.Fatal("the UI may include private notes")
	}
}

func TestExpandResolvesLinksByIDAndTitleAndFindsBacklinks(t *testing.T) {
	s := openTestStore(t)
	a, _ := s.Put(lesson("Alpha", "First."))
	b, _ := s.Put(lesson("Beta", "Points at [[Alpha]] and [[NOTE1]] and [[Missing one]]."))
	ex, err := s.Expand(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Links) != 2 || ex.Links[0].ID != a.ID || ex.Links[1].ID != a.ID {
		t.Fatalf("links: %+v", ex.Links)
	}
	if len(ex.Unresolved) != 1 || ex.Unresolved[0] != "Missing one" {
		t.Fatalf("unresolved: %v", ex.Unresolved)
	}
	ex, _ = s.Expand("alpha")
	if len(ex.Backlinks) != 1 || ex.Backlinks[0].ID != b.ID {
		t.Fatalf("backlinks: %+v", ex.Backlinks)
	}
	if w := s.LinkWarnings(b); len(w) != 1 {
		t.Fatalf("link warnings: %v", w)
	}
	// A linked note cannot be deleted; an unlinked one can.
	if err := s.Delete(a.ID); err == nil {
		t.Fatal("linked note delete must be refused")
	}
	if err := s.Delete(b.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), b.ID+".md")); !os.IsNotExist(err) {
		t.Fatal("file must be gone")
	}
}

func TestArchiveLeavesTheServableSetAndIsReversible(t *testing.T) {
	s := openTestStore(t)
	n, _ := s.Put(lesson("Old", "b"))
	if _, err := s.SetArchived(n.ID, true); err != nil {
		t.Fatal(err)
	}
	if s.Count(Filter{}) != 0 || s.Count(Filter{IncludeArchived: true}) != 1 {
		t.Fatal("archived note must leave the default listing")
	}
	s.SetArchived(n.ID, false)
	if s.Count(Filter{}) != 1 {
		t.Fatal("restore")
	}
	st := s.Stats()
	if st.Total != 1 || st.Active != 1 || st.ByKind[KindLesson] != 1 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestOpenQuarantinesABrokenFileAndKeepsTheRest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "junk.md"), []byte("no frontmatter here"), 0o644)
	os.WriteFile(filepath.Join(dir, "NOTE7.md"), Marshal(Note{ID: "NOTE7", Kind: KindLesson, Title: "ok", Body: "b", Scope: ScopeWorkspace, Confidence: ConfidenceInferred}), 0o644)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Quarantined()) != 1 || s.Count(Filter{}) != 1 {
		t.Fatalf("quarantine: %v / %d", s.Quarantined(), s.Count(Filter{}))
	}
	n, _ := s.Put(lesson("next", "b"))
	if n.ID != "NOTE8" {
		t.Fatalf("counter must continue past the loaded id: %s", n.ID)
	}
}

func TestFrontmatterHandlesHostileScalars(t *testing.T) {
	n := Note{ID: "NOTE1", Kind: KindLesson, Scope: ScopeWorkspace, Confidence: ConfidenceInferred,
		Title: `Colons: and "quotes" and #hashes`, Body: "---\nnot a fence inside body\n", Tags: []string{"a,b", "c"},
		Verification: "42", Source: "agent"}
	n.Normalize()
	back, err := Unmarshal(Marshal(n))
	if err != nil {
		t.Fatal(err)
	}
	if back.Title != n.Title || back.Verification != "42" || len(back.Tags) != 2 || back.Tags[0] != "a,b" {
		t.Fatalf("hostile scalars: %+v", back)
	}
	if back.Body != "---\nnot a fence inside body" {
		t.Fatalf("body: %q", back.Body)
	}
	// Hand-edited plain forms still load.
	hand := "---\nid: NOTE3\nkind: lesson\ntitle: plain title here\nscope: workspace\nconfidence: inferred\ntags: a, b\narchived: yes\n---\nbody\n"
	h, err := Unmarshal([]byte(hand))
	if err != nil || h.Title != "plain title here" || len(h.Tags) != 2 || !h.Archived || h.Body != "body" {
		t.Fatalf("hand-edited: %+v %v", h, err)
	}
}

func TestLineAndFirstSentence(t *testing.T) {
	n := Note{ID: "NOTE4", Kind: KindGotcha, Confidence: ConfidenceVerified, Title: "Mind the cap", Body: "# Heading\n\nThe hook output is capped at 10k. More text follows.", Occurrences: 3, Updated: 1000}
	line := n.Line(time.Unix(1000+7200, 0))
	for _, want := range []string{"[gotcha·verified]", "Mind the cap", "The hook output is capped at 10k.", "NOTE4", "seen 3×", "2h"} {
		if !strings.Contains(line, want) {
			t.Fatalf("line %q lacks %q", line, want)
		}
	}
	if got := FirstSentence("v1.2 is out. Next.", 100); got != "v1.2 is out." {
		t.Fatalf("first sentence: %q", got)
	}
	if got := Age(3 * 86400); got != "3d" {
		t.Fatalf("age: %q", got)
	}
}
