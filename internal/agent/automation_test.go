package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
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
	e := &AutomationEngine{}
	a := db.Automation{TriggerKind: db.TriggerBoard, MaxIterations: 5}
	ev := db.BoardChangeEvent{TaskID: "tsk_1", Title: "Ship it", Op: db.BoardOpMove, FromState: "todo", ToState: "done"}
	got := renderAutomationPrompt("[{{op}}] {{title}} {{from}}→{{to}} ({{toLabel}})", e.boardVars(a, ev))
	want := "[move] Ship it todo→done (Bitti)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
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
