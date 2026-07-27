package db

// SessionAsk is a durably-suspended human-in-the-loop prompt (Durable Ask, MVP):
// a native chat turn that called ask_user at a "clean" tool-loop point is parked
// here instead of blocking a goroutine, so the wait survives a restart/crash. It
// mirrors the flow await-input waiting model (see FlowRun waiting status).
//
// Lifecycle: created in SessionAskWaiting; ClaimSessionAsk CAS-transitions it to
// SessionAskResolved (exactly one answerer wins) and stamps Answer; the sweeper
// fails an expired one to SessionAskTimeout; a stopped/cancelled turn closes it to
// SessionAskCancelled. Only Waiting rows are restored (re-rendered) at boot.
type SessionAsk struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId"`
	Status    string `json:"status"`
	// Kind is the interaction kind. MVP: "ask" (ask_user). permission/plan later.
	Kind string `json:"kind"`
	// CallID is the pending ask_user tool_use id: on resume the answer is folded
	// back as this call's tool_result so the loop continues in-place.
	CallID string `json:"callId"`
	// Payload is the card descriptor (question/options/…), persisted so a window
	// re-opened after a restart can re-render the prompt from disk.
	Payload string `json:"payload"`
	// State is the serialized suspend snapshot the resume driver re-enters with
	// (marshalled req.Messages + pre-suspend steps + turn meta + iter + active
	// tools). Opaque to the DB layer.
	State string `json:"state"`
	// Answer is the winning reply, stamped atomically by ClaimSessionAsk.
	Answer string `json:"answer,omitempty"`
	// TimeoutSec bounds the wait (0 = no timeout) for the sweeper.
	TimeoutSec int   `json:"timeoutSec,omitempty"`
	CreatedAt  int64 `json:"createdAt"`
	UpdatedAt  int64 `json:"updatedAt"`
}

// SessionAsk lifecycle statuses.
const (
	SessionAskWaiting   = "waiting"
	SessionAskResolved  = "resolved"
	SessionAskCancelled = "cancelled"
	SessionAskTimeout   = "timeout"
)
