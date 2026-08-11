package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/tools"
	"github.com/bilal-arikan/tionswarm/internal/turnqueue"
)

// inboxSessionKind is the persistent per-agent session that accumulates direct
// messages from other agents (the agent's "📥 Inbox"). One per recipient agent.
const inboxSessionKind = "inbox"

// formatAgentMessage wraps a peer message with its sender identity, mirroring
// Claude Code's <teammate_message teammate_id=…> tag: the recipient sees exactly
// who wrote it and can reply by sending back to that name.
func formatAgentMessage(from, summary, message string) string {
	attrs := fmt.Sprintf(" from=%q", from)
	if s := strings.TrimSpace(summary); s != "" {
		attrs += fmt.Sprintf(" summary=%q", s)
	}
	return fmt.Sprintf("<agent_message%s>\n%s\n</agent_message>", attrs, message)
}

// DeliverAgentMessage sends a direct message from one agent to another: it appends
// the sender-tagged message to the recipient's persistent inbox session and runs
// the recipient's turn in the background (fire-and-forget), exactly like a spawn
// but into a durable inbox rather than a fresh session. The sender returns
// immediately; the recipient's reply lands in its inbox (not back in the sender's
// conversation — the recipient may message back with send_message).
//
// Guards: the shared SpawnMaxConcurrent cap bounds background turns; the turn is
// autonomous (daily budget applies); resolveAgent is workspace-scoped so a message
// can never cross workspaces; self-messaging is rejected.
func (r *Runtime) DeliverAgentMessage(ctx context.Context, fromAgentID, toRef, summary, message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", fmt.Errorf("message is required")
	}
	fromName := r.agentName(fromAgentID)
	if fromName == "" {
		fromName = "another agent"
	}

	// Broadcast: deliver to every OTHER agent in the workspace (Faz 3). Expensive
	// (one background turn each), so the model is told to use it sparingly.
	if strings.TrimSpace(toRef) == "*" {
		return r.broadcastAgentMessage(ctx, fromAgentID, fromName, summary, message)
	}

	target, err := r.resolveAgent(ctx, toRef)
	if err != nil {
		return "", err
	}
	if target.ID == fromAgentID {
		return "", fmt.Errorf("cannot send a message to yourself")
	}
	if err := r.deliverOne(ctx, fromAgentID, fromName, target, summary, message); err != nil {
		return "", err
	}
	return fmt.Sprintf("Message delivered to %q (inbox). It processes it in the background; the reply is NOT relayed here — it may message you back with send_message.", target.Name), nil
}

// broadcastAgentMessage delivers a message to every other agent's inbox. Each
// delivery takes a background-turn slot; recipients past the concurrency cap are
// skipped (best-effort) and reported, rather than failing the whole broadcast.
func (r *Runtime) broadcastAgentMessage(ctx context.Context, fromAgentID, fromName, summary, message string) (string, error) {
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		return "", err
	}
	delivered, skipped := 0, 0
	for _, a := range agents {
		if a.ID == fromAgentID {
			continue
		}
		if err := r.deliverOne(ctx, fromAgentID, fromName, a, summary, message); err != nil {
			skipped++
			r.logger.Warn("broadcast: delivery skipped", "to", a.ID, "error", err)
			continue
		}
		delivered++
	}
	if delivered == 0 {
		if skipped > 0 {
			return "", fmt.Errorf("broadcast reached no one (%d recipient(s) over the delivery limit); try again shortly", skipped)
		}
		return "", fmt.Errorf("no other agents in this workspace to broadcast to")
	}
	out := fmt.Sprintf("Broadcast delivered to %d agent(s); each processes it in its own inbox in the background.", delivered)
	if skipped > 0 {
		out += fmt.Sprintf(" %d skipped (delivery limit).", skipped)
	}
	return out, nil
}

// deliverOne appends the sender-tagged message to one recipient's inbox and fires
// its background turn (fire-and-forget). Takes a concurrency slot, released when
// the inbox turn finishes. fromAgentID is the sender (the message author in the
// participant model); fromName is its display name for the visible tag.
func (r *Runtime) deliverOne(ctx context.Context, fromAgentID, fromName string, target db.Agent, summary, message string) error {
	if !r.acquireSpawnSlot() {
		return fmt.Errorf("message delivery limit reached (%d concurrent background turns); try again once some finish", r.tun.SpawnMaxConcurrent())
	}
	inbox, err := r.db.GetOrCreateKindSession(ctx, target.ID, inboxSessionKind, "📥 Inbox")
	if err != nil {
		r.releaseSpawnSlot()
		return err
	}
	text := formatAgentMessage(fromName, summary, message)
	// Generic participant model: the inbox message is projected to the recipient
	// as a "user"-role input (so it drives the turn), but its true author is the
	// SENDING agent — record that so the roster + labelMultiAgentHistory attribute
	// it correctly ("[Ada → Kai (you)]"), not as a human "user" message. The
	// formatAgentMessage wrapper is kept for the human-readable inbox view + summary.
	// Bridge the delivery to the hub too (session_user_message → KindUserMessage,
	// _Docs/58) so a window watching the recipient's inbox renders the incoming peer
	// message live and in order before the reply, not only on reload.
	if _, err := r.recordInjectedUserMessage(ctx, db.Message{
		SessionID:   inbox.ID,
		Role:        "user",
		Text:        text,
		AuthorKind:  db.AuthorAgent,
		AuthorID:    fromAgentID,
		RecipientID: target.ID,
	}); err != nil {
		r.releaseSpawnSlot()
		return err
	}
	r.logger.Info("agent message: delivered", "to", target.ID, "session", inbox.ID)
	// Fire-and-forget: process detached from the caller's context.
	go r.runInboxDelivery(target, inbox.ID, text)
	return nil
}

// runInboxDelivery executes the recipient's background turn on its inbox session:
// it runs a history-aware turn (so the agent sees the whole inbox thread, not just
// the new message), records the reply (or failure) as an assistant turn, then
// releases the concurrency slot and notifies. Mirrors runSpawn but on a durable
// inbox session.
func (r *Runtime) runInboxDelivery(agent db.Agent, inboxID, prompt string) {
	defer r.releaseSpawnSlot()

	// Same hard ceiling + idle watchdog as spawn/worker turns: a productive turn
	// runs up to SpawnTimeout, a hung one is reclaimed after SpawnIdleTimeout.
	hardCap, idleCap := r.tun.SpawnTimeout(), r.tun.SpawnIdleTimeout()

	// Serialize this peer delivery with any concurrent turn on the same session
	// (user chat / inbox worker / wake) — and, for a coordinator, its auto turns —
	// via the single per-session turn slot.
	release := r.claimSessionTurnSlot(inboxID, turnqueue.KindPeer, "ajan mesajı")
	defer release()

	r.trackSession(inboxID)

	// Single-shot idle-resume (FND-708844f8): an idle-cut inbox turn gets ONE more
	// attempt under a fresh window before reconcileTurnOutcome marks it unfinished.
	// Mark as async chat (a human may read the inbox) + autonomous, and stamp the
	// session so the history-aware runner targets it.
	var (
		turnCtx context.Context
		meta    *turnMeta
	)
	turnStart := time.Now()
	ctx, cancel, output, steps, err := r.runTurnWithIdleResume(context.Background(), hardCap, idleCap, r.tun.IdleResumeMax(),
		func(attemptCtx context.Context, _ context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			turnCtx = tools.WithAsyncChat(WithSessionID(WithCallKind(attemptCtx, KindSpawn), inboxID))
			turnCtx, meta = WithTurnMeta(turnCtx)
			p := prompt
			if attempt > 1 {
				p = resumeContinuationPrompt(prompt, prevOutput)
			}
			return r.runSessionTurn(turnCtx, agent, inboxID, p, true)
		})
	defer cancel()
	r.untrackSession(inboxID)

	// A watchdog cut (hard/idle) or a self-truncated loop hands back salvaged text;
	// lead it with the outcome note (nil error) so it records as an explaining reply,
	// not a clean one, and the inbox event reports failure rather than success.
	output, steps, err, truncated := r.reconcileTurnOutcome(ctx, output, steps, err, hardCap, idleCap)
	if err != nil {
		r.logger.Error("agent message: invoke failed",
			"session", inboxID, "agent", agent.ID,
			"provider", agent.Provider, "model", agent.Model, "error", err)
		r.recordTurnError(ctx, inboxID, agent.ID, err, steps, meta, time.Since(turnStart).Milliseconds(), "⚠️ Inbox mesajı işlenemedi:")
		r.emitInboxEvent(agent, inboxID, false)
		return
	}
	if _, addErr := r.recordAssistantReply(ctx, inboxID, agent.ID, output, steps, meta, time.Since(turnStart).Milliseconds(), "ℹ️ Ajan bu mesaj için boş yanıt döndürdü."); addErr != nil {
		r.logger.Warn("agent message: failed to record reply", "session", inboxID, "error", addErr)
	}
	if truncated {
		r.logger.Warn("agent message: turn truncated", "session", inboxID, "agent", agent.ID, "hardCap", hardCap, "idleCap", idleCap)
	}
	r.logger.Info("agent message: processed", "session", inboxID, "agent", agent.ID)
	r.emitInboxEvent(agent, inboxID, !truncated)
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

// emitInboxEvent publishes a notification for a processed inbox message, deep-
// linking to the inbox transcript in the executions feed.
func (r *Runtime) emitInboxEvent(agent db.Agent, sessionID string, ok bool) {
	level, title := "success", "📥 Mesaj işlendi — "+agent.Name
	if !ok {
		level, title = "error", "📥 Mesaj işlenemedi — "+agent.Name
	}
	r.publish(events.Event{
		Type:   "chat",
		Level:  level,
		Title:  title,
		Target: map[string]string{"view": "executions", "sessionId": sessionID},
	})
}
