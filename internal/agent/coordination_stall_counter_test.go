package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestStallNudgeReachesNextTurnContextAndCounts verifies the first escalation tier:
// a detected stall injects the corrective note as a real user-role message, so it is
// part of the context the model is handed on its NEXT turn (not just a log line), and
// the stall is counted on the session so a later tier can act on the history.
func TestStallNudgeReachesNextTurnContextAndCounts(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	coord := newTestCoordinator(t, rt, 0)
	slot := rt.coordSlotFor(coord)

	rt.injectStallNudge(coord, "A", slot, true /* re-arm this batch */)

	// The note must be assembled into the next turn's provider messages — that is
	// what "the agent sees it" means. Anything that only logs would pass a weaker
	// assertion but leave the model blind.
	msgs, err := rt.db.ListMessages(ctx, coord)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	assembled := conversation.ToProviderMessages(ctx, msgs)
	var seen string
	for _, m := range assembled {
		if strings.Contains(m.Text, "coordination-guard") {
			seen = m.Text
			break
		}
	}
	if seen == "" {
		t.Fatalf("corrective note absent from assembled turn context; got %d messages", len(assembled))
	}
	// The three things the note must actually say (per the tier-1 contract).
	for _, want := range []string{"spawn_worker", "list_workers", "describing a spawn is not spawning it"} {
		if !strings.Contains(seen, want) {
			t.Errorf("corrective note missing %q", want)
		}
	}

	// The turn is NOT killed: tier 1 warns only. The slot re-arms one more turn
	// instead of halting.
	slot.mu.Lock()
	halted, pending := slot.stallHalted, slot.pending
	slot.mu.Unlock()
	if halted {
		t.Error("tier 1 must not halt the coordinator")
	}
	if !pending {
		t.Error("tier 1 must re-arm one more turn so the agent can act on the note")
	}

	// Counted and persisted, so a later tier can escalate on history rather than on
	// an in-memory streak that a restart erases.
	got, err := rt.db.GetSession(ctx, coord)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.StallNudges != 1 {
		t.Fatalf("StallNudges = %d after one stall, want 1", got.StallNudges)
	}

	rt.injectStallNudge(coord, "A", slot, false)
	got, _ = rt.db.GetSession(ctx, coord)
	if got.StallNudges != 2 {
		t.Fatalf("StallNudges = %d after two stalls, want 2", got.StallNudges)
	}
}

// TestRunWorkerPersistsFailedRunState drives the REAL runWorker path with no usable
// provider, so the turn dies on a provider error — the branch most at risk of being
// left unrecorded. It must still end with RunState=failed on the session, because a
// run that died is exactly the one an operator must be able to tell from a live one.
func TestRunWorkerPersistsFailedRunState(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	ag, err := rt.db.CreateAgent(ctx, db.Agent{Name: "W", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	worker, err := rt.db.CreateSession(ctx, db.Session{AgentID: ag.ID, Kind: "chat", Role: "worker"})
	if err != nil {
		t.Fatalf("create worker session: %v", err)
	}
	coord := newTestCoordinator(t, rt, 0)

	rt.runWorker(ag, worker.ID, "do the thing", coord)

	got, err := rt.db.GetSession(ctx, worker.ID)
	if err != nil {
		t.Fatalf("get worker session: %v", err)
	}
	if got.RunState != turnStatusFailed {
		t.Fatalf("RunState = %q after a provider-error run, want %q", got.RunState, turnStatusFailed)
	}
	if got.RunStateAt == 0 {
		t.Error("RunStateAt must be stamped when RunState is written")
	}
	if got.State != "active" {
		t.Fatalf("State = %q, want active (run outcome must not touch the visibility field)", got.State)
	}
}

// TestWorkerRunStateRecordedForEveryOutcome verifies every outcome in the turn-status
// vocabulary round-trips onto the session, and that none of them leaks into State.
func TestWorkerRunStateRecordedForEveryOutcome(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	for _, tc := range []struct{ name, status string }{
		{"completed", turnStatusCompleted},
		{"failed", turnStatusFailed},
		{"killed", turnStatusKilled},
		{"timeout", turnStatusTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: "AGT1"})
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			if err := rt.db.SetSessionRunState(ctx, sess.ID, tc.status, 1700000000); err != nil {
				t.Fatalf("set run state: %v", err)
			}
			got, err := rt.db.GetSession(ctx, sess.ID)
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			if got.RunState != tc.status {
				t.Fatalf("RunState = %q, want %q", got.RunState, tc.status)
			}
			// State stays the visibility field the sidebar filter and the API validator
			// depend on; the outcome must never have leaked into it.
			if got.State != "active" {
				t.Fatalf("State = %q, want active (run outcome must not touch it)", got.State)
			}
		})
	}
}
