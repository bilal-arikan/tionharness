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
		Usage: db.SessionUsage{InputTokens: 150_000, OutputTokens: 20_000, CacheReadTokens: 17_000},
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
	v, err := ProjectSession(sessionFixture(now), LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()

	if !strings.Contains(v.Header, "214 msg") || !strings.Contains(v.Header, "187k tok") {
		t.Errorf("header cost/size wrong: %q", v.Header)
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

	v, err := ProjectSession(in, LevelCard, LensHealth)
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

func TestSessionErrorsLensDropsRoutineLines(t *testing.T) {
	in := sessionFixture(time.Now())
	in.Session.StuckTurns = 1

	v, err := ProjectSession(in, LevelCard, LensErrors)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "StuckTurns 1") {
		t.Errorf("errors lens dropped a failure signal:\n%s", txt)
	}
	for _, unwanted := range []string{"todo:", "özet:", "compaction", "son hareket"} {
		if strings.Contains(txt, unwanted) {
			t.Errorf("errors lens leaked %q:\n%s", unwanted, txt)
		}
	}
}

func TestSessionCoordinatorLineageInHeader(t *testing.T) {
	in := sessionFixture(time.Now())
	in.Session.CoordinatorMode = true
	in.Session.CoordinatorSessionID = "SESroot"
	in.Session.CoordinatorDepth = 2

	v, err := ProjectSession(in, LevelCard, LensHealth)
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
	v, err := ProjectSession(sessionFixture(time.Now()), LevelTiny, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
}

func TestSessionRejectsEmptyInput(t *testing.T) {
	if _, err := ProjectSession(SessionInput{}, LevelCard, LensHealth); err == nil {
		t.Fatal("expected an error for an empty session, got a view")
	}
}
