package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/providers"
)

// SendMessageResult is the outcome of an inter-agent message: the resolved
// recipient and the inbox session the message landed in.
type SendMessageResult struct {
	AgentName string
	SessionID string
}

// SendMessageFunc delivers a message from one agent to another agent's inbox and
// wakes the recipient so it processes the message on its next autonomous tick.
// Implemented in the agent package (which owns the runtime, the agent store and
// the wake channel) and injected at construction so the tools package need not
// import the agent package. target is a display name or id; the implementation
// resolves it and returns a plain error the model can react to.
//
// Unlike call_agent (synchronous delegation that blocks for a reply), this is
// fire-and-forget: the message is queued and the sender continues immediately.
type SendMessageFunc func(ctx context.Context, fromAgentID, target, message string) (SendMessageResult, error)

// sendAgentMessageInput is the argument shape for the send_agent_message tool.
type sendAgentMessageInput struct {
	Agent   string `json:"agent"`
	Message string `json:"message"`
}

// SendAgentMessageTool lets one agent send an asynchronous message to another
// agent in the same workspace. The message is appended to the recipient's inbox
// session and the recipient is woken to handle it on its own time — the sender
// does not wait for a reply. Use it for hand-offs and notifications in
// multi-agent orchestration; use call_agent when you need an answer back now.
type SendAgentMessageTool struct {
	selfID string
	send   SendMessageFunc
}

// NewSendAgentMessageTool constructs the tool bound to the calling agent's id
// and the delivery function.
func NewSendAgentMessageTool(selfID string, send SendMessageFunc) SendAgentMessageTool {
	return SendAgentMessageTool{selfID: selfID, send: send}
}

func (SendAgentMessageTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "send_agent_message",
		Description: "Send an asynchronous message to another agent in this workspace. " +
			"The message is delivered to that agent's inbox and it is woken to handle it " +
			"on its own; you do NOT wait for a reply (use call_agent when you need an " +
			"answer back immediately). Use this for hand-offs, notifications and " +
			"fire-and-forget coordination between agents.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "agent": { "type": "string", "description": "The name (or id) of the recipient agent." },
    "message": { "type": "string", "description": "The message to deliver to that agent." }
  },
  "required": ["agent", "message"],
  "additionalProperties": false
}`),
	}
}

func (t SendAgentMessageTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in sendAgentMessageInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid send_agent_message input: %w", err)
	}
	in.Agent = strings.TrimSpace(in.Agent)
	in.Message = strings.TrimSpace(in.Message)
	if in.Agent == "" || in.Message == "" {
		return "", fmt.Errorf("both \"agent\" and \"message\" are required")
	}
	if t.send == nil {
		return "", fmt.Errorf("inter-agent messaging is not available in this context")
	}
	res, err := t.send(ctx, t.selfID, in.Agent, in.Message)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Message delivered to agent %q (inbox session %s). It will process the message on its next tick.", res.AgentName, res.SessionID), nil
}
