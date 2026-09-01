package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

type blockingOperationProvider struct{ release <-chan struct{} }

func (blockingOperationProvider) Name() string { return "test-blocking" }

func (p blockingOperationProvider) Complete(context.Context, providers.Request) (*providers.Response, error) {
	<-p.release
	return &providers.Response{Text: "late"}, nil
}

func waitForOperationLeaseCleanup(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for (len(operationLeaseAdmission) != 0 || DetachedOperationCount() != 0) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if slots, detached := len(operationLeaseAdmission), DetachedOperationCount(); slots != 0 || detached != 0 {
		t.Fatalf("operation lease cleanup incomplete: slots=%d detached=%d", slots, detached)
	}
}

func TestRunWithOperationLeasePreCancelledDoesNotInvokeOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	_, err := RunWithOperationLease(ctx, func(context.Context) (providers.ToolResult, error) {
		calls.Add(1)
		return providers.ToolResult{}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancelled", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("operation calls = %d, want 0", calls.Load())
	}
}

func TestRunWithOperationLeaseRecoversOperationPanic(t *testing.T) {
	result, err := RunWithOperationLease(context.Background(), func(context.Context) (string, error) {
		panic("provider exploded")
	})
	if result != "" {
		t.Fatalf("panic result = %q, want zero value", result)
	}
	var panicErr *OperationPanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("error = %T %v, want OperationPanicError", err, err)
	}
	if !strings.Contains(err.Error(), "provider exploded") || !strings.Contains(err.Error(), "TestRunWithOperationLeaseRecoversOperationPanic") {
		t.Fatalf("panic error lacks value or stack context: %v", err)
	}
}

func TestRunWithOperationLeasePanicReleasesAdmissionAndAccounting(t *testing.T) {
	waitForOperationLeaseCleanup(t)
	_, err := RunWithOperationLease(context.Background(), func(context.Context) (string, error) {
		panic("slot test")
	})
	var panicErr *OperationPanicError
	if !errors.As(err, &panicErr) {
		t.Fatalf("panic error = %v", err)
	}
	waitForOperationLeaseCleanup(t)
	result, err := RunWithOperationLease(context.Background(), func(context.Context) (string, error) {
		return "next admitted", nil
	})
	if err != nil || result != "next admitted" {
		t.Fatalf("next operation = %q, %v", result, err)
	}
}

func TestRunWithOperationLeaseCancellationWinsPanicResult(t *testing.T) {
	for i := 0; i < 20; i++ {
		parent, cancel := context.WithCancelCause(context.Background())
		ctx, stop := WithActivityTimeout(parent, 0, time.Second)
		started := make(chan struct{})
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			_, err := RunWithOperationLease(ctx, func(context.Context) (string, error) {
				close(started)
				<-release
				panic("late panic")
			})
			done <- err
		}()
		<-started
		cancel(ErrOperationLeaseTimeout)
		close(release)
		if err := <-done; !errors.Is(err, ErrOperationLeaseTimeout) {
			t.Fatalf("iteration %d: error = %v, want cancellation cause", i, err)
		}
		waitForOperationLeaseCleanup(t)
		stop()
	}
}

func TestRunWithOperationLeaseCancellationWinsReadyResult(t *testing.T) {
	for i := 0; i < 20; i++ {
		parent, cancel := context.WithCancelCause(context.Background())
		ctx, stop := WithActivityTimeout(parent, 0, time.Second)
		started := make(chan struct{})
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			result, err := RunWithOperationLease(ctx, func(context.Context) (string, error) {
				close(started)
				<-release
				return "late", nil
			})
			if result != "" {
				done <- errors.New("late result accepted")
				return
			}
			done <- err
		}()
		<-started
		cancel(ErrOperationLeaseTimeout)
		close(release)
		if err := <-done; !errors.Is(err, ErrOperationLeaseTimeout) {
			t.Fatalf("iteration %d: error = %v", i, err)
		}
		stop()
	}
}

func TestRunWithOperationLeaseAdmissionIsBoundedAndRecovers(t *testing.T) {
	release := make(chan struct{})
	var started atomic.Int32
	var callers sync.WaitGroup
	callers.Add(maxConcurrentOperationLeases)
	errs := make(chan error, maxConcurrentOperationLeases)
	for i := 0; i < maxConcurrentOperationLeases; i++ {
		go func() {
			defer callers.Done()
			ctx, stop := WithActivityTimeout(context.Background(), 0, 40*time.Millisecond)
			defer stop()
			_, err := RunWithOperationLease(ctx, func(context.Context) (string, error) {
				started.Add(1)
				<-release
				return "late", nil
			})
			errs <- err
		}()
	}
	deadline := time.Now().Add(time.Second)
	for started.Load() != maxConcurrentOperationLeases && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if started.Load() != maxConcurrentOperationLeases {
		close(release)
		t.Fatalf("started = %d, want %d", started.Load(), maxConcurrentOperationLeases)
	}
	callers.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, ErrOperationLeaseTimeout) {
			close(release)
			t.Fatalf("filled operation error = %v", err)
		}
	}

	startedAt := time.Now()
	_, err := RunWithOperationLease(context.Background(), func(context.Context) (string, error) {
		return "must not run", nil
	})
	if !errors.Is(err, ErrOperationLeaseBusy) {
		close(release)
		t.Fatalf("overflow error = %v, want admission error", err)
	}
	if time.Since(startedAt) > 50*time.Millisecond {
		close(release)
		t.Fatal("overflow operation did not fail fast")
	}

	close(release)
	deadline = time.Now().Add(time.Second)
	for len(operationLeaseAdmission) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := len(operationLeaseAdmission); got != 0 {
		t.Fatalf("admission slots did not recover: %d still held", got)
	}
	result, err := RunWithOperationLease(context.Background(), func(context.Context) (string, error) {
		return "recovered", nil
	})
	if err != nil || result != "recovered" {
		t.Fatalf("recovered operation = %q, %v", result, err)
	}
}

func TestRecordedCompleteOperationLeaseStopsStuckProvider(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rt, _ := newTestRuntime(t, workDir)
	row, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Lease", Provider: "test-blocking", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := WithActivityTimeout(context.Background(), 0, 80*time.Millisecond)
	defer stop()
	release := make(chan struct{})
	defer close(release)
	started := time.Now()
	resp, err := rt.recordedComplete(ctx, row, blockingOperationProvider{release: release}, providers.Request{Model: "test"})
	if !errors.Is(err, ErrOperationLeaseTimeout) {
		t.Fatalf("error = %v, want operation lease timeout", err)
	}
	if resp != nil {
		t.Fatalf("late response applied: %+v", resp)
	}
	if time.Since(started) > 200*time.Millisecond {
		t.Fatal("caller remained blocked after operation lease")
	}
}

func TestRunWithOperationLeaseDropsLateToolResult(t *testing.T) {
	ctx, stop := WithActivityTimeout(context.Background(), 0, 80*time.Millisecond)
	defer stop()
	release := make(chan struct{})
	var returned atomic.Bool
	result, err := RunWithOperationLease(ctx, func(context.Context) (providers.ToolResult, error) {
		<-release
		returned.Store(true)
		return providers.ToolResult{Content: "late tool output"}, nil
	})
	if !errors.Is(err, ErrOperationLeaseTimeout) {
		t.Fatalf("error = %v, want operation lease timeout", err)
	}
	if result.Content != "" {
		t.Fatalf("late tool result applied: %+v", result)
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for !returned.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !returned.Load() {
		t.Fatal("test cleanup could not release detached tool goroutine")
	}
}
