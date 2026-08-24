package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestWake_ScheduleWakeArmsViaContext drives the self-wake path: with a wake
// scheduler wired into the turn context (as the chat layer does), the agent's
// schedule_wake call reaches the scheduler with the right delay/prompt and the
// scheduler's handle flows back as the tool result.
func TestWake_ScheduleWakeArmsViaContext(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Waiting for the job.", tc("c1", "schedule_wake", map[string]any{
			"delaySeconds": 30,
			"prompt":       "Check the scan results and report.",
			"reason":       "waiting for background scan",
		})),
		sayText("Paused — will resume after the wait."),
	)
	h := newHarness(t, prov)

	var gotDelay int
	var gotPrompt, gotReason string
	h.decorate = func(ctx context.Context) context.Context {
		return tools.WithWakeScheduler(ctx, func(_ context.Context, delay int, prompt, reason string) (string, error) {
			gotDelay, gotPrompt, gotReason = delay, prompt, reason
			return "wake armed in 30s", nil
		})
	}

	ag := h.newAgent("Waiter")
	sess := h.newSession(ag)

	res := h.send(ag, sess, "Wait for the scan, then report.")

	if gotDelay != 30 || gotPrompt != "Check the scan results and report." || gotReason != "waiting for background scan" {
		t.Fatalf("wake scheduler got (delay=%d, prompt=%q, reason=%q)", gotDelay, gotPrompt, gotReason)
	}
	step := findToolStep(res.steps, "schedule_wake")
	if step == nil {
		t.Fatalf("no schedule_wake step in trace: %+v", res.steps)
	}
	if step.IsError || !strings.Contains(step.Output, "wake armed") {
		t.Errorf("schedule_wake output = %q (err=%v)", step.Output, step.IsError)
	}
}

// TestWake_NoSchedulerOutsideChat verifies the guard: without a wake scheduler in
// context (an autonomous/headless turn), schedule_wake fails cleanly with a clear
// message rather than silently doing nothing.
func TestWake_NoSchedulerOutsideChat(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Trying to wait.", tc("c1", "schedule_wake", map[string]any{
			"delaySeconds": 10,
			"prompt":       "resume",
		})),
		sayText("Cannot wait here."),
	)
	h := newHarness(t, prov) // no decorate → no wake scheduler wired
	ag := h.newAgent("Lonely")
	sess := h.newSession(ag)

	res := h.send(ag, sess, "Please wait then continue.")

	step := findToolStep(res.steps, "schedule_wake")
	if step == nil {
		t.Fatalf("no schedule_wake step in trace: %+v", res.steps)
	}
	if !step.IsError || !strings.Contains(step.Output, "interactive chat turn") {
		t.Errorf("expected a clear no-scheduler error, got %q (err=%v)", step.Output, step.IsError)
	}
}
