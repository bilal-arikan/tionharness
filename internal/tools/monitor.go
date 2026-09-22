package tools

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// monitorMaxLive caps concurrent ARMED monitors per session. Deliberately lower
	// than bgShellMaxLive (16): every armed monitor can wake the agent, so the brake
	// on "how many things can interrupt me" is tighter than on "how many processes
	// can run".
	monitorMaxLive = 8
	// monitorPayloadBytes bounds one event's payload. A matching line from a chatty
	// process can be arbitrarily long; the wake prompt must stay small.
	monitorPayloadBytes = 2 * 1024
	// monitorEventsPerWake bounds how many matching events ride along in a single
	// wake. Excess is reported as a dropped count rather than inflating the prompt.
	monitorEventsPerWake = 10
	// monitorPollInterval is the poller's tick. One ticker per monitor manager.
	monitorPollInterval = time.Second
	// monitorMinCooldown floors the user-supplied cooldown so a monitor on a busy
	// source cannot wake the agent on every tick.
	monitorMinCooldown = 5 * time.Second
)

// MonitorEvent is one observation from a source: the moment it was seen and the
// (already size-capped) text that was observed.
type MonitorEvent struct {
	At      time.Time
	Payload string
}

// MonitorSource is the pluggable observation surface behind a monitor. v1 ships
// only a background-shell source (monitor_source_shell.go); file/URL/WebSocket
// sources implement this same interface without touching the manager.
//
// Filtering is NOT a source concern: a source reports everything it saw and the
// monitor layer decides what matches.
type MonitorSource interface {
	// Poll returns the events observed since the previous call. done=true means the
	// source can never produce again (the process exited, the file was removed) —
	// the monitor then enters its terminal state and reason explains why. An err is
	// transient: it is recorded as the last error and polling continues.
	Poll(ctx context.Context) (events []MonitorEvent, done bool, reason string, err error)
	// Describe returns a short human-readable identification of the source, shown in
	// the monitor list (e.g. `shell bg1`).
	Describe() string
	// Close releases whatever the source holds. Idempotent.
	Close()
}

// WakeNowFunc delivers an immediate self-wake into a session. It is the ONLY way
// a monitor reaches the agent: the manager never runs a turn itself. Returns the
// armed schedule id (for the log) or an error the monitor records.
type WakeNowFunc func(ctx context.Context, prompt, reason string) (string, error)

// monitorState is a monitor's lifecycle position.
type monitorState string

const (
	// monitorArmed is watching and able to fire.
	monitorArmed monitorState = "armed"
	// monitorStopped was stopped by the agent (stop action) or by session teardown.
	monitorStopped monitorState = "stopped"
	// monitorDone reached a terminal condition on its own: the source can no longer
	// produce, or max_fires was exhausted. reason says which.
	monitorDone monitorState = "done"
)

// monitorEntry is one tracked monitor.
type monitorEntry struct {
	id        string
	src       MonitorSource
	pattern   *regexp.Regexp
	cooldown  time.Duration
	maxFires  int // 0 = unlimited
	createdAt time.Time

	mu        sync.Mutex
	state     monitorState
	reason    string // why it left the armed state
	seen      int    // events observed from the source
	matched   int    // events that passed the filter
	delivered int    // wakes actually delivered
	dropped   int    // matching events not carried into a wake (cap/cooldown)
	lastMatch time.Time
	lastFire  time.Time
	lastErr   string
	pending   []MonitorEvent // matches waiting for the cooldown to elapse
}

// monitorSnapshot is a lock-free, read-only copy of a monitorEntry for reporting.
// It exists so the entry's mutex is never copied along with its fields.
type monitorSnapshot struct {
	id        string
	src       MonitorSource
	pattern   *regexp.Regexp
	cooldown  time.Duration
	maxFires  int
	createdAt time.Time
	state     monitorState
	reason    string
	seen      int
	matched   int
	delivered int
	dropped   int
	lastMatch time.Time
	lastFire  time.Time
	lastErr   string
}

// snapshot copies the mutable fields under the lock for reporting.
func (e *monitorEntry) snapshot() monitorSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return monitorSnapshot{
		id: e.id, src: e.src, pattern: e.pattern, cooldown: e.cooldown,
		maxFires: e.maxFires, createdAt: e.createdAt,
		state: e.state, reason: e.reason, seen: e.seen, matched: e.matched,
		delivered: e.delivered, dropped: e.dropped, lastMatch: e.lastMatch,
		lastFire: e.lastFire, lastErr: e.lastErr,
	}
}

// terminate moves the entry out of the armed state exactly once and closes its
// source. Returns false when it had already left that state, which is how the
// "terminal condition reported ONCE" guarantee is enforced.
func (e *monitorEntry) terminate(state monitorState, reason string) bool {
	e.mu.Lock()
	if e.state != monitorArmed {
		e.mu.Unlock()
		return false
	}
	e.state = state
	e.reason = reason
	e.pending = nil
	e.mu.Unlock()
	e.src.Close()
	return true
}

// MonitorManager owns one session's monitors and the single goroutine that polls
// them. It is memory-only: monitors do not survive a process restart, and a wake
// in flight when the process dies is lost (fireWake is at-most-once by design).
//
// A nil *MonitorManager is a valid value: Start reports that monitoring is
// unavailable, which is the behaviour on non-session (catalog/preview) builds.
type MonitorManager struct {
	wake WakeNowFunc

	mu       sync.Mutex
	monitors map[string]*monitorEntry
	seq      int
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopped  bool
}

// NewMonitorManager builds a manager whose monitors wake through wake. A nil wake
// is rejected at Start time rather than silently producing monitors that observe
// but can never report.
func NewMonitorManager(wake WakeNowFunc) *MonitorManager {
	return &MonitorManager{wake: wake, monitors: map[string]*monitorEntry{}}
}

// Start arms a monitor on src, firing a wake whenever an observed event matches
// pattern. cooldown is floored at monitorMinCooldown; maxFires<=0 means unlimited.
// The caller's src is owned by the manager from here on (it is closed on stop,
// terminal state or teardown) EXCEPT when Start returns an error.
func (m *MonitorManager) Start(src MonitorSource, pattern string, cooldown time.Duration, maxFires int) (string, error) {
	if m == nil {
		return "", fmt.Errorf("monitoring is not available in this context")
	}
	if m.wake == nil {
		return "", fmt.Errorf("monitoring is not available in this context (no wake channel)")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid filter regex %q: %w", pattern, err)
	}
	if cooldown < monitorMinCooldown {
		cooldown = monitorMinCooldown
	}

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return "", fmt.Errorf("monitoring is shutting down for this session")
	}
	live := 0
	for _, e := range m.monitors {
		if e.snapshotState() == monitorArmed {
			live++
		}
	}
	if live >= monitorMaxLive {
		m.mu.Unlock()
		return "", fmt.Errorf("too many monitors armed (%d); stop one with monitor (action=stop) first", live)
	}
	m.seq++
	id := "mon" + strconv.Itoa(m.seq)
	m.monitors[id] = &monitorEntry{
		id: id, src: src, pattern: re, cooldown: cooldown,
		maxFires: maxFires, createdAt: time.Now(), state: monitorArmed,
	}
	m.ensurePollerLocked()
	m.mu.Unlock()
	return id, nil
}

// snapshotState reads the state under the entry lock.
func (e *monitorEntry) snapshotState() monitorState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// ensurePollerLocked starts the single poll goroutine on first use. Caller holds
// m.mu.
func (m *MonitorManager) ensurePollerLocked() {
	if m.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.wg.Add(1)
	go m.poll(ctx)
}

// poll is the manager's single goroutine: one ticker drives every monitor.
func (m *MonitorManager) poll(ctx context.Context) {
	defer m.wg.Done()
	t := time.NewTicker(monitorPollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.tick(ctx)
		}
	}
}

// tick polls every armed monitor once and delivers whatever became deliverable.
// Exported behaviour is tested through this method (tests call it directly rather
// than waiting on the ticker).
func (m *MonitorManager) tick(ctx context.Context) {
	m.mu.Lock()
	armed := make([]*monitorEntry, 0, len(m.monitors))
	for _, e := range m.monitors {
		if e.snapshotState() == monitorArmed {
			armed = append(armed, e)
		}
	}
	m.mu.Unlock()
	for _, e := range armed {
		m.tickOne(ctx, e)
	}
}

// tickOne polls one monitor, files the matches and fires a wake when the cooldown
// allows. A terminal source is reported once, with any final matches still
// delivered before the monitor closes.
func (m *MonitorManager) tickOne(ctx context.Context, e *monitorEntry) {
	events, done, doneReason, err := e.src.Poll(ctx)

	e.mu.Lock()
	if e.state != monitorArmed {
		e.mu.Unlock()
		return
	}
	if err != nil {
		// Transient: recorded and surfaced in list, polling continues. A source that
		// can never recover reports done=true instead.
		e.lastErr = err.Error()
	}
	for _, ev := range events {
		e.seen++
		if !e.pattern.MatchString(ev.Payload) {
			continue
		}
		e.matched++
		e.lastMatch = ev.At
		if len(e.pending) >= monitorEventsPerWake {
			e.dropped++
			continue
		}
		e.pending = append(e.pending, ev)
	}
	// Cooldown gate: a monitor fires at most once per cooldown window, so a chatty
	// source coalesces into one wake carrying the batch.
	ready := len(e.pending) > 0 && time.Since(e.lastFire) >= e.cooldown
	var batch []MonitorEvent
	if ready {
		batch = e.pending
		e.pending = nil
		e.lastFire = time.Now()
	}
	dropped := e.dropped
	id, maxFires, delivered := e.id, e.maxFires, e.delivered
	e.mu.Unlock()

	if len(batch) > 0 {
		prompt, reason := monitorWakePrompt(id, e.src.Describe(), batch, dropped)
		if _, werr := m.wake(ctx, prompt, reason); werr != nil {
			e.mu.Lock()
			e.lastErr = "wake failed: " + werr.Error()
			// Put the batch back so the next tick retries rather than losing the match.
			e.pending = append(batch, e.pending...)
			if len(e.pending) > monitorEventsPerWake {
				e.dropped += len(e.pending) - monitorEventsPerWake
				e.pending = e.pending[:monitorEventsPerWake]
			}
			e.mu.Unlock()
		} else {
			e.mu.Lock()
			e.delivered++
			delivered = e.delivered
			e.mu.Unlock()
		}
	}

	if maxFires > 0 && delivered >= maxFires {
		e.terminate(monitorDone, fmt.Sprintf("max_fires (%d) reached", maxFires))
		return
	}
	if done {
		if doneReason == "" {
			doneReason = "source ended"
		}
		e.terminate(monitorDone, doneReason)
	}
}

// monitorWakePrompt renders the wake prompt and its short reason for a batch of
// matches. Payloads are already capped per event by the source.
func monitorWakePrompt(id, source string, batch []MonitorEvent, dropped int) (prompt, reason string) {
	var b strings.Builder
	fmt.Fprintf(&b, "Monitor %s (%s) matched %d event(s):\n", id, source, len(batch))
	for _, ev := range batch {
		fmt.Fprintf(&b, "[%s] %s\n", ev.At.Format(time.RFC3339), ev.Payload)
	}
	if dropped > 0 {
		fmt.Fprintf(&b, "(%d further matching event(s) were dropped: per-wake cap is %d)\n", dropped, monitorEventsPerWake)
	}
	b.WriteString("Decide what to do about this, then continue or stop the monitor with monitor (action=stop).")
	return b.String(), fmt.Sprintf("monitor %s matched on %s", id, source)
}

// Stop stops a monitor by id. Stopping an already-stopped monitor is a no-op
// success (idempotent); an unknown id is an actionable error, matching
// ShellManager.Kill.
func (m *MonitorManager) Stop(id string) (string, error) {
	if m == nil {
		return "", fmt.Errorf("monitoring is not available in this context")
	}
	m.mu.Lock()
	e := m.monitors[id]
	m.mu.Unlock()
	if e == nil {
		return "", fmt.Errorf("no monitor %q — use monitor (action=list) to see active monitors", id)
	}
	if !e.terminate(monitorStopped, "stopped by agent") {
		s := e.snapshot()
		return fmt.Sprintf("%s was already %s (%s)", id, s.state, s.reason), nil
	}
	return fmt.Sprintf("Stopped %s; it will not wake this session again.", id), nil
}

// List renders one line per monitor: id, source, filter, age, counters, last
// match, state and last error.
func (m *MonitorManager) List() string {
	if m == nil {
		return "(monitoring unavailable)"
	}
	m.mu.Lock()
	entries := make([]*monitorEntry, 0, len(m.monitors))
	for _, e := range m.monitors {
		entries = append(entries, e)
	}
	m.mu.Unlock()
	if len(entries) == 0 {
		return "(no monitors)"
	}
	snaps := make([]monitorSnapshot, 0, len(entries))
	for _, e := range entries {
		snaps = append(snaps, e.snapshot())
	}
	slices.SortFunc(snaps, func(a, b monitorSnapshot) int {
		return a.createdAt.Compare(b.createdAt)
	})
	var b strings.Builder
	for _, s := range snaps {
		fmt.Fprintf(&b, "%s\t%s\tfilter=%s\t%s\tage=%s\tseen=%d matched=%d delivered=%d dropped=%d",
			s.id, s.src.Describe(), s.pattern.String(), s.state,
			time.Since(s.createdAt).Round(time.Second), s.seen, s.matched, s.delivered, s.dropped)
		if !s.lastMatch.IsZero() {
			fmt.Fprintf(&b, "\tlast_match=%s", s.lastMatch.Format(time.RFC3339))
		}
		if s.reason != "" {
			fmt.Fprintf(&b, "\treason=%s", s.reason)
		}
		if s.lastErr != "" {
			fmt.Fprintf(&b, "\tlast_error=%s", s.lastErr)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// Close stops every monitor and the poll goroutine, and waits for it to exit.
// Called on session teardown; idempotent, and safe on a manager that never
// started a poller.
func (m *MonitorManager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	cancel := m.cancel
	m.cancel = nil
	entries := make([]*monitorEntry, 0, len(m.monitors))
	for _, e := range m.monitors {
		entries = append(entries, e)
	}
	m.monitors = map[string]*monitorEntry{}
	m.mu.Unlock()

	for _, e := range entries {
		e.terminate(monitorStopped, "session ended")
	}
	if cancel != nil {
		cancel()
		m.wg.Wait()
	}
}
