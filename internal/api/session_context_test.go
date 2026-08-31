package api

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
)

func appendContextDebugEvent(t *testing.T, sessionID string, database *db.DB, event db.DebugEvent) {
	t.Helper()
	event.Type = db.DebugLLMCall
	if err := database.AppendDebugEvent(sessionID, event, 0); err != nil {
		t.Fatalf("append debug event: %v", err)
	}
}

func TestComputeCLIOverheadSelectsNewestChatAndWorkerIndependently(t *testing.T) {
	_, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	session, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "context"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	appendContextDebugEvent(t, session.ID, wsp.DB, db.DebugEvent{Kind: "chat", In: 100, Calls: 1})
	appendContextDebugEvent(t, session.ID, wsp.DB, db.DebugEvent{Kind: "task", In: 900, Calls: 3})
	appendContextDebugEvent(t, session.ID, wsp.DB, db.DebugEvent{Kind: "chat", In: 600, CacheRead: 300, Calls: 3})
	appendContextDebugEvent(t, session.ID, wsp.DB, db.DebugEvent{Kind: "spawned", In: 800, CacheWrite: 200, Calls: 2})

	got := computeCLIOverhead(ctx, wsp, "claude-cli", session.ID, 250, 2)
	if got.ChatMeasuredTokens != 300 || got.ChatCalls != 3 {
		t.Fatalf("chat measurement = %d/%d, want 300/3", got.ChatMeasuredTokens, got.ChatCalls)
	}
	if got.WorkerMeasuredTokens != 500 || got.WorkerCalls != 2 || got.WorkerKind != "spawned" {
		t.Fatalf("worker measurement = %d/%d kind %q, want 500/2 spawned", got.WorkerMeasuredTokens, got.WorkerCalls, got.WorkerKind)
	}
	if got.MeasuredTokens != got.ChatMeasuredTokens || got.Calls != got.ChatCalls || got.OverheadTokens != 50 {
		t.Fatalf("legacy chat aliases = measured %d calls %d overhead %d", got.MeasuredTokens, got.Calls, got.OverheadTokens)
	}
}

func TestComputeCLIOverheadCallFloorAndSingleKindEvents(t *testing.T) {
	tests := []struct {
		name       string
		event      db.DebugEvent
		wantChat   int
		wantWorker int
		wantKind   string
	}{
		{name: "chat calls zero", event: db.DebugEvent{Kind: "chat", In: 90}, wantChat: 90},
		{name: "worker calls negative", event: db.DebugEvent{Kind: "flow", In: 75, Calls: -2}, wantWorker: 75, wantKind: "flow"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, wsp := newWorkspaceServer(t)
			session, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat", Title: tt.name})
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			appendContextDebugEvent(t, session.ID, wsp.DB, tt.event)

			got := computeCLIOverhead(context.Background(), wsp, "claude-cli", session.ID, 100, 0)
			if got.ChatMeasuredTokens != tt.wantChat || got.WorkerMeasuredTokens != tt.wantWorker || got.WorkerKind != tt.wantKind {
				t.Fatalf("got chat=%d worker=%d kind=%q", got.ChatMeasuredTokens, got.WorkerMeasuredTokens, got.WorkerKind)
			}
			if tt.wantChat > 0 && got.ChatCalls != 1 {
				t.Fatalf("chat calls=%d, want 1", got.ChatCalls)
			}
			if tt.wantWorker > 0 && got.WorkerCalls != 1 {
				t.Fatalf("worker calls=%d, want 1", got.WorkerCalls)
			}
		})
	}
}

func TestComputeCLIOverheadUsageFallbackIsChatOnly(t *testing.T) {
	_, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	session, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "fallback"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := wsp.DB.AddSessionUsageKind(ctx, session.ID, "agent", "chat", "claude-cli", "model", db.UsageDelta{
		Calls: 1, ProviderCalls: 2, InputTokens: 200, CacheReadTokens: 600,
	}); err != nil {
		t.Fatalf("add session usage: %v", err)
	}

	got := computeCLIOverhead(ctx, wsp, "claude-cli", session.ID, 500, 0)
	if got.ChatMeasuredTokens != 400 || got.ChatCalls != 2 {
		t.Fatalf("chat fallback=%d/%d, want 400/2", got.ChatMeasuredTokens, got.ChatCalls)
	}
	if got.WorkerMeasuredTokens != 0 || got.WorkerCalls != 0 || got.WorkerKind != "" {
		t.Fatalf("usage fallback must not populate worker: %+v", got)
	}
	if got.OverheadTokens != 0 {
		t.Fatalf("negative chat overhead must clamp to zero, got %d", got.OverheadTokens)
	}
}

func TestComputeCLIOverheadProviderAndAgentPreviewBehavior(t *testing.T) {
	_, wsp := newWorkspaceServer(t)
	if got := computeCLIOverhead(context.Background(), wsp, "anthropic", "session", 100, 2); got != nil {
		t.Fatalf("non claude-cli preview = %+v, want nil", got)
	}

	got := computeCLIOverhead(context.Background(), wsp, "claude-cli", "", 100, 2)
	if got.ChatMeasuredTokens != 0 || got.WorkerMeasuredTokens != 0 {
		t.Fatalf("agent preview must remain unmeasured: %+v", got)
	}
	if want := conversation.PredictCLIOverhead(2); got.PredictedOverhead != want {
		t.Fatalf("agent preview prediction=%d, want %d", got.PredictedOverhead, want)
	}
}
