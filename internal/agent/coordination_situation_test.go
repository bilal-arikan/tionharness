package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestCoordinatorAgentRosterBlockTagsCapability: the roster must say, per agent,
// whether it can write — that label is what stops a file-writing brief from
// being handed to a read-only worker in the first place.
func TestCoordinatorAgentRosterBlockTagsCapability(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Worker: Planner", AllowedTools: `["Read","Glob","Grep"]`}); err != nil {
		t.Fatalf("create planner: %v", err)
	}
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Worker: Coder", AllowedTools: `["Read","Write","Edit","Bash"]`}); err != nil {
		t.Fatalf("create coder: %v", err)
	}

	block := rt.coordinatorAgentRosterBlock(ctx)
	if block == "" {
		t.Fatal("roster block must render when agents exist")
	}
	for _, want := range []string{"Worker: Planner", "READ-ONLY", "Worker: Coder", "read+write"} {
		if !strings.Contains(block, want) {
			t.Errorf("roster block missing %q:\n%s", want, block)
		}
	}
}

// TestCoordinatorSituationBlockSkipsNonCoordinator keeps the block off ordinary
// chat turns — it is only worth its tokens where it replaces tool calls.
func TestCoordinatorSituationBlockSkipsNonCoordinator(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	if got := rt.CoordinatorSituationBlock(context.Background(), db.Session{ID: "SES1"}); got != "" {
		t.Fatalf("non-coordinator session must get no situation block, got %q", got)
	}
}

// TestCoordinatorSituationFreshnessNote pins the sentence that does the work: it
// must name the three tools it replaces AND leave the drill-down path open.
func TestCoordinatorSituationFreshnessNote(t *testing.T) {
	for _, want := range []string{"list_workers", "list_agents", "get_view", "AUTHORITATIVE"} {
		if !strings.Contains(coordinatorSituationFreshnessNote, want) {
			t.Errorf("freshness note must mention %q:\n%s", want, coordinatorSituationFreshnessNote)
		}
	}
}

// TestReviewGateBlockCountsRounds: the gate must stay silent under budget and
// fire — naming the card and forbidding another review round — at the budget.
func TestReviewGateBlockCountsRounds(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	task, err := rt.db.CreateTask(ctx, db.Task{Title: "node konumu kalıcılığı", BoardState: db.BoardInProgress})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	for i := 1; i < db.ReviewRoundBudget; i++ {
		if err := rt.db.MoveTask(ctx, task.ID, db.BoardReview); err != nil {
			t.Fatalf("move to review: %v", err)
		}
		if err := rt.db.MoveTask(ctx, task.ID, db.BoardInProgress); err != nil {
			t.Fatalf("move back: %v", err)
		}
	}
	if got := rt.reviewGateBlock(ctx); got != "" {
		t.Fatalf("gate must stay silent below budget, got:\n%s", got)
	}
	if err := rt.db.MoveTask(ctx, task.ID, db.BoardReview); err != nil {
		t.Fatalf("move to review: %v", err)
	}
	if err := rt.db.MoveTask(ctx, task.ID, db.BoardInProgress); err != nil {
		t.Fatalf("move back: %v", err)
	}
	block := rt.reviewGateBlock(ctx)
	if block == "" {
		t.Fatal("gate must fire once the round budget is exhausted")
	}
	for _, want := range []string{task.ID, "NOT the next action", "Ask the user"} {
		if !strings.Contains(block, want) {
			t.Errorf("gate block missing %q:\n%s", want, block)
		}
	}
}

// TestReviewBouncesIgnoresPassingMoves: review→done is a PASS, not a round, and
// must not count against the budget.
func TestReviewBouncesIgnoresPassingMoves(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	task, err := rt.db.CreateTask(ctx, db.Task{Title: "done path", BoardState: db.BoardInProgress})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := rt.db.MoveTask(ctx, task.ID, db.BoardReview); err != nil {
		t.Fatalf("move to review: %v", err)
	}
	if err := rt.db.MoveTask(ctx, task.ID, db.BoardDone); err != nil {
		t.Fatalf("move to done: %v", err)
	}
	got, err := rt.db.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.ReviewBounces != 0 {
		t.Fatalf("review→done counted as a failed round (bounces=%d)", got.ReviewBounces)
	}
}

// TestCoordinatorBlocksExcludeArchivedCards: archived cards are finished work the
// user already cleared off the board. Reading them made the coordinator's board
// block report a 270-card board where the UI showed 117, and would have let an
// archived card raise the review gate.
func TestCoordinatorBlocksExcludeArchivedCards(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	if _, err := rt.db.CreateTask(ctx, db.Task{Title: "canlı kart", BoardState: db.BoardInProgress}); err != nil {
		t.Fatalf("create active: %v", err)
	}
	archived, err := rt.db.CreateTask(ctx, db.Task{Title: "arşivlenmiş kart", BoardState: db.BoardInProgress})
	if err != nil {
		t.Fatalf("create archived: %v", err)
	}
	// Push the archived card past the review budget BEFORE archiving it, so the
	// only reason it must not appear is the archive flag.
	for i := 0; i < db.ReviewRoundBudget; i++ {
		if err := rt.db.MoveTask(ctx, archived.ID, db.BoardReview); err != nil {
			t.Fatalf("move to review: %v", err)
		}
		if err := rt.db.MoveTask(ctx, archived.ID, db.BoardInProgress); err != nil {
			t.Fatalf("move back: %v", err)
		}
	}
	if err := rt.db.SetTaskArchived(ctx, archived.ID, true); err != nil {
		t.Fatalf("archive: %v", err)
	}

	board := rt.coordinatorBoardBlock(ctx)
	if !strings.Contains(board, "1 kart") {
		t.Errorf("board block must count only the active card:\n%s", board)
	}
	if gate := rt.reviewGateBlock(ctx); gate != "" {
		t.Errorf("an archived card must not raise the review gate:\n%s", gate)
	}
}
