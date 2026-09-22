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
// a source and is woken when an observation matches a regex. It pairs a session's
// MonitorManager (lifecycle + wake delivery) with the source backends — a
// ShellManager for background shells and a Sandbox for watched files.
type MonitorTool struct {
	mgr   *MonitorManager
	shell *ShellManager
	sb    Sandbox
}

// NewMonitorTool binds the tool to a session's monitor and shell managers, and
// the sandbox a watched file path resolves against. The managers may be nil
// (catalog/preview builds) — the tool then reports monitoring is unavailable
// rather than pretending to arm.
func NewMonitorTool(m *MonitorManager, sh *ShellManager, sb Sandbox) MonitorTool {
	return MonitorTool{mgr: m, shell: sh, sb: sb}
}

func (MonitorTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "monitor",
		Description: "Watch something and WAKE you when it matches a pattern, so you can end your turn instead " +
			"of polling in a loop. Set `action`: `start`, `list` (every monitor with its state and counters), " +
			"or `stop` (needs monitor_id; idempotent). `start` needs `filter` (a Go regex) plus ONE source: " +
			"`shell_id` (a background shell, matched per output line), `path` (a file, matched per appended " +
			"line), or `url` (http/https re-fetched periodically and matched when the body CHANGES, or ws/wss " +
			"matched per incoming message). Optional cooldown_seconds, max_fires, and interval_seconds (http " +
			"polling rate, minimum 30). A match starts a NEW turn carrying the matching events. A source that " +
			"ends — process exited, file removed, URL unreachable, socket closed — is reported ONCE and the " +
			"monitor stops. Monitors live in memory only: they are lost on restart, and a wake is at-most-once " +
			"(an event is lost if the process dies mid-delivery), so do not rely on one for anything that must " +
			"not be missed.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"action":{"type":"string","enum":["start","list","stop"],"description":"What to do"},
				"shell_id":{"type":"string","description":"Background shell to watch (e.g. bg1). One of shell_id/path/url is required for start."},
				"path":{"type":"string","description":"File to tail; each appended line is matched against filter. One of shell_id/path/url is required for start."},
				"url":{"type":"string","description":"http(s) URL re-fetched periodically (fires when the body changes) or ws(s) URL subscribed to (fires per message). One of shell_id/path/url is required for start."},
				"filter":{"type":"string","description":"Go regex matched against each observed event (required for start)"},
				"interval_seconds":{"type":"integer","description":"How often to re-fetch an http(s) url (default and minimum 30; ignored for other sources)"},
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
			json.RawMessage(`{"action":"start","path":"logs/app.log","filter":"(?i)fatal"}`),
			json.RawMessage(`{"action":"start","url":"https://status.example.com/health","filter":".","interval_seconds":60}`),
			json.RawMessage(`{"action":"start","url":"wss://example.com/events","filter":"\"type\":\"deploy\""}`),
			json.RawMessage(`{"action":"list"}`),
			json.RawMessage(`{"action":"stop","monitor_id":"mon1"}`),
		},
	}
}

func (t MonitorTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Action    string `json:"action"`
		ShellID   string `json:"shell_id"`
		Path      string `json:"path"`
		URL       string `json:"url"`
		Filter    string `json:"filter"`
		Interval  int    `json:"interval_seconds"`
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
		if strings.TrimSpace(args.Filter) == "" {
			return "", fmt.Errorf("filter is required for action \"start\" (a regex matched against each observed event)")
		}
		if t.mgr == nil {
			return "", fmt.Errorf("monitoring is not available in this context")
		}
		src, err := t.buildSource(ctx, monitorSourceArgs{
			ShellID:  args.ShellID,
			Path:     args.Path,
			URL:      args.URL,
			Interval: args.Interval,
		})
		if err != nil {
			return "", err
		}
		id, err := t.mgr.Start(src, args.Filter, time.Duration(args.Cooldown)*time.Second, args.MaxFires)
		if err != nil {
			src.Close()
			return "", err
		}
		return fmt.Sprintf("Armed %s on %s (filter %q). You can end your turn now: a match will wake you.", id, src.Describe(), args.Filter), nil
	default:
		return "", fmt.Errorf("unknown action %q (use start/list/stop)", args.Action)
	}
}
