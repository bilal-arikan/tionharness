package agent

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
)

// TestDeliverPrompt_RecordsErrorReply verifies that a failed scheduled prompt
// surfaces the failure as an assistant turn inside the schedule session, instead
// of leaving the thread with a lone user prompt and no reply. Regression guard
// for the "Günlük Özet Ver cevap gelmiyor" report: the agent's provider is
// unconfigured (no key), so invokeTraced errors — the user must still see why.
func TestDeliverPrompt_RecordsErrorReply(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	sched := NewScheduler(rt.db, rt, slog.New(slog.NewTextHandler(discardWriter{}, nil)))
	ctx := context.Background()

	// Agent on the anthropic provider with no API key → registry Get fails.
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Özetçi", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sc := db.Schedule{AgentID: agent.ID, Prompt: "Günlük özet ver", CronExpr: "0 * * * *", Enabled: true}

	sessionID, fireErr := sched.deliverPrompt(ctx, sc)
	if fireErr == nil {
		t.Fatal("expected deliverPrompt to fail with unconfigured provider")
	}
	if sessionID == "" {
		t.Fatal("expected a session id even on failure (for deep-linking)")
	}

	msgs, err := rt.db.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected user prompt + error reply (2 messages), got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Text != "Günlük özet ver" {
		t.Errorf("first message should be the user prompt, got %+v", msgs[0])
	}
	reply := msgs[1]
	if reply.Role != "assistant" {
		t.Errorf("reply role = %q, want assistant", reply.Role)
	}
	if reply.AgentID != agent.ID {
		t.Errorf("reply agentId = %q, want %q (avatar/identity must render)", reply.AgentID, agent.ID)
	}
	if !strings.Contains(reply.Text, "çalıştırılamadı") || !strings.Contains(reply.Text, fireErr.Error()) {
		t.Errorf("error reply must contain the failure reason, got %q", reply.Text)
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
