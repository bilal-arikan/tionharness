package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/codemode"
	"github.com/bilal-arikan/swarmgo/internal/mcp"
)

func runCodeEntries() []mcp.CatalogEntry {
	return []mcp.CatalogEntry{{
		Server:         "demo",
		NamespacedName: "demo__ping",
		Tool: mcp.Tool{
			Name:        "ping",
			Description: "Echo a message back.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}}}`),
		},
	}}
}

func TestRunCodeRequiresCatalog(t *testing.T) {
	tool := NewRunCodeTool(NewSandbox(t.TempDir()), nil, nil, nil, nil, nil, nil, nil)
	if _, err := tool.Call(t.Context(), []byte(`{}`)); err == nil {
		t.Fatal("expected error when no MCP tools are available")
	}
}

func TestRunCodeDiscoveryListsModules(t *testing.T) {
	dir := t.TempDir()
	tool := NewRunCodeTool(NewSandbox(dir), runCodeEntries(), nil, nil, nil, nil, nil, nil)
	out, err := tool.Call(t.Context(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "demo: ping") {
		t.Fatalf("listing must name the module + function, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".swarmgo", "mcp", "demo.py")); err != nil {
		t.Fatalf("bindings not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".swarmgo", "mcp", "_bridge.py")); err != nil {
		t.Fatalf("_bridge.py not written: %v", err)
	}
}

func TestRunCodeScriptCallsMCPAndReturnsOnlyStdout(t *testing.T) {
	if !pythonAvailable() {
		t.Skip("python not available")
	}
	dir := t.TempDir()
	caller := func(_ context.Context, namespaced string, args json.RawMessage) (mcp.CallToolResult, error) {
		if namespaced != "demo__ping" {
			t.Errorf("unexpected tool %q", namespaced)
		}
		return mcp.CallToolResult{Text: `{"rows":[1,2,3,4,5]}`}, nil
	}
	tool := NewRunCodeTool(NewSandbox(dir), runCodeEntries(), caller, nil, nil, nil, nil, nil)

	script := "from demo import ping\n" +
		"r = ping(msg='hi')\n" + // large-ish structured result stays in the variable
		"print('count:', len(r['rows']))\n" // only the aggregate returns
	args, _ := json.Marshal(map[string]any{"script": script})
	out, err := tool.Call(t.Context(), args)
	if err != nil {
		t.Fatalf("run_code failed: %v", err)
	}
	if !strings.Contains(out, "count: 5") {
		t.Fatalf("stdout missing, got:\n%s", out)
	}
	if strings.Contains(out, `"rows"`) {
		t.Fatalf("raw data leaked into the tool result:\n%s", out)
	}
	if !strings.Contains(out, "1 tool call(s): demo__ping") {
		t.Fatalf("mcp call summary missing, got:\n%s", out)
	}
}

func TestRunCodeScriptFailureIsLoud(t *testing.T) {
	if !pythonAvailable() {
		t.Skip("python not available")
	}
	dir := t.TempDir()
	caller := func(context.Context, string, json.RawMessage) (mcp.CallToolResult, error) {
		return mcp.CallToolResult{Text: "unused"}, nil
	}
	tool := NewRunCodeTool(NewSandbox(dir), runCodeEntries(), caller, nil, nil, nil, nil, nil)
	args, _ := json.Marshal(map[string]any{"script": "raise SystemExit('boom')"})
	if _, err := tool.Call(t.Context(), args); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("script failure must surface loudly with its output, got: %v", err)
	}
}

func TestRunCodeGateDeniesInScriptCallAndObserverRecords(t *testing.T) {
	if !pythonAvailable() {
		t.Skip("python not available")
	}
	dir := t.TempDir()
	caller := func(context.Context, string, json.RawMessage) (mcp.CallToolResult, error) {
		t.Error("dispatcher must not run for a gate-denied call")
		return mcp.CallToolResult{}, nil
	}
	gate := func(_ context.Context, tool string, _ json.RawMessage) (bool, string) {
		return false, "permission denied by user: " + tool + " was not approved"
	}
	var observed []codemode.CallObservation
	observe := func(_ context.Context, ob codemode.CallObservation) { observed = append(observed, ob) }
	tool := NewRunCodeTool(NewSandbox(dir), runCodeEntries(), caller, nil, nil, nil, gate, observe)

	script := "from demo import ping\n" +
		"import _bridge\n" +
		"try:\n" +
		"    ping(msg='hi')\n" +
		"    print('CALL SUCCEEDED')\n" +
		"except _bridge.MCPError as e:\n" +
		"    print('denied:', e)\n"
	args, _ := json.Marshal(map[string]any{"script": script})
	out, err := tool.Call(t.Context(), args)
	if err != nil {
		t.Fatalf("run_code failed: %v", err)
	}
	if strings.Contains(out, "CALL SUCCEEDED") || !strings.Contains(out, "denied:") {
		t.Fatalf("gate denial must reach the script as MCPError, got:\n%s", out)
	}
	if !strings.Contains(out, "1 denied by permission gate") {
		t.Fatalf("summary must count the denial, got:\n%s", out)
	}
	if len(observed) != 1 || !observed[0].Denied || observed[0].Tool != "demo__ping" {
		t.Fatalf("observer must record the denial, got %+v", observed)
	}
}

func TestRunCodeObserverRecordsDispatchedCalls(t *testing.T) {
	if !pythonAvailable() {
		t.Skip("python not available")
	}
	dir := t.TempDir()
	caller := func(context.Context, string, json.RawMessage) (mcp.CallToolResult, error) {
		return mcp.CallToolResult{Text: `{"ok":true}`}, nil
	}
	gate := func(context.Context, string, json.RawMessage) (bool, string) { return true, "" }
	var observed []codemode.CallObservation
	observe := func(_ context.Context, ob codemode.CallObservation) { observed = append(observed, ob) }
	tool := NewRunCodeTool(NewSandbox(dir), runCodeEntries(), caller, nil, nil, nil, gate, observe)

	args, _ := json.Marshal(map[string]any{"script": "from demo import ping\nprint(ping(msg='x')['ok'])"})
	out, err := tool.Call(t.Context(), args)
	if err != nil {
		t.Fatalf("run_code failed: %v", err)
	}
	if !strings.Contains(out, "True") {
		t.Fatalf("script output missing, got:\n%s", out)
	}
	if len(observed) != 1 {
		t.Fatalf("observed = %+v, want exactly 1 record", observed)
	}
	ob := observed[0]
	if ob.Tool != "demo__ping" || ob.Denied || ob.IsError || ob.OutBytes == 0 {
		t.Fatalf("unexpected observation: %+v", ob)
	}
}

func TestRunCodeBridgeEnforcesAgentFilterInScript(t *testing.T) {
	if !pythonAvailable() {
		t.Skip("python not available")
	}
	dir := t.TempDir()
	caller := func(context.Context, string, json.RawMessage) (mcp.CallToolResult, error) {
		t.Error("dispatcher must not run for a filtered tool")
		return mcp.CallToolResult{}, nil
	}
	// The allow filter drops demo__ping from the BINDINGS; calling _bridge.call
	// directly (simulating a bypass attempt) must be rejected by the bridge too.
	allow := func(name string) bool { return name != "demo__ping" }
	tool := NewRunCodeTool(NewSandbox(dir), runCodeEntries(), caller, nil, nil, allow, nil, nil)
	script := "import _bridge\n" +
		"try:\n" +
		"    _bridge.call('demo__ping', {})\n" +
		"    print('CALL SUCCEEDED')\n" +
		"except _bridge.MCPError as e:\n" +
		"    print('rejected:', e)\n"
	args, _ := json.Marshal(map[string]any{"script": script})
	out, err := tool.Call(t.Context(), args)
	if err != nil {
		t.Fatalf("run_code failed: %v", err)
	}
	if strings.Contains(out, "CALL SUCCEEDED") || !strings.Contains(out, "rejected:") {
		t.Fatalf("filtered tool must be rejected by the bridge, got:\n%s", out)
	}
}

func TestRunCodeDispatchesBuiltinModule(t *testing.T) {
	if !pythonAvailable() {
		t.Skip("python not available")
	}
	dir := t.TempDir()
	builtins := []codemode.BuiltinDef{{
		Name:        "echo_tool",
		Description: "Echo the args back.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"number"}}}`),
	}}
	var gotName string
	callBI := func(_ context.Context, name string, _ json.RawMessage) (mcp.CallToolResult, error) {
		gotName = name // must be the BARE built-in name, not the swarmgo__ namespace
		return mcp.CallToolResult{Text: `{"echoed":true}`}, nil
	}
	// No MCP entries at all — built-ins alone drive run_code.
	tool := NewRunCodeTool(NewSandbox(dir), nil, nil, builtins, callBI, nil, nil, nil)

	script := "from swarmgo import echo_tool\nprint('ok:', echo_tool(x=1)['echoed'])"
	args, _ := json.Marshal(map[string]any{"script": script})
	out, err := tool.Call(t.Context(), args)
	if err != nil {
		t.Fatalf("run_code failed: %v", err)
	}
	if !strings.Contains(out, "ok: True") {
		t.Fatalf("built-in dispatch output missing, got:\n%s", out)
	}
	if gotName != "echo_tool" {
		t.Fatalf("dispatcher must receive the bare built-in name, got %q", gotName)
	}
	if !strings.Contains(out, "swarmgo__echo_tool") {
		t.Fatalf("summary should count the namespaced built-in call, got:\n%s", out)
	}
}

// TestRunCodeBuiltinsOnlyDiscovery verifies run_code is usable with ZERO MCP
// servers: the discovery listing shows the swarmgo built-ins module.
func TestRunCodeBuiltinsOnlyDiscovery(t *testing.T) {
	dir := t.TempDir()
	builtins := []codemode.BuiltinDef{{
		Name: "list_flows", Description: "List flows.", InputSchema: json.RawMessage(`{"type":"object"}`),
	}}
	tool := NewRunCodeTool(NewSandbox(dir), nil, nil, builtins, nil, nil, nil, nil)
	out, err := tool.Call(t.Context(), []byte(`{}`))
	if err != nil {
		t.Fatalf("discovery failed: %v", err)
	}
	if !strings.Contains(out, "swarmgo: list_flows") {
		t.Fatalf("listing must name the swarmgo module + function, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".swarmgo", "mcp", "swarmgo.py")); err != nil {
		t.Fatalf("swarmgo bindings not written: %v", err)
	}
}
