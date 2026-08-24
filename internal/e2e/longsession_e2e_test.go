package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestLongSession_CompactsWhenOverBudget grows a session past the context-token
// budget, then drives a turn and asserts the conversation manager folded the old
// history into a rolling summary: the summary is persisted, only the most recent
// turns are sent verbatim, and the agent still answers normally.
func TestLongSession_CompactsWhenOverBudget(t *testing.T) {
	prov := newScriptedProvider(sayText("Acknowledged — continuing the long session."))
	h := newHarness(t, prov)

	// Tight budget so a modest backlog trips compaction; keep the 4 newest verbatim.
	const keepRecent = 4
	h.convo.SetLimits(150, keepRecent)

	ag := h.newAgent("Marathon")
	sess := h.newSession(ag)

	// Pre-seed a long backlog directly (cheap; bypasses the model). Each message is
	// padded so the running history comfortably exceeds the token budget.
	ctx := context.Background()
	const backlog = 24
	pad := strings.Repeat("context ", 12)
	for i := 0; i < backlog; i++ {
		role := providers.RoleUser
		if i%2 == 1 {
			role = providers.RoleAssistant
		}
		if _, err := h.db.AddMessage(ctx, db.Message{
			SessionID: sess.ID,
			Role:      role,
			Text:      fmt.Sprintf("message %d: %s", i, pad),
		}); err != nil {
			t.Fatalf("seed backlog %d: %v", i, err)
		}
	}

	res := h.send(ag, sess, "Where are we now?")

	// Compaction ran this turn.
	if !res.prep.Compacted {
		t.Fatalf("expected the turn to compact the backlog, but it did not (contextTokens=%d)", res.prep.ContextTokens)
	}
	// The rolling summary was produced by the (intercepted) summarize call and
	// injected for this turn.
	if res.prep.Summary != prov.summaryReply {
		t.Errorf("prep summary = %q, want %q", res.prep.Summary, prov.summaryReply)
	}
	if prov.compactCalls != 1 {
		t.Errorf("summarize calls = %d, want exactly 1", prov.compactCalls)
	}

	// Only the recent tail (kept verbatim) plus the new user message are sent —
	// far fewer than the full backlog.
	if got := len(res.prep.Messages); got > keepRecent+1 {
		t.Errorf("sent %d messages after compaction, want <= %d", got, keepRecent+1)
	}

	// The fold was persisted to the session for the next turn to build on.
	got, _ := h.db.GetSession(ctx, sess.ID)
	if got.Summary != prov.summaryReply {
		t.Errorf("persisted session summary = %q, want %q", got.Summary, prov.summaryReply)
	}
	if got.SummaryMsgCount == 0 {
		t.Errorf("SummaryMsgCount not advanced after compaction")
	}

	// The agent still answered normally despite the fold.
	if res.resp.Text != "Acknowledged — continuing the long session." {
		t.Errorf("final answer = %q", res.resp.Text)
	}
}
