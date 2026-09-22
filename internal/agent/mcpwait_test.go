package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// fakeMCPBackend serves the minimum MCP over HTTP the pool needs: an initialize
// handshake and a tools/list advertising the given tool names. Counts dials so a
// test can prove a warm-up actually connected.
func fakeMCPBackend(t *testing.T, toolNames ...string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var initializes atomic.Int32
	list := make([]map[string]any, 0, len(toolNames))
	for _, n := range toolNames {
		list = append(list, map[string]any{
			"name": n, "description": n + " description",
			"inputSchema": map[string]any{"type": "object"},
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			initializes.Add(1)
			result = map[string]any{"protocolVersion": "2025-03-26"}
		case "tools/list":
			result = map[string]any{"tools": list}
		default:
			result = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}))
	t.Cleanup(srv.Close)
	return srv, &initializes
}

// enableMCPServer stores an enabled HTTP MCP server pointing at url.
func enableMCPServer(t *testing.T, rt *Runtime, name, url string) db.MCPServer {
	t.Helper()
	m, err := rt.db.CreateMCPServer(context.Background(), db.MCPServer{
		Name: name, Transport: db.MCPTransportHTTP, URL: url, Args: "[]",
		EnvConfig: "{}", HeadersConfig: "{}", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create MCP server %q: %v", name, err)
	}
	return m
}

// THE acceptance test for mid-turn catalog growth: after wait_for_mcp_servers
// returns, the warmed server's tools must be in what the tool loop SHIPS on its
// next iteration — not merely present in some registry field. This drives the
// real shipFor/ActiveDefs path, so a regression in laziness, activation or the
// frozen-epoch merge fails here.
func TestWaitForMCPServersMakesToolsShippableSameTurn(t *testing.T) {
	backend, _ := fakeMCPBackend(t, "echo")
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "fake", backend.URL)

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	active := tools.NewActiveTools()
	ctx := withActiveTools(context.Background(), active)

	// The catalog build that opens the turn. It DOES dial (the server is up), so to
	// prove the mid-turn path we clear the registry's view by building against the
	// pool and then asserting on what the wait tool adds beyond it.
	reg := rt.buildRegistry(ctx, agent)
	toolFilter := rt.toolFilter(ctx, agent)
	reg.ConfigureAutoActivation(active, toolFilter)

	shipped := func() map[string]bool {
		out := map[string]bool{}
		for _, d := range reg.ActiveDefs(toolFilter, active.Snapshot()) {
			out[d.Name] = true
		}
		return out
	}

	// Before the call the MCP tool is lazy/name-only, so it is NOT shipped.
	if shipped()["fake__echo"] {
		t.Fatalf("fake__echo is shipped before activation; the tier default is not applied")
	}
	if !reg.Has("wait_for_mcp_servers") {
		t.Fatalf("wait_for_mcp_servers is not registered despite an enabled MCP server + pool")
	}
	// The tool is name-only, so a cold call would spend a round-trip on
	// auto-activation. Activate it first, as the model does after reading the name
	// in the load-on-demand catalog.
	active.Activate("wait_for_mcp_servers")

	res := reg.Call(ctx, providers.ToolCall{
		ID: "c1", Name: "wait_for_mcp_servers",
		Input: json.RawMessage(`{"servers":["fake"],"timeoutSeconds":20}`),
	})
	if res.IsError {
		t.Fatalf("wait_for_mcp_servers failed: %s", res.Content)
	}
	if !strings.Contains(res.Content, "fake: ready") {
		t.Fatalf("result does not report the server ready:\n%s", res.Content)
	}

	// The next loop iteration recomputes ActiveDefs from this same registry+active
	// set — which is exactly what shipped() does here.
	if !shipped()["fake__echo"] {
		t.Fatalf("fake__echo is NOT in ActiveDefs after the wait; it is not callable this turn.\nresult was:\n%s", res.Content)
	}
}

// The same guarantee through the loop's ACTUAL shipping function, including a
// FROZEN prompt epoch. mergeFrozenToolDefs ships a frozen snapshot plus only the
// names in the active set, so a tool merged mid-turn but left inactive would be
// silently dropped from the request — present in the registry, invisible to the
// model. This pins the activation that prevents it.
func TestWaitForMCPServersToolsSurviveFrozenEpochMerge(t *testing.T) {
	backend, _ := fakeMCPBackend(t, "echo")
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "fake", backend.URL)

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	active := tools.NewActiveTools()
	ctx := withActiveTools(context.Background(), active)
	reg := rt.buildRegistry(ctx, agent)
	toolFilter := rt.toolFilter(ctx, agent)
	reg.ConfigureAutoActivation(active, toolFilter)
	active.Activate("wait_for_mcp_servers")

	// Freeze the tools block BEFORE the wait, exactly as a turn whose epoch was
	// already established does. fake__echo is lazy, so it is not in the snapshot.
	frozen := reg.ActiveDefs(toolFilter, active.Snapshot())
	shipFor := func() []providers.ToolDef {
		return mergeFrozenToolDefs(frozen, reg.ActiveDefs(toolFilter, active.Snapshot()), active.Snapshot())
	}
	for _, d := range shipFor() {
		if d.Name == "fake__echo" {
			t.Fatalf("fake__echo shipped before the wait; the frozen snapshot is wrong")
		}
	}

	res := reg.Call(ctx, providers.ToolCall{
		ID: "c1", Name: "wait_for_mcp_servers", Input: json.RawMessage(`{"servers":["fake"]}`),
	})
	if res.IsError {
		t.Fatalf("wait failed: %s", res.Content)
	}

	var shipped bool
	for _, d := range shipFor() {
		if d.Name == "fake__echo" {
			shipped = true
		}
	}
	if !shipped {
		t.Fatalf("fake__echo is not shipped by the frozen-epoch merge; the model cannot call it this turn")
	}
}

// An unknown server name is a hard error, not a "not ready" verdict: the model
// mistyped or is asking about a server this workspace does not have, and a soft
// answer would send it debugging a connection that was never configured.
func TestWaitForMCPServersRejectsUnknownServer(t *testing.T) {
	backend, _ := fakeMCPBackend(t, "echo")
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "fake", backend.URL)

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "wait_for_mcp_servers", map[string]any{"servers": []string{"nope"}})
	if !res.IsError {
		t.Fatalf("expected an error for an unknown server, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "nope") || !strings.Contains(res.Content, "fake") {
		t.Fatalf("error should name the unknown server AND list the enabled ones: %s", res.Content)
	}
}

// A dead server is reported, not raised: the caller asked whether the servers are
// up and both answers are information. The breaker is notified so the automatic
// catalog path stays protected.
func TestWaitForMCPServersReportsDeadServerAndNotesBreaker(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	// A URL with nothing listening: the dial fails fast rather than hanging.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := dead.URL
	dead.Close()
	enableMCPServer(t, rt, "down", url)

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "wait_for_mcp_servers", map[string]any{
		"servers": []string{"down"}, "timeoutSeconds": 5,
	})
	if res.IsError {
		t.Fatalf("a dead server must be REPORTED, not raised as a tool error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "NOT ready") {
		t.Fatalf("result should mark the server not ready:\n%s", res.Content)
	}
	if isOpen, _, streak := rt.mcpFailStreaks.Open("down"); !isOpen || streak == 0 {
		t.Fatalf("failed wait must Note() the breaker: open=%v streak=%d", isOpen, streak)
	}
}

// An OPEN breaker is bypassed exactly once by this tool — that is the whole point
// of an explicit wait — and a server that comes up CLEARS its streak, so the next
// ordinary catalog build dials it instead of waiting out the cooldown.
func TestWaitForMCPServersBypassesOpenBreakerAndClearsOnSuccess(t *testing.T) {
	backend, initializes := fakeMCPBackend(t, "echo")
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "fake", backend.URL)

	// Drive the breaker open, as a run of failed catalog builds would.
	for range 5 {
		rt.mcpFailStreaks.Note("fake")
	}
	if isOpen, _, _ := rt.mcpFailStreaks.Open("fake"); !isOpen {
		t.Fatalf("precondition: breaker should be open")
	}
	before := initializes.Load()

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "wait_for_mcp_servers", map[string]any{"servers": []string{"fake"}})
	if res.IsError {
		t.Fatalf("wait failed: %s", res.Content)
	}
	if !strings.Contains(res.Content, "fake: ready") {
		t.Fatalf("an open breaker must be bypassed by an explicit wait:\n%s", res.Content)
	}
	if got := initializes.Load(); got <= before {
		t.Fatalf("initializes = %d (was %d): the server was never dialed, so the breaker was not bypassed", got, before)
	}
	if isOpen, _, streak := rt.mcpFailStreaks.Open("fake"); isOpen || streak != 0 {
		t.Fatalf("a server that came up must Clear() its streak: open=%v streak=%d", isOpen, streak)
	}
}

// Scoped servers must warm the CALLER'S OWN (session, agent) pool slot — the same
// key buildRegistry uses — or the warmed connection is not the one later calls
// dispatch through.
func TestWaitForMCPServersWarmsCallerScopedSlot(t *testing.T) {
	backend, _ := fakeMCPBackend(t, "echo")
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	m := enableMCPServer(t, rt, "scoped", backend.URL)
	m.Scope = "scoped"
	if _, err := rt.db.UpdateMCPServer(context.Background(), m.ID, m); err != nil {
		t.Fatalf("mark server scoped: %v", err)
	}

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	ctx := withActiveTools(WithSessionID(context.Background(), "sess-7"), tools.NewActiveTools())
	reg := rt.buildRegistry(ctx, agent)
	res := reg.Call(ctx, providers.ToolCall{
		ID: "c1", Name: "wait_for_mcp_servers", Input: json.RawMessage(`{"servers":["scoped"]}`),
	})
	if res.IsError {
		t.Fatalf("wait failed: %s", res.Content)
	}

	want := "sess-7|a1"
	var found bool
	for _, s := range rt.mcpPool.Stats() {
		if s.Server == "scoped" && s.Scoped && s.ScopeKey == want && s.Alive {
			found = true
		}
	}
	if !found {
		t.Fatalf("no live scoped slot for %q; stats = %+v", want, rt.mcpPool.Stats())
	}
}

// The CLI backends own their own MCP clients, so TionHarness's pool has no honest
// answer about them. The tool must be withheld from the CLI bridge rather than
// reporting a state it cannot observe.
func TestWaitForMCPServersWithheldFromCLIBridge(t *testing.T) {
	for _, provider := range []string{"", providerKindCodexCLI} {
		if !cliBridgeExcludes("wait_for_mcp_servers", provider) {
			t.Errorf("provider %q: wait_for_mcp_servers must be excluded from the CLI bridge", provider)
		}
	}
}
