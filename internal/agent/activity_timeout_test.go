package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestActivityTimeoutIdleCancels(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 0, 40*time.Millisecond)
	defer stop()
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), ErrTurnIdleTimeout) {
			t.Fatalf("cause = %v, want idle timeout", context.Cause(ctx))
		}
	case <-time.After(time.Second):
		t.Fatal("idle watchdog did not cancel a silent run")
	}
}

func TestSemanticProgressSurvivesPastLegacyHardLimit(t *testing.T) {
	const (
		legacyHardLimit = 5 * time.Millisecond
		idle            = 10 * time.Millisecond
		runFor          = 20 * time.Millisecond
	)
	base := time.Unix(1_700_000_000, 0)
	now := base
	ctx, cancel := context.WithCancelCause(context.Background())
	tracker := &ActivityTracker{
		last:    ActivitySnapshot{LastProgressAt: base, Kind: "run_started"},
		sources: make(map[string]sourceActivity),
		idle:    idle,
		cancel:  cancel,
		now:     func() time.Time { return now },
	}

	// Advance beyond the removed hard ceiling while each semantic update remains
	// inside the idle window. No wall clock or scheduler timing participates.
	for elapsed := 4 * time.Millisecond; elapsed <= runFor; elapsed += 4 * time.Millisecond {
		now = base.Add(elapsed)
		if !tracker.ObserveStep(TurnStep{Kind: StepDelta, ID: "stream", Text: elapsed.String()}) {
			t.Fatalf("semantic progress at %v was not recorded", elapsed)
		}
		remaining, expired := tracker.expireIfIdle()
		if expired || remaining != idle {
			t.Fatalf("productive run expired at %v: remaining=%v expired=%v", elapsed, remaining, expired)
		}
	}
	if runFor <= legacyHardLimit {
		t.Fatalf("test invariant: run %v must exceed legacy hard limit %v", runFor, legacyHardLimit)
	}
	select {
	case <-ctx.Done():
		t.Fatalf("productive run was cancelled: %v", context.Cause(ctx))
	default:
	}
}

func TestSemanticProgressNoProgressCancelsAtIdle(t *testing.T) {
	const idle = 240 * time.Millisecond
	ctx, stop := WithChatActivityTimeout(context.Background(), 0, idle)
	defer stop()
	started := time.Now()
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), ErrTurnIdleTimeout) {
			t.Fatalf("cause = %v, want idle timeout", context.Cause(ctx))
		}
		if elapsed := time.Since(started); elapsed < idle {
			t.Fatalf("idle watchdog fired early: %v < %v", elapsed, idle)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("silent run was not cancelled at idle deadline")
	}
}

func TestSemanticTrackerRejectsEmptyDuplicateRunningAndTombstone(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 0, time.Second)
	defer stop()
	tracker := ActivityTrackerFrom(ctx)
	if tracker.ObserveStep(TurnStep{Kind: StepDelta}) {
		t.Fatal("empty delta counted as progress")
	}
	step := TurnStep{Kind: StepDelta, Text: "new"}
	if !tracker.ObserveStep(step) {
		t.Fatal("new non-empty delta did not count")
	}
	seq := tracker.Snapshot().Sequence
	for _, duplicate := range []TurnStep{
		step,
		{Kind: StepTool, ID: "tool-1", Running: true},
		{Kind: StepTool, ID: "tool-1", Running: true},
		{Kind: StepTombstone, Ref: "tool-1"},
	} {
		tracker.ObserveStep(duplicate)
	}
	if got := tracker.Snapshot().Sequence; got != seq {
		t.Fatalf("noise advanced sequence: got %d want %d", got, seq)
	}
}

func TestSemanticTrackerTerminalTransitionCountsOnce(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 0, time.Second)
	defer stop()
	tracker := ActivityTrackerFrom(ctx)
	terminal := TurnStep{Kind: StepTool, ID: "tool-1", Tool: "Bash", Output: "done"}
	if !tracker.ObserveStep(terminal) {
		t.Fatal("tool completion did not count")
	}
	if tracker.ObserveStep(terminal) {
		t.Fatal("duplicate tool completion counted twice")
	}
	snap := tracker.Snapshot()
	if snap.Kind != "tool_terminal" || snap.Sequence != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestSemanticTrackerCountsNewToolAndChildOutput(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 0, time.Second)
	defer stop()
	tracker := ActivityTrackerFrom(ctx)
	chunk := TurnStep{Kind: StepTool, ID: "tool-1", Running: true, Append: true, Output: "chunk"}
	if !tracker.ObserveStep(chunk) || tracker.ObserveStep(chunk) {
		t.Fatal("new tool chunk must count exactly once")
	}
	child := TurnStep{Kind: StepSubagent, ID: "child-1", SubSteps: []TurnStep{{Kind: StepDelta, Text: "work"}}}
	if !tracker.ObserveStep(child) {
		t.Fatal("verified child progress did not count")
	}
}

func TestSemanticTrackerDeduplicatesAlternatingSources(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 0, time.Second)
	defer stop()
	tracker := ActivityTrackerFrom(ctx)
	a := TurnStep{Kind: StepToolDelta, ID: "A", Output: "same"}
	b := TurnStep{Kind: StepToolDelta, ID: "B", Output: "same"}
	if !tracker.ObserveStep(a) || !tracker.ObserveStep(b) {
		t.Fatal("first output from each source must count")
	}
	seq := tracker.Snapshot().Sequence
	for i := 0; i < 10; i++ {
		tracker.ObserveStep(a)
		tracker.ObserveStep(b)
	}
	if got := tracker.Snapshot().Sequence; got != seq {
		t.Fatalf("alternating duplicates advanced sequence: got %d want %d", got, seq)
	}
}

func TestSemanticTrackerSubagentRequiresVerifiedProgress(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 0, time.Second)
	defer stop()
	tracker := ActivityTrackerFrom(ctx)
	for _, noise := range []TurnStep{
		{Kind: StepSubagent, ID: "child", Running: true},
		{Kind: StepSubagent, ID: "child", Running: true, SubSteps: []TurnStep{{Kind: StepTool, Running: true}}},
	} {
		if tracker.ObserveStep(noise) {
			t.Fatalf("running/empty child snapshot counted: %+v", noise)
		}
	}
	output := TurnStep{Kind: StepSubagent, ID: "child", Running: true, SubSteps: []TurnStep{{Kind: StepDelta, Text: "new output"}}}
	if !tracker.ObserveStep(output) || tracker.ObserveStep(output) {
		t.Fatal("new child output must count exactly once")
	}
	terminal := TurnStep{Kind: StepSubagent, ID: "child", Status: "completed", SubSteps: output.SubSteps}
	if !tracker.ObserveStep(terminal) || tracker.ObserveStep(terminal) {
		t.Fatal("child terminal transition must count exactly once")
	}
}

func TestSemanticTrackerBoundsSourceState(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 0, time.Second)
	defer stop()
	tracker := ActivityTrackerFrom(ctx)
	for i := 0; i < maxActivitySources+50; i++ {
		tracker.ObserveStep(TurnStep{Kind: StepToolDelta, ID: time.Duration(i).String(), Output: "chunk"})
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if len(tracker.sources) != maxActivitySources || len(tracker.order) != maxActivitySources {
		t.Fatalf("dedup state grew past cap: sources=%d order=%d", len(tracker.sources), len(tracker.order))
	}
}

func TestActivityTimerResetRaceDoesNotCancelFreshProgress(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	tracker := &ActivityTracker{
		last:   ActivitySnapshot{LastProgressAt: time.Now().Add(-time.Hour), Kind: "stale"},
		idle:   time.Minute,
		cancel: cancel,
	}

	// Model a fired timer callback waiting on the tracker lock. Record fresh
	// progress while holding that same lock, then let the stale callback re-check
	// the timestamp. No scheduler timing margin is involved in this interleaving.
	tracker.mu.Lock()
	callbackStarted := make(chan struct{})
	type expiryResult struct {
		remaining time.Duration
		expired   bool
	}
	result := make(chan expiryResult, 1)
	go func() {
		close(callbackStarted)
		remaining, expired := tracker.expireIfIdle()
		result <- expiryResult{remaining: remaining, expired: expired}
	}()
	<-callbackStarted
	tracker.last.Sequence++
	tracker.last.LastProgressAt = time.Now()
	tracker.last.Kind = "assistant_content"
	tracker.mu.Unlock()

	got := <-result
	if got.expired || got.remaining <= 0 {
		t.Fatalf("stale callback treated fresh progress as expired: %+v", got)
	}
	select {
	case <-ctx.Done():
		t.Fatalf("stale timer cancelled fresh progress: %v", context.Cause(ctx))
	default:
	}
}

func TestOperationLeaseStopsOpaqueCall(t *testing.T) {
	ctx := context.WithValue(context.Background(), operationLeaseKey{}, 25*time.Millisecond)
	leaseCtx, release := WithOperationLease(ctx)
	defer release()
	<-leaseCtx.Done()
	if !errors.Is(context.Cause(leaseCtx), ErrOperationLeaseTimeout) {
		t.Fatalf("cause = %v, want operation lease", context.Cause(leaseCtx))
	}
}

func TestActivityStopIsConcurrentAndIdempotent(t *testing.T) {
	_, stop := WithActivityTimeout(context.Background(), 0, time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stop()
		}()
	}
	wg.Wait()
}

func TestSpawnIdleTimeoutTunable(t *testing.T) {
	var tun Tunables
	if got := tun.SpawnIdleTimeout(); got != time.Duration(DefaultSpawnIdleTimeoutMinutes)*time.Minute {
		t.Fatalf("default idle = %v", got)
	}
	tun.SetSpawnIdleTimeoutMinutes(12)
	if got := tun.SpawnIdleTimeout(); got != 12*time.Minute {
		t.Fatalf("override idle = %v", got)
	}
}
