package db

import (
	"testing"
)

func TestCurrentAppendTargetMatchesReconciledHistory(t *testing.T) {
	msgs := []Message{{ID: "one", Role: "assistant", Steps: `[{"kind":"tool"}]`, CreatedAt: 10, AuthorKind: AuthorAgent, AuthorID: "A1"}}
	s := reconcileHeader(Session{ID: "S", Kind: "chat"}, msgs)
	m := Message{ID: "two", Role: "assistant", Steps: `[{"kind":"tool"},{"kind":"tool"}]`, CreatedAt: 20, AuthorKind: AuthorAgent, AuthorID: "A2"}
	target, added, err := currentAppendTarget(s, msgs, m)
	expected := reconcileHeader(s, append(msgs, m))
	if err != nil || !added || target.MessageCount != expected.MessageCount || target.ToolCallCount != expected.ToolCallCount || len(target.Participants) != 2 {
		t.Fatalf("incremental counters: %+v %v", target, err)
	}
	if duplicate, added, err := currentAppendTarget(target, append(msgs, m), m); err != nil || added || duplicate.MessageCount != 2 {
		t.Fatalf("duplicate append: %+v %v %v", duplicate, added, err)
	}
}
