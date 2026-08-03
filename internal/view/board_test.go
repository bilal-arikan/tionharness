package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// boardFixture builds a board with one card per interesting condition.
func boardFixture(now time.Time) BoardInput {
	fresh := now.Add(-1 * time.Hour).UnixMilli()
	old := now.Add(-9 * 24 * time.Hour).UnixMilli()
	yesterday := now.Add(-25 * time.Hour).UnixMilli()

	return BoardInput{
		Now: now,
		Tasks: []db.Task{
			{ID: "T1", Title: "backlog item", BoardState: db.BoardTodo, UpdatedAt: old, OwnerAgentID: "AG1"},
			{ID: "T2", Title: "stuck work", BoardState: db.BoardInProgress, UpdatedAt: old, OwnerAgentID: "AG1"},
			{ID: "T3", Title: "active work", BoardState: db.BoardInProgress, UpdatedAt: fresh, OwnerAgentID: "AG1"},
			{ID: "T4", Title: "broke", BoardState: db.BoardFailed, UpdatedAt: yesterday, OwnerAgentID: "AG1"},
			{ID: "T5", Title: "waits on T2", BoardState: db.BoardTodo, UpdatedAt: fresh,
				Dependencies: `["T2"]`, OwnerAgentID: "AG1"},
			{ID: "T6", Title: "late", BoardState: db.BoardTodo, UpdatedAt: fresh,
				DueDate: now.Add(-48 * time.Hour).Format("2006-01-02"), OwnerAgentID: "AG1"},
			{ID: "T7", Title: "ownerless", BoardState: db.BoardTodo, UpdatedAt: fresh},
			{ID: "T8", Title: "shipped", BoardState: db.BoardDone, UpdatedAt: old, OwnerAgentID: "AG1"},
		},
	}
}

func TestBoardCardSurfacesEverySignal(t *testing.T) {
	now := time.Now()
	v, err := ProjectBoard(boardFixture(now), LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()

	if !strings.Contains(v.Header, "8 kart") {
		t.Errorf("header lacks card count: %q", v.Header)
	}
	// Histogram order follows the real board (todo before in_progress before …).
	if !strings.Contains(txt, "todo 4 | in_progress 2 | done 1 | failed 1") {
		t.Errorf("histogram wrong or misordered:\n%s", txt)
	}
	// Only the in-progress card is stale — a backlog card that has not moved is
	// normal and must NOT be flagged.
	if !strings.Contains(txt, "hareketsiz: T2") {
		t.Errorf("stale in-progress card not flagged:\n%s", txt)
	}
	if strings.Contains(txt, "hareketsiz: T1") || strings.Contains(txt, "T1,") {
		t.Errorf("stale backlog card wrongly flagged:\n%s", txt)
	}
	if !strings.Contains(txt, "başarısız kart: T4") {
		t.Errorf("failed card missing:\n%s", txt)
	}
	if !strings.Contains(txt, "bloke: T5") {
		t.Errorf("dependency-blocked card missing:\n%s", txt)
	}
	if !strings.Contains(txt, "gecikmiş: T6") {
		t.Errorf("overdue card missing:\n%s", txt)
	}
	if !strings.Contains(txt, "1 kartın sahibi yok") {
		t.Errorf("unassigned count wrong (done cards must not count):\n%s", txt)
	}
	// A card-level board is a signal report, not a card list — and it says so.
	if v.Elided != 8 {
		t.Errorf("card level must report all 8 cards as elided, got %d", v.Elided)
	}
}

func TestBoardStaleLensDropsRoutineSignals(t *testing.T) {
	v, err := ProjectBoard(boardFixture(time.Now()), LevelCard, LensStale)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "hareketsiz") {
		t.Errorf("stale lens dropped its own signal:\n%s", txt)
	}
	if strings.Contains(txt, "Δ24s") || strings.Contains(txt, "sahibi yok") {
		t.Errorf("stale lens leaked routine signals:\n%s", txt)
	}
}

func TestBoardFullListsCardsAndCountsDropped(t *testing.T) {
	now := time.Now()
	in := boardFixture(now)
	for i := 0; i < 60; i++ {
		in.Tasks = append(in.Tasks, db.Task{
			ID:    "X" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Title: "filler", BoardState: db.BoardTodo, UpdatedAt: now.UnixMilli(),
		})
	}
	v, err := ProjectBoard(in, LevelFull, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if v.Elided != len(in.Tasks)-boardFullCards {
		t.Errorf("dropped count = %d, want %d", v.Elided, len(in.Tasks)-boardFullCards)
	}
	if !strings.Contains(v.Text(), "kart gizlendi") {
		t.Errorf("elision not rendered:\n%s", v.Text())
	}
}

func TestBoardEmptyIsExplicit(t *testing.T) {
	v, err := ProjectBoard(BoardInput{Now: time.Now()}, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	// An empty board must SAY it is empty rather than render a blank body that
	// reads like a failed projection.
	if !strings.Contains(v.Text(), "kart yok") {
		t.Errorf("empty board not stated:\n%s", v.Text())
	}
	if v.Elided != 0 {
		t.Errorf("empty board must not claim elision, got %d", v.Elided)
	}
}

func TestBoardCustomColumnsSortAfterBuiltins(t *testing.T) {
	now := time.Now()
	in := BoardInput{Now: now, Tasks: []db.Task{
		{ID: "A", BoardState: "zeta_custom", UpdatedAt: now.UnixMilli()},
		{ID: "B", BoardState: db.BoardTodo, UpdatedAt: now.UnixMilli()},
		{ID: "C", BoardState: "alpha_custom", UpdatedAt: now.UnixMilli()},
	}}
	v, err := ProjectBoard(in, LevelCard, LensHealth)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "todo 1 | alpha_custom 1 | zeta_custom 1") {
		t.Errorf("custom columns must follow built-ins, alphabetically:\n%s", v.Text())
	}
}
