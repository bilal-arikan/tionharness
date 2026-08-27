package agent

import (
	"sync"
	"sync/atomic"
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
const mcpFailStreakThreshold = 3

// mcpFailStreaks tracks consecutive catalog-build failures per server name.
// Process-local and reset on success, like anomalyNotified: this is an operational
// signal, not persisted state.
type mcpFailStreaks struct {
	m sync.Map // server name -> *atomic.Int64
}

// note increments the server's streak and returns the new value.
func (s *mcpFailStreaks) note(name string) int64 {
	v, _ := s.m.LoadOrStore(name, new(atomic.Int64))
	return v.(*atomic.Int64).Add(1)
}

// clear resets the server's streak after a successful catalog build. A server that
// recovers and fails again must escalate again -- the threshold measures a CURRENT
// outage, not a lifetime failure count.
func (s *mcpFailStreaks) clear(name string) {
	if v, ok := s.m.Load(name); ok {
		v.(*atomic.Int64).Store(0)
	}
}
