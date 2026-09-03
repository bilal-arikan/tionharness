package repair

import (
	"sync"
	"time"
)

// An MCP server whose catalog build fails is logged at WARN, once per turn. That
// is right for a blip -- a server restarting between turns should not shout -- but
// it reads identically to a server that has been down for hours, and the log is
// the only place it shows up on the claude-cli path (the native loop also cards it,
// see mcpnotice.go).
//
// On 2026-08-27 codebase-memory-mcp refused every client for ~9 hours. The log
// carried the same WARN line every 32 seconds and nothing ever escalated, so a
// workspace where no agent could start a turn looked, from the log level alone,
// exactly like normal operation. This adds the missing distinction: a streak of
// consecutive failures for the SAME server crosses into ERROR exactly once, so a
// standing outage is visible at a glance without the transient case becoming noise.
const FailStreakThreshold = 3

// BreakerCooldown is how long a server that crossed the threshold is skipped
// before the next build probes it again.
//
// On 2026-09-02 codebase-memory-mcp hung in initialize (nine instances plus a
// reindex fighting over one store). The pool has no dial deadline of its own, so
// every registry build -- every turn, the tools panel, the context preview --
// blocked on that handshake until the caller's context died; from the UI it
// looked like the app had stopped. The breaker turns a hung server into a
// missing server: after the threshold it is skipped for this long, recorded on
// the turn's failure card as "skipped", and probed once per cooldown.
const BreakerCooldown = 45 * time.Second

// FailStreaks tracks consecutive catalog-build failures per server name and
// the breaker window they open. Process-local and reset on success, like
// anomalyNotified: this is an operational signal, not persisted state.
type FailStreaks struct {
	mu  sync.Mutex
	m   map[string]*failState
	now func() time.Time // injectable clock for tests; nil => time.Now
}

type failState struct {
	streak    int64
	openUntil time.Time
}

func (s *FailStreaks) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *FailStreaks) state(name string) *failState {
	if s.m == nil {
		s.m = map[string]*failState{}
	}
	st := s.m[name]
	if st == nil {
		st = &failState{}
		s.m[name] = st
	}
	return st
}

// note increments the server's streak and returns the new value. EVERY failure
// (re)opens the breaker for one cooldown.
//
// Opening only at FailStreakThreshold left the expensive case unprotected: a
// server that hangs in initialize costs a full dial deadline PER build, so a cold
// outage burned threshold x DefaultDialTimeout (3 x 20s) of blocked UI before the
// breaker could skip anything -- which is precisely the minute-long freeze at
// project open this breaker was added for (2026-09-03). One failure is already
// proof the server is not answering; make the next build skip it and probe once
// per cooldown instead. The streak still counts up independently, so ERROR
// escalation continues to fire only at the threshold and a blip stays at WARN.
func (s *FailStreaks) Note(name string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state(name)
	st.streak++
	st.openUntil = s.clock().Add(BreakerCooldown)
	return st.streak
}

// clear resets the server's streak (and closes its breaker) after a successful
// catalog build. A server that recovers and fails again must escalate again --
// the threshold measures a CURRENT outage, not a lifetime failure count.
func (s *FailStreaks) Clear(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st := s.m[name]; st != nil {
		st.streak = 0
		st.openUntil = time.Time{}
	}
}

// open reports whether the server's breaker is open (skip the dial), how long
// until the next probe, and the current streak. Once the cooldown has passed
// the breaker reads closed so exactly one build probes the server; a failed
// probe re-opens it through note.
func (s *FailStreaks) Open(name string) (isOpen bool, retryIn time.Duration, streak int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.m[name]
	if st == nil {
		return false, 0, 0
	}
	rem := st.openUntil.Sub(s.clock())
	if rem <= 0 {
		return false, 0, st.streak
	}
	return true, rem, st.streak
}
