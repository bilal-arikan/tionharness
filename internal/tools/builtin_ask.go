package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// askOption deserialises one suggested answer that may arrive either as a plain
// string OR as a claude-cli AskUserQuestion-style object ({label|content|value|
// text|description}). It normalises to the display string TionSwarm shows as a
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

// askInput is the ask shape for the ask_user tool. It accepts TionSwarm's native
// {question, options} form AND claude-cli's AskUserQuestion {questions:[...]}
// wrapper (only the first question is used — TionSwarm asks one question per call).
type askInput struct {
	Question  string      `json:"question"`
	Options   flexOptions `json:"options"`
	Questions []struct {
		Question string      `json:"question"`
		Options  flexOptions `json:"options"`
	} `json:"questions"`
}

// ParseAskInput tolerantly decodes an ask_user payload, normalising every shape
// the model might send (TionSwarm's {question, options} and claude-cli's native
// AskUserQuestion: option objects and/or a questions[] wrapper) into a single
// question string + clean []string options. Shared by the native tool path and
// the claude-cli Interaction MCP bridge so both decode identically. When several
// questions are present only the first is returned (single-question callers).
func ParseAskInput(raw json.RawMessage) (string, []string, error) {
	qs, err := ParseAskInputMulti(raw)
	if err != nil {
		return "", nil, err
	}
	if len(qs) == 0 {
		return "", nil, nil
	}
	return qs[0].Question, qs[0].Options, nil
}

// ParseAskInputMulti decodes an ask_user payload into ALL of its questions,
// tolerating every shape the model might send: TionSwarm's single {question,
// options}, and claude-cli's AskUserQuestion {questions:[{question, options}...]}
// wrapper (option objects or plain strings). Blank questions are dropped. A single
// top-level {question, options} contributes one entry; when both a top-level
// question AND a questions[] wrapper are present, all are kept (top-level first).
func ParseAskInputMulti(raw json.RawMessage) ([]AskQuestion, error) {
	var in askInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	out := make([]AskQuestion, 0, 1+len(in.Questions))
	if q := strings.TrimSpace(in.Question); q != "" {
		out = append(out, AskQuestion{Question: q, Options: []string(in.Options)})
	}
	for _, w := range in.Questions {
		if q := strings.TrimSpace(w.Question); q != "" {
			out = append(out, AskQuestion{Question: q, Options: []string(w.Options)})
		}
	}
	return out, nil
}

// FormatMultiAnswer combines the user's per-question replies into a single labeled
// block for the model. rawAnswer is the client's submission: a JSON array of answer
// strings (one per question, in order). If it does not decode as an array (e.g. an
// older client sent plain text), the whole text is returned unchanged. A missing or
// blank entry is rendered as "(no answer)".
func FormatMultiAnswer(questions []AskQuestion, rawAnswer string) string {
	var answers []string
	if err := json.Unmarshal([]byte(rawAnswer), &answers); err != nil {
		return rawAnswer
	}
	var b strings.Builder
	for i, q := range questions {
		ans := ""
		if i < len(answers) {
			ans = strings.TrimSpace(answers[i])
		}
		if ans == "" {
			ans = "(no answer)"
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "[%d] %s\n→ %s", i+1, q.Question, ans)
	}
	return b.String()
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
		Description: "Ask the user one or more clarifying questions and wait for their answer(s). Use ONLY " +
			"when you genuinely cannot proceed without input (ambiguous requirement, a risky choice, missing " +
			"detail). One question: set \"question\" (+ optional \"options\"). Several at once — one card, " +
			"answered together: pass \"questions\" instead. Options are suggestions; the user may also type " +
			"a free-text reply.",
		// Schema mirrors claude-cli's native AskUserQuestion where it overlaps so a
		// model trained on that tool calls this one without a shape mismatch: an
		// option may be a plain string OR an object with a "label" (the native form).
		// Extra native fields (header/multiSelect/description) are accepted and
		// ignored; a native questions[] wrapper is tolerated too (see ParseAskInput).
		//
		// The option item is deliberately UNCONSTRAINED ("items": {}) with the accepted
		// shapes stated in prose instead of a duplicated string/object oneOf: flexOptions
		// decodes strings, {label}/{value}/{text}/{description} objects, mixes and bare
		// scalars alike, so the long oneOf only documented what the parser already
		// tolerates — and it was duplicated in the nested questions[] branch, making it
		// the single fattest part of this EAGER schema.
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "question": { "type": "string", "description": "The question to ask the user." },
    "options": {
      "type": "array",
      "description": "Optional suggested answers shown as clickable choices. Each item is a plain string, or an object with a \"label\" (AskUserQuestion-compatible).",
      "items": {}
    },
    "questions": {
      "type": "array",
      "description": "Ask SEVERAL questions at once (one card, answered together) instead of \"question\".",
      "items": {
        "type": "object",
        "properties": {
          "question": { "type": "string", "description": "The question to ask the user." },
          "options": { "type": "array", "description": "Optional suggested answers for this question (same shapes as above).", "items": {} }
        },
        "required": ["question"]
      }
    }
  }
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
	questions, err := ParseAskInputMulti(input)
	if err != nil {
		return "", argErrFor("ask_user", err)
	}
	if len(questions) == 0 {
		return "", fmt.Errorf("question is required")
	}
	// Several questions at once: present them together in one card via the
	// multi-asker. Fall back to asking each in sequence (and combining the
	// replies) when only the single-question asker is wired.
	if len(questions) > 1 {
		if multi := multiAskerFrom(ctx); multi != nil {
			return multi(ctx, questions)
		}
		ask := askerFrom(ctx)
		answers := make([]string, len(questions))
		for i, q := range questions {
			a, err := ask(ctx, q.Question, q.Options)
			if err != nil {
				return "", err
			}
			answers[i] = a
		}
		enc, _ := json.Marshal(answers)
		return FormatMultiAnswer(questions, string(enc)), nil
	}
	answer, err := askerFrom(ctx)(ctx, questions[0].Question, questions[0].Options)
	if err != nil {
		return "", err
	}
	return answer, nil
}
