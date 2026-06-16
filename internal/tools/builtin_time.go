package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bilal/swarmgo/internal/providers"
)

// TimeTool reports the current server time. It is the simplest possible tool
// and serves as a smoke test for the native agentic loop.
type TimeTool struct{}

func (TimeTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "get_current_time",
		Description: "Returns the current date and time in RFC3339 format (server local time).",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (TimeTool) Call(_ context.Context, _ json.RawMessage) (string, error) {
	return time.Now().Format(time.RFC3339), nil
}
