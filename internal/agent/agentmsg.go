package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/events"
	"github.com/bilal-arikan/swarmgo/internal/tools"
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
	target, err := r.resolveAgent(ctx, toRef)
	if err != nil {
		return "", err
	}
	if target.ID == fromAgentID {
		return "", fmt.Errorf("cannot send a message to yourself")
	}

	// Concurrency guard (shared with spawn): refuse once the background-turn cap is
	// reached. The slot is released when the inbox turn finishes.
	if !r.acquireSpawnSlot() {
		return "", fmt.Errorf("message delivery limit reached (%d concurrent background turns); try again once some finish", r.tun.SpawnMaxConcurrent())
	}

	inbox, err := r.db.GetOrCreateKindSession(ctx, target.ID, inboxSessionKind, "📥 Inbox")
	if err != nil {
		r.releaseSpawnSlot()
		return "", err
	}
	fromName := r.agentName(fromAgentID)
	if fromName == "" {
		fromName = "another agent"
	}
	text := formatAgentMessage(fromName, summary, message)
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: inbox.ID,
		Role:      "user",
		Text:      text,
	}); err != nil {
		r.releaseSpawnSlot()
		return "", err
	}

	r.logger.Info("agent message: delivered",
		"from", fromAgentID, "to", target.ID, "session", inbox.ID)

	// Fire-and-forget: process the inbox turn detached from the caller's context so
	// a finished tool call can never cancel it mid-flight.
	go r.runInboxDelivery(target, inbox.ID, text)

	return fmt.Sprintf("Message delivered to %q (inbox). It processes it in the background; the reply is NOT relayed here — it may message you back with send_message.", target.Name), nil
}

// runInboxDelivery executes the recipient's background turn on its inbox session:
// it runs a history-aware turn (so the agent sees the whole inbox thread, not just
// the new message), records the reply (or failure) as an assistant turn, then
// releases the concurrency slot and notifies. Mirrors runSpawn but on a durable
// inbox session.
func (r *Runtime) runInboxDelivery(agent db.Agent, inboxID, prompt string) {
	defer r.releaseSpawnSlot()

	ctx, cancel := context.WithTimeout(context.Background(), spawnTimeout)
	defer cancel()

	// Mark as async chat (a human may read the inbox) + autonomous, and stamp the
	// session so the history-aware runner targets it.
	turnCtx := tools.WithAsyncChat(WithSessionID(WithCallKind(ctx, KindSpawn), inboxID))

	r.trackSession(inboxID)
	output, steps, err := r.runSessionTurn(turnCtx, agent, inboxID, prompt, true)
	r.untrackSession(inboxID)

	if err != nil {
		r.logger.Error("agent message: invoke failed",
			"session", inboxID, "agent", agent.ID,
			"provider", agent.Provider, "model", agent.Model, "error", err)
		if _, addErr := r.db.AddMessage(ctx, db.Message{
			SessionID: inboxID,
			AgentID:   agent.ID,
			Role:      "assistant",
			Text:      "⚠️ Inbox mesajı işlenemedi:\n\n" + err.Error(),
			Steps:     encodeSteps(steps),
		}); addErr != nil {
			r.logger.Warn("agent message: failed to record error reply", "session", inboxID, "error", addErr)
		}
		r.emitInboxEvent(agent, inboxID, false)
		return
	}
	if strings.TrimSpace(output) == "" {
		output = "ℹ️ Ajan bu mesaj için boş yanıt döndürdü."
	}
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: inboxID,
		AgentID:   agent.ID,
		Role:      "assistant",
		Text:      output,
		Steps:     encodeSteps(steps),
	}); err != nil {
		r.logger.Warn("agent message: failed to record reply", "session", inboxID, "error", err)
	}
	r.logger.Info("agent message: processed", "session", inboxID, "agent", agent.ID)
	r.emitInboxEvent(agent, inboxID, true)
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
