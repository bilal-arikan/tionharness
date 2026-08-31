package api

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// compactionStep mirrors what traceStepToTurnStep produces for a finished
// CLI-side compaction: claude-cli's compact_boundary and codex-cli's
// context_compaction item both land on this shape.
func compactionStep(provider string, running bool) agent.TurnStep {
	return agent.TurnStep{
		ID:            "cmp-1",
		Kind:          "compaction",
		Source:        "cli-native",
		Provider:      provider,
		SessionAction: "native-compact",
		Trigger:       "auto",
		Running:       running,
	}
}

// The two directions of the re-baseline, at the exact decision the turn path
// makes: a completed CLI-side compaction moves the boundary to the pre-reply
// transcript length, and everything else leaves it alone. Both providers report
// it identically, so both are pinned here.
func TestCLICompactionBoundaryDetectsCompletedAutoCompaction(t *testing.T) {
	cases := []struct {
		name  string
		steps []agent.TurnStep
		want  bool
	}{
		{"no steps", nil, false},
		{"ordinary tool turn", []agent.TurnStep{{Kind: "tool", Tool: "Bash"}}, false},
		{"compaction still running", []agent.TurnStep{compactionStep("claude-cli", true)}, false},
		{"claude-cli auto compaction", []agent.TurnStep{compactionStep("claude-cli", false)}, true},
		{"codex-cli auto compaction", []agent.TurnStep{compactionStep("codex-cli", false)}, true},
		{
			"compaction among other steps",
			[]agent.TurnStep{{Kind: "tool", Tool: "Read"}, compactionStep("claude-cli", false), {Kind: "text"}},
			true,
		},
		{
			// A TionHarness-side fold is NOT a provider-side compaction: the CLI's own
			// window is untouched, so its retained trace must keep counting.
			"tionharness fold step",
			[]agent.TurnStep{{Kind: "compaction", Source: "tionharness", SessionAction: "restart-summary"}},
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			boundary, compacted := cliCompactionBoundary(tc.steps, 7)
			if compacted != tc.want {
				t.Fatalf("compacted = %v, want %v", compacted, tc.want)
			}
			// 7 = transcript length before this turn's reply; the reply's own trace
			// landed after the compaction and stays warm.
			if tc.want && boundary != 7 {
				t.Fatalf("boundary = %d, want 7", boundary)
			}
			if !tc.want && boundary != 0 {
				t.Fatalf("boundary = %d, want 0 when nothing compacted", boundary)
			}
		})
	}
}

// End-to-end over the store: the signal moves CLICompactMsgCount, its absence
// leaves it where it was, and a stale (smaller) boundary never rewinds it.
func TestSetSessionCLICompactBoundaryIsMonotonic(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	agentRow, err := database.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := database.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.CLICompactMsgCount != 0 {
		t.Fatalf("fresh session boundary = %d, want 0", sess.CLICompactMsgCount)
	}

	if err := database.SetSessionCLICompactBoundary(ctx, sess.ID, 6); err != nil {
		t.Fatalf("set boundary: %v", err)
	}
	got, err := database.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.CLICompactMsgCount != 6 {
		t.Fatalf("boundary after auto compaction = %d, want 6", got.CLICompactMsgCount)
	}

	if err := database.SetSessionCLICompactBoundary(ctx, sess.ID, 4); err != nil {
		t.Fatalf("set stale boundary: %v", err)
	}
	got, err = database.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.CLICompactMsgCount != 6 {
		t.Fatalf("boundary rewound to %d; a stale value must not move it back", got.CLICompactMsgCount)
	}
}

// The point of the whole re-baseline: after the CLI compacts itself, the trace it
// dropped stops being charged, while the trace it still holds keeps counting.
func TestWarmCLIStepBaselineFollowsCLICompaction(t *testing.T) {
	warm := db.Session{CLISessionID: "cli-1", CLISentMsgCount: 4}

	if got := warmCLIStepBaseline(db.Session{}, 10); got != -1 {
		t.Fatalf("cold thread baseline = %d, want -1 (nothing retained)", got)
	}
	if got := warmCLIStepBaseline(warm, 10); got != 0 {
		t.Fatalf("warm, never compacted = %d, want 0", got)
	}

	folded := warm
	folded.SummaryMsgCount = 3
	if got := warmCLIStepBaseline(folded, 10); got != 3 {
		t.Fatalf("rolling-summary baseline = %d, want 3", got)
	}

	compacted := folded
	compacted.CLICompactMsgCount = 7
	if got := warmCLIStepBaseline(compacted, 10); got != 7 {
		t.Fatalf("cli compaction baseline = %d, want 7 (the later of the two)", got)
	}

	// A boundary past the end of the transcript means the history shrank under it;
	// fall back to counting everything rather than silently zeroing the term.
	if got := warmCLIStepBaseline(compacted, 5); got != 0 {
		t.Fatalf("out-of-range baseline = %d, want 0", got)
	}
}

// The meter's own bucket must move with the baseline, or the gate and the bar
// disagree again — the exact drift contextOverheadTokens was written to close.
func TestBuildFillersDropsTraceBeforeCLICompactionBoundary(t *testing.T) {
	steps := `[{"kind":"tool","tool":"Bash","input":{"cmd":"ls"},"output":"` + strings.Repeat("x ", 400) + `"}]`
	pending := []db.Message{
		{Role: "assistant", Text: "old", Steps: steps},
		{Role: "assistant", Text: "new", Steps: steps},
	}
	stepTokens, _, err := conversation.EstimatePersistedSteps(steps)
	if err != nil {
		t.Fatal(err)
	}
	textOnly := 2 * (conversation.EstimateText("old") + conversation.MsgOverhead)

	sum := func(stepsFrom int) int {
		fillers, err := buildFillers("", pending, stepsFrom)
		if err != nil {
			t.Fatal(err)
		}
		total := 0
		for _, f := range fillers {
			total += f.Tokens
		}
		return total
	}

	if got, want := sum(0), textOnly+2*stepTokens; got != want {
		t.Fatalf("no cli compaction: total = %d, want %d", got, want)
	}
	if got, want := sum(1), textOnly+stepTokens; got != want {
		t.Fatalf("after cli compaction: total = %d, want %d (only the post-boundary trace)", got, want)
	}
}
