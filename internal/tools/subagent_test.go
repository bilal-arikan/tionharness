package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestRunSubagentRejectsOutOfEnumAxes: an out-of-enum wait/context used to be
// lower-cased and then silently ignored — wait="background" ran SYNCHRONOUSLY and
// context="inherit" ran ISOLATED, the exact opposite of the request, with nothing
// reported. The tool boundary now refuses instead of defaulting.
func TestRunSubagentRejectsOutOfEnumAxes(t *testing.T) {
	tool := NewRunSubagentTool()
	// No runner attached: a call that clears validation fails with the
	// runner-missing error, which is what proves the enum check let it through
	// rather than short-circuiting everything.
	ctx := context.Background()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"unknown wait", `{"target":"explore","task":"x","wait":"background"}`, `"wait" must be one of sync, async`},
		{"unknown context", `{"target":"explore","task":"x","context":"inherit"}`, `"context" must be one of isolated, inherited`},
		{"upper-case wait passes", `{"target":"explore","task":"x","wait":"ASYNC"}`, "subagents are not available in this context"},
		{"unset axes pass", `{"target":"explore","task":"x"}`, "subagents are not available in this context"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.Call(ctx, json.RawMessage(tc.input))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// TestRunSubagentPassesValidAxesThrough: validation must not rewrite the spec the
// runner receives.
func TestRunSubagentPassesValidAxesThrough(t *testing.T) {
	var got RunAgentSpec
	ctx := WithRunAgent(context.Background(), func(_ context.Context, s RunAgentSpec) (RunAgentResult, error) {
		got = s
		return RunAgentResult{AgentName: "x"}, nil
	})
	if _, err := NewRunSubagentTool().Call(ctx, json.RawMessage(`{"target":"explore","task":"x","wait":"async","context":"inherited"}`)); err != nil {
		t.Fatalf("call: %v", err)
	}
	if got.Wait != "async" || got.Context != "inherited" {
		t.Fatalf("spec reached the runner as wait=%q context=%q", got.Wait, got.Context)
	}
}
