package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// inbound.go is the RECIPIENT-side gate for messages addressed at an agent:
// send_message (peer inbox) and send_to_worker (coordinator → worker). Two
// guarantees live here, both absent before:
//
//  1. Inbound policy. The recipient (agent, or one of its sessions) decides
//     whether a message is accepted, held for approval, or refused, instead of
//     every sender being able to start a background turn unconditionally.
//  2. A durable delivery receipt (db.AgentMessage). Every attempt ends in
//     accepted / held / refused / dropped, so a message NEVER disappears
//     silently: a refusal and a capacity drop are both readable by the sender
//     afterwards, not just a transient tool-error string.
//
// The size guard (message_too_large) is enforced here too, so all three delivery
// paths share one limit and one error shape.

// ErrMessageTooLarge is the sentinel behind every "message_too_large" failure.
// Callers match it with errors.Is; the wrapped text carries the actual sizes.
var ErrMessageTooLarge = errors.New("message_too_large")

// messageTooLarge builds the explicit, machine-greppable size error. path names
// the delivery path so a failing agent knows which call to shrink.
func messageTooLarge(path string, size, max int) error {
	return fmt.Errorf("%w: %s message is %d bytes, over the %d byte limit; shorten it (or write the detail to a file/artifact and send the handle)",
		ErrMessageTooLarge, path, size, max)
}

// checkMessageSize rejects an oversized message body. It measures BYTES (the
// configured unit) but reports them as-is; nothing is truncated on this path —
// an over-limit send_message / send_to_worker fails loudly so the sender can
// decide what to drop, rather than the runtime guessing for it.
func (r *Runtime) checkMessageSize(path, message string) error {
	max := r.tun.AgentMessageMaxBytes()
	if n := len(message); n > max {
		return messageTooLarge(path, n, max)
	}
	return nil
}

// capNotification bounds the ONE-WAY worker→coordinator notification path. There
// is no sender turn left to hand an error back to, so instead of dropping the
// note (which would leave the coordinator waiting forever for a worker that has
// already finished) the note is cut at the byte limit and the cut is stated
// explicitly with the same message_too_large marker. Rune-safe: the cut never
// splits a multi-byte UTF-8 sequence.
func capNotification(note string, max int) (string, bool) {
	if max <= 0 || len(note) <= max {
		return note, false
	}
	cut := max
	for cut > 0 && !utf8.ValidString(note[:cut]) {
		cut--
	}
	return note[:cut] + fmt.Sprintf(
		"\n\n[message_too_large: %d bayt sınırı aşıldı (%d bayt); bildirim kırpıldı — ayrıntı için worker oturumunu aç]",
		max, len(note)), true
}

// inboundPolicyFor resolves the effective policy for a delivery: the target
// SESSION's setting wins when set, otherwise the recipient AGENT's, otherwise
// accept. Missing rows and invalid stored values are errors — a delivery is
// never silently let through on a lookup failure.
func (r *Runtime) inboundPolicyFor(ctx context.Context, agentID, sessionID string) (string, error) {
	var sessionPolicy, agentPolicy string
	if sessionID != "" {
		sess, err := r.db.GetSession(ctx, sessionID)
		if err != nil {
			return "", fmt.Errorf("cannot read inbound policy of session %s: %w", sessionID, err)
		}
		sessionPolicy = sess.InboundPolicy
	}
	if agentID != "" {
		ag, err := r.db.GetAgent(ctx, agentID)
		if err != nil {
			return "", fmt.Errorf("cannot read inbound policy of agent %s: %w", agentID, err)
		}
		agentPolicy = ag.InboundPolicy
	}
	return db.ResolveInboundPolicy(sessionPolicy, agentPolicy)
}

// errRefused reports a policy refusal to the sender, quoting the receipt id so
// the refusal can be looked up later.
type errRefused struct{ receipt db.AgentMessage }

func (e *errRefused) Error() string {
	return fmt.Sprintf("delivery refused by the recipient's inbound policy (receipt %s, status %q)", e.receipt.ID, e.receipt.Status)
}

// gateInbound applies the size guard and the inbound policy to one pending
// delivery and records its receipt. The returned receipt is ALWAYS persisted:
//
//	accepted → the caller proceeds with the actual delivery and, if that fails,
//	           must downgrade the receipt with dropDelivery (never silently).
//	held     → the caller must NOT deliver; the body is parked in the receipt and
//	           a later ReleaseHeldMessage completes the delivery.
//	refused  → the caller returns the error to the sender.
func (r *Runtime) gateInbound(ctx context.Context, pending db.AgentMessage) (db.AgentMessage, error) {
	path := "send_message"
	if pending.Channel == db.ChannelWorker {
		path = "send_to_worker"
	}
	if err := r.checkMessageSize(path, pending.Body); err != nil {
		return db.AgentMessage{}, err
	}
	policy, err := r.inboundPolicyFor(ctx, pending.ToAgentID, pending.ToSessionID)
	if err != nil {
		return db.AgentMessage{}, err
	}
	pending.Policy = policy
	switch policy {
	case db.InboundAccept:
		pending.Status = db.DeliveryAccepted
	case db.InboundHold:
		pending.Status = db.DeliveryHeld
		pending.Reason = "recipient inbound policy is \"hold\": the message is parked and waits for approval"
	case db.InboundRefuse:
		pending.Status = db.DeliveryRefused
		pending.Reason = "recipient inbound policy is \"refuse\""
	}
	receipt, err := r.db.CreateAgentMessage(ctx, pending)
	if err != nil {
		return db.AgentMessage{}, err
	}
	if receipt.Status == db.DeliveryRefused {
		return receipt, &errRefused{receipt: receipt}
	}
	return receipt, nil
}

// dropDelivery downgrades an accepted receipt to "dropped" after the delivery
// itself failed, so the sender's record matches what actually happened. The
// original cause is returned unchanged; a bookkeeping failure is logged, never
// substituted for it.
func (r *Runtime) dropDelivery(ctx context.Context, receiptID string, cause error) error {
	if receiptID == "" {
		return cause
	}
	if _, err := r.db.MarkAgentMessageDropped(ctx, receiptID, cause.Error()); err != nil {
		r.logger.Warn("inbound: failed to mark delivery dropped", "receipt", receiptID, "error", err)
	}
	return fmt.Errorf("%w (delivery receipt %s, status %q)", cause, receiptID, db.DeliveryDropped)
}

// heldNotice is what a sender is told when its message was parked. It is a
// SUCCESSFUL return (the message is stored and will be delivered on approval),
// but it deliberately never claims the recipient has seen it.
func heldNotice(receipt db.AgentMessage) string {
	return fmt.Sprintf("Message HELD (receipt %s): the recipient's inbound policy is %q, so the message is stored and waits for approval before delivery. It was NOT delivered yet and no turn was started.",
		receipt.ID, receipt.Policy)
}

// ListHeldMessages returns the messages parked for approval, oldest first. Pass
// an empty agentID for every recipient in the workspace.
func (r *Runtime) ListHeldMessages(ctx context.Context, agentID string) ([]db.AgentMessage, error) {
	return r.db.ListHeldAgentMessages(ctx, agentID)
}

// ReleaseHeldMessage approves a parked message and delivers it. The receipt is
// CAS-moved held → accepted first, so two concurrent approvals cannot deliver
// the same message twice; if the delivery then fails the receipt is downgraded
// to dropped and the error is returned.
func (r *Runtime) ReleaseHeldMessage(ctx context.Context, id string) (db.AgentMessage, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return db.AgentMessage{}, fmt.Errorf("a held message id is required")
	}
	receipt, err := r.db.ResolveHeldAgentMessage(ctx, id, db.DeliveryAccepted, "released for delivery")
	if err != nil {
		return db.AgentMessage{}, err
	}
	if err := r.deliverReleased(ctx, receipt); err != nil {
		return db.AgentMessage{}, r.dropDelivery(ctx, receipt.ID, err)
	}
	return receipt, nil
}

// RefuseHeldMessage rejects a parked message. The body stays on the receipt, so
// the refusal is auditable and the sender can see why nothing was delivered.
func (r *Runtime) RefuseHeldMessage(ctx context.Context, id, reason string) (db.AgentMessage, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return db.AgentMessage{}, fmt.Errorf("a held message id is required")
	}
	if strings.TrimSpace(reason) == "" {
		reason = "refused by the recipient"
	}
	return r.db.ResolveHeldAgentMessage(ctx, id, db.DeliveryRefused, reason)
}

// deliverReleased performs the delivery a held receipt was waiting for, on the
// channel it was captured from. An unknown channel is an error, not a no-op.
func (r *Runtime) deliverReleased(ctx context.Context, receipt db.AgentMessage) error {
	switch receipt.Channel {
	case db.ChannelInbox:
		target, err := r.db.GetAgent(ctx, receipt.ToAgentID)
		if err != nil {
			return fmt.Errorf("recipient agent %s is gone: %w", receipt.ToAgentID, err)
		}
		return r.deliverToInbox(ctx, receipt.FromAgentID, receipt.FromName, target, receipt.Summary, receipt.Body)
	case db.ChannelWorker:
		return r.deliverToWorker(ctx, receipt.ToSessionID, receipt.Body)
	}
	return fmt.Errorf("held message %s has an unknown delivery channel %q", receipt.ID, receipt.Channel)
}
