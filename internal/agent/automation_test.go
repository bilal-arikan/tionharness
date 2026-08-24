package agent

import (
	"context"
	"strings"
	"testing"

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
	ev := db.BoardChangeEvent{TaskID: "tsk_1", Title: "Ship it", Op: db.BoardOpMove, FromState: "todo", ToState: "done",
		Tags: []string{"urgent", "backend"}, Priority: "high"}
	vars := e.boardVars(context.Background(), a, ev)
	got := renderAutomationPrompt("[{{op}}] {{title}} {{from}}→{{to}} ({{toLabel}})", vars)
	want := "[move] Ship it todo→done (Bitti)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// tags / priority render; owner is empty (unassigned) with no db lookup.
	if v := renderAutomationPrompt("{{tags}}|{{priority}}|{{owner}}", vars); v != "urgent, backend|high|" {
		t.Fatalf("tags/priority/owner render = %q", v)
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
