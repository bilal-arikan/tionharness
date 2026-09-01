package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestPreparePersistsCLICompactionLifecycle(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agentRow, err := rt.db.CreateAgent(ctx, db.Agent{Name: "CLI", Provider: "codex-cli"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatal(err)
	}

	turn := toolLoopTurn{
		r:        rt,
		ctx:      WithSessionID(ctx, session.ID),
		agent:    agentRow,
		provider: providers.NewCodexCLI("codex", "gpt-5", filepath.Join(t.TempDir(), "codex-home")),
	}
	cleanup, err := turn.prepare()
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer cleanup()
	if turn.req.OnCLICompaction == nil {
		t.Fatal("CLI compaction lifecycle adapter is nil")
	}
	turn.req.OnCLICompaction(providers.CLICompactionEvent{
		Phase: providers.CLICompactionAttempt, Provider: "claude-cli",
		AttemptID: "attempt-1", Attempt: 1,
	})
	turn.req.OnCLICompaction(providers.CLICompactionEvent{
		Phase: providers.CLICompactionSuccess, Provider: "claude-cli",
		AttemptID: "attempt-1", Attempt: 1, DurationMs: 1,
	})
	rt.emitCLIToolDebug(turn.ctx, agentRow, []providers.TraceStep{{
		Kind: "compaction", Source: "cli-native", Provider: "claude-cli", SessionAction: "native-compact",
	}})

	events, err := rt.db.ReadDebugEvents(ctx, session.ID, db.DebugCLICompaction, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || !strings.HasPrefix(events[0].AttemptID, "fp:") || !strings.HasPrefix(events[0].AgentID, "fp:") {
		t.Fatalf("journal events = %+v", events)
	}
	summary, err := rt.db.GetDebugSummary(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Compactions != 1 {
		t.Fatalf("Compactions = %d, want one lifecycle success despite trace scan", summary.Compactions)
	}
}
