package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/logbuf"
	"github.com/bilal-arikan/tionharness/internal/providers"
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
		Description: "Read recent log entries from the application + workspace log stream (for self-diagnosis). Optionally filter by minimum level (debug|info|warn|error), a case-insensitive substring query (matches message, attrs, and component/session/agent/workspace), an exact component (e.g. api, agent, scheduler) and/or an exact session id. Returns the most recent matching entries.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"limit":{"type":"integer","description":"Max entries to return (default 100, max 500)"},
				"level":{"type":"string","enum":["debug","info","warn","error"],"description":"Minimum log level to include"},
				"q":{"type":"string","description":"Case-insensitive substring matched in the message, attributes, and the component/session/agent/workspace fields"},
				"component":{"type":"string","description":"Exact component filter (originating subsystem, e.g. api, agent, scheduler, mcp, db)"},
				"session":{"type":"string","description":"Exact session id filter"}
			},
			"additionalProperties":false
		}`),
	}
}

// logEntryMatches mirrors api.entryMatches (kept local to avoid an import
// cycle): free-text search over message, promoted source fields and attrs.
func logEntryMatches(e logbuf.Entry, q string) bool {
	if strings.Contains(strings.ToLower(e.Message), q) {
		return true
	}
	for _, v := range []string{e.Component, e.Session, e.Agent, e.Workspace} {
		if v != "" && strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	for k, v := range e.Attrs {
		if strings.Contains(strings.ToLower(k), q) || strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}

func (t ReadLogsTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Limit     int    `json:"limit"`
		Level     string `json:"level"`
		Q         string `json:"q"`
		Component string `json:"component"`
		Session   string `json:"session"`
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
	component := strings.TrimSpace(in.Component)
	session := strings.TrimSpace(in.Session)

	all := t.logs.Entries(0)
	filtered := make([]logbuf.Entry, 0, len(all))
	for _, e := range all {
		if logLevelRank(e.Level) < minLevel {
			continue
		}
		if component != "" && e.Component != component {
			continue
		}
		if session != "" && e.Session != session {
			continue
		}
		if q != "" && !logEntryMatches(e, q) {
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
