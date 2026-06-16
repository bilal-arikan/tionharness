package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/providers"
)

// askInput is the ask shape for the ask_user tool.
type askInput struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

// AskUserTool lets the agent pause and ask the user a clarifying question,
// blocking the turn until the user answers. It only works in interactive chat
// (where the SSE stream stays open and an asker is wired into the context);
// in autonomous runs it returns an error so the model proceeds on its own.
type AskUserTool struct{}

// NewAskUserTool constructs the ask_user tool.
func NewAskUserTool() AskUserTool { return AskUserTool{} }

func (AskUserTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "ask_user",
		Description: "Ask the user a clarifying question and wait for their answer. " +
			"Use ONLY when you genuinely cannot proceed without input (ambiguous " +
			"requirement, a risky choice, missing detail). Optionally provide a few " +
			"suggested answers; the user may also type a free-text reply.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "question": { "type": "string", "description": "The question to ask the user." },
    "options": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Optional suggested answers shown as clickable choices."
    }
  },
  "required": ["question"],
  "additionalProperties": false
}`),
	}
}

func (AskUserTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in askInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid ask_user input: %w", err)
	}
	if strings.TrimSpace(in.Question) == "" {
		return "", fmt.Errorf("question is required")
	}
	ask := askerFrom(ctx)
	if ask == nil {
		return "", fmt.Errorf("ask_user is only available in interactive chat sessions; proceed without asking")
	}
	answer, err := ask(ctx, in.Question, in.Options)
	if err != nil {
		return "", err
	}
	return answer, nil
}
