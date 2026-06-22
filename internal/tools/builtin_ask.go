package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
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
	// Bail early in autonomous/flow runs before touching the payload. IsAutonomous
	// is the fast path (set by WithCallKind for every non-chat entry point);
	// askerFrom falls back for contexts that were not stamped with WithCallKind.
	if IsAutonomous(ctx) || askerFrom(ctx) == nil {
		// In an asynchronous chat run (a schedule_wake turn delivered back into its
		// live session) the user IS present, just not able to answer a blocking
		// prompt mid-turn. Tell the model to ask in its normal reply and end the
		// turn so the user can read it and respond in the chat — rather than the
		// fully-headless "proceed without asking", which would make it guess.
		if IsAsyncChat(ctx) {
			return "", fmt.Errorf("ask_user can't block in this asynchronous chat turn. Write your question as your normal reply text and end the turn; the user will read it and answer in the chat, which continues the conversation")
		}
		return "", fmt.Errorf("ask_user is only available in interactive chat sessions; proceed without asking")
	}
	ask := askerFrom(ctx)
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
