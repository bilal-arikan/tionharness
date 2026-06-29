package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParseAskInput covers every option/question shape a model might send: the
// native SwarmGo form, claude-cli's AskUserQuestion option objects (the SES73
// failure), a single scalar, a mixed array, and the native questions[] wrapper.
func TestParseAskInput(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantQ    string
		wantOpts []string
	}{
		{
			name:     "swarmgo string array",
			raw:      `{"question":"Pick one","options":["A","B"]}`,
			wantQ:    "Pick one",
			wantOpts: []string{"A", "B"},
		},
		{
			name:     "native option objects (content)",
			raw:      `{"question":"Approve?","options":[{"content":"Yes, apply"},{"content":"No"}]}`,
			wantQ:    "Approve?",
			wantOpts: []string{"Yes, apply", "No"},
		},
		{
			name:     "native option objects (label + description)",
			raw:      `{"question":"Q","options":[{"label":"Opt 1","description":"long"},{"label":"Opt 2"}]}`,
			wantQ:    "Q",
			wantOpts: []string{"Opt 1", "Opt 2"},
		},
		{
			name:     "single scalar option",
			raw:      `{"question":"Q","options":"only"}`,
			wantQ:    "Q",
			wantOpts: []string{"only"},
		},
		{
			name:     "mixed string and object",
			raw:      `{"question":"Q","options":["A",{"label":"B"}]}`,
			wantQ:    "Q",
			wantOpts: []string{"A", "B"},
		},
		{
			name:     "native questions wrapper",
			raw:      `{"questions":[{"question":"Wrapped?","header":"h","multiSelect":false,"options":[{"label":"X"}]}]}`,
			wantQ:    "Wrapped?",
			wantOpts: []string{"X"},
		},
		{
			name:     "no options",
			raw:      `{"question":"Just asking"}`,
			wantQ:    "Just asking",
			wantOpts: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, opts, err := ParseAskInput(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("ParseAskInput error: %v", err)
			}
			if q != tc.wantQ {
				t.Errorf("question = %q, want %q", q, tc.wantQ)
			}
			if strings.Join(opts, "|") != strings.Join(tc.wantOpts, "|") {
				t.Errorf("options = %v, want %v", opts, tc.wantOpts)
			}
		})
	}
}
