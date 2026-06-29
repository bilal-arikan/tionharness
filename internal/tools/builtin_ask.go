package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// askOption deserialises one suggested answer that may arrive either as a plain
// string OR as a claude-cli AskUserQuestion-style object ({label|content|value|
// text|description}). It normalises to the display string SwarmGo shows as a
// clickable choice — models trained on the native tool emit the object form, and
// without this the strict []string decode failed (the SES73 ask_user errors).
type askOption string

func (o *askOption) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*o = askOption(s)
		return nil
	}
	var obj struct {
		Label       string `json:"label"`
		Content     string `json:"content"`
		Value       string `json:"value"`
		Text        string `json:"text"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	switch {
	case obj.Label != "":
		*o = askOption(obj.Label)
	case obj.Content != "":
		*o = askOption(obj.Content)
	case obj.Value != "":
		*o = askOption(obj.Value)
	case obj.Text != "":
		*o = askOption(obj.Text)
	default:
		*o = askOption(obj.Description)
	}
	return nil
}

// flexOptions deserialises the "options" field whatever shape the model sends:
// an array of strings, an array of native option objects, a mix of the two, or a
// single scalar (string or object). Empty/blank entries are dropped. The result
// is always a clean []string for the UI's clickable choices.
type flexOptions []string

func (f *flexOptions) UnmarshalJSON(data []byte) error {
	var arr []askOption
	if err := json.Unmarshal(data, &arr); err == nil {
		out := make([]string, 0, len(arr))
		for _, o := range arr {
			if s := strings.TrimSpace(string(o)); s != "" {
				out = append(out, s)
			}
		}
		*f = out
		return nil
	}
	var one askOption
	if err := json.Unmarshal(data, &one); err != nil {
		return err
	}
	if s := strings.TrimSpace(string(one)); s != "" {
		*f = []string{s}
	}
	return nil
}

// askInput is the ask shape for the ask_user tool. It accepts SwarmGo's native
// {question, options} form AND claude-cli's AskUserQuestion {questions:[...]}
// wrapper (only the first question is used — SwarmGo asks one question per call).
type askInput struct {
	Question  string      `json:"question"`
	Options   flexOptions `json:"options"`
	Questions []struct {
		Question string      `json:"question"`
		Options  flexOptions `json:"options"`
	} `json:"questions"`
}

// ParseAskInput tolerantly decodes an ask_user payload, normalising every shape
// the model might send (SwarmGo's {question, options} and claude-cli's native
// AskUserQuestion: option objects and/or a questions[] wrapper) into a single
// question string + clean []string options. Shared by the native tool path and
// the claude-cli Interaction MCP bridge so both decode identically.
func ParseAskInput(raw json.RawMessage) (string, []string, error) {
	var in askInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", nil, err
	}
	question := strings.TrimSpace(in.Question)
	options := []string(in.Options)
	if question == "" && len(in.Questions) > 0 {
		question = strings.TrimSpace(in.Questions[0].Question)
		if len(options) == 0 {
			options = []string(in.Questions[0].Options)
		}
	}
	return question, options, nil
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
		// Schema mirrors claude-cli's native AskUserQuestion where it overlaps so a
		// model trained on that tool calls this one without a shape mismatch: an
		// option may be a plain string OR an object with a "label" (the native form).
		// Extra native fields (header/multiSelect/description) are accepted and
		// ignored; a native questions[] wrapper is tolerated too (see ParseAskInput).
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "question": { "type": "string", "description": "The question to ask the user." },
    "options": {
      "type": "array",
      "description": "Optional suggested answers shown as clickable choices. Each item may be a plain string OR an object with a \"label\" (AskUserQuestion-compatible).",
      "items": {
        "oneOf": [
          { "type": "string" },
          {
            "type": "object",
            "properties": {
              "label": { "type": "string", "description": "The choice text shown to the user." },
              "description": { "type": "string", "description": "Optional longer explanation (ignored by SwarmGo)." }
            }
          }
        ]
      }
    }
  },
  "required": ["question"]
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
	question, options, err := ParseAskInput(input)
	if err != nil {
		return "", argErrFor("ask_user", err)
	}
	if question == "" {
		return "", fmt.Errorf("question is required")
	}
	answer, err := ask(ctx, question, options)
	if err != nil {
		return "", err
	}
	return answer, nil
}
