package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// workspaceFixture builds a workspace with one instance of each condition the
// projection is supposed to notice.
func workspaceFixture(now time.Time) WorkspaceInput {
	fresh := now.Add(-2 * time.Hour).Unix()
	old := now.Add(-9 * 24 * time.Hour).Unix()

	return WorkspaceInput{
		Now:         now,
		TokensToday: 1_250_000,
		Agents:      []db.Agent{{ID: "AG1", Name: "builder"}, {ID: "AG2", Name: "researcher"}},
		Sessions: []db.Session{
			{ID: "SES1", AgentID: "AG1", UpdatedAt: fresh},
			{ID: "SES2", AgentID: "AG1", UpdatedAt: fresh, StuckTurns: 2, Title: "takılan iş"},
			{ID: "SES3", AgentID: "AG2", UpdatedAt: old},                      // idle
			{ID: "SES4", AgentID: "AG2", UpdatedAt: fresh, State: "archived"}, // excluded
			{ID: "SES5", AgentID: "AG1", UpdatedAt: fresh, CoordinatorMode: true},
		},
		Tasks: []db.Task{
			{ID: "T1", BoardState: db.BoardTodo, UpdatedAt: fresh},
			{ID: "T2", BoardState: db.BoardInProgress, UpdatedAt: old}, // stale
			{ID: "T3", BoardState: db.BoardFailed, UpdatedAt: fresh},
		},
		FlowRuns: []db.FlowRun{
			{ID: "RUN1", Status: db.FlowSuccess, CreatedAt: fresh, UpdatedAt: fresh},
			{ID: "RUN2", Status: db.FlowRunning, CreatedAt: fresh, UpdatedAt: fresh},
			{ID: "RUN3", Status: db.FlowWaiting, CreatedAt: fresh, UpdatedAt: fresh},
			{ID: "RUN4", Status: db.FlowFailure, CreatedAt: old, UpdatedAt: old, Error: "provider 429"},
		},
		Schedules: []db.Schedule{
			{ID: "SCH1", Enabled: true, LastDeliveryStatus: "error", LastDeliveryError: "cron hedefi yok"},
			{ID: "SCH2", Enabled: false, LastDeliveryStatus: "error"}, // paused on purpose
		},
		WaitingAsks: []db.SessionAsk{
			{ID: "SAK1", SessionID: "SES1", CreatedAt: now.Add(-90 * time.Minute).Unix()},
			{ID: "SAK2", SessionID: "SES3", CreatedAt: now.Add(-10 * time.Minute).Unix()},
		},
	}
}

func TestWorkspaceHeaderCountsWhatMatters(t *testing.T) {
	now := time.Now()
	v, err := ProjectWorkspace(workspaceFixture(now), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}

	// 5 sessions total; 3 active (fresh + non-archived); the archived one is
	// excluded from "active" but still counted in the total.
	for _, want := range []string{"2 ajan", "5 oturum (3 aktif)", "3 kart", "4 koşu", "1.2M tok bugün"} {
		if !strings.Contains(v.Header, want) {
			t.Errorf("header missing %q: %q", want, v.Header)
		}
	}
}

func TestWorkspaceSurfacesEverySignal(t *testing.T) {
	now := time.Now()
	v, err := ProjectWorkspace(workspaceFixture(now), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()

	for _, want := range []string{
		"1 oturum takılmış",
		"SES2",
		"2 oturum cevap bekliyor",
		"en eskisi 1sa'dir", // the OLDEST ask (90 min), not the newest (10 min)
		"session:SES1",
		"1 başarısız akış koşusu: RUN4",
		"1 zamanlama son çalışmada hata verdi", // the disabled one must not count
		"1 başarısız kart: T3",
		"hareketsiz: T2",
		"1 akış koşusu girdi bekliyor",
		"1 koordinatör oturumu",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
	// The run rollup line reports each status.
	if !strings.Contains(txt, "1 çalışıyor · 1 bekliyor · 1 başarısız") {
		t.Errorf("run rollup wrong:\n%s", txt)
	}
}

func TestWorkspaceDisabledScheduleIsNotAFailure(t *testing.T) {
	now := time.Now()
	in := workspaceFixture(now)
	// Only the deliberately-paused schedule is left; a paused schedule that last
	// errored is not a problem, and reporting it trains the reader to ignore the
	// line entirely.
	in.Schedules = []db.Schedule{{ID: "SCH2", Enabled: false, LastDeliveryStatus: "error"}}

	v, err := ProjectWorkspace(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if strings.Contains(v.Text(), "zamanlama son çalışmada hata") {
		t.Errorf("disabled schedule wrongly reported as broken:\n%s", v.Text())
	}
}

func TestWorkspaceFullListsTheSpecifics(t *testing.T) {
	now := time.Now()
	v, err := ProjectWorkspace(workspaceFixture(now), LevelFull)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	for _, want := range []string{"stuck   session:SES2", "failed  run:RUN4", "provider 429", "cron hedefi yok"} {
		if !strings.Contains(txt, want) {
			t.Errorf("full detail missing %q:\n%s", want, txt)
		}
	}
}

func TestWorkspaceHandlesPointAtTheTrouble(t *testing.T) {
	v, err := ProjectWorkspace(workspaceFixture(time.Now()), LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	kinds := map[Kind]string{}
	for _, h := range v.Handles {
		kinds[h.Ref.Kind] = h.Ref.ID
	}
	if kinds[KindFlowRun] != "RUN4" {
		t.Errorf("no handle for the failed run: %+v", v.Handles)
	}
	if kinds[KindSession] != "SES2" {
		t.Errorf("no handle for the stuck session: %+v", v.Handles)
	}
	if _, ok := kinds[KindBoard]; !ok {
		t.Errorf("no handle for the board: %+v", v.Handles)
	}
}

func TestWorkspaceTinyIsHeaderOnly(t *testing.T) {
	v, err := ProjectWorkspace(workspaceFixture(time.Now()), LevelTiny)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Body != "" {
		t.Errorf("tiny level must not emit a body: %q", v.Body)
	}
}
