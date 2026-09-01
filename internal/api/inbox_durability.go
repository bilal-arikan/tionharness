package api

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
	"github.com/bilal-arikan/tionharness/internal/workspace"
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
// exactly the "agent never starts despite repeated sends" wedge.
//
// Settings-driven (TurnWatchdogMin, default 120 min), floored at the spawn/schedule
// ceilings by agent.Tunables.TurnWatchdog. It was a fixed 20 minutes, which was
// TIGHTER than the deadlines those knobs already grant: a healthy interactive turn
// running a long build/test loop (heavy Rust/Go compiles, 100+ tool calls) was
// force-cancelled mid-tool as if it were hung, losing the in-flight step. Only a
// genuinely stuck turn should ever reach this.
func (s *Server) inboxTurnWatchdog() time.Duration {
	if s.tun == nil {
		return agent.DefaultTurnWatchdogMinutes * time.Minute
	}
	return s.tun.TurnWatchdog()
}

// inboxTurnIdleWatchdog is the INACTIVITY window that complements the ceiling
// above: a queued turn that publishes no event at all (no tool step, no thinking,
// no token delta) for this long is wedged and reclaimed early, rather than holding
// the queue until the generous wall-clock cap. Together they read as "silent for X,
// or running for Y" — which is what actually distinguishes a hang from a slow turn.
func (s *Server) inboxTurnIdleWatchdog() time.Duration {
	if s.tun == nil {
		return agent.DefaultTurnIdleWatchdogMinutes * time.Minute
	}
	return s.tun.TurnIdleWatchdog()
}

// turnIdleFor reports how long the session has been silent, measuring from the
// last event published to its hub (falling back to startedAt before the first
// event lands, so a turn that dies during setup is still reclaimed). ok=false
// means there is no usable signal and the caller must not judge the turn idle.
func (s *Server) turnIdleFor(wsID, sessionID string, startedAt time.Time) (time.Duration, bool) {
	if s.hub == nil {
		return 0, false
	}
	since := startedAt
	if last, ok := s.hub.LastActivity(wsID, sessionID); ok && last.After(since) {
		since = last
	}
	if since.IsZero() {
		return 0, false
	}
	return time.Since(since), true
}

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
// queue.
//
// A malformed payload is an ERROR, never an empty queue. The two unmarshal errors
// used to be discarded, so a corrupt sidecar silently became "no queued messages"
// at boot — every waiting user message, including the interrupted in-flight head,
// dropped with nothing in the log. The caller must quarantine the file and say so
// (quarantineInbox).
func decodeInbox(data []byte) (persistedInbox, error) {
	if trimmed := skipLeadingWS(data); len(trimmed) > 0 && trimmed[0] == '[' {
		var legacy []inboxItem
		if err := json.Unmarshal(data, &legacy); err != nil {
			return persistedInbox{}, fmt.Errorf("legacy inbox array: %w", err)
		}
		return persistedInbox{Items: legacy}, nil
	}
	var pi persistedInbox
	if err := json.Unmarshal(data, &pi); err != nil {
		return persistedInbox{}, fmt.Errorf("inbox object: %w", err)
	}
	return pi, nil
}

// quarantineInbox preserves an unparseable inbox.json (renaming it aside rather
// than letting the next flush overwrite it) and reports the loss on every channel:
// the server log and the session's debug journal. Queued messages are user input
// with no other copy, so this is deliberately loud — a corrupt queue that vanished
// quietly was indistinguishable from a session that was never written to.
func (s *Server) quarantineInbox(wsp *workspace.Workspace, sessionID string, cause error) {
	if wsp == nil || wsp.DB == nil {
		return
	}
	dest, err := wsp.DB.QuarantineInbox(sessionID)
	if s.logger != nil {
		if err != nil {
			s.logger.Error("corrupt inbox.json could not be quarantined; queued messages are lost",
				"workspace", wsp.ID, "session", sessionID, "error", err, "parseError", cause)
		} else {
			s.logger.Error("inbox.json was corrupt: quarantined, its queued messages were NOT dispatched",
				"workspace", wsp.ID, "session", sessionID, "quarantine", dest, "parseError", cause)
		}
	}
	if aerr := wsp.DB.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type:   db.DebugError,
		Name:   "inbox_corrupt",
		Detail: "durable inbox sidecar was unreadable; queued messages were not dispatched",
		Error:  cause.Error(),
		Err:    true,
	}); aerr != nil && s.logger != nil {
		s.logger.Error("record corrupt-inbox debug event failed", "session", sessionID, "error", aerr)
	}
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

// watchdogPollInterval is how often the queue watchdog re-checks a running turn.
// It bounds only how late an idle cut can be, so it trades a negligible amount of
// timer wakeups for cheap, per-tick liveness questions.
const watchdogPollInterval = 15 * time.Second

// watchdogDetachGrace is how long a force-cancelled turn is given to unwind before
// the worker gives up waiting on it. The goroutine may leak, but the queue moves.
const watchdogDetachGrace = 30 * time.Second

// runQueuedTurn runs one queued turn with a panic barrier (runTurnGuarded) AND a
// two-part watchdog: a generous wall-clock ceiling plus an INACTIVITY window. The
// turn runs in its own goroutine; when either bound trips, the worker force-cancels
// the live run so the goroutine unblocks and the queue keeps moving.
//
// The idle half is what makes the pair honest. Wall clock alone cannot separate a
// wedged turn from a slow one, so the ceiling has to be generous — and a generous
// ceiling is exactly what a real wedge exploits. Silence is the signal that tells
// them apart: a turn still emitting steps is working no matter how long it runs,
// while one that has published nothing for the idle window is not coming back.
//
// Every cancellation is recorded (see recordWatchdogCut), not just the ones that
// refuse to unwind: a cut that unwound quickly used to leave no trace at all in the
// transcript, so the turn simply stopped mid-sentence with no visible reason.
func (s *Server) runQueuedTurn(wsp *workspace.Workspace, sessionID string, req chatReq) {
	hard := s.inboxTurnWatchdog()
	idleWindow := s.inboxTurnIdleWatchdog()
	startedAt := time.Now()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTurnGuarded(wsp, req)
	}()

	ticker := time.NewTicker(watchdogPollInterval)
	defer ticker.Stop()
	var reason, detail string
	var elapsed time.Duration
poll:
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			elapsed = time.Since(startedAt)
			if elapsed >= hard {
				reason = "watchdog"
				detail = fmt.Sprintf("Tur süre sınırını aştı ve iptal edildi (watchdog, %s).", hard)
				break poll
			}
			if idle, ok := s.turnIdleFor(wsp.ID, sessionID, startedAt); ok && idle >= idleWindow {
				reason = "watchdog-idle"
				detail = fmt.Sprintf("Tur %s boyunca hiçbir etkinlik üretmedi ve iptal edildi (boşta izleyicisi).", idleWindow)
				break poll
			}
		}
	}

	if s.logger != nil {
		s.logger.Error("queued turn exceeded watchdog; force-cancelling",
			"session", sessionID, "reason", reason, "elapsed", elapsed.String(),
			"hard", hard.String(), "idle", idleWindow.String())
	}
	// Cancel the live run so the turn's context is cancelled and the provider tears
	// down any subprocess (see chat_stream.go: the run's cancel is the turn ctx).
	if info, live := s.runs.sessionRunInfo(wsp.ID, sessionID); live {
		if run := s.runs.get(info.RunID); run != nil {
			run.cancel()
		}
	}
	select {
	case <-done:
	case <-time.After(watchdogDetachGrace):
		reason += "-detached"
		detail += " Tur iptali de yanıtlamadı; kuyruk serbest bırakıldı."
	}
	s.recordWatchdogCut(wsp, sessionID, req.ClientMsgID, reason, detail)
}

// recordWatchdogCut surfaces a watchdog cancellation on every channel a user or a
// later analysis can read: a durable error card in the transcript, a debug.jsonl
// event, and a live turn_error for open windows.
//
// It runs for EVERY cut. Previously only a turn that also ignored the cancellation
// for 30s was recorded, so the common case — cancel arrives, turn unwinds promptly
// — produced a turn that just stopped, with the reason visible nowhere but the
// server log (WS15/SES76: the transcript ended mid-task with no error at all).
func (s *Server) recordWatchdogCut(wsp *workspace.Workspace, sessionID, clientMsgID, reason, detail string) {
	s.recordQueueTurnFailure(wsp, sessionID, reason, detail)
	s.publishHub(wsp.ID, sessionID, sessionhub.KindTurnError, map[string]any{
		"error":       detail,
		"reason":      reason,
		"clientMsgId": clientMsgID,
	}, false)
	s.hub.Commit(wsp.ID, sessionID)
}

// clearInflight drops the durable in-flight slot after a turn completes (or is
// abandoned) and re-persists, so a finished turn is never re-dispatched at boot.
func (s *Server) clearInflight(wsID, sessionID string) {
	s.inbox.lock()
	if ib := s.inbox.at(wsID, sessionID); ib != nil {
		ib.inflight = nil
	}
	s.inbox.unlock()
	s.flushInbox(wsID, sessionID)
}

// runTurnGuarded runs one queued turn with a panic barrier so a crash in the turn
// (a provider bug, a nil deref) surfaces as a logged error + a hub turn_error
// instead of killing the serial worker and wedging the session's queue. It lives
// here beside runQueuedTurn — the two form one durability unit (panic barrier +
// watchdog) that keeps a single bad turn from taking down the serial worker.
func (s *Server) runTurnGuarded(wsp *workspace.Workspace, req chatReq) {
	defer func() {
		if r := recover(); r != nil {
			if s.logger != nil {
				s.logger.Error("queued turn panicked", "session", req.SessionID, "panic", r,
					"stack", string(debug.Stack()))
			}
			detail := fmt.Sprintf("Tur beklenmedik bir hatayla çöktü (panic): %v", r)
			// Persist the crash as a durable error card + debug event BEFORE the hub
			// toast, so a panic that dies before the turn writes any trace is still
			// visible in the transcript and the Debug panel — not a silent hang.
			s.recordQueueTurnFailure(wsp, req.SessionID, "panic", detail)
			s.publishHub(wsp.ID, req.SessionID, sessionhub.KindTurnError, map[string]any{
				"error":       detail,
				"reason":      "panic",
				"clientMsgId": req.ClientMsgID,
			}, false)
			s.hub.Commit(wsp.ID, req.SessionID)
		}
	}()
	s.runChatTurn(context.Background(), wsp, req, nil)
}

// dropPoisonedInflight abandons a turn that has wedged the process too many times
// and tells every window why, so a message that reliably crashes generation stops
// blocking the whole queue.
func (s *Server) dropPoisonedInflight(wsID, sessionID string, item inboxItem) {
	if s.logger != nil {
		s.logger.Error("dropping poisoned queued turn", "workspace", wsID,
			"session", sessionID, "attempts", item.Attempts, "clientMsgId", item.ClientMsgID)
	}
	detail := "Mesaj, başlatılırken tekrarlanan çökmeler yüzünden düşürüldü (poison)."
	s.recordQueueTurnFailure(s.workspaceByID(wsID), sessionID, "poison", detail)
	s.publishHub(wsID, sessionID, sessionhub.KindTurnError, map[string]any{
		"error":       detail,
		"reason":      "poison",
		"clientMsgId": item.ClientMsgID,
	}, false)
	s.hub.Commit(wsID, sessionID)
	s.clearInflight(wsID, sessionID)
}

// recordQueueTurnFailure persists a queue-level turn failure (a panic-barrier
// catch, a watchdog cancel, a poison-guard drop) as BOTH a durable assistant
// error message (kind=error → a red card in the transcript that survives reload
// and is found by analyze-session) AND a debug.jsonl error event (so it surfaces
// in the Debug panel / read_session_debug / anomaly detection). Without this a
// crash that dies before the turn writes any trace was visible only as an
// ephemeral hub toast plus a server-log line — the exact reason a panicked turn
// looked like a silent hang. Best effort: it degrades to the hub-only signal the
// caller still emits when the workspace/DB is unavailable.
func (s *Server) recordQueueTurnFailure(wsp *workspace.Workspace, sessionID, reason, detail string) {
	if wsp == nil || wsp.DB == nil {
		return
	}
	database := wsp.DB
	ctx := context.Background()
	agentID := ""
	if sess, err := database.GetSession(ctx, sessionID); err == nil {
		agentID = sess.AgentID
	}
	// Durable transcript card: kind=error so it renders as a red bubble, survives a
	// reload, and analyze-session.ps1 / the Debug modal both find it.
	step := agent.TurnStep{Kind: agent.StepError, Text: detail, Reason: reason}
	if msg, err := database.AddMessage(ctx, db.Message{
		SessionID: sessionID,
		Role:      providers.RoleAssistant,
		AgentID:   agentID,
		Steps:     marshalSteps([]agent.TurnStep{step}),
	}); err != nil {
		if s.logger != nil {
			s.logger.Error("persist queue turn failure failed", "session", sessionID, "error", err)
		}
	} else {
		// Push the card onto the hub as a durable reply (not the ephemeral turn_error
		// toast) so every open window renders it. The caller's hub.Commit flushes it.
		s.publishHub(wsp.ID, sessionID, sessionhub.KindReply, msg, false)
	}
	// Structured observability record for the Debug panel / read_session_debug.
	if err := database.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type:    db.DebugError,
		AgentID: agentID,
		Name:    reason,
		Detail:  detail,
		Err:     true,
	}); err != nil && s.logger != nil {
		s.logger.Error("record queue turn failure debug event failed", "session", sessionID, "error", err)
	}
}
