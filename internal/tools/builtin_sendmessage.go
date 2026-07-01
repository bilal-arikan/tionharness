package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// SendMessageFunc delivers a direct message from the calling agent to another
// agent's inbox and runs that agent's turn in the background (fire-and-forget),
// returning a short confirmation. Implemented in the agent package (which owns the
// runtime) and injected at construction so the tools package need not import it.
type SendMessageFunc func(ctx context.Context, to, summary, message string) (string, error)

// sendMessageInput is the argument shape for the send_message tool.
type sendMessageInput struct {
	To      string `json:"to"`
	Message string `json:"message"`
	Summary string `json:"summary"`
}

// SendMessageTool lets an agent send a direct, addressed message to ANOTHER agent
// in the same workspace. The message lands in the recipient's persistent inbox
// tagged with the sender's identity, and the recipient processes it on its own in
// the background — the sender does not wait and the reply does not return to this
// conversation (the recipient may message back). This is the "peer DM" complement
// to run_subagent (isolated task + result) and spawn (fresh background session).
type SendMessageTool struct {
	selfID string
	send   SendMessageFunc
}

// NewSendMessageTool constructs the tool bound to the calling agent's id and the
// delivery function.
func NewSendMessageTool(selfID string, send SendMessageFunc) *SendMessageTool {
	return &SendMessageTool{selfID: selfID, send: send}
}

func (*SendMessageTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "send_message",
		Description: "Send a direct message to ANOTHER agent in this workspace. The message lands in " +
			"that agent's inbox tagged with your name, and it processes the message on its own, in the " +
			"background — you do NOT wait for it, and its reply does NOT come back into this conversation " +
			"(it may message you back with send_message, landing in YOUR inbox). Your plain reply text is " +
			"NOT visible to other agents; to reach one you MUST use this tool. Refer to the agent by name. " +
			"To REPLY to a message you received, set `to` to the `from` name on that message. Set `to` to " +
			"\"*\" to broadcast to every other agent (expensive — one background turn each; use sparingly). " +
			"Use this for ongoing peer collaboration; use run_subagent when you need a result back in THIS turn.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "to": { "type": "string", "description": "Recipient agent name (or id), or \"*\" to broadcast to all other agents. To reply, use the sender's 'from' name." },
    "message": { "type": "string", "description": "The message to deliver to that agent." },
    "summary": { "type": "string", "description": "Optional 5-10 word preview shown in the UI." }
  },
  "required": ["to", "message"],
  "additionalProperties": false
}`),
	}
}

func (t *SendMessageTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[sendMessageInput]("send_message", input)
	if err != nil {
		return "", err
	}
	in.To = strings.TrimSpace(in.To)
	in.Message = strings.TrimSpace(in.Message)
	in.Summary = strings.TrimSpace(in.Summary)
	if in.To == "" || in.Message == "" {
		return "", fmt.Errorf("both \"to\" and \"message\" are required")
	}
	if t.send == nil {
		return "", fmt.Errorf("agent messaging is not available in this context")
	}
	return t.send(ctx, in.To, in.Summary, in.Message)
}
