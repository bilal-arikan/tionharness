package view

import (
	"strings"
	"testing"
)

func TestCapLinesKeepsWholeRecords(t *testing.T) {
	lines := []string{"aaaa", "bbbb", "cccc"} // 5 bytes each with the newline
	kept, dropped := CapLines(lines, 12)

	// Two fit (10 bytes); the third would overflow and must be dropped ENTIRELY
	// rather than half-written — a truncated record still reads like a complete
	// one, which is the failure this helper exists to prevent.
	if len(kept) != 2 || dropped != 1 {
		t.Fatalf("kept=%v dropped=%d; want 2 kept, 1 dropped", kept, dropped)
	}
	for _, k := range kept {
		if len(k) != 4 {
			t.Errorf("a kept line was truncated: %q", k)
		}
	}
}

func TestCapLinesEverythingFits(t *testing.T) {
	lines := []string{"a", "b"}
	kept, dropped := CapLines(lines, 1000)
	if len(kept) != 2 || dropped != 0 {
		t.Errorf("kept=%v dropped=%d; want everything kept", kept, dropped)
	}
}

func TestCapLinesNoBudgetDropsAll(t *testing.T) {
	// A caller with no room left must be told it lost everything, not handed a
	// silently empty slice that reads as "there was nothing".
	kept, dropped := CapLines([]string{"a", "b", "c"}, 0)
	if len(kept) != 0 || dropped != 3 {
		t.Errorf("kept=%v dropped=%d; want 0 kept, 3 dropped", kept, dropped)
	}
	if kept, dropped = CapLines([]string{"a"}, -5); dropped != 1 {
		t.Errorf("negative budget: kept=%v dropped=%d; want 1 dropped", kept, dropped)
	}
}

func TestCapLinesEmptyInput(t *testing.T) {
	kept, dropped := CapLines(nil, 100)
	if len(kept) != 0 || dropped != 0 {
		t.Errorf("kept=%v dropped=%d; want both zero", kept, dropped)
	}
}

// TestDecodeStepsIsTheSingleHome pins the shared decoder both view and insight
// now use — the duplicate copies each package had grown are gone.
func TestDecodeStepsIsTheSingleHome(t *testing.T) {
	raw := `[
	  {"kind":"tool","tool":"Bash","isError":true,"output":"exit 1"},
	  {"kind":"todo","todos":[{"content":"a","status":"completed"}]},
	  {"kind":"error","reason":"provider_error","text":"429"}
	]`
	steps := DecodeSteps(raw)
	if len(steps) != 3 {
		t.Fatalf("decoded %d steps, want 3", len(steps))
	}
	if !steps[0].IsError || steps[0].Tool != "Bash" || steps[0].Output != "exit 1" {
		t.Errorf("tool step decoded wrong: %+v", steps[0])
	}
	if todos := steps[1].TodoItems(); len(todos) != 1 || todos[0].Status != "completed" {
		t.Errorf("todo step decoded wrong: %+v", todos)
	}
	if steps[2].Reason != "provider_error" {
		t.Errorf("error step decoded wrong: %+v", steps[2])
	}
}

func TestDecodeStepsToleratesGarbage(t *testing.T) {
	// One corrupt message must not take down a whole summary, so a bad trace
	// yields no steps rather than an error.
	for _, raw := range []string{"", "[]", "not json", `{"kind":"tool"}`} {
		if got := DecodeSteps(raw); got != nil {
			t.Errorf("DecodeSteps(%q) = %v; want nil", raw, got)
		}
	}
}

func TestTodoItemsReadsLegacyToolInput(t *testing.T) {
	// Older traces carried the checklist in the todo_write tool input rather
	// than the step's own field.
	s := Step{Kind: "tool", Tool: "todo_write",
		Input: []byte(`{"todos":[{"content":"eski","status":"pending"}]}`)}
	todos := s.TodoItems()
	if len(todos) != 1 || todos[0].Content != "eski" {
		t.Errorf("legacy todo input not read: %+v", todos)
	}
	// A non-todo step yields nothing even if it happens to carry input.
	if got := (Step{Kind: "tool", Tool: "Bash", Input: []byte(`{"todos":[{}]}`)}).TodoItems(); got != nil {
		t.Errorf("non-todo step returned todos: %+v", got)
	}
}

// TestCapLinesJoinAccounting guards the off-by-one that would make a caller
// exceed its real budget: the helper must account for the newline it knows the
// caller will add.
func TestCapLinesJoinAccounting(t *testing.T) {
	lines := []string{"12345", "12345"}
	kept, _ := CapLines(lines, 11) // 6 + 6 = 12 > 11 → only one fits
	if len(kept) != 1 {
		t.Fatalf("kept %d lines, want 1", len(kept))
	}
	if joined := strings.Join(kept, "\n") + "\n"; len(joined) > 11 {
		t.Errorf("joined output %d bytes exceeds the 11-byte budget", len(joined))
	}
}
