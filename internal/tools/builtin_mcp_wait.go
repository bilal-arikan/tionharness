package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// MCP wait bounds. The default is generous enough for a cold stdio server that
// indexes on startup, and the maximum keeps a mistyped timeout from pinning a
// turn for minutes. Both are wall-clock over the WHOLE set — servers are dialed
// in parallel, so the cost is the slowest one, not the sum.
const (
	defaultMCPWaitSeconds = 30
	maxMCPWaitSeconds     = 120
)

// mcpWaitInput is the argument shape for the wait_for_mcp_servers tool.
type mcpWaitInput struct {
	Servers        []string `json:"servers"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}

// MCPServerWaitResult is one server's verdict, as reported back to the model.
type MCPServerWaitResult struct {
	Server string // server name as configured
	Ready  bool   // connected and its tools are now callable this turn
	Tools  int    // tools merged into the live registry (0 when not ready)
	Err    string // why it is not ready; empty when Ready
}

// MCPWaiter warms the named MCP servers and merges their tools into the LIVE
// session registry, returning one result per server. An empty servers slice means
// "every enabled server". The implementation lives in the agent layer (it owns
// the pool, the circuit breaker and the caller's scope key); this package only
// defines the contract so the tool stays free of that dependency.
//
// It returns an error only when the request itself cannot be served (unknown
// server name, no MCP configured). A server that fails to connect is NOT an
// error — it comes back as a result with Ready=false and Err set, because the
// question the model asked ("are they up?") is answered either way.
type MCPWaiter func(ctx context.Context, servers []string) ([]MCPServerWaitResult, error)

// MCPServerWaitTool blocks until the named MCP servers are connected (or have
// definitively failed), then re-registers their tools so they are callable in the
// SAME turn — the tool loop recomputes the active tool set every iteration, so a
// server warmed here is usable on the model's very next call.
//
// It exists because MCP connections are lazy: a server that was not dialed when
// the turn started contributes no tools, and an agent that has just enabled or
// restarted one would otherwise have to end its turn and hope the next build
// picks it up. This makes that wait explicit and bounded.
//
// Registered only on the native tool loop. The CLI backends (claude-cli,
// codex-cli) own their MCP clients entirely — TionHarness's pool never dials for
// them, so it has no honest answer to give; the tool is withheld there rather
// than reporting a state it cannot observe.
type MCPServerWaitTool struct {
	wait MCPWaiter
}

// NewMCPServerWaitTool binds the tool to the runtime's waiter.
func NewMCPServerWaitTool(wait MCPWaiter) MCPServerWaitTool {
	return MCPServerWaitTool{wait: wait}
}

func (MCPServerWaitTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "wait_for_mcp_servers",
		Description: "Block until the given MCP servers are connected (or have failed), then make their " +
			"tools callable IN THIS TURN. Use after enabling, adding or restarting an MCP server, or when a " +
			"namespaced tool you expect is missing because its server was never dialed. Omit `servers` to wait " +
			"for every enabled server. Returns a per-server verdict: ready (with the number of tools now " +
			"available) or the reason it is not. Bounded by timeoutSeconds over the whole set — servers are " +
			"dialed in parallel, so waiting for five is not five times the cost.",
		InputSchema: json.RawMessage(fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "servers": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Server names to wait for. Omit or leave empty to wait for every enabled server."
    },
    "timeoutSeconds": {
      "type": "integer",
      "minimum": 1,
      "maximum": %d,
      "description": "Overall deadline for the whole set (default %d)."
    }
  },
  "additionalProperties": false
}`, maxMCPWaitSeconds, defaultMCPWaitSeconds)),
	}
}

func (t MCPServerWaitTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[mcpWaitInput]("wait_for_mcp_servers", input)
	if err != nil {
		return "", err
	}
	if t.wait == nil {
		return "", fmt.Errorf("wait_for_mcp_servers: no MCP waiter is wired for this turn")
	}
	timeout := in.TimeoutSeconds
	if timeout == 0 {
		timeout = defaultMCPWaitSeconds
	}
	if timeout < 0 || timeout > maxMCPWaitSeconds {
		return "", fmt.Errorf("timeoutSeconds must be between 1 and %d", maxMCPWaitSeconds)
	}
	var names []string
	for _, s := range in.Servers {
		if s = strings.TrimSpace(s); s != "" {
			names = append(names, s)
		}
	}

	wctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	results, err := t.wait(wctx, names)
	if err != nil {
		return "", err
	}
	return formatMCPWaitResults(results, timeout), nil
}

// formatMCPWaitResults renders the per-server verdicts as the compact text the
// model reads. Ready servers come first (that is the actionable part), each with
// the tool count now live; failures follow with their reason.
func formatMCPWaitResults(results []MCPServerWaitResult, timeout int) string {
	if len(results) == 0 {
		return "no MCP servers matched the request; nothing to wait for"
	}
	sorted := make([]MCPServerWaitResult, len(results))
	copy(sorted, results)
	slices.SortStableFunc(sorted, func(a, b MCPServerWaitResult) int {
		less := func(a, b MCPServerWaitResult) bool {
			if a.Ready != b.Ready {
				return a.Ready
			}
			return a.Server < b.Server
		}
		switch {
		case less(a, b):
			return -1
		case less(b, a):
			return 1
		}
		return 0
	})

	var b strings.Builder
	ready := 0
	for _, r := range sorted {
		if r.Ready {
			ready++
		}
	}
	fmt.Fprintf(&b, "%d/%d MCP server(s) ready (waited up to %ds)\n", ready, len(sorted), timeout)
	for _, r := range sorted {
		if r.Ready {
			fmt.Fprintf(&b, "- %s: ready, %d tool(s) now callable this turn\n", r.Server, r.Tools)
			continue
		}
		reason := r.Err
		if reason == "" {
			reason = "not connected"
		}
		fmt.Fprintf(&b, "- %s: NOT ready — %s\n", r.Server, reason)
	}
	if ready > 0 {
		b.WriteString("Ready servers' tools are registered for this turn; call them by their namespaced names.\n")
	}
	return b.String()
}
