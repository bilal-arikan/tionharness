package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestRunSubagentRejectsOutOfEnumAxes: an out-of-enum context used to be
// lower-cased and then silently ignored — context="inherit" ran ISOLATED, the
// exact opposite of the request, with nothing reported. The tool boundary now
// refuses instead of defaulting.
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
		{"unknown context", `{"target":"explore","task":"x","context":"inherit"}`, `"context" must be one of isolated, inherited`},
		{"upper-case context passes", `{"target":"explore","task":"x","context":"INHERITED"}`, "subagents are not available in this context"},
		{"unset axes pass", `{"target":"explore","task":"x"}`, "subagents are not available in this context"},
		// "wait" is a dead axis: the historical "sync" value stays accepted so a
		// frozen prompt keeps working, "async" is refused rather than downgraded.
		{"wait sync passes", `{"target":"explore","task":"x","wait":"sync"}`, "subagents are not available in this context"},
		{"wait async refused", `{"target":"explore","task":"x","wait":"async"}`, "always synchronous"},
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
	if _, err := NewRunSubagentTool().Call(ctx, json.RawMessage(`{"target":"explore","task":"x","context":"inherited"}`)); err != nil {
		t.Fatalf("call: %v", err)
	}
	if got.Context != "inherited" {
		t.Fatalf("spec reached the runner as context=%q", got.Context)
	}
}

// TestRunSubagentWaitSyncIsIgnored: the deprecated axis must not reach the runner
// in any shape — a call carrying wait:"sync" produces the same spec as one that
// omits it.
func TestRunSubagentWaitSyncIsIgnored(t *testing.T) {
	call := func(input string) RunAgentSpec {
		t.Helper()
		var got RunAgentSpec
		ctx := WithRunAgent(context.Background(), func(_ context.Context, s RunAgentSpec) (RunAgentResult, error) {
			got = s
			return RunAgentResult{AgentName: "x"}, nil
		})
		if _, err := NewRunSubagentTool().Call(ctx, json.RawMessage(input)); err != nil {
			t.Fatalf("call: %v", err)
		}
		return got
	}
	withWait := call(`{"target":"explore","task":"x","wait":"sync"}`)
	without := call(`{"target":"explore","task":"x"}`)
	// DeepEqual rather than ==: RunAgentSpec carries the fan-out task slice and is
	// no longer a comparable struct.
	if !reflect.DeepEqual(withWait, without) {
		t.Fatalf("wait:\"sync\" changed the spec: %+v vs %+v", withWait, without)
	}
}

// TestFormatRunAgentResultIsSyncOnly: the rendering has a single shape — the
// finished reply. There is no detached-run branch announcing a session id to poll.
func TestFormatRunAgentResultIsSyncOnly(t *testing.T) {
	out, err := FormatRunAgentResult(RunAgentResult{AgentName: "explore", Reply: "done"})
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if !strings.Contains(out, `Result from subagent "explore"`) || !strings.Contains(out, "done") {
		t.Fatalf("unexpected rendering: %q", out)
	}
	for _, banned := range []string{"async", "background", "session id"} {
		if strings.Contains(strings.ToLower(out), banned) {
			t.Fatalf("rendering still mentions %q: %q", banned, out)
		}
	}
}
