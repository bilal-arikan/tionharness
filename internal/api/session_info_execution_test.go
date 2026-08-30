package api

import (
	"strconv"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestApplyExecutionObservability(t *testing.T) {
	resp := sessionInfoResp{}
	history := []db.Message{
		{Role: "user", DurationMs: 999, Steps: `[{"kind":"tool"}]`},
		{
			Role:       "assistant",
			DurationMs: 1250,
			StopReason: "end_turn",
			Usage:      &db.MessageUsage{InputTokens: 1200, OutputTokens: 340},
			Steps:      `[{"kind":"tool","input":{"token":"secret"},"output":"secret"},{"kind":"subagent","subSteps":[{"kind":"error","reason":"permission_denied","text":"secret detail"}]}]`,
		},
	}

	applyExecutionObservability(&resp, history)
	if resp.DurationMs != 1250 || resp.InputTokens != 1200 || resp.OutputTokens != 340 {
		t.Fatalf("turn totals = duration %d, input %d, output %d", resp.DurationMs, resp.InputTokens, resp.OutputTokens)
	}
	if resp.PersistedSteps != 3 {
		t.Fatalf("PersistedSteps = %d, want 3", resp.PersistedSteps)
	}
	if resp.StopReason != "end_turn" || resp.ErrorSummary != "permission_denied" {
		t.Fatalf("terminal summary = stop %q, error %q", resp.StopReason, resp.ErrorSummary)
	}
}

func TestApplyExecutionObservabilityReportsMalformedPersistedSteps(t *testing.T) {
	resp := sessionInfoResp{}
	applyExecutionObservability(&resp, []db.Message{{Role: "assistant", Steps: "{"}})
	if resp.ErrorSummary != "persisted_steps_invalid" {
		t.Fatalf("ErrorSummary = %q, want persisted_steps_invalid", resp.ErrorSummary)
	}
}

func TestApplyExecutionObservabilityDoesNotExposeUntrustedErrorReason(t *testing.T) {
	const secret = "api_key=secret-value"
	const toolPayload = `{"tool":"Bash","input":{"command":"printenv"},"output":"token"}`
	resp := sessionInfoResp{}
	applyExecutionObservability(&resp, []db.Message{{
		Role:  "assistant",
		Steps: `[{"kind":"error","reason":` + strconv.Quote(secret+" "+toolPayload) + `}]`,
	}})

	if resp.ErrorSummary != "persisted_step_error" {
		t.Fatalf("ErrorSummary = %q, want persisted_step_error", resp.ErrorSummary)
	}
	if strings.Contains(resp.ErrorSummary, secret) || strings.Contains(resp.ErrorSummary, toolPayload) {
		t.Fatalf("ErrorSummary exposed untrusted persisted content: %q", resp.ErrorSummary)
	}
}

func TestTerminalRunState(t *testing.T) {
	for _, state := range []string{"completed", "failed", "killed", "timeout", "incomplete"} {
		if !terminalRunState(state) {
			t.Errorf("terminalRunState(%q) = false", state)
		}
	}
	for _, state := range []string{"", "running"} {
		if terminalRunState(state) {
			t.Errorf("terminalRunState(%q) = true", state)
		}
	}
}
