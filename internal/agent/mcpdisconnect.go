package agent

import (
	"cmp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Mid-session MCP disconnect surfacing (TSK915).
//
// internal/mcp reports an UNEXPECTED connection death (clean shutdowns are
// filtered out down there — see internal/mcp/disconnect.go). This file turns
// that into the two things a user and a model actually need:
//
//   - a ws:mcp_status workspace-stream event, so the Tools screen can mark the
//     server disconnected without polling; and
//   - a per-turn system note, so the model learns its tools are gone instead of
//     discovering it by calling one that no longer exists.
//
// SCOPE: this covers the NATIVE tool loop only. The claude-cli and codex-cli
// providers spawn their own MCP clients inside the CLI process; TionHarness
// never holds those connections and cannot observe them dying. A disconnect on
// a CLI-backed turn surfaces only as that CLI's own tool error.
//
// Auto-reconnect is deliberately NOT implemented here: Pool.Call already
// re-dials a dead connection transparently on next use, so the recovery path
// exists; a background retry loop (with its paired "recovered" event) is a
// separate change.

// disconnectNoteTTL bounds how long a disconnect stays eligible for an agent
// note. A death the model was never told about is only worth mentioning while
// it is still the likely explanation for a missing tool; beyond this window the
// pool has almost certainly re-dialed on some later call and a note would
// mislead more than it helps.
const disconnectNoteTTL = 10 * time.Minute

// mcpDisconnect is one recorded death, kept until a turn reports it.
type mcpDisconnect struct {
	ev mcp.DisconnectEvent
	at time.Time
}

// mcpDisconnectLog records unexpected MCP deaths so the next turn on the
// affected session can tell the model about them. It is small and bounded: one
// entry per (scopeKey, server), overwritten by a newer death.
type mcpDisconnectLog struct {
	mu   sync.Mutex
	byID map[string]mcpDisconnect
	now  func() time.Time // injectable clock for tests; nil => time.Now
}

func newMCPDisconnectLog() *mcpDisconnectLog {
	return &mcpDisconnectLog{byID: map[string]mcpDisconnect{}}
}

func (l *mcpDisconnectLog) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

// record stores a death. The key keeps scoped and shared deaths of the same
// server apart, so one session's lost connection does not mask another's.
func (l *mcpDisconnectLog) record(ev mcp.DisconnectEvent) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.byID[ev.ScopeKey+"\x00"+ev.Server] = mcpDisconnect{ev: ev, at: l.clock()}
}

// take returns the deaths relevant to sessionID and REMOVES them, so the note is
// emitted at most once per server per turn even if the turn is retried.
//
// Relevance: a shared connection serves every session, so its death is reported
// to all of them; a scoped connection belongs to one session and is reported
// only there. Entries older than disconnectNoteTTL are dropped unreported.
func (l *mcpDisconnectLog) take(sessionID string) []mcp.DisconnectEvent {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.clock().Add(-disconnectNoteTTL)
	var out []mcp.DisconnectEvent
	for k, d := range l.byID {
		if d.at.Before(cutoff) {
			delete(l.byID, k) // stale: the pool has long since had a chance to re-dial
			continue
		}
		if d.ev.Scoped && d.ev.SessionID() != sessionID {
			continue // another session's private connection
		}
		out = append(out, d.ev)
		delete(l.byID, k)
	}
	slices.SortFunc(out, func(a, b mcp.DisconnectEvent) int {
		return cmp.Compare(a.Server, b.Server)
	})
	return out
}

// formatDisconnectNote renders the model-facing explanation. It states plainly
// that the tools are unavailable for the REST OF THIS TURN, because that is the
// truth of the native loop: the tool catalog was built at the start of the turn
// and a dead server's tools are not coming back into it mid-turn.
func formatDisconnectNote(evs []mcp.DisconnectEvent) string {
	if len(evs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(evs))
	for _, ev := range evs {
		part := ev.Server
		if ev.Error != "" {
			part += " (" + shortDisconnectErr(ev.Error) + ")"
		}
		if ev.PendingCalls > 0 {
			part += ", interrupting in-flight call(s)"
		}
		parts = append(parts, part)
	}
	return "[mcp] MCP server connection lost: " + strings.Join(parts, "; ") +
		" — its tools are unavailable for the rest of this turn. Do not call them again; " +
		"finish with what you have, or use wait_for_mcp_servers on a later turn to reconnect."
}

// disconnectErrLimit caps one server's error text in the note; the full error
// stays in the log and in the event payload.
const disconnectErrLimit = 160

func shortDisconnectErr(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if len(s) > disconnectErrLimit {
		return s[:disconnectErrLimit] + "…"
	}
	return s
}

// foldMCPDisconnects injects a system note for any MCP server that died since
// the last iteration, mirroring foldSteer: the text is appended to the in-flight
// conversation so the MODEL sees it before its next call, and a step is emitted
// so the USER sees it in the transcript.
//
// The dedupe is the log's take(): an event is removed as it is reported, so a
// server is named at most once per turn no matter how many iterations follow.
// The native loop is the only caller — a CLI provider's MCP clients live in the
// subprocess and never reach our pool.
func (t *toolLoopTurn) foldMCPDisconnects() {
	if t.r == nil || t.r.mcpDeaths == nil {
		return
	}
	evs := t.r.mcpDeaths.take(SessionIDFrom(t.ctx))
	note := formatDisconnectNote(evs)
	if note == "" {
		return
	}
	t.req.Messages = append(t.req.Messages, providers.Message{Role: t.steerRole, Text: note})
	st := TurnStep{Kind: StepRecovery, Reason: MCPDisconnectReason, Text: note}
	t.steps = append(t.steps, st)
	t.emitStep(st)
	t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugError, AgentID: t.agent.ID, Detail: note, Err: true})
}

// MCPDisconnectReason is the machine tag on the emitted StepRecovery, the
// counterpart of repair.FailureReason for a mid-turn death (that one covers a
// server that was already down when the catalog was built).
const MCPDisconnectReason = "mcp_server_disconnected"

// MCPStatusPayload is the Data shape of a ws:mcp_status event. It mirrors
// mcp.DisconnectEvent and adds the op so the type can carry a future
// "reconnected" without a second event type.
type MCPStatusPayload struct {
	Op           string `json:"op"` // "disconnected"
	Server       string `json:"server"`
	Scoped       bool   `json:"scoped"`
	ScopeKey     string `json:"scopeKey,omitempty"`
	SessionID    string `json:"sessionId,omitempty"` // set for a scoped connection
	Error        string `json:"error,omitempty"`
	PendingCalls int    `json:"pendingCalls"`
	At           int64  `json:"at"`
}

// MCPStatusDisconnected is the op for an unexpected connection loss.
const MCPStatusDisconnected = "disconnected"

// handleMCPDisconnect is the pool callback: it records the death for the next
// turn's note and publishes the workspace-stream event for the UI.
func (r *Runtime) handleMCPDisconnect(ev mcp.DisconnectEvent) {
	if r == nil {
		return
	}
	r.mcpDeaths.record(ev)
	p := MCPStatusPayload{
		Op:           MCPStatusDisconnected,
		Server:       ev.Server,
		Scoped:       ev.Scoped,
		ScopeKey:     ev.ScopeKey,
		SessionID:    ev.SessionID(),
		Error:        ev.Error,
		PendingCalls: ev.PendingCalls,
		At:           time.Now().Unix(),
	}
	target := map[string]string{"server": ev.Server, "op": MCPStatusDisconnected}
	if p.SessionID != "" {
		target["sessionId"] = p.SessionID
	}
	r.emitWorkspaceEvent(events.TypeWSMCPStatus, target, p)
}
