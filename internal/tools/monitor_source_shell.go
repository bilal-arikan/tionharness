package tools

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// shellSource observes a background shell's output for a monitor. It keeps its
// OWN absolute byte cursor (cur) and reads through bgWriter.drainFrom, so polling
// never consumes output that shell_manage (action=output) has not delivered yet —
// the two readers are fully independent.
type shellSource struct {
	mgr *ShellManager
	id  string
	cur int64
	// exited latches once the process end has been reported, so the terminal
	// condition is produced exactly once even if the manager polls again.
	exited bool
}

// NewShellSource binds a monitor source to background shell shellID. An unknown
// id is a real error: silently monitoring nothing would leave the agent waiting
// for a wake that can never come.
func NewShellSource(mgr *ShellManager, shellID string) (MonitorSource, error) {
	p := mgr.get(shellID)
	if p == nil {
		return nil, fmt.Errorf("no background shell %q — use shell_manage (action=list) to see running shells", shellID)
	}
	// Start at the CURRENT end of the output: a monitor reports what happens from
	// now on, not the backlog the agent has already seen.
	_, cur, _ := p.w.drainFrom(0)
	return &shellSource{mgr: mgr, id: shellID, cur: cur}, nil
}

// Poll returns one event per new output LINE. Line granularity is what makes a
// regex filter meaningful ("ERROR" should match a line, not a chunk boundary).
func (s *shellSource) Poll(_ context.Context) ([]MonitorEvent, bool, string, error) {
	p := s.mgr.get(s.id)
	if p == nil {
		// The shell was pruned out of the manager's retention window; nothing further
		// can ever arrive from it.
		return nil, true, "background shell " + s.id + " is no longer tracked", nil
	}
	out, next, lost := p.w.drainFrom(s.cur)
	s.cur = next

	now := time.Now()
	var events []MonitorEvent
	if lost {
		events = append(events, MonitorEvent{
			At:      now,
			Payload: "[earlier output rolled off the 256KB buffer before the monitor read it]",
		})
	}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if len(line) > monitorPayloadBytes {
			line = line[:monitorPayloadBytes] + "…[truncated]"
		}
		events = append(events, MonitorEvent{At: now, Payload: line})
	}

	// Report the exit only AFTER the final output has been handed over, so a match
	// on the last line is not lost to the terminal state.
	if done, code := p.exitStatus(); done && !s.exited {
		s.exited = true
		return events, true, fmt.Sprintf("background shell %s exited (code %d)", s.id, code), nil
	}
	return events, false, "", nil
}

// Describe identifies the source in the monitor list.
func (s *shellSource) Describe() string { return "shell " + s.id }

// Close releases the source. The shell itself is owned by the ShellManager and is
// deliberately NOT killed: stopping a monitor must not stop the process it watched.
func (s *shellSource) Close() {}
