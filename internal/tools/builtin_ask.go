package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/providers"
)

// flexStringSlice deserialises a JSON value that may be either a string or an
// array of strings — models occasionally emit "options": "single" instead of
// the documented array form, which would otherwise crash the decoder.
type flexStringSlice []string

func (f *flexStringSlice) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*f = arr
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s != "" {
		*f = []string{s}
	}
	return nil
}

// askInput is the ask shape for the ask_user tool.
type askInput struct {
	Question string          `json:"question"`
	Options  flexStringSlice `json:"options"`
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
	// Bail early in autonomous/flow runs — avoids surfacing a JSON parse error
	// when the model passes a malformed payload that would never be answered anyway.
	ask := askerFrom(ctx)
	if ask == nil {
		return "", fmt.Errorf("ask_user is only available in interactive chat sessions; proceed without asking")
	}
	var in askInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid ask_user input: %w", err)
	}
	if strings.TrimSpace(in.Question) == "" {
		return "", fmt.Errorf("question is required")
	}
	answer, err := ask(ctx, in.Question, in.Options)
	if err != nil {
		return "", err
	}
	return answer, nil
}
