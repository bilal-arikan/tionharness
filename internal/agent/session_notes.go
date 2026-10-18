package agent

// Session-level helpers shared by spawn_session, schedules, automations, durable
// asks and auto-continue: injected user notes, agent-authored notes, the
// history-aware turn runner, the active-session check and boot recovery of
// detached spawn turns.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// recordInjectedUserNote persists a runtime-injected user-role note (a gate
// record, an auto-continue nudge) AND bridges it live to the session hub via the
// bus. Interactive user messages already reach the hub from the send-queue worker
// (chat_stream publishHub KindUserMessage); these injected ones bypassed it, so a
// window watching a spawned/autonomous session rendered the assistant reply that
// followed WITHOUT the message it answered — it looked like a duplicate reply
// appearing out of nowhere, and only a page reload restored the real order
// (_Docs/58, _Docs/47). Returns the persisted message so callers can chain.
func (r *Runtime) recordInjectedUserNote(ctx context.Context, sessionID, origin, text string) (db.Message, error) {
	return r.recordInjectedUserMessage(ctx, db.Message{
		SessionID: sessionID,
		Role:      "user",
		Origin:    origin,
		Text:      text,
	})
}

// sessionAgentID resolves which agent owns a session, for attributing a note that
// session's agent wrote into ANOTHER session. It returns "" when there is nothing
// to credit (no session id, or the session is gone) — attribution is a display
// nicety, so a failed lookup must degrade to an unattributed note rather than
// fail the delivery that carries the actual work.
func (r *Runtime) sessionAgentID(ctx context.Context, sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return ""
	}
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return ""
	}
	return sess.AgentID
}

// recordAgentAuthoredNote persists a runtime-injected user-role turn that was
// WRITTEN BY AN AGENT — a spawn prompt —
// attributing it to that agent instead of leaving it an anonymous bubble
// (TSK507).
//
// Attribution is what makes the frontend render it as an incoming agent message
// (MessageList's isPeer → PeerTurn, keyed on role "user" + authorKind "agent" +
// authorId) rather than as the human's own turn, so a reader can tell at a glance
// that another agent said this.
//
// fromAgentID empty means there is no agent author to credit (a human-initiated
// spawn from the UI): the note stays a plain bubble, which is correct — inventing
// an author would misattribute a human's words to an agent.
//
// fromAgentID is also REQUIRED TO RESOLVE to a real agent before it is stamped.
// It is fed from SpawnOptions.CreatedBy, a provenance field that carries
// non-agent origins too ("automation:<id>" — see automation.go), and the frontend
// resolves authorId against the agent roster to name the sender. Stamping an id
// no agent owns would render an unnamed agent bubble, so an unresolvable author
// degrades to a plain note instead.
func (r *Runtime) recordAgentAuthoredNote(ctx context.Context, sessionID, fromAgentID, toAgentID, text string) (db.Message, error) {
	fromAgentID = strings.TrimSpace(fromAgentID)
	if fromAgentID == "" {
		return r.recordInjectedUserNote(ctx, sessionID, "", text)
	}
	if _, err := r.db.GetAgent(ctx, fromAgentID); err != nil {
		return r.recordInjectedUserNote(ctx, sessionID, "", text)
	}
	return r.recordInjectedUserMessage(ctx, db.Message{
		SessionID:   sessionID,
		Role:        "user",
		Text:        text,
		AuthorKind:  db.AuthorAgent,
		AuthorID:    fromAgentID,
		RecipientID: toAgentID,
	})
}

// recordInjectedUserMessage is the general form of recordInjectedUserNote for
// injected user turns that carry extra participant fields (an agent-authored spawn
// prompt stamps AuthorKind/AuthorID/RecipientID; a flow opening prompt is a plain
// bubble). It persists m (Role should be "user") AND bridges it live to the hub.
// Returns the persisted message.
func (r *Runtime) recordInjectedUserMessage(ctx context.Context, m db.Message) (db.Message, error) {
	msg, err := r.db.AddMessage(ctx, m)
	if err != nil {
		return db.Message{}, err
	}
	r.emitInjectedUserNote(msg.SessionID, msg)
	return msg, nil
}

// emitInjectedUserNote broadcasts a just-persisted injected user message so the
// bus→hub bridge (bridgeBusToHub) can render it live on every window watching the
// session, mirroring how emitTurnStart / publishAutonomousReply cover the rest of
// an autonomous turn. No-op on an empty session id or a marshal error (best-effort
// live signal; the durable transcript backstops it on reload).
func (r *Runtime) emitInjectedUserNote(sessionID string, msg db.Message) {
	if sessionID == "" {
		return
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	r.publish(events.Event{
		Type:   events.TypeSessionUserMessage,
		Target: map[string]string{"view": "chat", "sessionId": sessionID},
		Msg:    b,
	})
}

// runSessionTurn runs a history-aware turn for agent on sessionID using the
// installed wake-turn runner (full chat composition: history, author labels, tool
// recap, memory, summary). Falls back to the prompt-only invoke when no runner is
// wired yet (before the api server installs it).
func (r *Runtime) runSessionTurn(ctx context.Context, agent db.Agent, sessionID, prompt string, autonomous bool) (string, []TurnStep, error) {
	if r.wakeTurn != nil {
		return r.wakeTurn(ctx, agent, sessionID, prompt)
	}
	return r.invokeTraced(ctx, agent, prompt, autonomous)
}

// isSessionActive reports whether a session holds at least one autonomous-turn
// registration — running OR queued.
func (r *Runtime) isSessionActive(id string) bool {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	return len(r.activeSessions[id]) > 0
}

// IsSessionActive is isSessionActive for callers outside the package.
func (r *Runtime) IsSessionActive(id string) bool { return r.isSessionActive(id) }

// RecoverOrphanedTurns reclaims detached spawn turns (spawn_session children)
// that were mid-flight when the process died. They run as fire-and-forget
// goroutines with no crash sidecar, so a restart left the session ending on a
// USER message with no reply. At boot each such session gets an interrupted
// assistant reply so it no longer reads as frozen (and a second boot skips it).
//
// Best-effort and idempotent. Detection is "a spawned session whose LAST message
// is a user turn" — at boot nothing is running, so that is exactly a killed
// mid-turn.
func (r *Runtime) RecoverOrphanedTurns(ctx context.Context) {
	sessions, err := r.db.ListSessions(ctx, "")
	if err != nil {
		r.logger.Error("orphan recovery skipped; session list failed", "error", err)
		r.emitDebug(ctx, db.DebugEvent{
			Type:   db.DebugError,
			Name:   "orphan_recovery_failed",
			Detail: "orphaned-turn recovery skipped: session list failed",
			Error:  err.Error(),
			Err:    true,
		})
		return
	}
	for _, sess := range sessions {
		if sess.State == "archived" {
			continue // do not resurrect work the user has archived
		}
		// A spawn_session child is a "chat" by kind (TSK1005), so the lineage
		// says whether it ran a detached background turn.
		if sess.Kind != "spawned" && sess.Lineage().Kind != db.OriginSpawn {
			continue
		}
		if r.isSessionActive(sess.ID) {
			continue
		}
		msgs, err := r.db.ListMessages(ctx, sess.ID)
		if err != nil || len(msgs) == 0 {
			continue
		}
		if msgs[len(msgs)-1].Role != "user" {
			continue // finished
		}
		r.recordInterruptedReply(ctx, sess, "⚠️ Spawn turu süreç yeniden başlarken yarıda kaldı.")
		r.logger.Info("recover: orphaned spawn reclaimed", "session", sess.ID)
	}
}

// recordInterruptedReply persists a crash-recovered assistant reply on an
// orphaned session so it reads as finished (Interrupted) instead of hanging on a
// trailing user message. Best-effort; a write failure is logged, not fatal.
func (r *Runtime) recordInterruptedReply(ctx context.Context, sess db.Session, text string) {
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID:   sess.ID,
		AgentID:     sess.AgentID,
		Role:        "assistant",
		Text:        text,
		Interrupted: true,
	}); err != nil {
		r.logger.Warn("recover: failed to record interrupted reply", "session", sess.ID, "error", err)
	}
}

// nowMs is the wall clock in unix milliseconds.
func nowMs() int64 { return time.Now().UnixMilli() }
