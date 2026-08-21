package agent

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// A server whose catalog build fails loses ALL of its tools for the turn. The
// pool reports that per server, but buildRegistry could only log it — so the
// user saw a silent capability drop and the model saw a tool that simply did
// not exist. The failures are collected on the turn context here and emitted
// once by the native tool loop as a StepRecovery card, the same channel the
// loop already uses for its other non-happy-path branches (and the counterpart
// of the codex path's dropped-server text step).

// mcpFailureReason is the machine tag on the emitted StepRecovery.
const mcpFailureReason = "mcp_servers_unavailable"

// mcpErrTextLimit caps one server's error text in the note. The card explains
// WHY a tool is missing; the full error stays in the logger.Warn.
const mcpErrTextLimit = 160

// mcpFailedServer is one MCP server that could not be catalogued this turn.
type mcpFailedServer struct {
	Name string
	Err  string
}

// mcpFailureCollector gathers per-server catalog failures during registry build.
// The registry is built once per turn, but Catalog may be reached concurrently
// from preview/subagent paths sharing a context, so the mutex is not optional.
type mcpFailureCollector struct {
	mu    sync.Mutex
	items map[string]string
}

type mcpFailureKey struct{}

// withMCPFailures attaches a fresh collector to ctx and returns it. Callers
// that do not attach one (catalog previews, tests) make record a no-op.
func withMCPFailures(ctx context.Context) (context.Context, *mcpFailureCollector) {
	c := &mcpFailureCollector{}
	return context.WithValue(ctx, mcpFailureKey{}, c), c
}

// mcpFailuresFrom returns the collector on ctx, or nil when none is wired.
func mcpFailuresFrom(ctx context.Context) *mcpFailureCollector {
	c, _ := ctx.Value(mcpFailureKey{}).(*mcpFailureCollector)
	return c
}

// record notes that server name failed with the pool's error message. A nil
// collector (no turn listening) is a no-op so build paths need no branch; an
// empty message is still recorded — the server IS down, and the note renders
// the missing reason explicitly rather than dropping the server.
func (c *mcpFailureCollector) record(name, msg string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = map[string]string{}
	}
	c.items[name] = msg
}

// list returns the collected failures sorted by server name, so the note text
// is deterministic.
func (c *mcpFailureCollector) list() []mcpFailedServer {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.items))
	for n := range c.items {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]mcpFailedServer, 0, len(names))
	for _, n := range names {
		out = append(out, mcpFailedServer{Name: n, Err: c.items[n]})
	}
	return out
}

// formatMCPFailureNote renders the user- and model-facing explanation. Each
// server is named with its (shortened) error so the model can tell WHY a tool
// it expected is missing. Returns "" when nothing failed.
func formatMCPFailureNote(fails []mcpFailedServer) string {
	if len(fails) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fails))
	for _, f := range fails {
		parts = append(parts, f.Name+" ("+shortMCPErr(f.Err)+")")
	}
	return "[mcp] unavailable MCP server(s) omitted from this turn: " +
		strings.Join(parts, ", ") +
		" — their tools are missing from the tool catalog until the server is back up."
}

// shortMCPErr collapses an error to a single trimmed line bounded by
// mcpErrTextLimit. An empty error becomes an explicit placeholder rather than
// an empty parenthesis, so the note never hides that a reason was missing.
func shortMCPErr(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if s == "" {
		return "no error text"
	}
	if len(s) > mcpErrTextLimit {
		return s[:mcpErrTextLimit] + "…"
	}
	return s
}
