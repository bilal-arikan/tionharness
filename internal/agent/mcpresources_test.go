package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// resourceBackend is an HTTP MCP server with a resource surface, for the
// agent-layer wiring tests. advertise=false reproduces a server with no
// resource capability at all.
type resourceBackend struct {
	advertise bool
	resources []map[string]any
	templates []map[string]any
	contents  []map[string]any
}

func (b *resourceBackend) start(t *testing.T) string {
	t.Helper()
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
			caps := map[string]any{"tools": map[string]any{}}
			if b.advertise {
				caps["resources"] = map[string]any{}
			}
			result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": caps}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{}}
		case "resources/list":
			result = map[string]any{"resources": b.resources}
		case "resources/templates/list":
			result = map[string]any{"resourceTemplates": b.templates}
		case "resources/read":
			result = map[string]any{"contents": b.contents}
		default:
			result = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// callResourceTool dispatches through the production registry path with a
// SESSION bound, which the binary-write path needs (the scratchpad is resolved
// from the turn's session id).
func callResourceTool(t *testing.T, rt *Runtime, agent db.Agent, sessionID, name string, args map[string]any) providers.ToolResult {
	t.Helper()
	ctx := withActiveTools(WithSessionID(context.Background(), sessionID), tools.NewActiveTools())
	input, _ := json.Marshal(args)
	reg := rt.buildRegistry(ctx, agent)
	if !reg.Has(name) {
		t.Fatalf("tool %q not registered", name)
	}
	return reg.Call(ctx, providers.ToolCall{ID: "call_1", Name: name, Input: input})
}

// The pair is registered exactly where the wait tool is: a pool AND at least one
// configured server. Without a server there is nothing to list.
func TestMCPResourceToolsRegisteredWithAnEnabledServer(t *testing.T) {
	b := &resourceBackend{advertise: true}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}

	reg := rt.buildRegistry(context.Background(), agent)
	if reg.Has("list_mcp_resources") || reg.Has("read_mcp_resource") {
		t.Fatal("resource tools registered with no MCP server configured")
	}

	enableMCPServer(t, rt, "docs", b.start(t))
	reg = rt.buildRegistry(context.Background(), agent)
	if !reg.Has("list_mcp_resources") || !reg.Has("read_mcp_resource") {
		t.Fatal("resource tools are not registered despite an enabled MCP server + pool")
	}
	// Both ship at the summary tier: named AND one-line-described in the
	// load-on-demand catalog, because "resource" is not self-evident from the name.
	for _, name := range []string{"list_mcp_resources", "read_mcp_resource"} {
		if got := reg.VisibilityOf(name); got != tools.VisibilitySummary {
			t.Errorf("%s tier = %q, want %q", name, got, tools.VisibilitySummary)
		}
	}
}

// The listing carries uri, name, mimeType and description, and marks templates
// as templates so the model does not try to read "db://{table}" verbatim.
func TestListMCPResourcesReportsMetadataAndTemplates(t *testing.T) {
	b := &resourceBackend{
		advertise: true,
		resources: []map[string]any{{
			"uri": "file:///readme.md", "name": "readme",
			"mimeType": "text/markdown", "description": "project readme",
		}},
		templates: []map[string]any{{"uriTemplate": "db://{table}", "name": "table rows"}},
	}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "docs", b.start(t))

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "list_mcp_resources", map[string]any{})
	if res.IsError {
		t.Fatalf("list_mcp_resources failed: %s", res.Content)
	}
	for _, want := range []string{"file:///readme.md", "readme", "text/markdown", "project readme", "db://{table}", "TEMPLATE"} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("listing does not mention %q:\n%s", want, res.Content)
		}
	}
}

// A server with NO resource capability must be reported explicitly — not
// omitted, and not shown as an error. Silence would leave the model unable to
// tell "this server has nothing" from "I never asked".
func TestListMCPResourcesReportsUnsupportedServer(t *testing.T) {
	b := &resourceBackend{advertise: false}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "toolsonly", b.start(t))

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "list_mcp_resources", map[string]any{})
	if res.IsError {
		t.Fatalf("an unsupported server must not fail the call: %s", res.Content)
	}
	if !strings.Contains(res.Content, "toolsonly") || !strings.Contains(res.Content, "no resource support") {
		t.Fatalf("the unsupported server is not called out:\n%s", res.Content)
	}
}

// A server exposing zero resources answers an EMPTY LIST, not an error.
func TestListMCPResourcesEmptyServerIsNotAnError(t *testing.T) {
	b := &resourceBackend{advertise: true}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "empty", b.start(t))

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "list_mcp_resources", map[string]any{})
	if res.IsError {
		t.Fatalf("empty resource set reported as an error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "no resources") {
		t.Fatalf("an empty server is not described:\n%s", res.Content)
	}
}

// One failing server must not fail the whole listing: the healthy server's
// resources still come back, with the broken one described beside them.
func TestListMCPResourcesOneBrokenServerDoesNotFailTheCall(t *testing.T) {
	good := &resourceBackend{advertise: true, resources: []map[string]any{{"uri": "file:///ok.md"}}}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "good", good.start(t))
	// A server pointed at a closed port: dialing it is a hard per-server failure.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	enableMCPServer(t, rt, "dead", deadURL)

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "list_mcp_resources", map[string]any{})
	if res.IsError {
		t.Fatalf("one dead server failed the whole call: %s", res.Content)
	}
	if !strings.Contains(res.Content, "file:///ok.md") {
		t.Errorf("the healthy server's resources were lost:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "dead") || !strings.Contains(res.Content, "ERROR") {
		t.Errorf("the dead server's failure is not surfaced:\n%s", res.Content)
	}
}

// An unknown server name is a hard error (the model mistyped), not an empty
// listing that would send it debugging a server that was never configured.
func TestListMCPResourcesRejectsUnknownServer(t *testing.T) {
	b := &resourceBackend{advertise: true}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "docs", b.start(t))

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "list_mcp_resources", map[string]any{"server": "nope"})
	if !res.IsError {
		t.Fatalf("unknown server accepted: %s", res.Content)
	}
	if !strings.Contains(res.Content, "docs") {
		t.Errorf("the error does not list the servers that DO exist: %s", res.Content)
	}
}

// Text content is returned inline.
func TestReadMCPResourceInlinesText(t *testing.T) {
	b := &resourceBackend{
		advertise: true,
		contents:  []map[string]any{{"uri": "file:///a.txt", "mimeType": "text/plain", "text": "hello resource"}},
	}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "docs", b.start(t))

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "read_mcp_resource", map[string]any{"server": "docs", "uri": "file:///a.txt"})
	if res.IsError {
		t.Fatalf("read failed: %s", res.Content)
	}
	if !strings.Contains(res.Content, "hello resource") || !strings.Contains(res.Content, "text/plain") {
		t.Fatalf("text content not inlined:\n%s", res.Content)
	}
}

// THE acceptance test for criterion 5: binary content must reach a FILE, never
// the model context. The assertion is two-sided — the file exists with the exact
// bytes, AND no base64 of those bytes appears in the returned text.
func TestReadMCPResourceWritesBinaryToFile(t *testing.T) {
	raw := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0xFF, 0x00}
	encoded := base64.StdEncoding.EncodeToString(raw)
	b := &resourceBackend{
		advertise: true,
		contents:  []map[string]any{{"uri": "file:///logo.png", "mimeType": "image/png", "blob": encoded}},
	}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "docs", b.start(t))

	sess, err := rt.db.CreateSession(context.Background(), db.Session{AgentID: "a1", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callResourceTool(t, rt, agent, sess.ID, "read_mcp_resource",
		map[string]any{"server": "docs", "uri": "file:///logo.png"})
	if res.IsError {
		t.Fatalf("read failed: %s", res.Content)
	}
	if strings.Contains(res.Content, encoded) {
		t.Fatal("base64 blob was inlined into the model context")
	}
	if !strings.Contains(res.Content, "image/png") {
		t.Errorf("mimeType missing from the result:\n%s", res.Content)
	}
	path := savedResourcePath(t, res.Content)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the saved blob at %s: %v", path, err)
	}
	if string(got) != string(raw) {
		t.Fatalf("saved bytes differ from the resource content")
	}
	// It must land under the SESSION scratchpad, where the agent is told to keep
	// durable output and where session deletion will clean it up.
	pad, err := rt.sessionScratchpad(sess.ID)
	if err != nil {
		t.Fatalf("scratchpad: %v", err)
	}
	if !strings.HasPrefix(path, pad) {
		t.Errorf("blob saved outside the session scratchpad: %s (pad %s)", path, pad)
	}
}

// savedResourcePath pulls the written file path out of the tool's output.
func savedResourcePath(t *testing.T, content string) string {
	t.Helper()
	const marker = "binary content written to: "
	i := strings.Index(content, marker)
	if i < 0 {
		t.Fatalf("no saved-file path in the result:\n%s", content)
	}
	rest := content[i+len(marker):]
	end := strings.Index(rest, " (")
	if end < 0 {
		t.Fatalf("malformed saved-file line: %s", rest)
	}
	return rest[:end]
}

// Truncation is EXPLICIT: the model is told the content was cut and by how
// much, so it never reasons over a prefix believing it has the whole document.
func TestReadMCPResourceTruncationIsReported(t *testing.T) {
	long := strings.Repeat("x", maxResourceTextBytes+500)
	b := &resourceBackend{
		advertise: true,
		contents:  []map[string]any{{"uri": "file:///big.txt", "mimeType": "text/plain", "text": long}},
	}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "docs", b.start(t))

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "read_mcp_resource", map[string]any{"server": "docs", "uri": "file:///big.txt"})
	if res.IsError {
		t.Fatalf("read failed: %s", res.Content)
	}
	if !strings.Contains(res.Content, "TRUNCATED") {
		t.Fatalf("a truncated read did not say so:\n%s", res.Content[:200])
	}
	if !strings.Contains(res.Content, "of 66036 bytes") {
		t.Errorf("the full size is not reported, so the model cannot tell how much it is missing")
	}
	if len(res.Content) > maxResourceTextBytes+2048 {
		t.Errorf("result is %d bytes; the cap did not apply", len(res.Content))
	}
}

// Reading from a server with no resource capability is an explicit error rather
// than a bare JSON-RPC "method not found".
func TestReadMCPResourceUnsupportedServer(t *testing.T) {
	b := &resourceBackend{advertise: false}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	enableMCPServer(t, rt, "toolsonly", b.start(t))

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callTool(t, rt, agent, "read_mcp_resource", map[string]any{"server": "toolsonly", "uri": "file:///x"})
	if !res.IsError {
		t.Fatalf("expected an error, got: %s", res.Content)
	}
	if !strings.Contains(res.Content, "resource support") {
		t.Errorf("the error does not explain the missing capability: %s", res.Content)
	}
}

// Both tools are withheld from the CLI bridge for BOTH dialects: those backends
// own their MCP clients, and bridging would open a second connection to the same
// server.
func TestMCPResourceToolsWithheldFromCLIBridge(t *testing.T) {
	for _, provider := range []string{"", providerKindCodexCLI} {
		for _, name := range []string{"list_mcp_resources", "read_mcp_resource"} {
			if !cliBridgeExcludes(name, provider) {
				t.Errorf("provider %q: %s must be excluded from the CLI bridge", provider, name)
			}
		}
	}
}

// A scoped server is read over the CALLER's own pool slot, not a second
// workspace-wide connection: the slot key must carry "<sessionID>|<agentID>".
func TestMCPResourcesUseTheCallersScopedSlot(t *testing.T) {
	b := &resourceBackend{advertise: true, resources: []map[string]any{{"uri": "file:///s.md"}}}
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	t.Cleanup(rt.CloseMCP)
	m := enableMCPServer(t, rt, "scoped", b.start(t))
	m.Scope = "scoped"
	if _, err := rt.db.UpdateMCPServer(context.Background(), m.ID, m); err != nil {
		t.Fatalf("scope the server: %v", err)
	}

	agent := db.Agent{ID: "a1", Name: "A", MCPEnabled: true, PermissionMode: "auto"}
	res := callResourceTool(t, rt, agent, "sess-9", "list_mcp_resources", map[string]any{})
	if res.IsError {
		t.Fatalf("list failed: %s", res.Content)
	}
	var scoped bool
	for _, s := range rt.mcpPool.Stats() {
		if s.Server == "scoped" && s.ScopeKey == "sess-9|a1" {
			scoped = true
		}
	}
	if !scoped {
		t.Fatalf("no scoped pool slot for (sess-9, a1); resources were read over the wrong connection: %+v", rt.mcpPool.Stats())
	}
}
