package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
)

func TestWorkerSpawnPersistsCoordinatorOwnershipAndTerminalState(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Worker", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	coord := newTestCoordinator(t, rt, 0)
	res, err := rt.SpawnSession(ctx, agent.ID, "work", SpawnOptions{CoordinatorSessionID: coord})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	drainSpawns(t, rt)
	got, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	if got.CoordinatorSessionID != coord || got.ParentSessionID != coord {
		t.Fatalf("worker owner links = coordinator %q parent %q, want %q", got.CoordinatorSessionID, got.ParentSessionID, coord)
	}
	if got.RunState == "" || got.RunStateAt == 0 {
		t.Fatalf("worker terminal state missing: state=%q at=%d", got.RunState, got.RunStateAt)
	}
}

func TestWorkerReplyUsesSharedSessionCounters(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: "worker"})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}
	steps := []TurnStep{{Kind: StepTool, Tool: "one"}, {Kind: StepText}, {Kind: StepTool, Tool: "two"}}
	if err := rt.recordAssistantMessage(ctx, sess.ID, "AGT1", "done", steps, nil, 1); err != nil {
		t.Fatalf("record worker reply: %v", err)
	}
	got, err := rt.db.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	if got.MessageCount != 1 || got.ToolCallCount != 2 {
		t.Fatalf("worker counters = messages %d tools %d, want 1/2", got.MessageCount, got.ToolCallCount)
	}
}

func TestCoordinatorBlockedTagAndNotificationAreOnce(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.bus = events.NewBus()
	ctx := context.Background()
	coord := newTestCoordinator(t, rt, 0)
	id, ch := rt.bus.Subscribe()
	defer rt.bus.Unsubscribe(id)
	rt.markCoordinatorBlocked(ctx, coord)
	rt.markCoordinatorBlocked(ctx, coord)

	sess, err := rt.db.GetSession(ctx, coord)
	if err != nil {
		t.Fatalf("get coordinator: %v", err)
	}
	if !containsTag(sess.Tags, TagBlocked) {
		t.Fatalf("blocked tag absent: %v", sess.Tags)
	}
	var notices int
	deadline := time.After(100 * time.Millisecond)
	for {
		select {
		case ev := <-ch:
			if ev.Type == events.TypeCoordination && ev.Title == "🧭 Koordinatör bloke oldu" {
				notices++
			}
		case <-deadline:
			if notices != 1 {
				t.Fatalf("blocked notifications = %d, want 1", notices)
			}
			return
		}
	}
}
