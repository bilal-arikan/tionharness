package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/conversation"
)

// TestCompactionLeadStep covers the budgeted (auto) fold surfaced at the head of
// a turn: the structural fields the compaction card renders, plus the Text
// headline older clients and already-persisted traces still rely on.
func TestCompactionLeadStep(t *testing.T) {
	step := compactionLeadStep(conversation.Compaction{
		FoldedMsgs:   11,
		BeforeTokens: 48000,
		AfterTokens:  9000,
		Trigger:      conversation.TriggerAuto,
	})

	if step.Kind != agent.StepCompaction {
		t.Fatalf("Kind = %q, want %q", step.Kind, agent.StepCompaction)
	}
	if step.FoldedMsgs != 11 {
		t.Errorf("FoldedMsgs = %d, want 11", step.FoldedMsgs)
	}
	if step.BeforeTokens != 48000 {
		t.Errorf("BeforeTokens = %d, want 48000", step.BeforeTokens)
	}
	if step.AfterTokens != 9000 {
		t.Errorf("AfterTokens = %d, want 9000", step.AfterTokens)
	}
	if step.Trigger != conversation.TriggerAuto {
		t.Errorf("Trigger = %q, want %q", step.Trigger, conversation.TriggerAuto)
	}
	if strings.TrimSpace(step.Text) == "" {
		t.Fatal("Text is empty; clients that predate the compaction kind would render nothing")
	}
	if !strings.Contains(step.Text, "11") {
		t.Errorf("Text does not carry the folded-message count: %q", step.Text)
	}
}

// A manual /compact fold goes through the same builder; only the trigger differs.
func TestCompactionLeadStepManualTrigger(t *testing.T) {
	step := compactionLeadStep(conversation.Compaction{FoldedMsgs: 3, Trigger: conversation.TriggerManual})

	if step.Kind != agent.StepCompaction {
		t.Fatalf("Kind = %q, want %q", step.Kind, agent.StepCompaction)
	}
	if step.Trigger != conversation.TriggerManual {
		t.Errorf("Trigger = %q, want %q", step.Trigger, conversation.TriggerManual)
	}
}
