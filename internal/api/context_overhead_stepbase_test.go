package api

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// stepBaseStubProvider is the smallest provider that can serve a rolling fold:
// Prepare only ever calls Complete on it, to produce the new summary.
type stepBaseStubProvider struct{ summary string }

func (p stepBaseStubProvider) Name() string { return "stub" }
func (p stepBaseStubProvider) Complete(_ context.Context, _ providers.Request) (*providers.Response, error) {
	return &providers.Response{Text: p.summary, StopReason: providers.StopEndTurn}, nil
}

// stepBaseFixture builds a warm-CLI-shaped workspace: a session whose transcript
// carries a persisted tool trace on every assistant message.
func stepBaseFixture(t *testing.T, msgs int, trace string) (*Server, *workspace.Workspace, db.Agent, db.Session, []db.Message) {
	t.Helper()
	ctx := context.Background()
	s, wsp := newWorkspaceServer(t)

	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := wsp.DB.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	history := make([]db.Message, 0, msgs)
	for i := 0; i < msgs/2; i++ {
		history = append(history,
			db.Message{Role: providers.RoleUser, Text: strings.Repeat("question ", 40)},
			db.Message{Role: providers.RoleAssistant, Text: strings.Repeat("answer ", 40), Steps: trace})
	}

	return s, wsp, agentRow, sess, history
}

// TestContextOverheadTokensReturnsWarmStepBaseline pins the FIRST link of the
// chain the three turn call sites depend on: contextOverheadTokens must report
// the index its persisted-Steps term was charged from — max(SummaryMsgCount,
// CLICompactMsgCount) on a warm thread, -1 on a cold one — and the term itself
// must cover exactly history[stepBase:]. Without this the returned value could
// drift from the charged range and every downstream deduction would be wrong.
func TestContextOverheadTokensReturnsWarmStepBaseline(t *testing.T) {
	ctx := context.Background()
	trace := `[{"kind":"tool","tool":"Bash","input":{"cmd":"ls"},"output":"` + strings.Repeat("payload ", 300) + `"}]`
	s, wsp, _, sess, history := stepBaseFixture(t, 6, trace)

	warm := sess
	warm.CLISessionID = "cli-1"
	warm.CLISentMsgCount = 4
	warm.SummaryMsgCount = 1
	warm.CLICompactMsgCount = 2 // the later of the two boundaries wins

	total, stepBase, err := s.contextOverheadTokens(ctx, wsp, warm, history, false)
	if err != nil {
		t.Fatalf("contextOverheadTokens: %v", err)
	}
	if stepBase != 2 {
		t.Fatalf("stepBase = %d, want 2 (max of the summary and CLI boundaries)", stepBase)
	}
	fillers := 0
	for _, f := range s.systemFillers(ctx, wsp, warm, history, false) {
		fillers += f.Tokens
	}
	wantSteps, err := conversation.EstimatePersistedStepTokens(history[stepBase:])
	if err != nil {
		t.Fatalf("estimate steps: %v", err)
	}
	if got := total - fillers; got != wantSteps {
		t.Fatalf("charged step term = %d, want %d (history[%d:])", got, wantSteps, stepBase)
	}

	// Cold thread: nothing is retained, so nothing may be charged and the baseline
	// must say so rather than pointing at index 0.
	coldTotal, coldBase, err := s.contextOverheadTokens(ctx, wsp, sess, history, false)
	if err != nil {
		t.Fatalf("contextOverheadTokens (cold): %v", err)
	}
	if coldBase != -1 {
		t.Fatalf("cold stepBase = %d, want -1", coldBase)
	}
	if coldTotal != fillers {
		t.Fatalf("cold total = %d, want %d (fillers only)", coldTotal, fillers)
	}
}

// TestContextOverheadStepBaseReachesPrepare walks the whole chain the three turn
// call sites (chat_stream.go, chat_btw.go, wake_turn.go) wire up: the (total,
// stepBase) pair contextOverheadTokens returns is stamped on the context with
// WithContextOverhead + WithContextOverheadStepBase, and Prepare's fold then
// drops exactly the trace of the messages it summarized away.
//
// This proves the VALUES compose end to end. It does NOT execute the three call
// sites themselves — they sit inside full HTTP/turn paths — so deleting a
// plumbing line there still fails no test.
func TestContextOverheadStepBaseReachesPrepare(t *testing.T) {
	ctx := context.Background()
	trace := `[{"kind":"tool","tool":"Bash","input":{"cmd":"ls"},"output":"` + strings.Repeat("payload ", 300) + `"}]`
	s, wsp, agentRow, sess, history := stepBaseFixture(t, 6, trace)

	warm := sess
	warm.CLISessionID = "cli-1"
	warm.CLISentMsgCount = 4
	warm.SummaryMsgCount = 1
	warm.CLICompactMsgCount = 2

	overhead, stepBase, err := s.contextOverheadTokens(ctx, wsp, warm, history, false)
	if err != nil {
		t.Fatalf("contextOverheadTokens: %v", err)
	}

	const keepRecent = 2
	m := conversation.NewManager()
	// Pending window starts at SummaryMsgCount, so the fold boundary lands on
	// newCount = 1 + (5 - keepRecent) = 4 and history[stepBase:4] must leave.
	pending := history[warm.SummaryMsgCount:]
	m.SetLimits(conversation.EstimateTokens("", pending)+overhead/2, keepRecent)

	// Exactly what the call sites do with the pair.
	turnCtx := conversation.WithContextOverhead(ctx, overhead)
	turnCtx = conversation.WithContextOverheadStepBase(turnCtx, stepBase)

	prep, err := m.Prepare(turnCtx, wsp.DB, stepBaseStubProvider{summary: "ROLLED UP"}, warm, agentRow, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !prep.Compacted {
		t.Fatalf("fixture did not fold (overhead=%d)", overhead)
	}

	const newCount = 4
	dropped, err := conversation.EstimatePersistedStepTokens(history[stepBase:newCount])
	if err != nil {
		t.Fatalf("estimate dropped: %v", err)
	}
	if dropped == 0 {
		t.Fatal("fixture folds no charged trace; the assertion below would be vacuous")
	}
	want := conversation.EstimateTokens("ROLLED UP", history[newCount:]) + overhead - dropped
	if prep.Fold.AfterTokens != want {
		t.Fatalf("AfterTokens = %d, want %d (stepBase %d did not reach Prepare: folded trace %d still charged)",
			prep.Fold.AfterTokens, want, stepBase, dropped)
	}
}
