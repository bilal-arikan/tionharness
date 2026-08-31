package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestManualCompactReplyPersistsCompactionStep(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	agentRow, err := database.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "codex-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	session, err := database.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	result := summaryResult{
		Body: "özet",
		Fold: conversation.Compaction{
			FoldedMsgs:   6,
			BeforeTokens: 42000,
			AfterTokens:  8000,
			Trigger:      conversation.TriggerManual,
		},
		Provider: providers.NewCodexCLI("codex", "", ""),
	}
	message, err := database.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		AgentID:   agentRow.ID,
		Text:      result.Body,
		Steps:     result.stepsJSON(),
	})
	if err != nil {
		t.Fatalf("persist reply: %v", err)
	}

	var steps []agent.TurnStep
	if err := json.Unmarshal([]byte(message.Steps), &steps); err != nil {
		t.Fatalf("parse persisted steps: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("persisted steps = %d, want 1: %s", len(steps), message.Steps)
	}
	step := steps[0]
	if step.Kind != agent.StepCompaction || step.Trigger != conversation.TriggerManual {
		t.Fatalf("unexpected compaction identity: %+v", step)
	}
	if step.Source != "tionharness" || step.Provider != "codex-cli" || step.SessionAction != "restart-summary" {
		t.Fatalf("unexpected compaction provenance: %+v", step)
	}
	if step.FoldedMsgs != 6 || step.BeforeTokens != 42000 || step.AfterTokens != 8000 {
		t.Fatalf("unexpected compaction metrics: %+v", step)
	}
}

func TestSummaryResultWithoutFoldHasNoSteps(t *testing.T) {
	if got := (summaryResult{Body: "nothing to fold"}).stepsJSON(); got != "[]" {
		t.Fatalf("steps = %s, want []", got)
	}
}

func TestSummaryResultNativeCompactionStepsTakePrecedence(t *testing.T) {
	step := agent.TurnStep{
		Kind: agent.StepCompaction, Trigger: conversation.TriggerManual,
		Source: "cli-native", Provider: "claude-cli", SessionAction: "native-compact",
	}
	got := (summaryResult{Steps: []agent.TurnStep{step}}).stepsJSON()
	var steps []agent.TurnStep
	if err := json.Unmarshal([]byte(got), &steps); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Source != "cli-native" || steps[0].SessionAction != "native-compact" {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestNativeCompactionStepPreservesLifecycleIdentity(t *testing.T) {
	running := nativeCompactionStep(providers.TraceStep{
		ID: "claude-compact-1", Running: true, Kind: "compaction",
		Source: "cli-native", Provider: "claude-cli", SessionAction: "native-compact",
	})
	completed := nativeCompactionStep(providers.TraceStep{
		ID: "claude-compact-1", Kind: "compaction",
		Source: "cli-native", Provider: "claude-cli", SessionAction: "native-compact",
	})
	tombstone := nativeCompactionStep(providers.TraceStep{Kind: "tombstone", Ref: "claude-compact-1"})

	if running.ID != completed.ID || !running.Running || completed.Running {
		t.Fatalf("lifecycle identity/state mismatch: running=%+v completed=%+v", running, completed)
	}
	if completed.Trigger != conversation.TriggerManual || completed.Source != "cli-native" || completed.Provider != "claude-cli" || completed.SessionAction != "native-compact" {
		t.Fatalf("completed provenance = %+v", completed)
	}
	if tombstone.Kind != agent.StepTombstone || tombstone.Ref != running.ID || tombstone.Trigger != "" {
		t.Fatalf("failure tombstone = %+v", tombstone)
	}
}

func TestNativeCompactionStepPersistsCLIProvenance(t *testing.T) {
	result := summaryResult{Steps: []agent.TurnStep{nativeCompactionStep(providers.TraceStep{
		ID: "cmp-1", Kind: "compaction", Source: "cli-native",
		Provider: "codex-cli", SessionAction: "native-compact",
	})}}
	var steps []agent.TurnStep
	if err := json.Unmarshal([]byte(result.stepsJSON()), &steps); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Trigger != conversation.TriggerManual || steps[0].Source != "cli-native" || steps[0].SessionAction != "native-compact" {
		t.Fatalf("native steps = %+v", steps)
	}
}

func TestCompactCommandHeadersSeparateNativeAndCustom(t *testing.T) {
	native, nativeOK := summaryHeader("compact")
	custom, customOK := summaryHeader("compact-custom")
	if !nativeOK || !customOK || native == custom {
		t.Fatalf("headers: native=%q/%v custom=%q/%v", native, nativeOK, custom, customOK)
	}
}
