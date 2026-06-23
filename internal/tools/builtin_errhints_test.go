package tools

import (
	"errors"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/orchestration"
)

// The exact unmarshal error SES2 produced when a parallel node used
// branches:[strings] must map to the parallel fan-out fix hint.
func TestGraphSchemaHint_ParallelMisuse(t *testing.T) {
	bad := `{"start":"research","nodes":[{"id":"research","type":"parallel","branches":["flight","hotel"],"next":"synthesize"}]}`
	_, err := orchestration.ParseGraph(bad)
	if err == nil {
		t.Fatal("expected parse error for branches-as-strings on a parallel node")
	}
	hint := graphSchemaHint(err.Error())
	if !strings.Contains(hint, `"parallel":["id1","id2"],"joinNext"`) {
		t.Fatalf("hint should steer to parallel/joinNext, got: %s", hint)
	}
}

func TestGraphSchemaHint_Fallback(t *testing.T) {
	if got := graphSchemaHint("some unrelated error"); !strings.Contains(got, "Node fields:") {
		t.Fatalf("fallback should include the field cheat-sheet, got: %s", got)
	}
}

func TestArgErr_HasActionableSuffix(t *testing.T) {
	err := argErr(errors.New("json: cannot unmarshal number into field .name of type string"))
	msg := err.Error()
	if !strings.Contains(msg, "invalid arguments") || !strings.Contains(msg, "input schema") {
		t.Fatalf("argErr should keep the cause and add a schema hint, got: %s", msg)
	}
}

func TestEnumErr_ListsAllowed(t *testing.T) {
	msg := enumErr("boardState", "doing", "todo", "in_progress", "done").Error()
	if !strings.Contains(msg, "doing") || !strings.Contains(msg, "in_progress") {
		t.Fatalf("enumErr should show the bad value and allowed set, got: %s", msg)
	}
}
