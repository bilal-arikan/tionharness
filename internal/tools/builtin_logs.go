package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/logbuf"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// ReadLogsTool lets an agent read the recent application + workspace log stream
// (the same ring buffer the Logs UI shows). Useful for self-diagnosis: checking
// why a scheduled run reported a failure, what it reported, etc.
type ReadLogsTool struct {
	logs *logbuf.Buffer
}

// NewReadLogsTool binds read_logs to the process-wide log buffer.
func NewReadLogsTool(logs *logbuf.Buffer) ReadLogsTool {
	return ReadLogsTool{logs: logs}
}

// logLevelRank maps a slog level name to an ordered rank for min-level
// filtering. Mirrors api.levelRank (kept local to avoid an import cycle).
func logLevelRank(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return 1
	case "INFO":
		return 2
	case "WARN", "WARNING":
		return 3
	case "ERROR":
		return 4
	default:
		return 0
	}
}

func (ReadLogsTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "read_logs",
		Description: "Read recent log entries from the application + workspace log stream (for self-diagnosis). Optionally filter by minimum level (debug|info|warn|error) and a case-insensitive substring query. Returns the most recent matching entries.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"limit":{"type":"integer","description":"Max entries to return (default 100, max 500)"},
				"level":{"type":"string","enum":["debug","info","warn","error"],"description":"Minimum log level to include"},
				"q":{"type":"string","description":"Case-insensitive substring to match in the message"}
			},
			"additionalProperties":false
		}`),
	}
}

func (t ReadLogsTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Limit int    `json:"limit"`
		Level string `json:"level"`
		Q     string `json:"q"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	if t.logs == nil {
		return "Logs are not available in this context.", nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	minLevel := logLevelRank(in.Level)
	q := strings.ToLower(strings.TrimSpace(in.Q))

	all := t.logs.Entries(0)
	filtered := make([]logbuf.Entry, 0, len(all))
	for _, e := range all {
		if logLevelRank(e.Level) < minLevel {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Message), q) {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	if len(filtered) == 0 {
		return "(no matching log entries)", nil
	}
	b, err := json.Marshal(filtered)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
