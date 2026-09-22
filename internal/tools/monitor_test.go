package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSource is a scripted MonitorSource: each Poll pops one batch off the queue.
// Once the queue is empty it reports done, so a test never spins.
type fakeSource struct {
	mu      sync.Mutex
	batches [][]string
	done    bool
	reason  string
	err     error
	closed  int
}

func (f *fakeSource) Poll(context.Context) ([]MonitorEvent, bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.batches) == 0 {
		return nil, f.done, f.reason, f.err
	}
	batch := f.batches[0]
	f.batches = f.batches[1:]
	evs := make([]MonitorEvent, 0, len(batch))
	for _, line := range batch {
		evs = append(evs, MonitorEvent{At: time.Now(), Payload: line})
	}
	return evs, false, "", f.err
}

func (f *fakeSource) Describe() string { return "fake" }

func (f *fakeSource) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
}

func (f *fakeSource) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// recordingWake captures the wakes a monitor delivers.
type recordingWake struct {
	mu      sync.Mutex
	prompts []string
	err     error
}

func (w *recordingWake) fn(_ context.Context, prompt, _ string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return "", w.err
	}
	w.prompts = append(w.prompts, prompt)
	return fmt.Sprintf("SCH%d", len(w.prompts)), nil
}

func (w *recordingWake) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.prompts)
}

func (w *recordingWake) last() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.prompts) == 0 {
		return ""
	}
	return w.prompts[len(w.prompts)-1]
}

// TestMonitorMatchWakes pins the core contract: only a line matching the filter
// wakes the agent, and the wake prompt carries the matching payload.
func TestMonitorMatchWakes(t *testing.T) {
	wake := &recordingWake{}
	m := NewMonitorManager(wake.fn)
	defer m.Close()
	src := &fakeSource{batches: [][]string{
		{"starting up", "all good"},
		{"ERROR: disk full"},
	}}
	if _, err := m.Start(src, "ERROR", time.Millisecond, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx := context.Background()
	m.tick(ctx) // non-matching batch
	if wake.count() != 0 {
		t.Fatalf("non-matching output woke the agent: %q", wake.last())
	}
	m.tick(ctx) // matching batch
	if wake.count() != 1 {
		t.Fatalf("want 1 wake after a match, got %d", wake.count())
	}
	if !strings.Contains(wake.last(), "ERROR: disk full") {
		t.Fatalf("wake prompt lost the payload: %q", wake.last())
	}
}

// TestMonitorTerminalStateReportedOnce guards that a source that can no longer
// produce ends the monitor exactly once: the source is closed a single time and
// further ticks neither poll it again nor wake.
func TestMonitorTerminalStateReportedOnce(t *testing.T) {
	wake := &recordingWake{}
	m := NewMonitorManager(wake.fn)
	defer m.Close()
	src := &fakeSource{done: true, reason: "process exited"}
	id, err := m.Start(src, "anything", time.Millisecond, 0)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx := context.Background()
	m.tick(ctx)
	m.tick(ctx)
	m.tick(ctx)

	if got := src.closeCount(); got != 1 {
		t.Fatalf("terminal source closed %d times, want exactly 1", got)
	}
	list := m.List()
	if !strings.Contains(list, string(monitorDone)) || !strings.Contains(list, "process exited") {
		t.Fatalf("terminal state/reason missing from list: %q", list)
	}
	// Stopping a monitor that already ended is a no-op success, not an error.
	out, err := m.Stop(id)
	if err != nil {
		t.Fatalf("Stop on a finished monitor: %v", err)
	}
	if !strings.Contains(out, "already") {
		t.Fatalf("want an already-ended note, got %q", out)
	}
}

// TestMonitorStop covers both stop paths: a stopped monitor no longer wakes, and
// an unknown id is an actionable error rather than a silent success.
func TestMonitorStop(t *testing.T) {
	wake := &recordingWake{}
	m := NewMonitorManager(wake.fn)
	defer m.Close()
	src := &fakeSource{batches: [][]string{{"hit"}, {"hit"}}}
	id, err := m.Start(src, "hit", time.Millisecond, 0)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := m.Stop(id); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	m.tick(context.Background())
	if wake.count() != 0 {
		t.Fatalf("a stopped monitor still woke the agent (%d times)", wake.count())
	}
	if _, err := m.Stop("mon-nope"); err == nil {
		t.Fatal("stopping an unknown monitor must be an error")
	}
}

// TestMonitorMaxFiresDisarms pins that max_fires ends the monitor with a reason
// once the budget is spent.
func TestMonitorMaxFiresDisarms(t *testing.T) {
	wake := &recordingWake{}
	m := NewMonitorManager(wake.fn)
	defer m.Close()
	src := &fakeSource{batches: [][]string{{"hit"}, {"hit"}, {"hit"}}}
	if _, err := m.Start(src, "hit", time.Nanosecond, 1); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx := context.Background()
	m.tick(ctx)
	m.tick(ctx)
	m.tick(ctx)
	if wake.count() != 1 {
		t.Fatalf("max_fires=1 delivered %d wakes", wake.count())
	}
	if list := m.List(); !strings.Contains(list, "max_fires") {
		t.Fatalf("disarm reason missing from list: %q", list)
	}
}

// TestMonitorLiveCap pins the per-session brake on armed monitors.
func TestMonitorLiveCap(t *testing.T) {
	m := NewMonitorManager((&recordingWake{}).fn)
	defer m.Close()
	for i := range monitorMaxLive {
		if _, err := m.Start(&fakeSource{}, "x", time.Second, 0); err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
	}
	if _, err := m.Start(&fakeSource{}, "x", time.Second, 0); err == nil {
		t.Fatalf("armed more than the cap of %d monitors", monitorMaxLive)
	}
	// Freeing a slot lets the next one arm.
	if _, err := m.Stop("mon1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := m.Start(&fakeSource{}, "x", time.Second, 0); err != nil {
		t.Fatalf("Start after freeing a slot: %v", err)
	}
}

// TestMonitorPerWakeCapReportsDropped pins that a burst beyond the per-wake event
// cap is reported as a dropped count instead of inflating the prompt.
func TestMonitorPerWakeCapReportsDropped(t *testing.T) {
	wake := &recordingWake{}
	m := NewMonitorManager(wake.fn)
	defer m.Close()
	burst := make([]string, monitorEventsPerWake+5)
	for i := range burst {
		burst[i] = fmt.Sprintf("hit %d", i)
	}
	if _, err := m.Start(&fakeSource{batches: [][]string{burst}}, "hit", time.Nanosecond, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	m.tick(context.Background())
	if wake.count() != 1 {
		t.Fatalf("want a single coalesced wake, got %d", wake.count())
	}
	if !strings.Contains(wake.last(), "5 further matching event(s) were dropped") {
		t.Fatalf("dropped count not reported: %q", wake.last())
	}
}

// TestMonitorCooldownCoalesces pins that matches inside the cooldown window ride
// in ONE wake rather than waking per event.
func TestMonitorCooldownCoalesces(t *testing.T) {
	wake := &recordingWake{}
	m := NewMonitorManager(wake.fn)
	defer m.Close()
	src := &fakeSource{batches: [][]string{{"hit a"}, {"hit b"}}}
	// A long cooldown: the second tick's match must not produce a second wake.
	if _, err := m.Start(src, "hit", time.Hour, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx := context.Background()
	m.tick(ctx)
	m.tick(ctx)
	if wake.count() != 1 {
		t.Fatalf("cooldown did not hold: %d wakes", wake.count())
	}
}

// TestMonitorCloseStopsEverything is the teardown guard: Close ends the poller and
// every monitor, so nothing can wake a session that no longer exists.
func TestMonitorCloseStopsEverything(t *testing.T) {
	wake := &recordingWake{}
	m := NewMonitorManager(wake.fn)
	src := &fakeSource{batches: [][]string{{"hit"}}}
	if _, err := m.Start(src, "hit", time.Nanosecond, 0); err != nil {
		t.Fatalf("Start: %v", err)
	}
	m.Close()
	if got := src.closeCount(); got != 1 {
		t.Fatalf("source closed %d times on teardown, want 1", got)
	}
	m.tick(context.Background())
	if wake.count() != 0 {
		t.Fatalf("a torn-down manager still woke the session")
	}
	m.Close() // idempotent
}

// TestMonitorStartRejectsBadRegex keeps an invalid filter a real error: arming a
// monitor that can never match would leave the agent waiting forever.
func TestMonitorStartRejectsBadRegex(t *testing.T) {
	m := NewMonitorManager((&recordingWake{}).fn)
	defer m.Close()
	if _, err := m.Start(&fakeSource{}, "([unclosed", time.Second, 0); err == nil {
		t.Fatal("invalid regex must be rejected")
	}
}

// TestNilMonitorManagerIsUsable pins the nil-manager contract used by
// catalog/preview builds: an explicit unavailable error, never a silent success.
func TestNilMonitorManagerIsUsable(t *testing.T) {
	var m *MonitorManager
	if _, err := m.Start(&fakeSource{}, "x", time.Second, 0); err == nil {
		t.Fatal("nil manager must report monitoring is unavailable")
	}
	if _, err := m.Stop("mon1"); err == nil {
		t.Fatal("nil manager Stop must error")
	}
	if got := m.List(); got != "(monitoring unavailable)" {
		t.Fatalf("nil manager List: %q", got)
	}
	m.Close() // must not panic
}
