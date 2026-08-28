package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func sessionFixture(now time.Time) SessionInput {
	return SessionInput{
		Now: now,
		Session: db.Session{
			ID: "SES9a1", AgentID: "builder", Title: "auth refactor",
			MessageCount: 214, Kind: "chat",
			CreatedAt: now.Add(-72 * time.Hour).Unix(),
			UpdatedAt: now.Add(-4 * time.Minute).Unix(),
			Summary:   "JWT'den session cookie'ye geçiş yapılıyor.", SummaryMsgCount: 120,
		},
		Messages: []db.Message{
			{Role: "user", Text: "devam et"},
			{Role: "assistant", Text: "tamam", Steps: `[
				{"kind":"todo","todos":[
					{"content":"şemayı çıkar","status":"completed"},
					{"content":"handler'ı yaz","status":"completed"},
					{"content":"testleri güncelle","status":"in_progress"},
					{"content":"dokümanı güncelle","status":"pending"}]}]`},
		},
		TailFrom: 174,
	}
}

func TestSessionCardSummarisesWithoutTranscript(t *testing.T) {
	now := time.Now()
	v, err := ProjectSession(sessionFixture(now), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()

	if !strings.Contains(v.Header, "214 msg") {
		t.Errorf("header size wrong: %q", v.Header)
	}
	// Spend belongs to the budget view; the session header must not carry it.
	if strings.Contains(v.Header, " tok") || strings.Contains(v.Header, "$") {
		t.Errorf("header still reports spend: %q", v.Header)
	}
	if !strings.Contains(v.Header, "agent:builder") {
		t.Errorf("header lacks agent: %q", v.Header)
	}
	// The checklist is the single most informative line about a working session.
	if !strings.Contains(txt, "todo: 2/4 tamam") || !strings.Contains(txt, "şu an: testleri güncelle") {
		t.Errorf("todo rollup wrong:\n%s", txt)
	}
	if !strings.Contains(txt, "compaction") {
		t.Errorf("compaction signal missing:\n%s", txt)
	}
	// The projection only read the tail — it must say how much it skipped.
	if v.Elided != 174 {
		t.Errorf("skipped-message count = %d, want 174", v.Elided)
	}
	if !strings.Contains(txt, "eski mesaj gizlendi") {
		t.Errorf("elision not rendered:\n%s", txt)
	}
}

func TestSessionSurfacesTrouble(t *testing.T) {
	now := time.Now()
	in := sessionFixture(now)
	in.Session.StuckTurns = 3
	in.Session.Tags = []string{"proje-x", "stuck", "tool-error"}
	in.Session.ParentSessionID = "SES001"
	in.Messages = append(in.Messages, db.Message{
		Role:  "assistant",
		Steps: `[{"kind":"tool","tool":"Bash","isError":true,"output":"exit 1: permission denied"}]`,
	})
	in.WaitingAsk = &db.SessionAsk{ID: "SAK1", SessionID: "SES9a1", Kind: "ask",
		CreatedAt: now.Add(-30 * time.Minute).Unix()}

	v, err := ProjectSession(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()

	for _, want := range []string{
		"StuckTurns 3",
		"cevap bekleyen soru",
		"son hata: Bash: exit 1: permission denied",
		"handoff ile session:SES001",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
	// Only alarming tags are surfaced; organisational ones are the user's business.
	if !strings.Contains(txt, "stuck, tool-error") {
		t.Errorf("signal tags wrong:\n%s", txt)
	}
	if strings.Contains(txt, "proje-x") {
		t.Errorf("organisational tag leaked into the health signals:\n%s", txt)
	}
}

// TestSessionKindInHeader pins the kind in the header, including the "chat"
// default: whether a session accepts a new user turn depends on it, so leaving it
// implicit made a machine-written transcript read like an ordinary conversation.
func TestSessionKindInHeader(t *testing.T) {
	now := time.Now()

	v, err := ProjectSession(sessionFixture(now), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Header, "tür:chat") {
		t.Errorf("chat kind missing from header: %q", v.Header)
	}

	in := sessionFixture(now)
	in.Session.Kind = "flow"
	v, err = ProjectSession(in, LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Header, "tür:flow") {
		t.Errorf("kind missing from header: %q", v.Header)
	}

	// An empty Kind is the persisted spelling of a plain chat session.
	in.Session.Kind = ""
	v, err = ProjectSession(in, LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Header, "tür:chat") {
		t.Errorf("empty kind must render as chat: %q", v.Header)
	}
}

func TestSessionCoordinatorLineageInHeader(t *testing.T) {
	in := sessionFixture(time.Now())
	in.Session.CoordinatorMode = true
	in.Session.CoordinatorSessionID = "SESroot"
	in.Session.CoordinatorDepth = 2

	v, err := ProjectSession(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	// A mid-level node is BOTH worker and coordinator; the header must say so,
	// because it changes how every other line should be read.
	if !strings.Contains(v.Header, "mid-level coordinator") {
		t.Errorf("coordination lineage missing: %q", v.Header)
	}
}

func TestSessionTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectSession(sessionFixture(time.Now()), LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
}

func TestSessionRejectsEmptyInput(t *testing.T) {
	if _, err := ProjectSession(SessionInput{}, LevelCard); err == nil {
		t.Fatal("expected an error for an empty session, got a view")
	}
}

// TestSessionCardCarriesWorkingDirAndRunState pins the two facts that change what
// a reader may conclude about a session: which directory its tools resolve
// against, and how its last background turn ended. A session that died with
// StuckTurns still zero used to read as healthy.
func TestSessionCardCarriesWorkingDirAndRunState(t *testing.T) {
	now := time.Now()
	in := sessionFixture(now)
	in.Session.WorkingDir = `C:\Users\user\Desktop\Projects\TionHarness`
	in.Session.RunState = "failed"
	in.Session.RunStateAt = now.Add(-2 * time.Hour).Unix()

	v, err := ProjectSession(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	// clipPath normalises separators, so the bash-mounted and Windows spellings of
	// the same directory render identically.
	if !strings.Contains(txt, "cwd: ") || !strings.Contains(txt, "TionHarness") {
		t.Errorf("working dir missing:\n%s", txt)
	}
	if strings.Contains(txt, `\`) {
		t.Errorf("path separators must be normalised:\n%s", txt)
	}
	if !strings.Contains(txt, "son arka plan turu: failed") || !strings.Contains(txt, "2sa önce") {
		t.Errorf("run state missing:\n%s", txt)
	}

	// A clean run and an unset working dir are the expected cases and cost nothing.
	quiet := sessionFixture(now)
	quiet.Session.RunState = "completed"
	q, err := ProjectSession(quiet, LevelCard)
	if err != nil {
		t.Fatalf("project quiet: %v", err)
	}
	if strings.Contains(q.Text(), "cwd:") || strings.Contains(q.Text(), "arka plan turu") {
		t.Errorf("default state must not render a line:\n%s", q.Text())
	}
}
