package view

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// boardFixture builds a board with one card per interesting condition.
func boardFixture(now time.Time) BoardInput {
	// Task.UpdatedAt is unix SECONDS (db.now()), not millis.
	fresh := now.Add(-1 * time.Hour).Unix()
	old := now.Add(-9 * 24 * time.Hour).Unix()
	yesterday := now.Add(-25 * time.Hour).Unix()

	return BoardInput{
		Now: now,
		Tasks: []db.Task{
			{ID: "T1", Title: "backlog item", BoardState: db.BoardTodo, UpdatedAt: old, OwnerAgentID: "AG1"},
			{ID: "T2", Title: "stuck work", BoardState: db.BoardInProgress, UpdatedAt: old, OwnerAgentID: "AG1"},
			{ID: "T3", Title: "active work", BoardState: db.BoardInProgress, UpdatedAt: fresh, OwnerAgentID: "AG1"},
			{ID: "T4", Title: "broke", BoardState: db.BoardFailed, UpdatedAt: yesterday, OwnerAgentID: "AG1"},
			{ID: "T5", Title: "waits on T2", BoardState: db.BoardTodo, UpdatedAt: fresh,
				Dependencies: `["T2"]`, OwnerAgentID: "AG1"},
			{ID: "T6", Title: "late", BoardState: db.BoardTodo, UpdatedAt: fresh, OwnerAgentID: "AG1"},
			{ID: "T7", Title: "ownerless", BoardState: db.BoardTodo, UpdatedAt: fresh},
			{ID: "T8", Title: "shipped", BoardState: db.BoardDone, UpdatedAt: old, OwnerAgentID: "AG1"},
		},
	}
}

func TestBoardCardSurfacesEverySignal(t *testing.T) {
	now := time.Now()
	v, err := ProjectBoard(boardFixture(now), LevelCard)
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
	if !strings.Contains(txt, "1 kartın sahibi yok") {
		t.Errorf("unassigned count wrong (done cards must not count):\n%s", txt)
	}
	// A card-level board is a signal report, not a card list — and it says so.
	if v.Elided != 8 {
		t.Errorf("card level must report all 8 cards as elided, got %d", v.Elided)
	}
}

func TestBoardFullListsCardsAndCountsDropped(t *testing.T) {
	now := time.Now()
	in := boardFixture(now)
	for i := 0; i < 60; i++ {
		in.Tasks = append(in.Tasks, db.Task{
			ID:    "X" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Title: "filler", BoardState: db.BoardTodo, UpdatedAt: now.Unix(),
		})
	}
	v, err := ProjectBoard(in, LevelFull)
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
	v, err := ProjectBoard(BoardInput{Now: time.Now()}, LevelCard)
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
		{ID: "A", BoardState: "zeta_custom", UpdatedAt: now.Unix()},
		{ID: "B", BoardState: db.BoardTodo, UpdatedAt: now.Unix()},
		{ID: "C", BoardState: "alpha_custom", UpdatedAt: now.Unix()},
	}}
	v, err := ProjectBoard(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if !strings.Contains(v.Text(), "todo 1 | alpha_custom 1 | zeta_custom 1") {
		t.Errorf("custom columns must follow built-ins, alphabetically:\n%s", v.Text())
	}
}

// TestBoardCardDrilldownExtras pins the three card facts a reader acts on that the
// board roll-up has no room for: the flow a flow-backed card runs, its worktree
// state, and an unreadable dependency list reported as such (NOT as "no deps",
// which is the opposite fact).
func TestBoardCardDrilldownExtras(t *testing.T) {
	now := time.Now()
	in := boardFixture(now)
	in.Tasks = append(in.Tasks, db.Task{
		ID: "T9", Title: "flow kart", BoardState: db.BoardInProgress, UpdatedAt: now.Unix(),
		FlowID: "FL2", WorktreeState: "conflict", WorktreeBranch: "task/T9",
		WorktreeLastError: "merge conflict in internal/view/board.go",
		Dependencies:      `{bozuk`,
	})
	in.Sub = "T9"

	v, err := ProjectBoard(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	for _, want := range []string{
		"akış: flow:FL2",
		"worktree: conflict · task/T9",
		"worktree hatası: merge conflict",
		"bağımlılık: (liste okunamadı)",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}

	// A card with none of these stays silent — the lines are earned, not padded.
	in.Sub = "T3"
	plain, err := ProjectBoard(in, LevelCard)
	if err != nil {
		t.Fatalf("project plain: %v", err)
	}
	if strings.Contains(plain.Text(), "worktree") || strings.Contains(plain.Text(), "akış:") {
		t.Errorf("plain card must not render empty extras:\n%s", plain.Text())
	}
}

// TestBoardFlagsReviewTreadmill: a card that has exhausted its verification
// round budget is a board-level signal, not just a coordinator-prompt one — the
// same condition surfaces wherever the board is read.
func TestBoardFlagsReviewTreadmill(t *testing.T) {
	now := time.Now()
	in := boardFixture(now)
	in.Tasks = append(in.Tasks,
		db.Task{ID: "T9", Title: "treadmill", BoardState: db.BoardInProgress,
			UpdatedAt: now.Unix(), ReviewBounces: db.ReviewRoundBudget},
		// Under budget: not a treadmill yet, must NOT be named.
		db.Task{ID: "T10", Title: "one bounce", BoardState: db.BoardInProgress,
			UpdatedAt: now.Unix(), ReviewBounces: 1},
		// Shipped despite the rounds: no longer on a treadmill.
		db.Task{ID: "T11", Title: "shipped late", BoardState: db.BoardDone,
			UpdatedAt: now.Unix(), ReviewBounces: db.ReviewRoundBudget + 2},
	)
	v, err := ProjectBoard(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	// Assert on the treadmill LINE, not the whole view: T10 legitimately appears
	// in the unrelated "changed in 24h" signal.
	var line string
	for _, ln := range strings.Split(txt, "\n") {
		if strings.Contains(ln, "doğrulama bütçesini doldurdu") {
			line = ln
			break
		}
	}
	if line == "" {
		t.Fatalf("board must flag the over-budget card:\n%s", txt)
	}
	if !strings.Contains(line, "T9") {
		t.Errorf("treadmill signal must name T9: %q", line)
	}
	if strings.Contains(line, "T10") || strings.Contains(line, "T11") {
		t.Errorf("treadmill signal must not name an under-budget or finished card: %q", line)
	}
}

// TestBoardCardDetailShowsReviewRounds: the drill-down reports rounds from the
// FIRST bounce — the reader deciding whether to open another round needs it
// before the budget is gone.
func TestBoardCardDetailShowsReviewRounds(t *testing.T) {
	now := time.Now()
	in := BoardInput{
		Now: now,
		Sub: "T1",
		Tasks: []db.Task{
			{ID: "T1", Title: "twice back", BoardState: db.BoardInProgress, UpdatedAt: now.Unix(), ReviewBounces: 2},
		},
	}
	v, err := ProjectBoard(in, LevelCard)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	txt := v.Text()
	if !strings.Contains(txt, "doğrulama turu: 2/3 başarısız") {
		t.Errorf("card detail must report the round count:\n%s", txt)
	}
	if strings.Contains(txt, "bütçe doldu") {
		t.Errorf("under-budget card must not claim the budget is spent:\n%s", txt)
	}
}
