package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// The retained-trace term is a property of the THREAD, not of the provider name:
// a CLI resume ships only the delta after CLISentMsgCount, so a warm thread still
// holds the earlier tool calls while a cold turn replays the composed history and
// retains nothing.
func TestHasWarmCLIThread(t *testing.T) {
	cases := []struct {
		name    string
		session db.Session
		want    bool
	}{
		{"cold: no cli session", db.Session{}, false},
		{"cold: id but nothing sent", db.Session{CLISessionID: "cli-1"}, false},
		{"cold: sent count without id", db.Session{CLISentMsgCount: 4}, false},
		{"warm", db.Session{CLISessionID: "cli-1", CLISentMsgCount: 4}, true},
	}
	for _, tc := range cases {
		if got := hasWarmCLIThread(tc.session); got != tc.want {
			t.Errorf("%s: hasWarmCLIThread = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A cold turn must not be billed for a trace the provider is not holding, and a
// warm one must be — the gap that let the meter and the fold gate disagree.
func TestBuildFillersCountsRetainedTraceOnlyWhenWarm(t *testing.T) {
	steps := `[{"kind":"tool","tool":"Bash","input":{"cmd":"ls"},"output":"` + strings.Repeat("x ", 400) + `"}]`
	msgs := []db.Message{{Role: "assistant", Text: "done", Steps: steps}}

	sum := func(stepsFrom int) int {
		fillers, err := buildFillers("", msgs, stepsFrom)
		if err != nil {
			t.Fatal(err)
		}
		total := 0
		for _, f := range fillers {
			total += f.Tokens
		}
		return total
	}

	textOnly := conversation.EstimateText("done") + conversation.MsgOverhead
	if got := sum(-1); got != textOnly {
		t.Fatalf("cold total = %d, want %d (text only)", got, textOnly)
	}
	stepTokens, _, err := conversation.EstimatePersistedSteps(steps)
	if err != nil {
		t.Fatal(err)
	}
	if got := sum(0); got != textOnly+stepTokens {
		t.Fatalf("warm total = %d, want %d (text + retained trace)", got, textOnly+stepTokens)
	}
}
