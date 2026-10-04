package awareness

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/notes"
	"github.com/bilal-arikan/tionharness/internal/progress"
)

// fakeStore is an in-memory Store.
type fakeStore struct {
	root      string
	sessions  []db.Session
	messages  map[string][]db.Message
	asks      []db.SessionAsk
	tasks     []db.Task
	agents    []db.Agent
	runs      []db.FlowRun
	artifacts map[string][]db.Artifact
}

func newFakeStore(t *testing.T) *fakeStore {
	return &fakeStore{root: t.TempDir(), messages: map[string][]db.Message{}, artifacts: map[string][]db.Artifact{}}
}

func (f *fakeStore) GetSession(_ context.Context, id string) (db.Session, error) {
	for _, s := range f.sessions {
		if s.ID == id {
			return s, nil
		}
	}
	return db.Session{}, db.ErrNotFound
}
func (f *fakeStore) ListSessions(context.Context, string) ([]db.Session, error) {
	return f.sessions, nil
}
func (f *fakeStore) ListMessages(_ context.Context, id string) ([]db.Message, error) {
	return f.messages[id], nil
}
func (f *fakeStore) ListMessagesTail(_ context.Context, id string, n int) ([]db.Message, int, error) {
	m := f.messages[id]
	if len(m) > n {
		return m[len(m)-n:], len(m) - n, nil
	}
	return m, 0, nil
}
func (f *fakeStore) ListWaitingSessionAsks(context.Context) ([]db.SessionAsk, error) {
	return f.asks, nil
}
func (f *fakeStore) ListTasks(context.Context) ([]db.Task, error)   { return f.tasks, nil }
func (f *fakeStore) ListAgents(context.Context) ([]db.Agent, error) { return f.agents, nil }
func (f *fakeStore) ListFlowRuns(context.Context, string, int) ([]db.FlowRun, error) {
	return f.runs, nil
}
func (f *fakeStore) ListSchedules(context.Context) ([]db.Schedule, error) { return nil, nil }
func (f *fakeStore) ListArtifacts(_ context.Context, id string) ([]db.Artifact, error) {
	return f.artifacts[id], nil
}
func (f *fakeStore) SessionDir(id string) (string, error) {
	d := filepath.Join(f.root, "sessions", id)
	return d, os.MkdirAll(d, 0o755)
}

func newService(t *testing.T, store *fakeStore) *Service {
	svc := New(store, store.root, DefaultSettings, slog.New(slog.NewTextHandler(io.Discard, nil)))
	clock := time.Unix(1_800_000_000, 0)
	svc.SetClock(func() time.Time { clock = clock.Add(time.Second); return clock })
	return svc
}

func toolStep(tool string, isErr bool) string {
	b, _ := json.Marshal([]map[string]any{{"kind": "tool", "tool": tool, "isError": isErr, "output": "boom at /x"}})
	return string(b)
}

func todoStep(items ...[2]string) string {
	var todos []map[string]string
	for _, it := range items {
		todos = append(todos, map[string]string{"content": it[0], "status": it[1]})
	}
	b, _ := json.Marshal([]map[string]any{{"kind": "todo", "todos": todos}})
	return string(b)
}

func baseInput(store *fakeStore, sessionID string) Input {
	return Input{
		Session: db.Session{ID: sessionID, AgentID: "AGT1", Kind: "chat", Title: "Current", CreatedAt: 1_799_990_000, UpdatedAt: 1_799_999_000},
		Agent:   db.Agent{ID: "AGT1", Name: "Ada"},
		Store:   store, WorkspaceName: "WS", Fresh: true,
	}
}

func TestBriefIsFrozenUntilInvalidatedAndNamesTheWorkspace(t *testing.T) {
	store := newFakeStore(t)
	store.agents = []db.Agent{{ID: "AGT1", Name: "Ada"}}
	store.sessions = []db.Session{
		{ID: "SES1", AgentID: "AGT1", Kind: "chat", Title: "Current", UpdatedAt: 1_799_999_000},
		{ID: "SES2", AgentID: "AGT1", Kind: "chat", Title: "Older work", MessageCount: 7, UpdatedAt: 1_799_990_000, Summary: "Did the thing."},
		{ID: "SES3", AgentID: "AGT1", Kind: "chat", Title: "Stuck one", StuckTurns: 2, UpdatedAt: 1_799_990_000},
	}
	store.asks = []db.SessionAsk{{ID: "ASK1", SessionID: "SES2"}}
	svc := newService(t, store)
	in := baseInput(store, "SES1")

	b1 := svc.Brief(context.Background(), in)
	for _, want := range []string{"# Session briefing", "## Workspace now", `WORKSPACE "WS"`, "## Open loops", "SES2", "stuck session", "## Other sessions", "Older work", "Did the thing.", "[context meter · brief"} {
		if !strings.Contains(b1.Text, want) {
			t.Fatalf("brief lacks %q:\n%s", want, b1.Text)
		}
	}
	if strings.Contains(b1.Text, "Current") && strings.Count(b1.Text, "SES1") > 0 {
		t.Fatal("the current session must not list itself")
	}
	// A later call without Fresh serves the frozen bytes even though the store moved.
	store.sessions = append(store.sessions, db.Session{ID: "SES9", Kind: "chat", Title: "Brand new", UpdatedAt: 1_800_000_500})
	in.Fresh = false
	b2 := svc.Brief(context.Background(), in)
	if b2.Text != b1.Text {
		t.Fatal("brief must be frozen between adopt points")
	}
	svc.InvalidateBrief("SES1")
	b3 := svc.Brief(context.Background(), in)
	if !strings.Contains(b3.Text, "Brand new") {
		t.Fatal("after invalidation the brief recomposes from live state")
	}
	// The sidecar mirrors the brief for the UI.
	seen := svc.Seen("SES1")
	if seen.Brief == nil || seen.Brief.Hash != b3.Hash {
		t.Fatalf("seen: %+v", seen.Brief)
	}
	if _, err := os.Stat(filepath.Join(store.root, "sessions", "SES1", stateFile)); err != nil {
		t.Fatalf("state sidecar: %v", err)
	}
}

func TestBriefServesReachingNotesInRuleOrderAndHonoursTheRanker(t *testing.T) {
	store := newFakeStore(t)
	svc := newService(t, store)
	ns, _ := notes.Open(filepath.Join(store.root, "notes"))
	put := func(n notes.Note) notes.Note {
		n.Body = "body"
		n.Confidence = notes.ConfidenceInferred
		out, err := ns.Put(n)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	put(notes.Note{Kind: notes.KindWork, Title: "Work log", Scope: notes.ScopeWorkspace})
	put(notes.Note{Kind: notes.KindLesson, Title: "Quote paths", Scope: notes.ScopeWorkspace, SourceAgent: "AGT2"})
	put(notes.Note{Kind: notes.KindLesson, Title: "Mine first", Scope: notes.ScopeWorkspace, SourceAgent: "AGT1"})
	put(notes.Note{Kind: notes.KindLesson, Title: "Not mine", Scope: notes.ScopeAgent, Agents: []string{"AGT2"}})
	put(notes.Note{Kind: notes.KindLesson, Title: "Project only", Scope: notes.ScopeProject, Projects: []string{"/repo/a"}})
	in := baseInput(store, "SES1")
	in.Notes = ns
	in.Cwd = "/repo/a/sub"
	b := svc.Brief(context.Background(), in)
	if strings.Contains(b.Text, "Not mine") {
		t.Fatal("another agent's note must not reach this agent")
	}
	for _, want := range []string{"Mine first", "Quote paths", "Project only", "Work log", "4 note(s) reach this session"} {
		if !strings.Contains(b.Text, want) {
			t.Fatalf("brief lacks %q:\n%s", want, b.Text)
		}
	}
	if strings.Index(b.Text, "Mine first") > strings.Index(b.Text, "Quote paths") || strings.Index(b.Text, "Quote paths") > strings.Index(b.Text, "Work log") {
		t.Fatalf("rule order: own lessons, other lessons, then work:\n%s", b.Text)
	}
	// A ranker (the decision authority) may reorder and shrink the list.
	svc.InvalidateBrief("SES1")
	in.RankNotes = func(_ context.Context, cands []notes.Note, limit int) []notes.Note {
		for _, c := range cands {
			if c.Title == "Work log" {
				return []notes.Note{c}
			}
		}
		return nil
	}
	b2 := svc.Brief(context.Background(), in)
	if strings.Contains(b2.Text, "Mine first") || !strings.Contains(b2.Text, "Work log") {
		t.Fatalf("ranker must decide what is inlined:\n%s", b2.Text)
	}
}

func TestBriefDegradesToPointersUnderASmallBudget(t *testing.T) {
	store := newFakeStore(t)
	for i := 0; i < 40; i++ {
		store.sessions = append(store.sessions, db.Session{ID: "SES" + strings.Repeat("9", 1+i%3) + string(rune('A'+i%26)), Kind: "chat", Title: strings.Repeat("long title ", 6), MessageCount: i, UpdatedAt: int64(1_799_000_000 + i)})
	}
	svc := newService(t, store)
	small := DefaultSettings()
	small.BriefBudgetBytes = 1024
	svc.settings = func() Settings { return small }
	b := svc.Brief(context.Background(), baseInput(store, "SES1"))
	if b.Bytes > 1024+meterReserve {
		t.Fatalf("over budget: %d", b.Bytes)
	}
	if !strings.Contains(b.Meter, "pointer:") {
		t.Fatalf("a squeezed brief must name what degraded: %q", b.Meter)
	}
	if !strings.Contains(b.Text, "# Session briefing") {
		t.Fatal("the pinned intro must survive")
	}
}

func TestTurnCarriesChecklistAndDedupesThePulse(t *testing.T) {
	store := newFakeStore(t)
	store.sessions = []db.Session{{ID: "SES1", Kind: "chat"}, {ID: "SES2", Kind: "chat", StuckTurns: 1}}
	store.messages["SES1"] = []db.Message{{ID: "m1", Steps: todoStep([2]string{"write tests", "in_progress"}, [2]string{"ship", "pending"})}}
	store.artifacts["SES1"] = []db.Artifact{{ID: "ART1", Title: "Plan", Kind: "markdown"}}
	svc := newService(t, store)
	in := baseInput(store, "SES1")
	in.Fresh = false
	lead := []Section{{Key: "clock", Text: "Current date: now", Priority: PriorityPinned, Volatile: true}}

	t1 := svc.Turn(context.Background(), in, lead)
	for _, want := range []string{"Current date: now", "## Active todo list", "write tests", "## Artifacts in this session", "ART1", "[workspace pulse", "1 stuck", "needs attention", "[context meter · turn"} {
		if !strings.Contains(t1.Text, want) {
			t.Fatalf("turn lacks %q:\n%s", want, t1.Text)
		}
	}
	// Same state next turn: the pulse is silent, the rest still rides.
	t2 := svc.Turn(context.Background(), in, lead)
	if strings.Contains(t2.Text, "[workspace pulse") {
		t.Fatal("an unchanged pulse must not be re-sent")
	}
	if !strings.Contains(t2.Text, "## Active todo list") {
		t.Fatal("the checklist rides every turn")
	}
	// The workspace changes: the pulse speaks again.
	store.sessions = append(store.sessions, db.Session{ID: "SES3", Kind: "chat", StuckTurns: 3})
	t3 := svc.Turn(context.Background(), in, lead)
	if !strings.Contains(t3.Text, "[workspace pulse") || !strings.Contains(t3.Text, "2 stuck") {
		t.Fatalf("a changed pulse must be delivered:\n%s", t3.Text)
	}
	// A decision authority may overrule the rule-based urgency.
	store.sessions = append(store.sessions, db.Session{ID: "SES4", Kind: "chat", StuckTurns: 3})
	in.JudgeUrgent = func(context.Context, string) (bool, bool) { return false, true }
	t4 := svc.Turn(context.Background(), in, lead)
	if strings.Contains(t4.Text, "needs attention") {
		t.Fatal("the authority's verdict must stand")
	}
	if seen := svc.Seen("SES1"); seen.Turns != 4 || seen.LastTurn == nil || seen.LastPulse == "" {
		t.Fatalf("seen: %+v", seen)
	}
}

func TestTurnOffersTheProgressFileAfterAContextResetButNotOnTheFirstTurn(t *testing.T) {
	store := newFakeStore(t)
	dir := t.TempDir()
	if err := progress.Save(dir, progress.Record{Todos: []progress.TodoItem{{Content: "resume me", Status: "pending"}}}); err != nil {
		t.Fatal(err)
	}
	svc := newService(t, store)
	in := baseInput(store, "SES1")
	in.ProgressDir, in.ProgressResume = dir, true
	in.Fresh = false
	if !strings.Contains(svc.Turn(context.Background(), in, nil).Text, "resume me") {
		t.Fatal("a session without its own checklist resumes the progress file")
	}
	in.Fresh = true
	if strings.Contains(svc.Turn(context.Background(), in, nil).Text, "resume me") {
		t.Fatal("on the first turn the brief owns the resumed progress")
	}
	if !strings.Contains(svc.Brief(context.Background(), in).Text, "## Resumed progress") {
		t.Fatal("the brief carries the resumed progress")
	}
}

func TestDigestCountsFactsPersistsOnceAndIndexes(t *testing.T) {
	store := newFakeStore(t)
	store.sessions = []db.Session{
		{ID: "SES1", AgentID: "AGT1", Kind: "chat", Title: "Fix build", MessageCount: 5, ToolCallCount: 3, CreatedAt: 100, UpdatedAt: 400},
		{ID: "SES7", CoordinatorSessionID: "SES1", Tags: []string{"error"}},
	}
	store.messages["SES1"] = []db.Message{
		{ID: "m1", Steps: toolStep("Bash", true)},
		{ID: "m2", Steps: toolStep("Bash", true)},
		{ID: "m3", Steps: toolStep("Read", false)},
		{ID: "m4", Steps: todoStep([2]string{"done it", "completed"}, [2]string{"left", "pending"})},
	}
	store.artifacts["SES1"] = []db.Artifact{{ID: "ART1", Title: "Patch"}}
	svc := newService(t, store)
	in := baseInput(store, "SES1")
	in.Session = store.sessions[0]

	d, changed, err := svc.Digest(context.Background(), in)
	if err != nil || !changed {
		t.Fatalf("digest: %v changed=%v", err, changed)
	}
	if d.ToolErrors != 2 || len(d.Tools) != 2 || d.Tools[0].Name != "Bash" || d.Tools[0].Errors != 2 {
		t.Fatalf("tool histogram: %+v", d.Tools)
	}
	if d.Todo.Done != 1 || d.Todo.Total != 2 || len(d.Todo.Open) != 1 {
		t.Fatalf("todo: %+v", d.Todo)
	}
	if d.Children != 1 || d.ChildrenFailed != 1 || len(d.Artifacts) != 1 || d.DurationSec != 300 {
		t.Fatalf("facts: %+v", d)
	}
	if !d.SuggestNote {
		t.Fatal("two tool errors and a failed child with no note → suggest a note")
	}
	for _, want := range []string{"# Session digest · SES1", "Bash×2(2 err)", "checklist: 1/2 done", "ART1", "child sessions: 1 (1 failed/stuck)", "open loops:", "suggestion:"} {
		if !strings.Contains(d.Text, want) {
			t.Fatalf("digest text lacks %q:\n%s", want, d.Text)
		}
	}
	if _, ok := svc.LoadDigest("SES1"); !ok {
		t.Fatal("digest sidecar missing")
	}
	// Unchanged facts → not rewritten.
	if _, changed, _ := svc.Digest(context.Background(), in); changed {
		t.Fatal("identical digest must not count as changed")
	}
	// A new message changes it.
	store.messages["SES1"] = append(store.messages["SES1"], db.Message{ID: "m5", Steps: toolStep("Edit", false)})
	if _, changed, _ := svc.Digest(context.Background(), in); !changed {
		t.Fatal("new facts must be detected")
	}
	rec := svc.RecentDigests(5, "")
	if len(rec) != 1 || rec[0].SessionID != "SES1" || rec[0].Errors != 2 || !strings.Contains(rec[0].Line, "Fix build") {
		t.Fatalf("index: %+v", rec)
	}
	if len(svc.RecentDigests(5, "SES1")) != 0 {
		t.Fatal("exclusion must work")
	}
	// The next session's brief lists it.
	in2 := baseInput(store, "SES2")
	b := svc.Brief(context.Background(), in2)
	if !strings.Contains(b.Text, "## Recently finished work") || !strings.Contains(b.Text, "SES1") {
		t.Fatalf("brief lacks recent work:\n%s", b.Text)
	}
	svc.Forget("SES1")
	if len(svc.AllDigests(0)) != 0 {
		t.Fatal("forget must prune the index")
	}
}

func TestDisabledLayerIsSilent(t *testing.T) {
	store := newFakeStore(t)
	svc := newService(t, store)
	off := DefaultSettings()
	off.Enabled = false
	svc.settings = func() Settings { return off }
	in := baseInput(store, "SES1")
	if b := svc.Brief(context.Background(), in); b.Text != "" {
		t.Fatalf("disabled brief must be empty: %q", b.Text)
	}
	tn := svc.Turn(context.Background(), in, []Section{{Key: "clock", Text: "now", Priority: PriorityPinned}})
	if !strings.Contains(tn.Text, "now") || strings.Contains(tn.Text, "pulse") {
		t.Fatalf("disabled turn carries only the caller's sections: %q", tn.Text)
	}
	if _, changed, _ := svc.Digest(context.Background(), in); changed {
		t.Fatal("disabled digest writes nothing")
	}
}

func TestSettingsNormalizedFillsZeros(t *testing.T) {
	s := Settings{Enabled: true, BriefBudgetBytes: 10}.Normalized()
	if s.BriefBudgetBytes != 1024 || s.TurnBudgetBytes != 16384 || s.NoteCount != 6 || s.StaleCardDays != 3 {
		t.Fatalf("normalized: %+v", s)
	}
}
