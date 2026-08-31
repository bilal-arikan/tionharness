package api

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// A Codex turn.completed usage value aggregates every internal model round-trip
// in the agentic turn. The next request contains one composed thread instead:
// visible messages plus the retained tool calls/results. Pin the meter to that
// composed estimate and make the aggregate usage deliberately irrelevant.
func TestCodexMultiToolFillersMatchComposedRequestEstimate(t *testing.T) {
	steps := `[
		{"kind":"text","text":"checking both files"},
		{"kind":"tool","tool":"Bash","input":{"cmd":"first"},"output":"first result"},
		{"kind":"tool","tool":"Read","input":{"path":"second"},"output":"second result"}
	]`
	message := db.Message{
		Role:  "assistant",
		Text:  "done",
		Steps: steps,
		Usage: &db.MessageUsage{InputTokens: 900000, OutputTokens: 100000},
	}

	fillers, err := buildFillers("", []db.Message{message}, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := 0
	for _, filler := range fillers {
		got += filler.Tokens
	}
	stepTokens, _, err := conversation.EstimatePersistedSteps(steps)
	if err != nil {
		t.Fatal(err)
	}
	want := conversation.EstimateText(message.Text) + conversation.MsgOverhead + stepTokens
	if got != want {
		t.Fatalf("meter estimate = %d, composed request estimate = %d", got, want)
	}
	if got == message.Usage.InputTokens+message.Usage.OutputTokens {
		t.Fatal("meter used aggregate turn.completed usage")
	}
}

func TestBuildFillersRejectsMalformedRetainedSteps(t *testing.T) {
	_, err := buildFillers("", []db.Message{{Role: "assistant", Steps: "{"}}, 0)
	if err == nil {
		t.Fatal("malformed retained Steps must be observable")
	}
}
