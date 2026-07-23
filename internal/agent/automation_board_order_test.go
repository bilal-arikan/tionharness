package agent

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// boardRule is a shorthand for an enabled board automation watching move→to.
func boardRule(id, to string, priority int, exclusive bool) db.Automation {
	return db.Automation{
		ID:             id,
		TriggerKind:    db.TriggerBoard,
		BoardOp:        db.BoardOpMove,
		BoardToState:   to,
		BoardPriority:  priority,
		BoardExclusive: exclusive,
		Enabled:        true,
	}
}

func ids(autos []db.Automation) []string {
	out := make([]string, 0, len(autos))
	for _, a := range autos {
		out = append(out, a.ID)
	}
	return out
}

func equalIDs(got []db.Automation, want ...string) bool {
	g := ids(got)
	if len(g) != len(want) {
		return false
	}
	for i := range g {
		if g[i] != want[i] {
			return false
		}
	}
	return true
}

func TestSelectBoardAutomationsOrdersByPriority(t *testing.T) {
	ev := db.BoardChangeEvent{Op: db.BoardOpMove, FromState: "pbi", ToState: "todo"}
	// Deliberately supplied out of order: the store returns newest-first, which is
	// unrelated to the intended fire order.
	autos := []db.Automation{
		boardRule("AUT4", "todo", 20, false), // planner runs second
		boardRule("AUT7", "todo", 10, false), // classifier runs first
	}
	got := selectBoardAutomations(autos, ev)
	if !equalIDs(got, "AUT7", "AUT4") {
		t.Fatalf("fire order = %v, want [AUT7 AUT4]", ids(got))
	}
}

func TestSelectBoardAutomationsTieBreaksOnID(t *testing.T) {
	ev := db.BoardChangeEvent{Op: db.BoardOpMove, ToState: "todo"}
	// Equal priority (the default 0) must still produce a stable order, otherwise
	// two rules on one column fire in map-iteration order and race.
	autos := []db.Automation{
		boardRule("AUT9", "todo", 0, false),
		boardRule("AUT2", "todo", 0, false),
		boardRule("AUT5", "todo", 0, false),
	}
	got := selectBoardAutomations(autos, ev)
	if !equalIDs(got, "AUT2", "AUT5", "AUT9") {
		t.Fatalf("tie-break order = %v, want [AUT2 AUT5 AUT9]", ids(got))
	}
}

func TestSelectBoardAutomationsExclusiveSuppressesOthers(t *testing.T) {
	ev := db.BoardChangeEvent{Op: db.BoardOpMove, ToState: "todo"}
	// The classifier claims the column: the planner must not fire on the same
	// event (this is the double-trigger the card reports).
	autos := []db.Automation{
		boardRule("AUT4", "todo", 20, false),
		boardRule("AUT7", "todo", 10, true),
	}
	got := selectBoardAutomations(autos, ev)
	if !equalIDs(got, "AUT7") {
		t.Fatalf("exclusive selection = %v, want [AUT7]", ids(got))
	}
}

func TestSelectBoardAutomationsExclusiveLowestPriorityWins(t *testing.T) {
	ev := db.BoardChangeEvent{Op: db.BoardOpMove, ToState: "todo"}
	// Two exclusive claims on one column: the lower priority wins, and it is the
	// only one returned.
	autos := []db.Automation{
		boardRule("AUT4", "todo", 5, true),
		boardRule("AUT7", "todo", 1, true),
	}
	got := selectBoardAutomations(autos, ev)
	if !equalIDs(got, "AUT7") {
		t.Fatalf("exclusive winner = %v, want [AUT7]", ids(got))
	}
}

func TestSelectBoardAutomationsExclusiveOnlyAffectsItsOwnEvent(t *testing.T) {
	// An exclusive rule on todo must not suppress a rule watching another column.
	autos := []db.Automation{
		boardRule("AUT7", "todo", 0, true),
		boardRule("AUT5", "in_progress", 0, false),
	}
	got := selectBoardAutomations(autos, db.BoardChangeEvent{Op: db.BoardOpMove, ToState: "in_progress"})
	if !equalIDs(got, "AUT5") {
		t.Fatalf("cross-column selection = %v, want [AUT5]", ids(got))
	}
}

func TestSelectBoardAutomationsSkipsNonBoardAndNonMatching(t *testing.T) {
	ev := db.BoardChangeEvent{Op: db.BoardOpMove, ToState: "todo"}
	autos := []db.Automation{
		{ID: "AUT1", TriggerKind: db.TriggerTag, TriggerTag: "loop", Enabled: true}, // tag kind
		boardRule("AUT3", "done", 0, false),                                         // different column
		boardRule("AUT7", "todo", 0, false),                                         // the only match
	}
	got := selectBoardAutomations(autos, ev)
	if !equalIDs(got, "AUT7") {
		t.Fatalf("filtered selection = %v, want [AUT7]", ids(got))
	}
}

func TestSelectBoardAutomationsNoMatch(t *testing.T) {
	got := selectBoardAutomations(
		[]db.Automation{boardRule("AUT7", "todo", 0, false)},
		db.BoardChangeEvent{Op: db.BoardOpMove, ToState: "review"},
	)
	if len(got) != 0 {
		t.Fatalf("expected no matches, got %v", ids(got))
	}
	if got := selectBoardAutomations(nil, db.BoardChangeEvent{Op: db.BoardOpMove, ToState: "todo"}); got != nil {
		t.Fatalf("nil input should return nil, got %v", ids(got))
	}
}
