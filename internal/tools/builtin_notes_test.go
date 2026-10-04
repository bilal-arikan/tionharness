package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/notes"
)

// fakeNotesBridge is a NotesBridge with a fixed project and a rule-only
// supersede judgement (near-identical titles).
type fakeNotesBridge struct {
	store   *notes.Store
	project string
}

func (b fakeNotesBridge) Store() *notes.Store            { return b.store }
func (b fakeNotesBridge) Project(context.Context) string { return b.project }
func (b fakeNotesBridge) Supersedes(_ context.Context, c, e notes.Note) bool {
	return TitleSimilarity(c.Title, e.Title) >= 0.8
}

func noteTools(t *testing.T, project string) (map[string]Tool, *notes.Store) {
	t.Helper()
	store, err := notes.Open(filepath.Join(t.TempDir(), "notes"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Tool{}
	for _, tl := range NewNotesTools(fakeNotesBridge{store: store, project: project}, "AGT1") {
		out[tl.Def().Name] = tl
	}
	return out, store
}

func callNote(t *testing.T, tl Tool, args map[string]any) (string, error) {
	t.Helper()
	b, _ := json.Marshal(args)
	return tl.Call(WithCurrentSession(context.Background(), "SES1"), b)
}

func TestRememberStampsProvenanceAndReach(t *testing.T) {
	tools, store := noteTools(t, "/repo/a")
	out, err := callNote(t, tools["remember"], map[string]any{
		"kind": "lesson", "title": "Quote shell paths", "body": "Paths with spaces break unquoted. See [[Shell basics]].",
		"scope": "project", "tags": []string{"shell"},
	})
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	if !strings.Contains(out, "recorded NOTE1") || !strings.Contains(out, "reach: project /repo/a") {
		t.Fatalf("reply: %q", out)
	}
	if !strings.Contains(out, "unresolved wikilinks") || !strings.Contains(out, "Shell basics") {
		t.Fatalf("a forward link is warned about, not refused: %q", out)
	}
	n, _ := store.Get("NOTE1")
	if n.SourceSession != "SES1" || n.SourceAgent != "AGT1" || n.Source != notes.SourceAgent || n.Confidence != notes.ConfidenceInferred {
		t.Fatalf("provenance/defaults: %+v", n)
	}
	if len(n.Projects) != 1 || n.Projects[0] != "/repo/a" {
		t.Fatalf("project reach comes from the session, not the model: %+v", n.Projects)
	}
	// Agent scope reaches the writer.
	callNote(t, tools["remember"], map[string]any{"kind": "gotcha", "title": "My habit", "body": "b", "scope": "agent"})
	n2, _ := store.Get("NOTE2")
	if n2.Scope != notes.ScopeAgent || len(n2.Agents) != 1 || n2.Agents[0] != "AGT1" {
		t.Fatalf("agent reach: %+v", n2)
	}
	// kind=work is refused here.
	if _, err := callNote(t, tools["remember"], map[string]any{"kind": "work", "title": "x", "body": "y"}); err == nil {
		t.Fatal("remember must refuse kind=work")
	}
	// Validation errors name the field.
	if _, err := callNote(t, tools["remember"], map[string]any{"kind": "lesson", "title": "v", "body": "b", "confidence": "verified"}); err == nil || !strings.Contains(err.Error(), "verification") {
		t.Fatalf("verified without verification must be refused: %v", err)
	}
}

func TestRememberFilesACorrectionForANearIdenticalTitle(t *testing.T) {
	tools, store := noteTools(t, "")
	callNote(t, tools["remember"], map[string]any{"kind": "decision", "title": "Use Redis for the cache", "body": "Redis it is."})
	out, err := callNote(t, tools["remember"], map[string]any{"kind": "decision", "title": "Use Redis for the cache", "body": "Redis was dropped; in-process cache."})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "supersedes NOTE1") {
		t.Fatalf("expected a correction: %q", out)
	}
	old, _ := store.Get("NOTE1")
	if old.SupersededBy != "NOTE2" || old.Body != "Redis it is." {
		t.Fatalf("old note must be retired, not rewritten: %+v", old)
	}
	// A different subject stays a separate note.
	out, _ = callNote(t, tools["remember"], map[string]any{"kind": "decision", "title": "Keep JSONL storage", "body": "No database."})
	if strings.Contains(out, "supersedes") {
		t.Fatalf("unrelated note must not supersede: %q", out)
	}
	// Explicit supersedes wins over detection.
	out, err = callNote(t, tools["remember"], map[string]any{"kind": "decision", "title": "Storage stays file based", "body": "Still no database.", "supersedes": "NOTE3"})
	if err != nil || !strings.Contains(out, "supersedes NOTE3") {
		t.Fatalf("explicit supersedes: %q %v", out, err)
	}
}

func TestRecordWorkBuildsSectionsAndDefaultsScope(t *testing.T) {
	tools, store := noteTools(t, "/repo/a")
	out, err := callNote(t, tools["record_work"], map[string]any{
		"title": "Landed the notes store", "summary": "Added internal/notes.",
		"decisions": []string{"Frontmatter over JSON"}, "learned": []string{"BOM bites"}, "open": []string{"UI"},
		"verification": "go test ./internal/notes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "filed NOTE1 [work·verified]") || !strings.Contains(out, "reach: project /repo/a") || !strings.Contains(out, "1 open item") {
		t.Fatalf("reply: %q", out)
	}
	n, _ := store.Get("NOTE1")
	for _, want := range []string{"## Decisions", "- Frontmatter over JSON", "## Learned", "## Open", "- UI", "## Verification"} {
		if !strings.Contains(n.Body, want) {
			t.Fatalf("body lacks %q:\n%s", want, n.Body)
		}
	}
	// No working directory → workspace scope, inferred without verification.
	tools2, store2 := noteTools(t, "")
	callNote(t, tools2["record_work"], map[string]any{"title": "t", "summary": "s"})
	n2, _ := store2.Get("NOTE1")
	if n2.Scope != notes.ScopeWorkspace || n2.Confidence != notes.ConfidenceInferred {
		t.Fatalf("defaults: %+v", n2)
	}
}

func TestNoteSearchHonoursReachAndExplainsEmptyResults(t *testing.T) {
	tools, store := noteTools(t, "/repo/a")
	out, _ := callNote(t, tools["note_search"], map[string]any{"query": "anything"})
	if !strings.Contains(out, "memory is empty") {
		t.Fatalf("empty store message: %q", out)
	}
	store.Put(notes.Note{Kind: notes.KindLesson, Title: "Theirs only", Body: "windows quoting", Scope: notes.ScopeAgent, Agents: []string{"AGT9"}, Confidence: notes.ConfidenceInferred})
	store.Put(notes.Note{Kind: notes.KindLesson, Title: "Ours", Body: "windows quoting too", Scope: notes.ScopeWorkspace, Confidence: notes.ConfidenceInferred})
	out, _ = callNote(t, tools["note_search"], map[string]any{"query": "windows quoting"})
	if !strings.Contains(out, "Ours") || strings.Contains(out, "Theirs only") {
		t.Fatalf("reach filter: %q", out)
	}
	out, _ = callNote(t, tools["note_search"], map[string]any{"query": "windows quoting", "all": true})
	if !strings.Contains(out, "Theirs only") {
		t.Fatalf("all=true ignores reach: %q", out)
	}
	out, _ = callNote(t, tools["note_search"], map[string]any{"query": "nomatchword"})
	if !strings.Contains(out, "no notes match") {
		t.Fatalf("miss message: %q", out)
	}
}

func TestNoteExpandAndCorrect(t *testing.T) {
	tools, store := noteTools(t, "")
	a, _ := store.Put(notes.Note{Kind: notes.KindLesson, Title: "Alpha", Body: "First.", Scope: notes.ScopeWorkspace, Confidence: notes.ConfidenceInferred})
	store.Put(notes.Note{Kind: notes.KindLesson, Title: "Beta", Body: "Points at [[Alpha]].", Scope: notes.ScopeWorkspace, Confidence: notes.ConfidenceInferred})
	out, err := callNote(t, tools["note_expand"], map[string]any{"id": "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# NOTE1 · Alpha", "linked from:", "NOTE2 [lesson] Beta", "First."} {
		if !strings.Contains(out, want) {
			t.Fatalf("expand lacks %q:\n%s", want, out)
		}
	}
	out, err = callNote(t, tools["note_correct"], map[string]any{"id": a.ID, "body": "First, corrected.", "reason": "it was wrong", "confidence": "verified", "verification": "checked"})
	if err != nil || !strings.Contains(out, "filed NOTE3 as the correction of NOTE1") {
		t.Fatalf("correct: %q %v", out, err)
	}
	fixed, _ := store.Get("NOTE3")
	if fixed.Supersedes != a.ID || fixed.Source != notes.SourceCorrection || !strings.Contains(fixed.Body, "Correction of NOTE1: it was wrong") || fixed.Confidence != notes.ConfidenceVerified {
		t.Fatalf("correction: %+v", fixed)
	}
	out, _ = callNote(t, tools["note_expand"], map[string]any{"id": a.ID})
	if !strings.Contains(out, "SUPERSEDED by NOTE3") || !strings.Contains(out, "corrected by (newer)") {
		t.Fatalf("retired note must point at its correction:\n%s", out)
	}
	private, _ := store.Put(notes.Note{Kind: notes.KindReference, Title: "Mine", Body: "secret", Scope: notes.ScopeWorkspace, Confidence: notes.ConfidenceInferred, Private: true})
	if _, err := callNote(t, tools["note_expand"], map[string]any{"id": private.ID}); err == nil {
		t.Fatal("private notes are never served")
	}
}

func TestTitleSimilarity(t *testing.T) {
	if s := TitleSimilarity("Use Redis for the cache", "Use Redis for the cache"); s != 1 {
		t.Fatalf("identical = %v", s)
	}
	if s := TitleSimilarity("Use Redis for the cache", "Keep JSONL storage"); s != 0 {
		t.Fatalf("disjoint = %v", s)
	}
	if s := TitleSimilarity("Quote Windows paths", "Quote Windows paths with spaces"); s < 0.5 || s >= 0.8 {
		t.Fatalf("partial overlap = %v", s)
	}
}
