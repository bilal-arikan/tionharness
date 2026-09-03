package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// bigToolResult builds a message carrying one oversized tool result, the shape
// the early prune targets.
func bigToolResult(callID string, size int) providers.Message {
	body := strings.Repeat("x", size)
	return providers.Message{
		Role:        providers.RoleUser,
		ToolResults: []providers.ToolResult{{CallID: callID, Content: body}},
	}
}

// earlyPruneTurn builds a turn whose in-flight history is well over the trigger
// ratio for a 200K-window model.
func earlyPruneTurn(t *testing.T) (*toolLoopTurn, *Runtime, db.Session) {
	t.Helper()
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agentRow, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Native", Provider: "anthropic", Model: "claude-sonnet-4-5-20250929",
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	// Size the fixture against the model's real window rather than a guess: the
	// trigger is a RATIO, so a hard-coded byte count silently stops exercising
	// the branch the day a family's window changes.
	window := providers.ContextWindowFor(agentRow.Provider, agentRow.Model)
	if window <= 0 {
		t.Fatalf("test model %q has no known context window", agentRow.Model)
	}
	// Dense (whitespace-free) content estimates at ~1.5 chars/token, so this many
	// runes per result puts four of them comfortably past earlyPruneRatio.
	each := int(float64(window)*earlyPruneRatio*1.5)/4 + 1024
	turn := &toolLoopTurn{
		r:          rt,
		ctx:        WithSessionID(ctx, session.ID),
		agent:      agentRow,
		provider:   providers.NewAnthropic("k"),
		keepRecent: 2,
		emit:       func(TurnStep) {},
		req: providers.Request{Messages: []providers.Message{
			bigToolResult("a", each),
			bigToolResult("b", each),
			bigToolResult("c", each),
			bigToolResult("d", each),
			{Role: providers.RoleAssistant, Text: "still working"},
			{Role: providers.RoleUser, Text: "carry on"},
		}},
	}
	return turn, rt, session
}

func TestEarlyPruneFiresBeforeOverflow(t *testing.T) {
	turn, rt, session := earlyPruneTurn(t)
	before := len(turn.req.Messages)

	turn.maybeEarlyPrune(earlyPruneMinIter)

	if !turn.earlyPruned {
		t.Fatal("early prune did not run on an over-ratio history")
	}
	if len(turn.req.Messages) != before {
		t.Errorf("messages = %d, want %d — a prune must not drop messages", len(turn.req.Messages), before)
	}
	// Bodies replaced, pairing intact.
	pruned := 0
	for _, m := range turn.req.Messages {
		for _, tr := range m.ToolResults {
			if tr.CallID == "" {
				t.Error("CallID lost; tool_use↔tool_result pairing would break")
			}
			if strings.HasPrefix(tr.Content, "[tool result pruned") {
				pruned++
			}
		}
	}
	if pruned == 0 {
		t.Fatal("no tool result body was replaced")
	}
	// The kept tail must be untouched: keepRecent=2 protects the last two.
	if last := turn.req.Messages[len(turn.req.Messages)-1]; last.Text != "carry on" {
		t.Errorf("tail rewritten: %+v", last)
	}
	// Visible: one compaction card and one journal entry tagged early_prune.
	if len(turn.steps) != 1 || turn.steps[0].Kind != StepCompaction {
		t.Fatalf("steps = %+v, want one compaction card", turn.steps)
	}
	// The journal redacts Detail and fingerprints Name, so assert on what
	// survives: exactly one compaction event carrying the bytes this prune saved.
	events, err := rt.db.ReadDebugEvents(context.Background(), session.ID, db.DebugCompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("journal = %+v, want exactly one compaction event", events)
	}
	if events[0].SavedBytes <= 0 {
		t.Errorf("SavedBytes = %d, want the dropped tool output to be recorded", events[0].SavedBytes)
	}
	if events[0].SummaryBytes != 0 {
		t.Errorf("SummaryBytes = %d, want 0 — a prune runs no summarizer", events[0].SummaryBytes)
	}
}

// The reason tag rides the event Detail, which the journal redacts. Assert the
// builder itself so the early/recovery distinction stays covered.
func TestEarlyPruneEventNamesItsReason(t *testing.T) {
	ev := prunedToolResultsEvent("agent-1", reasonEarlyPrune, conversation.PruneStat{
		Pruned: 2, BeforeTokens: 100, AfterTokens: 40, SavedBytes: 4096,
	})
	if !strings.Contains(ev.Detail, reasonEarlyPrune) {
		t.Errorf("Detail = %q, want it to name %q", ev.Detail, reasonEarlyPrune)
	}
	if ev.Name != conversation.TriggerPrune {
		t.Errorf("Name = %q, want %q", ev.Name, conversation.TriggerPrune)
	}
}

func TestEarlyPruneIsOneShotPerTurn(t *testing.T) {
	turn, _, _ := earlyPruneTurn(t)
	turn.maybeEarlyPrune(earlyPruneMinIter)
	first := len(turn.steps)
	// Re-arm the history; the pass must still refuse to run again.
	turn.req.Messages = append(turn.req.Messages, bigToolResult("e", 200_000), bigToolResult("f", 200_000))
	turn.maybeEarlyPrune(earlyPruneMinIter + 5)
	if len(turn.steps) != first {
		t.Fatalf("steps = %d, want %d — the prune must run at most once per turn", len(turn.steps), first)
	}
}

func TestEarlyPruneSkipsEarlyIterationsAndSmallHistories(t *testing.T) {
	turn, _, _ := earlyPruneTurn(t)
	turn.maybeEarlyPrune(earlyPruneMinIter - 1)
	if turn.earlyPruned || len(turn.steps) != 0 {
		t.Fatal("a turn below earlyPruneMinIter must not be pruned")
	}

	small, _, _ := earlyPruneTurn(t)
	small.req.Messages = []providers.Message{
		bigToolResult("a", 5_000),
		{Role: providers.RoleUser, Text: "carry on"},
	}
	small.maybeEarlyPrune(earlyPruneMinIter)
	if len(small.steps) != 0 {
		t.Fatalf("an under-ratio history must not be pruned: %+v", small.steps)
	}
	for _, tr := range small.req.Messages[0].ToolResults {
		if strings.HasPrefix(tr.Content, "[tool result pruned") {
			t.Error("body replaced despite being under the ratio")
		}
	}
}

// A CLI provider owns its own transcript, so pruning our delta cannot shrink
// what actually fills the window — the same exemption pruneAndRetry makes.
func TestEarlyPruneSkipsCLIProviders(t *testing.T) {
	turn, _, _ := earlyPruneTurn(t)
	turn.provider = providers.NewCodexCLI("codex", "gpt-5", filepath.Join(t.TempDir(), "codex-home"))
	turn.maybeEarlyPrune(earlyPruneMinIter)
	if turn.earlyPruned || len(turn.steps) != 0 {
		t.Fatal("a CLI-backed turn must not be pruned here")
	}
}
