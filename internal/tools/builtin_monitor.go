package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// MonitorTool lets an agent end its turn instead of polling: it arms a watcher on
// a background shell and is woken when the output matches a regex. It pairs a
// session's MonitorManager (lifecycle + wake delivery) with its ShellManager (the
// only source in v1).
type MonitorTool struct {
	mgr   *MonitorManager
	shell *ShellManager
}

// NewMonitorTool binds the tool to a session's monitor and shell managers. Both
// may be nil (catalog/preview builds) — the tool then reports monitoring is
// unavailable rather than pretending to arm.
func NewMonitorTool(m *MonitorManager, sh *ShellManager) MonitorTool {
	return MonitorTool{mgr: m, shell: sh}
}

func (MonitorTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "monitor",
		Description: "Watch a background shell's output and WAKE you when it matches a pattern, so you can end " +
			"your turn instead of polling shell_manage in a loop. Set `action`: `start` (needs shell_id and " +
			"filter — a Go regex matched per output line; optional cooldown_seconds and max_fires), `list` " +
			"(every monitor with its state and counters), or `stop` (needs monitor_id; idempotent). A match " +
			"starts a NEW turn carrying the matching lines. Monitors live in memory only: they are lost on " +
			"restart, and a wake is at-most-once (an event is lost if the process dies mid-delivery), so do " +
			"not rely on one for anything that must not be missed.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"action":{"type":"string","enum":["start","list","stop"],"description":"What to do"},
				"shell_id":{"type":"string","description":"Background shell to watch (required for start; e.g. bg1)"},
				"filter":{"type":"string","description":"Go regex matched against each new output line (required for start)"},
				"cooldown_seconds":{"type":"integer","description":"Minimum seconds between wakes from this monitor (default and minimum 5)"},
				"max_fires":{"type":"integer","description":"Stop the monitor after this many wakes (0 or omitted = unlimited)"},
				"monitor_id":{"type":"string","description":"The monitor id (required for stop; e.g. mon1)"}
			},
			"required":["action"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"action":"start","shell_id":"bg1","filter":"(?i)error|panic"}`),
			json.RawMessage(`{"action":"start","shell_id":"bg1","filter":"Listening on","max_fires":1}`),
			json.RawMessage(`{"action":"list"}`),
			json.RawMessage(`{"action":"stop","monitor_id":"mon1"}`),
		},
	}
}

func (t MonitorTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Action    string `json:"action"`
		ShellID   string `json:"shell_id"`
		Filter    string `json:"filter"`
		Cooldown  int    `json:"cooldown_seconds"`
		MaxFires  int    `json:"max_fires"`
		MonitorID string `json:"monitor_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErrFor("monitor", err)
	}
	switch strings.ToLower(strings.TrimSpace(args.Action)) {
	case "list":
		return t.mgr.List(), nil
	case "stop":
		id := strings.TrimSpace(args.MonitorID)
		if id == "" {
			return "", fmt.Errorf("monitor_id is required for action \"stop\"")
		}
		return t.mgr.Stop(id)
	case "start":
		shellID := strings.TrimSpace(args.ShellID)
		if shellID == "" {
			return "", fmt.Errorf("shell_id is required for action \"start\"")
		}
		if strings.TrimSpace(args.Filter) == "" {
			return "", fmt.Errorf("filter is required for action \"start\" (a regex matched against each output line)")
		}
		if t.mgr == nil {
			return "", fmt.Errorf("monitoring is not available in this context")
		}
		src, err := NewShellSource(t.shell, shellID)
		if err != nil {
			return "", err
		}
		id, err := t.mgr.Start(src, args.Filter, time.Duration(args.Cooldown)*time.Second, args.MaxFires)
		if err != nil {
			src.Close()
			return "", err
		}
		return fmt.Sprintf("Armed %s on shell %s (filter %q). You can end your turn now: a match will wake you.", id, shellID, args.Filter), nil
	default:
		return "", fmt.Errorf("unknown action %q (use start/list/stop)", args.Action)
	}
}
