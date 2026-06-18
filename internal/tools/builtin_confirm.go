package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/providers"
)

// confirmInput is the ask shape for the request_confirmation tool.
type confirmInput struct {
	Question string `json:"question"`
}

// ConfirmOptions are the suggested clickable choices shown for a confirmation.
var ConfirmOptions = []string{"Onayla", "İptal"}

// RequestConfirmationTool asks the user to approve a risky or irreversible action
// before the agent proceeds, blocking the turn until they decide. Like ask_user
// it only works in interactive chat (an asker wired into the context); autonomous
// runs return an error so the model proceeds on its own. The result is normalised
// to "confirmed" or "denied" so the model can branch reliably.
type RequestConfirmationTool struct{}

// NewRequestConfirmationTool constructs the request_confirmation tool.
func NewRequestConfirmationTool() RequestConfirmationTool { return RequestConfirmationTool{} }

func (RequestConfirmationTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "request_confirmation",
		Description: "Ask the user to confirm a risky, destructive or irreversible action " +
			"(deleting data, spending money, sending something external) BEFORE doing it. " +
			"Blocks until the user decides. Returns 'confirmed' or 'denied' — only proceed " +
			"with the action when the result is 'confirmed'.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "question": { "type": "string", "description": "What the user is being asked to confirm, phrased as a yes/no question." }
  },
  "required": ["question"],
  "additionalProperties": false
}`),
	}
}

func (RequestConfirmationTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	// Bail early in autonomous/flow runs before touching the payload.
	ask := askerFrom(ctx)
	if ask == nil {
		return "", fmt.Errorf("request_confirmation is only available in interactive chat sessions; do not take the risky action")
	}
	var in confirmInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid request_confirmation input: %w", err)
	}
	if strings.TrimSpace(in.Question) == "" {
		return "", fmt.Errorf("question is required")
	}
	answer, err := ask(ctx, in.Question, ConfirmOptions)
	if err != nil {
		return "", err
	}
	return NormalizeConfirmation(answer), nil
}

// NormalizeConfirmation maps a free-text or option answer to a stable verdict the
// model can branch on: "confirmed", "denied", or the raw answer when ambiguous.
func NormalizeConfirmation(answer string) string {
	a := strings.ToLower(strings.TrimSpace(answer))
	switch {
	case a == "":
		return "denied"
	case containsAny(a, "onayla", "onay", "evet", "yes", "confirm", "approve", "tamam", "ok", "kabul", "devam"):
		return "confirmed"
	case containsAny(a, "iptal", "hayır", "hayir", "no", "cancel", "deny", "reddet", "vazgeç", "vazgec", "durdur"):
		return "denied"
	default:
		// Ambiguous free text: surface it verbatim so the model can interpret.
		return "ambiguous: " + answer
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
