package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// The persisted models mix timestamp units and the types do not say which is
// which: db.now() (every entity's CreatedAt/UpdatedAt) and TraceEntry.At are unix
// SECONDS, while TraceEntry.StartMs/EndMs are MILLISECONDS.
//
// Reading a seconds value as millis does not fail loudly — it renders a card
// touched an hour ago as "20648g önce" and a 31-second node as "31ms". That is
// the worst possible failure for a projection whose entire promise is "these
// numbers are computed, so you can trust them", so each unit gets a test that
// pins a HUMAN-SCALE result rather than an exact string.

func TestBoardUsesSecondTimestamps(t *testing.T) {
	now := time.Now()
	in := BoardInput{Now: now, Tasks: []db.Task{
		{ID: "T1", Title: "az önce dokunuldu", BoardState: db.BoardInProgress,
			UpdatedAt: now.Add(-1 * time.Hour).Unix()},
	}}

	v, err := ProjectBoard(in, LevelFull)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()

	// An hour-old card renders in hours, and is NOT stale.
	if !strings.Contains(txt, "1sa önce") {
		t.Errorf("age misread (expected \"1sa önce\"):\n%s", txt)
	}
	if strings.Contains(txt, "hareketsiz") {
		t.Errorf("an hour-old card must not be flagged stale:\n%s", txt)
	}
	// …and it counts as recent movement.
	if !strings.Contains(txt, "Δ24s") {
		t.Errorf("an hour-old card must count as recent:\n%s", txt)
	}
}

func TestBoardStalenessUsesSecondTimestamps(t *testing.T) {
	now := time.Now()
	in := BoardInput{Now: now, Tasks: []db.Task{
		{ID: "T1", Title: "unutulmuş", BoardState: db.BoardInProgress,
			UpdatedAt: now.Add(-5 * 24 * time.Hour).Unix()},
	}}

	v, err := ProjectBoard(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "hareketsiz: T1") {
		t.Errorf("a 5-day-old in-progress card must be stale:\n%s", v.Text())
	}
}

func TestSessionUsesSecondTimestamps(t *testing.T) {
	now := time.Now()
	in := SessionInput{
		Now: now,
		Session: db.Session{
			ID: "SES1", AgentID: "a", Title: "t",
			CreatedAt: now.Add(-2 * time.Hour).Unix(),
			UpdatedAt: now.Add(-5 * time.Minute).Unix(),
		},
		WaitingAsk: &db.SessionAsk{ID: "SAK1", SessionID: "SES1", Kind: "ask",
			CreatedAt: now.Add(-3 * time.Minute).Unix()},
	}

	v, err := ProjectSession(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	for _, want := range []string{
		"2sa önce açıldı",  // Session.CreatedAt
		"son hareket: 5dk", // Session.UpdatedAt
		"3dk'dir bekliyor", // SessionAsk.CreatedAt
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q (timestamp unit misread):\n%s", want, txt)
		}
	}
}

func TestAgeReportsUnsetTimestampAsUnknown(t *testing.T) {
	// A zero stamp must read as unknown, not as 1970 — "20648g önce" is exactly
	// how a unit bug disguises itself as data.
	if got := age(tsSec(0), time.Now()); got != "?" {
		t.Errorf("age(unset) = %q, want ?", got)
	}
	if got := age(tsMs(0), time.Now()); got != "?" {
		t.Errorf("age(unset ms) = %q, want ?", got)
	}
}
