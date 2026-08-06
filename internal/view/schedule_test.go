package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestBoardCardSubProjectsOneCard pins the single-card drill-down: a sub id
// narrows the board projection to that card, and an unknown id is an error rather
// than a blank card.
func TestBoardCardSubProjectsOneCard(t *testing.T) {
	now := time.Now()
	in := boardFixture(now)
	in.Sub = "T4" // the failed card

	v, err := ProjectBoard(in, LevelCard, LensHealth)
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
	blocked, err := ProjectBoard(in, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project blocked card: %v", err)
	}
	if !strings.Contains(blocked.Text(), "bloke") {
		t.Errorf("blocked card missing ⛔ mark:\n%s", blocked.Text())
	}

	in.Sub = "NOPE"
	if _, err := ProjectBoard(in, LevelCard, LensHealth); err == nil {
		t.Error("unknown card id must error, not render a blank card")
	}
}

// TestScheduleProjectionSurfacesLastError pins the schedule drill-down: the last
// fire's error is the headline, the enabled/disabled state is explicit, and a
// disabled schedule still shows its error here (unlike the workspace roll-up).
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

	v, err := ProjectSchedule(ScheduleInput{Schedule: sc, Now: now}, LevelCard, LensHealth)
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

	// The errors lens keeps only the failure line.
	errView, _ := ProjectSchedule(ScheduleInput{Schedule: sc, Now: now}, LevelCard, LensErrors)
	if strings.Contains(errView.Text(), "prompt:") {
		t.Errorf("errors lens should drop the prompt line:\n%s", errView.Text())
	}
}
