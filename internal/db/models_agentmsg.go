package db

// models_agentmsg.go owns the inbound-message policy vocabulary and the durable
// DELIVERY RECEIPT for one agent→agent (or coordinator→worker) message.
//
// Before this existed, a peer message that could not be delivered surfaced only
// as a tool error string and left no trace: nothing recorded that a message had
// been refused, and a "hold for approval" state had nowhere to live. The receipt
// closes both gaps — every delivery attempt ends in exactly one of four terminal
// (or, for held, resumable) statuses that the SENDER can read back.

// Delivery receipt statuses. "dropped" is reserved for a delivery that was
// accepted by policy but could not be carried out (capacity, storage error) —
// it exists so such a failure is still recorded rather than vanishing.
const (
	DeliveryAccepted = "accepted"
	DeliveryRefused  = "refused"
	DeliveryDropped  = "dropped"
)

// AgentMessage is the durable receipt for one delivery attempt. It is written on
// EVERY attempt (accepted, held, refused, dropped) so the sender can look up what
// became of a message it sent, and so a held message survives a restart with its
// full body intact and can be released later.
type AgentMessage struct {
	ID string `json:"id"`
	// FromAgentID is the sender. Empty only for a delivery injected by the
	// runtime itself rather than by an agent.
	FromAgentID string `json:"fromAgentId,omitempty"`
	FromName    string `json:"fromName,omitempty"`
	// ToAgentID is the recipient agent; ToSessionID the session the message was
	// (or would be) delivered into — an inbox session for a peer DM, the worker
	// session for send_to_worker.
	ToAgentID   string `json:"toAgentId,omitempty"`
	ToSessionID string `json:"toSessionId,omitempty"`
	// Channel names the delivery path: "inbox" (send_message) or "worker"
	// (send_to_worker). It decides how a released hold is re-dispatched.
	Channel string `json:"channel"`
	Summary string `json:"summary,omitempty"`
	Body    string `json:"body"`
	Status  string `json:"status"`
	// Reason explains a non-accepted status (why it was refused/dropped, or that
	// it awaits approval). Never empty for refused/dropped.
	Reason    string `json:"reason,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Delivery channels.
const (
	ChannelInbox  = "inbox"
	ChannelWorker = "worker"
)
