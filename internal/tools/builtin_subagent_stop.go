package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// StopAgentFunc cancels one async subagent run started by the calling session.
// Implemented in the agent package (which owns the runtime's cancel registry and
// the session store) and injected via context, mirroring RunAgentFunc, so this
// built-in does not import the agent package.
//
// It reports whether a turn was actually in flight: "already finished" is a
// normal outcome the caller should be told about plainly, not an error.
type StopAgentFunc func(ctx context.Context, sessionID string) (stopped bool, err error)

// stopAgentKey keys the StopAgentFunc on a request context.
type stopAgentKey struct{}

// WithStopAgent attaches a stop-subagent runner to ctx for this turn.
func WithStopAgent(ctx context.Context, fn StopAgentFunc) context.Context {
	return context.WithValue(ctx, stopAgentKey{}, fn)
}

// StopAgentFrom returns the runner attached to ctx, or nil when subagent control
// is not wired for this turn.
func StopAgentFrom(ctx context.Context) StopAgentFunc {
	fn, _ := ctx.Value(stopAgentKey{}).(StopAgentFunc)
	return fn
}

// stopSubagentInput is the argument shape for the stop_subagent tool.
type stopSubagentInput struct {
	SessionID string `json:"session_id"`
}

// StopSubagentTool cancels an async subagent the caller started and is no longer
// waiting on. Without it a detached run has no off switch short of the human
// opening the session: it keeps spending the daily budget on work whose result
// nobody will read, and — for a target that writes files — keeps changing the
// repository under the caller's feet.
type StopSubagentTool struct{}

// NewStopSubagentTool constructs the stop_subagent tool.
func NewStopSubagentTool() StopSubagentTool { return StopSubagentTool{} }

func (StopSubagentTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "stop_subagent",
		Description: "Cancel an async subagent you started with run_subagent, using the session id it " +
			"returned. Use it when the work is no longer needed (you found the answer another way, the " +
			"task changed, or the run is clearly going wrong) — a detached run keeps spending budget and, " +
			"if its target edits files, keeps changing the repository. The run stops at its next " +
			"cancellation point and its transcript is kept, marked as killed, so you can still read how " +
			"far it got. Only your OWN subagent runs can be stopped. Stopping one that already finished " +
			"is not an error — you are told it had already ended.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "session_id": { "type": "string", "description": "The session id run_subagent returned for the async run." }
  },
  "required": ["session_id"],
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"session_id":"SES1042"}`),
		},
	}
}

func (StopSubagentTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[stopSubagentInput]("stop_subagent", input)
	if err != nil {
		return "", err
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return "", fmt.Errorf("\"session_id\" is required")
	}
	stop := StopAgentFrom(ctx)
	if stop == nil {
		return "", fmt.Errorf("subagent control is not available in this context")
	}
	stopped, err := stop(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !stopped {
		return fmt.Sprintf("Subagent session %s had already finished; nothing to stop.", sessionID), nil
	}
	return fmt.Sprintf("Cancelled subagent session %s. Its transcript is kept and marked killed.", sessionID), nil
}
