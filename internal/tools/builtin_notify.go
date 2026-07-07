package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// notifyInput is the argument shape for the notify tool.
type notifyInput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Level string `json:"level"`
}

// NotifyTool lets an agent raise a non-blocking desktop notification so it can
// reach the user when they are not looking at the app (a long job finished, an
// autonomous run needs attention). Unlike ask_user/request_confirmation it never
// blocks: it fires the notification and returns at once. When no notify sink is
// wired (autonomous run with no open client), it is a graceful no-op so the turn
// never breaks. Both provider paths share this one definition (single-schema rule).
type NotifyTool struct{}

// NewNotifyTool constructs the notify tool.
func NewNotifyTool() NotifyTool { return NotifyTool{} }

func (NotifyTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "notify",
		Description: "Raise a desktop notification to get the user's attention when they may not be " +
			"looking at the app (a long task finished, an autonomous run needs input, an error occurred). " +
			"Title and body reach the user EXACTLY as written (never summarized), so use this when content " +
			"must arrive verbatim mid-task — a progress update with specific numbers, a partial result. " +
			"Does NOT block — it fires the toast and returns immediately. Use ask_user/request_confirmation " +
			"when you actually need an answer; use notify only to inform.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "title": { "type": "string", "description": "Short notification headline." },
    "body": { "type": "string", "description": "Optional longer detail line." },
    "level": { "type": "string", "enum": ["info", "success", "error"], "description": "Severity, styling the toast. Defaults to info." }
  },
  "required": ["title"],
  "additionalProperties": false
}`),
	}
}

// validLevels gates the level field so a typo can't produce an unstyled toast.
var validLevels = map[string]bool{"info": true, "success": true, "error": true}

func (NotifyTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[notifyInput]("notify", input)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(in.Title) == "" {
		return "", fmt.Errorf("title is required")
	}
	level := strings.ToLower(strings.TrimSpace(in.Level))
	if level == "" {
		level = "info"
	}
	if !validLevels[level] {
		level = "info"
	}
	sink := notifyFrom(ctx)
	if sink == nil {
		// No client is listening (autonomous run / no open window): don't fail the
		// turn — report that the notification couldn't surface so the model proceeds.
		return "no notification channel is available for this turn (no open UI client); message not delivered", nil
	}
	if err := sink.Notify(ctx, NotifySpec{Title: in.Title, Body: in.Body, Level: level}); err != nil {
		return "", err
	}
	return "notification sent", nil
}
