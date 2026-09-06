package agent

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Timeout causes are attached with context cancellation causes so callers can
// distinguish watchdog cuts from an explicit user stop.
var (
	ErrTurnHardTimeout       = errors.New("turn hit its wall-clock ceiling")
	ErrTurnIdleTimeout       = errors.New("turn made no meaningful progress within the inactivity window")
	ErrOperationLeaseTimeout = errors.New("provider or tool operation exceeded its progress lease")
	ErrOperationLeaseBusy    = errors.New("provider or tool operation admission limit reached")
)

// ActivitySnapshot is a race-safe view of one run's semantic progress.
type ActivitySnapshot struct {
	LastProgressAt time.Time
	Kind           string
	Sequence       uint64
}

type activityTrackerKey struct{}
type operationLeaseKey struct{}

const maxActivitySources = 512

const maxConcurrentOperationLeases = 64

var operationLeaseAdmission = make(chan struct{}, maxConcurrentOperationLeases)

type sourceActivity struct {
	fingerprint string
	childCount  int
	terminal    string
}

// ActivityTracker owns one run's idle deadline. Only semantic progress advances
// it; transport heartbeat/keepalive and unrelated hub traffic never reach it.
type ActivityTracker struct {
	mu       sync.Mutex
	last     ActivitySnapshot
	sources  map[string]sourceActivity
	order    []string
	idle     time.Duration
	wake     chan struct{}
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	cancel   context.CancelCauseFunc
	ctx      context.Context
	now      func() time.Time
}

func newActivityTracker(ctx context.Context, cancel context.CancelCauseFunc, idle time.Duration) *ActivityTracker {
	now := time.Now
	t := &ActivityTracker{
		last:    ActivitySnapshot{LastProgressAt: now(), Kind: "run_started"},
		sources: make(map[string]sourceActivity),
		idle:    idle,
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		cancel:  cancel,
		ctx:     ctx,
		now:     now,
	}
	if idle > 0 {
		go t.watch()
	} else {
		close(t.done)
	}
	return t
}

func (t *ActivityTracker) watch() {
	defer close(t.done)
	timer := time.NewTimer(t.idle)
	defer timer.Stop()
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-t.stop:
			return
		case <-t.wake:
			resetTimer(timer, t.remaining())
		case <-timer.C:
			remaining, expired := t.expireIfIdle()
			if expired {
				// Re-checking last progress after the timer fires prevents a stale
				// callback racing with a new progress event from cancelling the run.
				return
			}
			resetTimer(timer, remaining)
		}
	}
}

func (t *ActivityTracker) expireIfIdle() (time.Duration, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	remaining := t.idle - t.nowTime().Sub(t.last.LastProgressAt)
	if remaining > 0 {
		return remaining, false
	}
	// Keep the semantic timestamp check and cancellation linearized with
	// Progress. Once this lock is released, no stale timer can cancel progress
	// that won the race and updated the snapshot first.
	t.cancel(ErrTurnIdleTimeout)
	return 0, true
}

func resetTimer(timer *time.Timer, d time.Duration) {
	if d <= 0 {
		d = time.Nanosecond
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(d)
}

func (t *ActivityTracker) remaining() time.Duration {
	t.mu.Lock()
	last := t.last.LastProgressAt
	t.mu.Unlock()
	return t.idle - t.nowTime().Sub(last)
}

func (t *ActivityTracker) nowTime() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// Progress records a verified state transition or newly produced content.
func (t *ActivityTracker) Progress(kind string) {
	if t == nil || strings.TrimSpace(kind) == "" {
		return
	}
	t.mu.Lock()
	t.last.Sequence++
	t.last.LastProgressAt = t.nowTime()
	t.last.Kind = kind
	t.mu.Unlock()
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// ObserveStep applies the semantic activity contract to a TurnStep. Empty and
// duplicate deltas, repeated running frames and tombstones are intentionally no-op.
func (t *ActivityTracker) ObserveStep(st TurnStep) bool {
	if t == nil || st.Kind == StepTombstone {
		return false
	}
	payload := strings.TrimSpace(st.Text)
	if st.Kind == StepToolDelta {
		payload = strings.TrimSpace(st.Output)
	}
	source := activitySource(st)
	sig := activityFingerprint(st, payload)

	t.mu.Lock()
	state, seen := t.sources[source]
	meaningful := false
	kind := ""
	switch st.Kind {
	case StepDelta, StepText, StepThinking:
		meaningful = payload != "" && (!seen || sig != state.fingerprint)
		kind = "assistant_content"
	case StepToolDelta:
		meaningful = payload != "" && (!seen || sig != state.fingerprint)
		kind = "tool_output"
	case StepSubagent:
		meaningful = meaningfulSubagentProgress(st, state)
		kind = "child_progress"
	case StepTool:
		if st.Append {
			meaningful = strings.TrimSpace(st.Output) != "" && (!seen || sig != state.fingerprint)
			kind = "tool_output"
		} else if !st.Running {
			meaningful = source != "" && state.terminal != sig
			kind = "tool_terminal"
		}
	}
	if meaningful {
		state.fingerprint = sig
		state.childCount = len(st.SubSteps)
		if (st.Kind == StepTool && !st.Running) || subagentTerminal(st) {
			state.terminal = sig
		}
		t.rememberSource(source, state, seen)
		t.last.Sequence++
		t.last.LastProgressAt = t.nowTime()
		t.last.Kind = kind
	}
	t.mu.Unlock()
	if meaningful {
		select {
		case t.wake <- struct{}{}:
		default:
		}
	}
	return meaningful
}

func activitySource(st TurnStep) string {
	id := st.ID
	if id == "" {
		id = st.Tool
	}
	if id == "" {
		id = string(st.Kind)
	}
	return string(st.Kind) + "\x00" + id
}

func activityFingerprint(st TurnStep, payload string) string {
	childTail := ""
	if len(st.SubSteps) > 0 {
		last := st.SubSteps[len(st.SubSteps)-1]
		childTail = fmt.Sprintf("%s|%s|%s|%s|%t|%s", last.Kind, last.ID, strings.TrimSpace(last.Text), strings.TrimSpace(last.Output), last.Running, last.Status)
	}
	return fmt.Sprintf("%s|%s|%s|%t|%t|%s|%d|%s", st.Kind, payload, strings.TrimSpace(st.Output), st.Running, st.Append, st.Status, len(st.SubSteps), childTail)
}

func meaningfulSubagentProgress(st TurnStep, previous sourceActivity) bool {
	if subagentTerminal(st) && previous.terminal != activityFingerprint(st, strings.TrimSpace(st.Text)) {
		return true
	}
	if len(st.SubSteps) <= previous.childCount {
		return false
	}
	for _, child := range st.SubSteps[previous.childCount:] {
		if strings.TrimSpace(child.Text) != "" || strings.TrimSpace(child.Output) != "" || (!child.Running && child.Status != "") {
			return true
		}
	}
	return false
}

func subagentTerminal(st TurnStep) bool {
	return !st.Running && st.Status != ""
}

func (t *ActivityTracker) rememberSource(source string, state sourceActivity, seen bool) {
	if !seen {
		if len(t.order) >= maxActivitySources {
			oldest := t.order[0]
			t.order = t.order[1:]
			delete(t.sources, oldest)
		}
		t.order = append(t.order, source)
	}
	t.sources[source] = state
}

func (t *ActivityTracker) Snapshot() ActivitySnapshot {
	if t == nil {
		return ActivitySnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.last
}

func (t *ActivityTracker) Stop() {
	if t == nil {
		return
	}
	t.stopOnce.Do(func() { close(t.stop) })
	<-t.done
}

// ActivityTrackerFrom returns the run-scoped tracker, when a watchdog is installed.
func ActivityTrackerFrom(ctx context.Context) *ActivityTracker {
	t, _ := ctx.Value(activityTrackerKey{}).(*ActivityTracker)
	return t
}

// ObserveActivityStep reports one step to the run-scoped semantic tracker.
func ObserveActivityStep(ctx context.Context, st TurnStep) bool {
	t := ActivityTrackerFrom(ctx)
	return t != nil && t.ObserveStep(st)
}

// WithOperationLease bounds one opaque non-streaming provider/tool operation.
// Its completion is progress; periodic heartbeat cannot extend this lease.
func WithOperationLease(ctx context.Context) (context.Context, func()) {
	d, _ := ctx.Value(operationLeaseKey{}).(time.Duration)
	if d <= 0 {
		return ctx, func() {}
	}
	leaseCtx, cancel := context.WithTimeoutCause(ctx, d, ErrOperationLeaseTimeout)
	return leaseCtx, cancel
}

var detachedOperations atomic.Int64

type operationResult[T any] struct {
	value T
	err   error
}

// OperationPanicError preserves both the recovered value and the child
// goroutine stack. Opaque provider/tool panics become ordinary lease results
// without being silently discarded or crashing the process.
type OperationPanicError struct {
	Value any
	Stack []byte
}

func (e *OperationPanicError) Error() string {
	return fmt.Sprintf("provider or tool operation panicked: %v\n%s", e.Value, e.Stack)
}

// RunWithOperationLease releases the caller even when an in-process dependency
// ignores context cancellation. The buffered channel lets a late result exit;
// detached work is measured until it eventually returns.
func RunWithOperationLease[T any](ctx context.Context, operation func(context.Context) (T, error)) (T, error) {
	var zero T
	if cause := context.Cause(ctx); cause != nil {
		return zero, cause
	}
	select {
	case operationLeaseAdmission <- struct{}{}:
	case <-ctx.Done():
		return zero, context.Cause(ctx)
	default:
		return zero, ErrOperationLeaseBusy
	}
	leaseDuration, _ := ctx.Value(operationLeaseKey{}).(time.Duration)
	startedAt := time.Now()
	leaseCtx, stop := WithOperationLease(ctx)
	defer stop()
	result := make(chan operationResult[T], 1)
	var state atomic.Int32 // 0 running, 1 detached, 2 finished
	go func() {
		outcome := operationResult[T]{}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					outcome.err = &OperationPanicError{Value: recovered, Stack: debug.Stack()}
				}
			}()
			outcome.value, outcome.err = operation(leaseCtx)
		}()
		<-operationLeaseAdmission
		if state.Swap(2) == 1 {
			detachedOperations.Add(-1)
		}
		result <- outcome
	}()
	select {
	case outcome := <-result:
		// A result racing with cancellation/deadline is never accepted after the
		// boundary, even if select happened to choose the buffered result first.
		if cause := operationLeaseCause(leaseCtx, startedAt, leaseDuration); cause != nil {
			return zero, cause
		}
		return outcome.value, outcome.err
	case <-leaseCtx.Done():
		cause := operationLeaseCause(leaseCtx, startedAt, leaseDuration)
		if state.CompareAndSwap(0, 1) {
			detachedOperations.Add(1)
		}
		return zero, cause
	}
}

func operationLeaseCause(ctx context.Context, startedAt time.Time, leaseDuration time.Duration) error {
	cause := context.Cause(ctx)
	if cause == nil && leaseDuration > 0 && time.Since(startedAt) >= leaseDuration {
		cause = ErrOperationLeaseTimeout
	}
	if cause == nil {
		return nil
	}
	// The run-idle timer and the shorter operation timer share Go's timer
	// scheduler. Preserve the earlier typed lease diagnosis once its own logical
	// deadline has elapsed, regardless of callback scheduling order.
	if errors.Is(cause, ErrTurnIdleTimeout) && leaseDuration > 0 && time.Since(startedAt) >= leaseDuration {
		return ErrOperationLeaseTimeout
	}
	return cause
}

func DetachedOperationCount() int64 { return detachedOperations.Load() }

func withActivityTimeout(parent context.Context, hard, idle time.Duration) (context.Context, func()) {
	return WithActivityTimeout(parent, hard, idle)
}

// WithChatActivityTimeout installs semantic idle tracking. The legacy hard
// parameter remains for source compatibility but never limits a chat run.
func WithChatActivityTimeout(parent context.Context, hard, idle time.Duration) (context.Context, func()) {
	_ = hard // legacy source compatibility; normal chat has no wall-clock ceiling
	return WithActivityTimeout(parent, 0, idle)
}

// WithActivityTimeout installs optional absolute and semantic-idle cancellation.
// The idle duration also bounds each opaque operation as a separate lease.
func WithActivityTimeout(parent context.Context, hard, idle time.Duration) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	var hardTimer *time.Timer
	if hard > 0 {
		hardTimer = time.AfterFunc(hard, func() { cancel(ErrTurnHardTimeout) })
	}
	tracker := newActivityTracker(ctx, cancel, idle)
	ctx = context.WithValue(ctx, activityTrackerKey{}, tracker)
	operationLease := idle
	if idle > 0 {
		// Fire the operation-specific diagnosis before the enclosing run-idle
		// deadline. Both remain derived from the same backwards-compatible knob.
		operationLease = idle * 3 / 4
		if operationLease <= 0 {
			operationLease = idle
		}
	}
	ctx = context.WithValue(ctx, operationLeaseKey{}, operationLease)
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			if hardTimer != nil {
				hardTimer.Stop()
			}
			cancel(context.Canceled)
			tracker.Stop()
		})
	}
}
