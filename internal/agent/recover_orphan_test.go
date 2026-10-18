package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestRecoverOrphanedSpawn verifies the boot recovery of a spawned turn killed
// mid-flight: it gets an interrupted reply so it no longer reads as frozen.
func TestRecoverOrphanedSpawn(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	spawned, err := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: "spawned", SourceID: "s:spawned"})
	if err != nil {
		t.Fatalf("create spawned session: %v", err)
	}
	// Orphaned mid-turn: only the opening user prompt, no assistant reply.
	if _, err := rt.db.AddMessage(ctx, db.Message{SessionID: spawned.ID, Role: "user", Text: "do the thing"}); err != nil {
		t.Fatalf("add spawn prompt: %v", err)
	}

	rt.RecoverOrphanedTurns(ctx)

	msgs, _ := rt.db.ListMessages(ctx, spawned.ID)
	if len(msgs) != 2 || msgs[1].Role != "assistant" || !msgs[1].Interrupted {
		t.Fatalf("spawned session should end with an interrupted assistant reply, got %+v", msgs)
	}

	// Idempotent: a second pass must NOT add another reply (last msg is now assistant).
	rt.RecoverOrphanedTurns(ctx)
	msgs2, _ := rt.db.ListMessages(ctx, spawned.ID)
	if len(msgs2) != 2 {
		t.Fatalf("second recovery must be a no-op, msgs=%d", len(msgs2))
	}
}

// TestRecoverSkipsCompletedAndNonSpawnedSessions verifies a spawned session that
// finished normally (last message is an assistant reply) is left untouched, and a
// plain chat with a trailing user message is not the recovery's business — the
// user is still there to resend it.
func TestRecoverSkipsCompletedAndNonSpawnedSessions(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	done, _ := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: "spawned", SourceID: "s:done"})
	rt.db.AddMessage(ctx, db.Message{SessionID: done.ID, Role: "user", Text: "task"})
	rt.db.AddMessage(ctx, db.Message{SessionID: done.ID, Role: "assistant", Text: "done"})
	chat, _ := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: "chat", SourceID: "s:chat"})
	rt.db.AddMessage(ctx, db.Message{SessionID: chat.ID, Role: "user", Text: "hello?"})

	rt.RecoverOrphanedTurns(ctx)

	if msgs, _ := rt.db.ListMessages(ctx, done.ID); len(msgs) != 2 {
		t.Fatalf("completed spawn must be untouched, got %d msgs", len(msgs))
	}
	if msgs, _ := rt.db.ListMessages(ctx, chat.ID); len(msgs) != 1 {
		t.Fatalf("a plain chat must not get a recovered reply, got %d msgs", len(msgs))
	}
}
