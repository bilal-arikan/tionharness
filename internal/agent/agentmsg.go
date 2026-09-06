package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// inboxSessionKind is the kind of the persistent per-agent session that
// accumulates direct messages from other agents. One such thread per recipient
// agent.
//
// It deliberately uses the ordinary, WRITABLE "chat" kind rather than a
// dedicated read-only "inbox" kind (TSK507). A peer thread is a real, live,
// single-agent conversation — not an orchestrator-owned run log — so the human
// must be able to join it from the composer: read what another agent sent, then
// answer, correct or add context in the same thread. Owning a separate kind was
// the only thing that made it read-only, since just the kinds listed in
// db.WritableSessionKinds accept a new user turn.
const inboxSessionKind = "chat"

// inboxSessionSource builds the stable SourceID identifying one agent's peer
// thread among its other "chat" sessions.
//
// The lookup MUST be keyed by (kind, sourceID) — GetOrCreateSourceSession — and
// never by (agentID, kind) alone: "chat" is also the kind of every ad-hoc
// session a human opens, so an (agentID, "chat") lookup would silently adopt the
// user's own first chat with that agent and deliver peer messages into it.
//
// The recipient's id is part of the SourceID because GetOrCreateSourceSession
// matches on (kind, sourceID) and ignores agentID: a constant source would make
// every agent in the workspace share one single thread.
//
// Delegates to db.PeerThreadSourceID so this and the boot migration that stamps
// the same id onto converted legacy sessions (db.migrateLegacyInboxSessions)
// cannot drift: if they disagreed, a migrated thread would not be found here and
// the next delivery would open a second one beside it.
func inboxSessionSource(agentID string) string {
	return db.PeerThreadSourceID(agentID)
}

// inboxSessionTitle labels the per-agent peer-message thread in the sidebar. It
// carries the recipient's name because an agent may own several "chat" sessions
// and this one is not an ad-hoc chat but the standing thread that every
// send_message delivery appends to.
func inboxSessionTitle(agentName string) string {
	name := strings.TrimSpace(agentName)
	if name == "" {
		return "💬 Mesajlar"
	}
	return "💬 " + name + " — Mesajlar"
}

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
	receipt, err := r.deliverOne(ctx, fromAgentID, fromName, target, summary, message)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Message delivered to %q (inbox, receipt %s). It processes it in the background; the reply is NOT relayed here — it may message you back with send_message.", target.Name, receipt.ID), nil
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
		_, err := r.deliverOne(ctx, fromAgentID, fromName, a, summary, message)
		if err != nil {
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
		out += fmt.Sprintf(" %d skipped (refused or over the delivery limit).", skipped)
	}
	return out, nil
}

// deliverOne runs the recipient-side gate (size limit + inbound policy, see
// inbound.go) for one recipient and, when the policy accepts, performs the
// delivery. It returns the durable receipt so the caller can tell the sender
// exactly what happened — accepted, held, or (via the error) refused/dropped.
// fromAgentID is the sender (the message author in the participant model);
// fromName is its display name for the visible tag.
func (r *Runtime) deliverOne(ctx context.Context, fromAgentID, fromName string, target db.Agent, summary, message string) (db.AgentMessage, error) {
	// ToSessionID is deliberately empty here: the inbox session is created on
	// demand by deliverToInbox, so a peer DM is gated on the recipient AGENT's
	// policy. A per-session override applies to sessions that already exist
	// (worker sessions — see SendToWorker).
	receipt, err := r.gateInbound(ctx, db.AgentMessage{
		FromAgentID: fromAgentID,
		FromName:    fromName,
		ToAgentID:   target.ID,
		Channel:     db.ChannelInbox,
		Summary:     summary,
		Body:        message,
	})
	if err != nil {
		return db.AgentMessage{}, err
	}
	if err := r.deliverToInbox(ctx, fromAgentID, fromName, target, summary, message); err != nil {
		return db.AgentMessage{}, r.dropDelivery(ctx, receipt.ID, err)
	}
	return receipt, nil
}

// deliverToInbox appends the sender-tagged message to one recipient's inbox and
// fires its background turn (fire-and-forget). Takes a concurrency slot, released
// when the inbox turn finishes. Called only after the gate accepted the delivery
// (directly, or later when a held message is released).
func (r *Runtime) deliverToInbox(ctx context.Context, fromAgentID, fromName string, target db.Agent, summary, message string) error {
	if !r.acquireSpawnSlot() {
		return fmt.Errorf("message delivery limit reached (%d concurrent background turns); try again once some finish", r.tun.SpawnMaxConcurrent())
	}
	inbox, err := r.db.GetOrCreateSourceSession(ctx, inboxSessionKind, inboxSessionSource(target.ID), target.ID, inboxSessionTitle(target.Name))
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
	// Own cancelable context for the delivery turn, registered BEFORE the goroutine
	// starts: send_agent_message returns to the sender's tool loop immediately, and a
	// "Durdur" on the recipient's inbox in that window must find something to cancel.
	// Registering it inside the goroutine — after a turn-slot claim that can queue
	// behind any other turn on the inbox session — left that window uncancellable.
	runCtx, cancelRun := context.WithCancel(context.Background())
	run := r.trackSession(inbox.ID, cancelRun)
	// Fire-and-forget: process detached from the caller's context.
	go r.runInboxDelivery(runCtx, cancelRun, run, target, inbox.ID, text)
	return nil
}

// runInboxDelivery executes the recipient's background turn on its inbox session:
// it runs a history-aware turn (so the agent sees the whole inbox thread, not just
// the new message), records the reply (or failure) as an assistant turn, then
// releases the concurrency slot and notifies. Mirrors runSpawn but on a durable
// inbox session.
//
// runCtx/cancelRun are created and registered by the caller (SendAgentMessage)
// before this goroutine starts, so the delivery is cancellable from the moment the
// send returns — including while it is still queued for the session's turn slot.
// run is that registration's handle: the untracks below must remove OUR marker
// only, since a turn queued behind us registers its own cancel before it waits.
func (r *Runtime) runInboxDelivery(runCtx context.Context, cancelRun context.CancelFunc, run *sessionRun, agent db.Agent, inboxID, prompt string) {
	defer r.releaseSpawnSlot()
	defer cancelRun()

	// Same hard ceiling + idle watchdog as spawn/worker turns: a productive turn
	// runs up to SpawnTimeout, a hung one is reclaimed after SpawnIdleTimeout.
	hardCap, idleCap := time.Duration(0), r.tun.SpawnIdleTimeout()

	// Serialize this peer delivery with any concurrent turn on the same session
	// (user chat / inbox worker / wake) — and, for a coordinator, its auto turns —
	// via the single per-session turn slot.
	release, slotErr := r.claimSessionTurnSlotCtx(runCtx, inboxID, turnqueue.KindPeer, "ajan mesajı")
	defer release()
	if slotErr != nil {
		// Stopped while waiting for the slot: the turn never ran, so record nothing and
		// close the inbox indicator with a failed outcome instead of starting work
		// nobody is waiting for.
		r.logger.Info("agent message: cancelled before its turn started",
			"session", inboxID, "agent", agent.ID)
		r.untrackSessionRun(inboxID, run)
		r.emitInboxEvent(agent, inboxID, false)
		return
	}

	// Single-shot idle-resume (FND-708844f8): an idle-cut inbox turn gets ONE more
	// attempt under a fresh window before reconcileTurnOutcome marks it unfinished.
	// Mark as async chat (a human may read the inbox) + autonomous, and stamp the
	// session so the history-aware runner targets it.
	var (
		turnCtx context.Context
		meta    *turnMeta
	)
	turnStart := time.Now()
	ctx, cancel, output, steps, err := r.runTurnWithIdleResume(runCtx, hardCap, idleCap, r.tun.IdleResumeMax(),
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
	r.untrackSessionRun(inboxID, run)

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
