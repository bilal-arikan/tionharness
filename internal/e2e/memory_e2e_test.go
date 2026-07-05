package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/memory"
)

// TestMemory_CoreMemoryPersistsAndReinjects covers the MemGPT-style core memory:
// the agent writes a durable fact about the user via core_memory_replace in one
// turn, it survives to disk, and it is re-injected verbatim into the NEXT turn's
// system context — so the agent "remembers" without it being in the chat history.
func TestMemory_CoreMemoryPersistsAndReinjects(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Noting that down.", tc("c1", "core_memory_replace", map[string]any{
			"section": "human",
			"content": "User is Bilal; lives in Turkey; prefers Turkish.",
		})),
		sayText("Got it, I'll remember."),
		sayText("You are Bilal."),
	)
	h := newHarness(t, prov)
	ag := h.newAgent("Remy")
	sess := h.newSession(ag)

	// Turn 1: the model records a fact into core memory.
	h.send(ag, sess, "Remember that I'm Bilal from Turkey.")

	// It is durably stored in the human section.
	human, err := h.rt.Memory().ReadCore(context.Background(), ag.ID, memory.CoreHuman)
	if err != nil {
		t.Fatalf("read core human: %v", err)
	}
	if !strings.Contains(human, "Bilal") {
		t.Fatalf("core human memory = %q, want it to mention Bilal", human)
	}

	// Turn 2: a fresh question. The composed request must carry the core memory in
	// its dynamic system suffix — re-injected every turn, not pulled from history.
	h.send(ag, sess, "Who am I?")
	if !strings.Contains(prov.lastReq.SystemDynamic, "Bilal") {
		t.Fatalf("core memory not re-injected into turn-2 system context:\n%s", prov.lastReq.SystemDynamic)
	}
}

// TestMemory_RecallTool seeds a durable fact in long-term memory, then a turn in
// which the model calls memory_recall and gets the fact back via similarity search.
func TestMemory_RecallTool(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Searching memory.", tc("c1", "memory_recall", map[string]any{
			"query": "launch code",
		})),
		sayText("The launch code is THUNDERBIRD."),
	)
	h := newHarness(t, prov)
	ag := h.newAgent("Archivist")
	sess := h.newSession(ag)

	// Seed a long-term memory directly (as a prior turn / reflection would have).
	if _, err := h.rt.Memory().Remember(context.Background(), ag.ID, db.MemoryDocument,
		"The launch code is THUNDERBIRD and must never be shared."); err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	res := h.send(ag, sess, "What's the launch code?")

	step := findToolStep(res.steps, "memory_recall")
	if step == nil {
		t.Fatalf("no memory_recall step in trace: %+v", res.steps)
	}
	if !strings.Contains(step.Output, "THUNDERBIRD") {
		t.Errorf("memory_recall output = %q, want it to surface the seeded fact", step.Output)
	}
}
