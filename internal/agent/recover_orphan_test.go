package agent

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestRecoverOrphanedWorker verifies the boot recovery of a worker turn killed
// mid-flight: it gets an interrupted reply AND its coordinator receives a synthetic
// killed task-notification (so the coordinator stops waiting).
func TestRecoverOrphanedWorker(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	// Stub the coordinator turn runner so NotifyCoordinator's enqueue doesn't try to
	// run a real provider turn; we only assert the notification was persisted.
	rt.coordRunFn = func(string) {}
	ctx := context.Background()

	coord, err := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: "chat", Role: "coordinator", SourceID: "s:coord"})
	if err != nil {
		t.Fatalf("create coordinator: %v", err)
	}
	worker, err := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: "worker", Role: "worker", SourceID: "s:worker", CoordinatorSessionID: coord.ID})
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}
	// Worker orphaned mid-turn: only the opening user prompt, no assistant reply.
	if _, err := rt.db.AddMessage(ctx, db.Message{SessionID: worker.ID, Role: "user", Text: "do the thing"}); err != nil {
		t.Fatalf("add worker prompt: %v", err)
	}

	rt.RecoverOrphanedTurns(ctx)

	// Worker now has an interrupted assistant reply as its last message.
	wm, _ := rt.db.ListMessages(ctx, worker.ID)
	if len(wm) != 2 || wm[1].Role != "assistant" || !wm[1].Interrupted {
		t.Fatalf("worker should end with an interrupted assistant reply, got %+v", wm)
	}
	// Coordinator received a synthetic killed task-notification.
	cm, _ := rt.db.ListMessages(ctx, coord.ID)
	if len(cm) != 1 || cm[0].Role != "user" || !strings.Contains(cm[0].Text, "<status>killed</status>") {
		t.Fatalf("coordinator should have a killed task-notification, got %+v", cm)
	}

	// Idempotent: a second pass must NOT add another reply (last msg is now assistant).
	rt.RecoverOrphanedTurns(ctx)
	wm2, _ := rt.db.ListMessages(ctx, worker.ID)
	if len(wm2) != 2 {
		t.Fatalf("second recovery must be a no-op, worker msgs=%d", len(wm2))
	}
}

// TestRecoverSkipsTrailingGateNote: a coordinator whose transcript ends with the
// record of a resolved human gate is finished, not orphaned — the note is a
// user-role message for display only and owes no reply. Before the check every
// restart re-enqueued such a root and it spawned workers on a done task.
func TestRecoverSkipsTrailingGateNote(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	var enqueued atomic.Int32
	rt.coordRunFn = func(string) { enqueued.Add(1) }
	ctx := context.Background()

	coord, _ := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: "chat", Role: "coordinator", CoordinatorMode: true, SourceID: "s:c-gate"})
	rt.db.AddMessage(ctx, db.Message{SessionID: coord.ID, Role: "user", Text: "task"})
	rt.db.AddMessage(ctx, db.Message{SessionID: coord.ID, Role: "assistant", Text: "done"})
	if _, err := rt.recordInjectedUserNote(ctx, coord.ID, noteOriginGate, `<gate trajectory="RTA1" phase="code" approved=true>Onayla</gate>`); err != nil {
		t.Fatal(err)
	}

	rt.RecoverOrphanedTurns(ctx)
	time.Sleep(150 * time.Millisecond) // the wake is asynchronous; give a wrong enqueue time to show

	cm, _ := rt.db.ListMessages(ctx, coord.ID)
	if len(cm) != 3 || enqueued.Load() != 0 {
		t.Fatalf("a trailing gate note must not resurrect the coordinator: msgs=%d enqueued=%d", len(cm), enqueued.Load())
	}
	// A genuine trailing prompt on the same shape of session is still reclaimed.
	other, _ := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: "chat", Role: "coordinator", CoordinatorMode: true, SourceID: "s:c-prompt"})
	rt.db.AddMessage(ctx, db.Message{SessionID: other.ID, Role: "user", Text: "<task-notification>x</task-notification>"})
	rt.RecoverOrphanedTurns(ctx)
	waitForCondition(t, 5*time.Second, "real trailing prompt re-enqueued", func() bool { return enqueued.Load() == 1 })
	drainSpawns(t, rt)
}

// TestRecoverSkipsCompletedWorker verifies a worker that finished normally (last
// message is an assistant reply) is left untouched.
func TestRecoverSkipsCompletedWorker(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.coordRunFn = func(string) {}
	ctx := context.Background()

	coord, _ := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Role: "coordinator", SourceID: "s:c2"})
	worker, _ := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: "worker", Role: "worker", SourceID: "s:w2", CoordinatorSessionID: coord.ID})
	rt.db.AddMessage(ctx, db.Message{SessionID: worker.ID, Role: "user", Text: "task"})
	rt.db.AddMessage(ctx, db.Message{SessionID: worker.ID, Role: "assistant", Text: "done"})

	rt.RecoverOrphanedTurns(ctx)

	wm, _ := rt.db.ListMessages(ctx, worker.ID)
	if len(wm) != 2 {
		t.Fatalf("completed worker must be untouched, got %d msgs", len(wm))
	}
	cm, _ := rt.db.ListMessages(ctx, coord.ID)
	if len(cm) != 0 {
		t.Fatalf("no notification for a completed worker, got %d", len(cm))
	}
}
