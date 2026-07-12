package api

import (
	"encoding/json"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/sessionhub"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// maxInboxAttempts caps how many times a single queued turn is (re)dispatched
// before it is dropped as a poison message. The durable in-flight slot re-runs an
// interrupted head at every boot; without this cap a turn that wedges the process
// on every attempt (a subprocess that hangs then gets hard-killed) would re-run
// forever and block the whole queue behind it. See _Docs/58.
const maxInboxAttempts = 3

// inboxTurnWatchdog bounds how long a single queued turn may run before the serial
// worker force-cancels it and moves on. Without this, a turn that never returns (a
// wedged subprocess whose own startup-watchdog failed to fire, a deadlock) holds
// running=true forever and every later message piles up behind it undrained —
// exactly the "agent never starts despite repeated sends" wedge. Generous by
// design: only a genuinely stuck turn ever hits it.
const inboxTurnWatchdog = 20 * time.Minute

// persistedInbox is the on-disk shape of a session's durable queue. It carries the
// in-flight head SEPARATELY from the WAITING tail so a turn that dies before writing
// its own inflight sidecar (a hard-killed hung subprocess) is still re-dispatched at
// the next boot instead of being lost. See _Docs/58 Faz 3.
type persistedInbox struct {
	Inflight *inboxItem  `json:"inflight,omitempty"`
	Items    []inboxItem `json:"items"`
}

// decodeInbox parses a persisted inbox payload, accepting BOTH the current object
// shape ({inflight,items}) and the legacy bare-array shape ([]inboxItem) written
// before the in-flight slot existed, so an in-place upgrade never strands an old
// queue. A malformed payload decodes to an empty queue (best effort, like the rest
// of boot recovery).
func decodeInbox(data []byte) persistedInbox {
	if trimmed := skipLeadingWS(data); len(trimmed) > 0 && trimmed[0] == '[' {
		var legacy []inboxItem
		_ = json.Unmarshal(data, &legacy)
		return persistedInbox{Items: legacy}
	}
	var pi persistedInbox
	_ = json.Unmarshal(data, &pi)
	return pi
}

// skipLeadingWS returns data with any leading JSON whitespace removed, so
// decodeInbox can sniff the first significant byte ('[' = legacy array vs '{' =
// object) regardless of pretty-printing.
func skipLeadingWS(b []byte) []byte {
	for len(b) > 0 {
		switch b[0] {
		case ' ', '\t', '\n', '\r':
			b = b[1:]
		default:
			return b
		}
	}
	return b
}

// runQueuedTurn runs one queued turn with a panic barrier (runTurnGuarded) AND a
// watchdog. The turn runs in its own goroutine; if it exceeds inboxTurnWatchdog the
// worker force-cancels the live run so the goroutine unblocks and the queue keeps
// moving. A turn that ignores cancellation for another 30s is detached (its
// goroutine may leak, but the queue is no longer wedged) with a visible turn_error.
func (s *Server) runQueuedTurn(wsp *workspace.Workspace, sessionID string, req chatReq) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTurnGuarded(wsp, req)
	}()
	select {
	case <-done:
		return
	case <-time.After(inboxTurnWatchdog):
	}
	if s.logger != nil {
		s.logger.Error("queued turn exceeded watchdog; force-cancelling",
			"session", sessionID, "after", inboxTurnWatchdog.String())
	}
	// Cancel the live run so the turn's context is cancelled and the provider tears
	// down any subprocess (see chat_stream.go: the run's cancel is the turn ctx).
	if info, live := s.runs.sessionRunInfo(sessionID); live {
		if run := s.runs.get(info.RunID); run != nil {
			run.cancel()
		}
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		s.publishHub(sessionID, sessionhub.KindTurnError, map[string]any{
			"error":  "turn exceeded its time limit and was cancelled",
			"reason": "watchdog",
		}, false)
		s.hub.Commit(sessionID)
	}
}

// clearInflight drops the durable in-flight slot after a turn completes (or is
// abandoned) and re-persists, so a finished turn is never re-dispatched at boot.
func (s *Server) clearInflight(sessionID string) {
	s.inbox.lock()
	if ib := s.inbox.sessions[sessionID]; ib != nil {
		ib.inflight = nil
	}
	s.inbox.unlock()
	s.persistInbox(sessionID)
	s.publishQueueUpdate(sessionID)
}

// dropPoisonedInflight abandons a turn that has wedged the process too many times
// and tells every window why, so a message that reliably crashes generation stops
// blocking the whole queue.
func (s *Server) dropPoisonedInflight(sessionID string, item inboxItem) {
	if s.logger != nil {
		s.logger.Error("dropping poisoned queued turn",
			"session", sessionID, "attempts", item.Attempts, "clientMsgId", item.ClientMsgID)
	}
	s.publishHub(sessionID, sessionhub.KindTurnError, map[string]any{
		"error":  "message dropped after repeated failures to start",
		"reason": "poison",
	}, false)
	s.hub.Commit(sessionID)
	s.clearInflight(sessionID)
}
