package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestBoardCardSubProjectsOneCard pins the single-card drill-down: a sub id
// narrows the board projection to that card, and an unknown id is an error rather
// than a blank card.
func TestBoardCardSubProjectsOneCard(t *testing.T) {
	now := time.Now()
	in := boardFixture(now)
	in.Sub = "T4" // the failed card

	v, err := ProjectBoard(in, LevelCard)
	if err != nil {
		t.Fatalf("project card: %v", err)
	}
	if !strings.HasPrefix(v.Header, "CARD T4") {
		t.Errorf("header not a single-card header: %q", v.Header)
	}
	if !strings.Contains(v.Text(), "başarısız") {
		t.Errorf("failed card must say so:\n%s", v.Text())
	}
	if v.Ref.Sub != "T4" {
		t.Errorf("ref.Sub = %q, want T4", v.Ref.Sub)
	}
	// A blocked/overdue card must carry its marks so the drill-down is self-contained.
	in.Sub = "T5" // waits on T2 (not done) → blocked
	blocked, err := ProjectBoard(in, LevelCard)
	if err != nil {
		t.Fatalf("project blocked card: %v", err)
	}
	if !strings.Contains(blocked.Text(), "bloke") {
		t.Errorf("blocked card missing ⛔ mark:\n%s", blocked.Text())
	}

	in.Sub = "NOPE"
	if _, err := ProjectBoard(in, LevelCard); err == nil {
		t.Error("unknown card id must error, not render a blank card")
	}
}

func TestScheduleProjectionSurfacesLastError(t *testing.T) {
	now := time.Now()
	sc := db.Schedule{
		ID:                 "SCH1",
		CronExpr:           "0 9 * * *",
		AgentID:            "AG1",
		Prompt:             "daily digest",
		Enabled:            false,
		LastRunAt:          now.Add(-2 * time.Hour).Unix(),
		LastDeliveryStatus: "error",
		LastDeliveryError:  "provider timeout",
	}

	v, err := ProjectSchedule(ScheduleInput{Schedule: sc, Now: now}, LevelCard)
	if err != nil {
		t.Fatalf("project schedule: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(v.Header, "disabled") {
		t.Errorf("header must state disabled state: %q", v.Header)
	}
	if !strings.Contains(txt, "provider timeout") {
		t.Errorf("last error must be surfaced even for a disabled schedule:\n%s", txt)
	}
	if !strings.Contains(txt, "agent:AG1") {
		t.Errorf("delivery target missing:\n%s", txt)
	}
}

// TestScheduleProjectionSurfacesLastError pins the schedule drill-down: the last
// fire's error is the headline, the enabled/disabled state is explicit, and a
// disabled schedule still shows its error here (unlike the workspace roll-up).

// TestProjectScheduleOneShotWake pins the one-shot (schedule_wake) shape: a wake
// carries no cron expression, so the header used to assert a broken `cron ""`.
// It fires at FireAt and delivers back into the originating session.
func TestProjectScheduleOneShotWake(t *testing.T) {
	now := time.Now()
	v, err := ProjectSchedule(ScheduleInput{
		Schedule: db.Schedule{
			ID: "SCH9", Enabled: true, OneShot: true,
			FireAt: now.Add(30 * time.Minute).Unix(), SessionID: "SES4",
			Reason: "derleme bitince kontrol", Prompt: "durumu özetle",
		},
		Now: now,
	}, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Header, "tek seferlik") || strings.Contains(v.Header, `cron ""`) {
		t.Errorf("a one-shot must not claim an empty cron: %q", v.Header)
	}
	txt := v.Text()
	for _, want := range []string{"ateşleme:", "hedef: session:SES4", "neden: derleme bitince kontrol"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

// TestProjectScheduleSessionModeIsFullOnly locks the level discipline for the
// per-fire session mode: long-tail detail, drill-down only.
func TestProjectScheduleSessionModeIsFullOnly(t *testing.T) {
	in := ScheduleInput{
		Schedule: db.Schedule{ID: "SCH8", Enabled: true, CronExpr: "0 9 * * *", AgentID: "AG1"},
		Now:      time.Now(),
	}
	card, err := ProjectSchedule(in, LevelCard)
	if err != nil {
		t.Fatalf("card: %v", err)
	}
	if strings.Contains(card.Text(), "oturum modu") {
		t.Errorf("session mode must not reach the card tier:\n%s", card.Text())
	}
	full, err := ProjectSchedule(in, LevelFull)
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if !strings.Contains(full.Text(), "oturum modu: reuse") {
		t.Errorf("full tier must resolve the session mode default:\n%s", full.Text())
	}
}
