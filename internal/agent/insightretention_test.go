package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/insight"
)

// TestArchiveOldInsightSessions: only the newest N scan sessions stay live; the
// rest are archived (not deleted — the transcript must remain readable).
func TestArchiveOldInsightSessions(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	var ids []string
	for i := 0; i < 5; i++ {
		s, err := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: db.SessionKindInsight, SourceID: "IRUN"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, s.ID)
	}
	// A normal chat session must be untouched by the insight retention pass.
	chat, err := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Title: "chat"})
	if err != nil {
		t.Fatal(err)
	}

	n, err := rt.archiveOldInsightSessions(ctx, 2)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n != 3 {
		t.Fatalf("archived = %d, want 3", n)
	}

	live := 0
	for _, id := range ids {
		s, err := rt.db.GetSession(ctx, id) // still there: archived, never deleted
		if err != nil {
			t.Fatalf("session %s vanished: %v", id, err)
		}
		if s.State != "archived" {
			live++
		}
	}
	if live != 2 {
		t.Fatalf("live insight sessions = %d, want 2", live)
	}
	if s, err := rt.db.GetSession(ctx, chat.ID); err != nil || s.State == "archived" {
		t.Fatalf("chat session must stay live: state=%v err=%v", s.State, err)
	}

	// Idempotent: a second pass has nothing left to archive.
	if n2, err := rt.archiveOldInsightSessions(ctx, 2); err != nil || n2 != 0 {
		t.Fatalf("second pass archived=%d err=%v, want 0/nil", n2, err)
	}
	// keep <= 0 means keep everything.
	if n3, err := rt.archiveOldInsightSessions(ctx, 0); err != nil || n3 != 0 {
		t.Fatalf("keep=0 must be a no-op: archived=%d err=%v", n3, err)
	}
}

// TestRunSessionRetentionDefault covers the settings resolution: unset → default,
// negative → keep everything.
func TestRunSessionRetentionDefault(t *testing.T) {
	if got := (insight.Settings{}).RunSessionRetention(); got != insight.DefaultMaxRunSessions {
		t.Fatalf("unset retention = %d, want %d", got, insight.DefaultMaxRunSessions)
	}
	if got := (insight.Settings{MaxRunSessions: 7}).RunSessionRetention(); got != 7 {
		t.Fatalf("explicit retention = %d, want 7", got)
	}
	if got := (insight.Settings{MaxRunSessions: -1}).RunSessionRetention(); got != 0 {
		t.Fatalf("negative retention = %d, want 0 (keep all)", got)
	}
}
