package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/mcp"
)

func sharedDeath(server string) mcp.DisconnectEvent {
	return mcp.DisconnectEvent{Server: server, Error: "EOF"}
}

func scopedDeath(server, sessionID string) mcp.DisconnectEvent {
	return mcp.DisconnectEvent{
		Server:   server,
		Scoped:   true,
		ScopeKey: sessionID + "|agent1",
		Error:    "EOF",
	}
}

// A shared connection serves every session, so its death is reported to whoever
// takes next.
func TestDisconnectLogSharedReachesAnySession(t *testing.T) {
	l := newMCPDisconnectLog()
	l.record(sharedDeath("gw"))

	got := l.take("sessA")
	if len(got) != 1 || got[0].Server != "gw" {
		t.Fatalf("shared death should reach any session, got %+v", got)
	}
}

// A scoped connection belongs to ONE session: another session must not be told
// its tools died, and the owner must still get the note.
func TestDisconnectLogScopedIsPrivateToItsSession(t *testing.T) {
	l := newMCPDisconnectLog()
	l.record(scopedDeath("gw", "sessA"))

	if got := l.take("sessB"); len(got) != 0 {
		t.Fatalf("sessB must not see sessA's scoped death, got %+v", got)
	}
	got := l.take("sessA")
	if len(got) != 1 || got[0].Server != "gw" {
		t.Fatalf("the owning session must see its death, got %+v", got)
	}
}

// take() IS the once-per-server-per-turn dedupe: a reported death is consumed.
func TestDisconnectLogTakeConsumes(t *testing.T) {
	l := newMCPDisconnectLog()
	l.record(sharedDeath("gw"))

	if got := l.take("sessA"); len(got) != 1 {
		t.Fatalf("first take should report, got %+v", got)
	}
	if got := l.take("sessA"); len(got) != 0 {
		t.Fatalf("second take must be empty (dedupe), got %+v", got)
	}
}

// Two deaths of the same server collapse to one entry: the model needs the
// current state, not a history.
func TestDisconnectLogCollapsesRepeatDeaths(t *testing.T) {
	l := newMCPDisconnectLog()
	l.record(sharedDeath("gw"))
	l.record(sharedDeath("gw"))

	if got := l.take("sessA"); len(got) != 1 {
		t.Fatalf("repeat deaths of one server should collapse, got %+v", got)
	}
}

// A death nobody reported within the TTL is dropped rather than surfacing later
// as a stale explanation for a tool that has since been re-dialed.
func TestDisconnectLogDropsStale(t *testing.T) {
	now := time.Now()
	l := newMCPDisconnectLog()
	l.now = func() time.Time { return now }
	l.record(sharedDeath("gw"))

	now = now.Add(disconnectNoteTTL + time.Minute)
	if got := l.take("sessA"); len(got) != 0 {
		t.Fatalf("a death older than the TTL must not be reported, got %+v", got)
	}
}

// Multiple servers are reported together, ordered, so the note is deterministic.
func TestDisconnectNoteNamesEveryServer(t *testing.T) {
	l := newMCPDisconnectLog()
	l.record(sharedDeath("zeta"))
	l.record(sharedDeath("alpha"))

	note := formatDisconnectNote(l.take("sessA"))
	if !strings.Contains(note, "alpha") || !strings.Contains(note, "zeta") {
		t.Fatalf("note must name both servers: %q", note)
	}
	if strings.Index(note, "alpha") > strings.Index(note, "zeta") {
		t.Fatalf("servers should be sorted for a deterministic note: %q", note)
	}
	// The whole point of the note: the model must know the tools are gone and
	// stop calling them.
	if !strings.Contains(note, "rest of this turn") {
		t.Fatalf("note must state the tools are gone for the turn: %q", note)
	}
}

// Nothing to report renders nothing — no empty card on a healthy turn.
func TestDisconnectNoteEmptyWhenNoDeaths(t *testing.T) {
	if note := formatDisconnectNote(nil); note != "" {
		t.Fatalf("want empty note, got %q", note)
	}
}

// An interrupted call is called out: it explains a tool result the model never
// received.
func TestDisconnectNoteMentionsPendingCalls(t *testing.T) {
	ev := sharedDeath("gw")
	ev.PendingCalls = 2
	note := formatDisconnectNote([]mcp.DisconnectEvent{ev})
	if !strings.Contains(note, "in-flight") {
		t.Fatalf("note should mention stranded calls: %q", note)
	}
}
