package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestRecordAssistantReply_EmptySubstitution verifies an empty reply is persisted
// as the placeholder text (never a silent no-reply) and a non-empty reply is kept.
func TestRecordAssistantReply_EmptySubstitution(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a := newFlowAgent(t, rt, "worker")
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "schedule"})
	if err != nil {
		t.Fatal(err)
	}

	out, err := rt.recordAssistantReply(ctx, sess.ID, a.ID, "   ", nil, nil, 12, "PLACEHOLDER")
	if err != nil {
		t.Fatalf("record failed: %v", err)
	}
	if out != "PLACEHOLDER" {
		t.Errorf("empty reply should become the placeholder, got %q", out)
	}

	if _, err := rt.recordAssistantReply(ctx, sess.ID, a.ID, "real answer", nil, nil, 34, "PLACEHOLDER"); err != nil {
		t.Fatalf("record failed: %v", err)
	}

	msgs, err := rt.db.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 persisted replies, got %d", len(msgs))
	}
	if msgs[0].Text != "PLACEHOLDER" || msgs[0].Role != "assistant" || msgs[0].AgentID != a.ID {
		t.Errorf("first reply not persisted as expected: %+v", msgs[0])
	}
	if msgs[1].Text != "real answer" {
		t.Errorf("second reply text = %q, want %q", msgs[1].Text, "real answer")
	}
}

// TestRecordTurnError_InlineFailure verifies a failed turn is persisted inline as
// an assistant message with the prefix + error text (best-effort, never panics).
func TestRecordTurnError_InlineFailure(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a := newFlowAgent(t, rt, "worker")
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "schedule"})
	if err != nil {
		t.Fatal(err)
	}

	rt.recordTurnError(ctx, sess.ID, a.ID, errors.New("boom"), nil, nil, 5, "⚠️ Çalıştırılamadı:")

	msgs, err := rt.db.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 error message, got %d", len(msgs))
	}
	if !strings.HasPrefix(msgs[0].Text, "⚠️ Çalıştırılamadı:") || !strings.Contains(msgs[0].Text, "boom") {
		t.Errorf("error message shape wrong: %q", msgs[0].Text)
	}
	if msgs[0].Role != "assistant" {
		t.Errorf("error turn should be an assistant message, got role %q", msgs[0].Role)
	}
}

func TestRecordChildAssistantMessageRedactsAsyncTrace(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a := newFlowAgent(t, rt, "worker")
	sess, err := rt.db.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "subagent", RunState: "running"})
	if err != nil {
		t.Fatal(err)
	}
	secret := json.RawMessage(`{"token":"raw-secret","credential":"never-persist"}`)
	steps := []TurnStep{{Kind: StepTool, Tool: "Bash", Input: secret, SubSteps: []TurnStep{{Kind: StepTool, Tool: "secret", Input: secret}}}}
	meta := &turnMeta{StopReason: providers.StopEndTurn}
	if err := rt.recordChildAssistantMessage(ctx, sess.ID, a.ID, "done", steps, meta, 7, turnStatusCompleted); err != nil {
		t.Fatal(err)
	}
	msgs, err := rt.db.ListMessages(ctx, sess.ID)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("messages = %d, err=%v", len(msgs), err)
	}
	if strings.Contains(msgs[0].Steps, "raw-secret") || strings.Contains(msgs[0].Steps, "never-persist") {
		t.Fatalf("child transcript leaked tool arguments: %s", msgs[0].Steps)
	}
	if msgs[0].StopReason != providers.StopEndTurn {
		t.Fatalf("stop reason = %q, want %q", msgs[0].StopReason, providers.StopEndTurn)
	}
	got, _ := rt.db.GetSession(ctx, sess.ID)
	if got.RunState != turnStatusCompleted {
		t.Fatalf("run state = %q, want completed", got.RunState)
	}
}

func TestRecordChildAssistantMessagePersistenceFailureMarksFailed(t *testing.T) {
	ctx := context.Background()
	storeRoot := filepath.Join(t.TempDir(), "store")
	database, err := db.Open(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	tun := NewTunables()
	registry := providers.NewRegistry()
	rt := NewRuntime(database, registry, tun, t.TempDir(), t.TempDir(), nil, nil, "", "", nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	a, err := database.CreateAgent(ctx, db.Agent{Name: "child", Provider: "anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := database.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "subagent", RunState: "running"})
	if err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(storeRoot, "sessions", sess.ID, "messages.jsonl")
	if err := os.Mkdir(transcriptPath, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := &turnMeta{StopReason: providers.StopEndTurn}
	err = rt.recordChildAssistantMessage(ctx, sess.ID, a.ID, "done", nil, meta, 9, turnStatusCompleted)
	if err == nil || !strings.Contains(err.Error(), "persist child transcript") {
		t.Fatalf("expected transcript persistence error, got %v", err)
	}
	got, getErr := database.GetSession(ctx, sess.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if got.RunState != turnStatusFailed {
		t.Fatalf("run state = %q after transcript failure, want %q", got.RunState, turnStatusFailed)
	}
}
