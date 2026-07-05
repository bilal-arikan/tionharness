package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// HandoffResult is the outcome of a context-reset handoff: the fresh session that
// continues the work and the handoff artifact written into the current session.
type HandoffResult struct {
	NewSessionID string
	AgentName    string
	ArtifactID   string
}

// HandoffFunc performs a context reset on the CURRENT session: it writes a handoff
// artifact capturing the session's state and spawns a fresh session to continue
// the work in a clean window. Implemented in the agent package (which owns the
// runtime + the originating session id from the context) and injected here so the
// tools package need not import it. reason is a short free-text label.
type HandoffFunc func(ctx context.Context, reason string) (HandoffResult, error)

// handoffSessionInput is the argument shape for the handoff_session tool.
type handoffSessionInput struct {
	Reason string `json:"reason"`
}

// HandoffSessionTool lets an agent deliberately reset its own context: when the
// conversation is long and the agent feels it is approaching the context limit,
// it writes a handoff document and continues the work in a FRESH session with a
// clean window — the Anthropic "context reset" pattern, which avoids the early
// wrap-up ("context anxiety") that in-place compaction alone leaves behind. The
// continuation runs in a SEPARATE session (visible in the activity feed); the
// current conversation does not continue, so do not promise further work here.
type HandoffSessionTool struct {
	handoff HandoffFunc
}

// NewHandoffSessionTool constructs the tool bound to the runtime's handoff impl.
func NewHandoffSessionTool(fn HandoffFunc) *HandoffSessionTool {
	return &HandoffSessionTool{handoff: fn}
}

func (*HandoffSessionTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "handoff_session",
		Description: "Reset your context by handing off to a FRESH session. Use this for a long-running task when " +
			"this conversation is getting long and you are approaching the context limit: it writes a structured " +
			"handoff document (objective, progress done/pending, environment, next concrete step) as an artifact, " +
			"then spawns a NEW session that continues the work in a clean window — instead of letting older turns be " +
			"silently summarized. The new session runs on its own (visible in the activity feed); THIS conversation " +
			"does not continue, so finish your current thought first and do not promise more work here. Returns the " +
			"new session id.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "reason": { "type": "string", "description": "Optional short note on why you are resetting (e.g. \"context nearly full\")." }
  },
  "additionalProperties": false
}`),
	}
}

func (t *HandoffSessionTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in handoffSessionInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErrFor("handoff_session", err)
		}
	}
	if t.handoff == nil {
		return "", fmt.Errorf("context-reset handoff is not available in this context")
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "agent"
	}
	res, err := t.handoff(ctx, reason)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Context reset complete. The work continues in a fresh session %q (id %s); a handoff artifact (%s) was written to this session. This conversation will not continue — the new session picks it up.",
		res.AgentName, res.NewSessionID, res.ArtifactID), nil
}
