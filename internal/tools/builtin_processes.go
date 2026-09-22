package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/procwatch"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// listProcessesDefaultLimit / listProcessesMaxLimit bound the tool's reply. The
// ledger holds a few hundred entries and each carries a command line, so an
// unbounded read would hand the model kilobytes of mostly-finished processes.
const (
	listProcessesDefaultLimit = 50
	listProcessesMaxLimit     = 200
)

// ListProcessesTool is the read-only agent view of the process ledger
// (internal/procwatch): the native processes TionHarness spawned for the
// agents — shell calls, code-mode interpreters, CLI transports, MCP servers,
// hooks, external tool runs.
//
// Read-only on purpose. An agent that could kill processes could kill the CLI
// transport of the turn it is running in; stopping one stays a user action in
// the workspace process panel, and an agent's own background shells already
// have shell_manage.
type ListProcessesTool struct{}

// NewListProcessesTool constructs the tool. It reads the process-wide ledger, so
// it needs no wiring.
func NewListProcessesTool() ListProcessesTool { return ListProcessesTool{} }

func (ListProcessesTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_processes",
		Description: "List the native OS processes TionHarness started on the agents' behalf: shell tool calls " +
			"(foreground and background), run_code/transform_data interpreters, agentic CLI transports " +
			"(claude-cli/codex-cli), stdio MCP servers, hooks and external tool runs. Each entry carries its " +
			"command line, pid, owner (session + agent), status (running|succeeded|failed|killed|timed_out), " +
			"exit code, start/end time and a short output tail. Use it to see what is still running — a build " +
			"that never exited, an MCP server that respawned — before starting another one. Read-only: stop a " +
			"process from the workspace process panel, or (for your own background shells) with shell_manage.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"status":{"type":"string","enum":["running","succeeded","failed","killed","timed_out"],"description":"Only processes in this state"},
				"kind":{"type":"string","enum":["shell","shell_background","code","provider","mcp","hook","external"],"description":"Only processes of this kind"},
				"session":{"type":"string","description":"Exact session id filter"},
				"limit":{"type":"integer","description":"Max entries to return (default 50, max 200)"}
			},
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"status":"running"}`),
			json.RawMessage(`{"kind":"shell_background","limit":10}`),
		},
	}
}

func (ListProcessesTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Status  string `json:"status"`
		Kind    string `json:"kind"`
		Session string `json:"session"`
		Limit   int    `json:"limit"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", argErr(err)
		}
	}
	limit := in.Limit
	if limit <= 0 {
		limit = listProcessesDefaultLimit
	}
	if limit > listProcessesMaxLimit {
		limit = listProcessesMaxLimit
	}
	f := procwatch.Filter{SessionID: strings.TrimSpace(in.Session), Limit: limit}
	if s := strings.TrimSpace(in.Status); s != "" {
		f.Statuses = []procwatch.Status{procwatch.Status(s)}
	}
	if k := strings.TrimSpace(in.Kind); k != "" {
		f.Kinds = []procwatch.Kind{procwatch.Kind(k)}
	}
	entries := procwatch.Default().List(f)
	if len(entries) == 0 {
		return "(no matching processes)", nil
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
