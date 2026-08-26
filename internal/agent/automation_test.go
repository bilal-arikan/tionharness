package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestRenderAutomationPrompt(t *testing.T) {
	vars := map[string]string{
		"result": "DONE", "title": "My loop", "sessionId": "SES3", "tag": "loop",
		"iteration": "2", "agent": "Bob",
	}

	// All placeholders substituted (including the extended set).
	got := renderAutomationPrompt("[{{tag}}] {{title}} ({{sessionId}}) #{{iteration}} by {{agent}}: {{result}}", vars)
	want := "[loop] My loop (SES3) #2 by Bob: DONE"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	// A template without {{result}} still carries the result forward (appended).
	got = renderAutomationPrompt("Keep going.", map[string]string{"result": "the result"})
	if !strings.Contains(got, "the result") || !strings.HasPrefix(got, "Keep going.") {
		t.Fatalf("result not appended: %q", got)
	}

	// Empty result + no placeholder → template unchanged (nothing appended).
	got = renderAutomationPrompt("Just do X.", map[string]string{"result": ""})
	if got != "Just do X." {
		t.Fatalf("unexpected mutation: %q", got)
	}
}

func TestFireBoardMoveMovesTask(t *testing.T) {
	e := backstopEngine(t)
	ctx := context.Background()
	task, err := e.db.CreateTask(ctx, db.Task{Title: "Move me", BoardState: db.BoardTodo})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	a := seedAutomation(t, e, db.Automation{
		TriggerKind:      db.TriggerBoard,
		BoardAction:      db.BoardActionMove,
		BoardMoveToState: db.BoardReview,
		MaxIterations:    3,
	})

	e.fireBoard(ctx, a, db.BoardChangeEvent{TaskID: task.ID, Title: task.Title, Op: db.BoardOpCreate, ToState: db.BoardTodo})

	got, err := e.db.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.BoardState != db.BoardReview {
		t.Fatalf("board state = %q, want %q", got.BoardState, db.BoardReview)
	}
	fired, err := e.db.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatalf("get automation: %v", err)
	}
	if fired.IterationCount != 1 {
		t.Fatalf("iteration count = %d, want 1", fired.IterationCount)
	}
}

func TestFireBoardArchiveRejectsStaleDoneEvent(t *testing.T) {
	e := backstopEngine(t)
	ctx := context.Background()
	task, err := e.db.CreateTask(ctx, db.Task{Title: "Review me", BoardState: db.BoardReview})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	a := seedAutomation(t, e, db.Automation{
		TriggerKind:   db.TriggerBoard,
		BoardAction:   db.BoardActionArchive,
		MaxIterations: 3,
	})

	// Simulate a queued done event that arrives after the card has already moved
	// to review. Archive decisions must use current persisted state, not stale event data.
	e.fireBoard(ctx, a, db.BoardChangeEvent{TaskID: task.ID, Title: task.Title, Op: db.BoardOpMove, ToState: db.BoardDone})

	got, err := e.db.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Archived {
		t.Fatal("review task was archived by stale done event")
	}
}

func TestBoardMoveReentrancyStopsAtMaxIterations(t *testing.T) {
	e := backstopEngine(t)
	ctx := context.Background()
	task, err := e.db.CreateTask(ctx, db.Task{Title: "Bounded chain", BoardState: db.BoardTodo})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	toReview := seedAutomation(t, e, db.Automation{
		TriggerKind: db.TriggerBoard, BoardOp: db.BoardOpMove, BoardToState: db.BoardTodo,
		BoardAction: db.BoardActionMove, BoardMoveToState: db.BoardReview, MaxIterations: 1,
	})
	toTodo := seedAutomation(t, e, db.Automation{
		TriggerKind: db.TriggerBoard, BoardOp: db.BoardOpMove, BoardToState: db.BoardReview,
		BoardAction: db.BoardActionMove, BoardMoveToState: db.BoardTodo, MaxIterations: 1,
	})
	e.db.SetBoardHook(func(ev db.BoardChangeEvent) {
		go e.OnBoardChange(context.Background(), ev)
	})

	if err := e.db.MoveTask(ctx, task.ID, db.BoardReview); err != nil {
		t.Fatalf("initial move: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		a, _ := e.db.GetAutomation(ctx, toReview.ID)
		b, _ := e.db.GetAutomation(ctx, toTodo.ID)
		if a.IterationCount == 1 && b.IterationCount == 1 {
			time.Sleep(50 * time.Millisecond)
			final, err := e.db.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatalf("get task: %v", err)
			}
			a, _ = e.db.GetAutomation(ctx, toReview.ID)
			b, _ = e.db.GetAutomation(ctx, toTodo.ID)
			if a.IterationCount != 1 || b.IterationCount != 1 {
				t.Fatalf("reentrant chain exceeded maxIterations: todo=%d review=%d", a.IterationCount, b.IterationCount)
			}
			if final.BoardState != db.BoardReview {
				t.Fatalf("final board state = %q, want %q", final.BoardState, db.BoardReview)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("reentrant move chain did not complete")
}

func TestBoardMatches(t *testing.T) {
	move := db.BoardChangeEvent{Op: db.BoardOpMove, FromState: "todo", ToState: "in_progress"}
	create := db.BoardChangeEvent{Op: db.BoardOpCreate, ToState: "todo"}

	cases := []struct {
		name string
		a    db.Automation
		ev   db.BoardChangeEvent
		want bool
	}{
		{"empty op defaults to move", db.Automation{TriggerKind: db.TriggerBoard}, move, true},
		{"empty op does not match create", db.Automation{TriggerKind: db.TriggerBoard}, create, false},
		{"any matches create", db.Automation{TriggerKind: db.TriggerBoard, BoardOp: db.BoardOpAny}, create, true},
		{"op filter mismatch", db.Automation{TriggerKind: db.TriggerBoard, BoardOp: db.BoardOpCreate}, move, false},
		{"to filter match", db.Automation{TriggerKind: db.TriggerBoard, BoardOp: db.BoardOpMove, BoardToState: "in_progress"}, move, true},
		{"to filter mismatch", db.Automation{TriggerKind: db.TriggerBoard, BoardOp: db.BoardOpMove, BoardToState: "done"}, move, false},
		{"from filter match", db.Automation{TriggerKind: db.TriggerBoard, BoardOp: db.BoardOpMove, BoardFromState: "todo"}, move, true},
		{"from filter mismatch", db.Automation{TriggerKind: db.TriggerBoard, BoardOp: db.BoardOpMove, BoardFromState: "review"}, move, false},
	}
	for _, c := range cases {
		if got := boardMatches(c.a, c.ev); got != c.want {
			t.Errorf("%s: boardMatches = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBoardVarsSubstitution(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	ctx := context.Background()
	owner, err := database.CreateAgent(ctx, db.Agent{Name: "Alice", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	e := &AutomationEngine{db: database}
	a := db.Automation{TriggerKind: db.TriggerBoard, MaxIterations: 5}
	tests := []struct {
		name string
		ev   db.BoardChangeEvent
		want string
	}{
		{
			name: "populated optional fields",
			ev: db.BoardChangeEvent{
				TaskID: "tsk_1", Title: "Ship it", Op: db.BoardOpMove, FromState: "todo", ToState: "done",
				Tags: []string{"urgent", "backend"}, Priority: "high", OwnerAgentID: owner.ID,
			},
			want: "Card tsk_1 Ship it todo→done (priority: high, owner: Alice, tags: urgent, backend)",
		},
		{
			name: "empty optional fields",
			ev: db.BoardChangeEvent{
				TaskID: "tsk_2", Title: "Triage", Op: db.BoardOpMove, FromState: "todo", ToState: "failed",
			},
			want: "Card tsk_2 Triage todo→failed (priority: unset, owner: unassigned, tags: none)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := e.boardVars(ctx, a, tt.ev)
			got := renderAutomationPrompt("Card {{taskId}} {{title}} {{from}}→{{to}} (priority: {{priority}}, owner: {{owner}}, tags: {{tags}})", vars)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestCrossedMultiple(t *testing.T) {
	cases := []struct {
		name                string
		prev, now, interval int64
		want                bool
	}{
		{"crosses 100k boundary", 90_000, 110_000, 100_000, true},
		{"stays below boundary", 10_000, 90_000, 100_000, false},
		{"stays above without new multiple", 110_000, 150_000, 100_000, false},
		{"exact boundary hit", 90_000, 100_000, 100_000, true},
		{"just past a prior boundary", 100_000, 100_050, 100_000, false},
		{"crosses second boundary", 190_000, 210_000, 100_000, true},
		{"no movement", 100_000, 100_000, 100_000, false},
		{"first crossing from zero", 0, 1_000, 1_000, true},
		{"zero interval never crosses", 0, 500_000, 0, false},
		{"negative prev clamps to zero", -5, 1_000, 1_000, true},
	}
	for _, c := range cases {
		if got := crossedMultiple(c.prev, c.now, c.interval); got != c.want {
			t.Errorf("%s: crossedMultiple(%d,%d,%d) = %v, want %v", c.name, c.prev, c.now, c.interval, got, c.want)
		}
	}
}

func TestTokenVarsSubstitution(t *testing.T) {
	e := &AutomationEngine{}
	a := db.Automation{TriggerKind: db.TriggerToken, TokenScope: db.TokenScopeSession, TokenThreshold: 100_000, MaxIterations: 5}
	vars := e.tokenVars(a, "SES7", 200_000)
	got := renderAutomationPrompt("{{scope}} {{sessionId}} {{tokens}}/{{threshold}} #{{iteration}}", vars)
	want := "session SES7 200000/100000 #1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// Workspace scope defaults an empty TokenScope to "session" only when unset;
	// here an explicit workspace scope renders with an empty sessionId.
	aw := db.Automation{TriggerKind: db.TriggerToken, TokenScope: db.TokenScopeWorkspace, TokenThreshold: 50_000}
	if v := renderAutomationPrompt("{{scope}}|{{sessionId}}", e.tokenVars(aw, "", 50_000)); v != "workspace|" {
		t.Fatalf("workspace vars render = %q", v)
	}
}

func TestContainsTag(t *testing.T) {
	if !containsTag([]string{"a", "loop", "b"}, "loop") {
		t.Fatal("should find loop")
	}
	if containsTag([]string{"a", "b"}, "loop") {
		t.Fatal("should not find loop")
	}
	if containsTag(nil, "loop") {
		t.Fatal("nil should not match")
	}
}
