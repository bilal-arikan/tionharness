package codemode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/mcp"
)

func entry(server, tool, desc string) mcp.CatalogEntry {
	return mcp.CatalogEntry{
		Server:         server,
		NamespacedName: mcp.NamespaceTool(server, tool),
		Tool: mcp.Tool{
			Name:        tool,
			Description: desc,
			InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		},
	}
}

func TestWriteBindingsGeneratesModules(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mcp")
	entries := []mcp.CatalogEntry{
		entry("linear", "list_issues", "List issues."),
		entry("linear", "create_issue", "Create an issue."),
		entry("my-server", "import", "Keyword + dash stress test."),
	}
	modules, err := WriteBindings(dir, entries, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "_bridge.py")); err != nil {
		t.Fatalf("_bridge.py missing: %v", err)
	}
	if funcs := modules["linear"]; len(funcs) != 2 {
		t.Fatalf("linear functions = %v, want 2", funcs)
	}
	// Dash in the server name becomes "_" in the module; keyword tool name gains
	// a suffix — but the ORIGINAL namespaced name must be what the wrapper calls.
	funcs, ok := modules["my_server"]
	if !ok || len(funcs) != 1 || funcs[0] != "import_" {
		t.Fatalf("my_server functions = %v (ok=%v), want [import_]", funcs, ok)
	}
	src, err := os.ReadFile(filepath.Join(dir, "my_server.py"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `_bridge.call("my-server__import", args)`) {
		t.Fatalf("wrapper must call the original namespaced name, got:\n%s", src)
	}
	if !strings.Contains(string(src), "Input schema:") {
		t.Fatal("docstring must embed the input schema")
	}
}

func TestWriteBindingsAppliesAllowFilter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mcp")
	entries := []mcp.CatalogEntry{
		entry("demo", "allowed", "ok"),
		entry("demo", "blocked", "no"),
	}
	modules, err := WriteBindings(dir, entries, nil, func(name string) bool { return name == "demo__allowed" })
	if err != nil {
		t.Fatal(err)
	}
	if funcs := modules["demo"]; len(funcs) != 1 || funcs[0] != "allowed" {
		t.Fatalf("functions = %v, want [allowed]", funcs)
	}
	src, _ := os.ReadFile(filepath.Join(dir, "demo.py"))
	if strings.Contains(string(src), "blocked") {
		t.Fatal("blocked tool leaked into the bindings")
	}
}

func TestWriteBindingsRegeneratesFromScratch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mcp")
	if _, err := WriteBindings(dir, []mcp.CatalogEntry{entry("old", "gone", "x")}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteBindings(dir, []mcp.CatalogEntry{entry("new", "here", "y")}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.py")); !os.IsNotExist(err) {
		t.Fatal("stale module from a removed server must not linger")
	}
	if _, err := os.Stat(filepath.Join(dir, "new.py")); err != nil {
		t.Fatalf("new module missing: %v", err)
	}
}

func TestWriteBindingsBuiltinModule(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mcp")
	builtins := []BuiltinDef{
		{Name: "update_flow", Description: "Edit a flow.", InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}}}`)},
		{Name: "secret", Description: "Manage the vault.", InputSchema: json.RawMessage(`{"type":"object","properties":{"action":{"type":"string"}}}`)},
		{Name: "blocked_builtin", Description: "no", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	allow := func(name string) bool { return name != "blocked_builtin" }
	modules, err := WriteBindings(dir, nil, builtins, allow)
	if err != nil {
		t.Fatal(err)
	}
	funcs, ok := modules[BuiltinServer]
	if !ok {
		t.Fatalf("built-in module %q missing; modules=%v", BuiltinServer, modules)
	}
	if len(funcs) != 2 {
		t.Fatalf("built-in funcs = %v, want 2 (blocked one filtered)", funcs)
	}
	src, err := os.ReadFile(filepath.Join(dir, BuiltinServer+".py"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `_bridge.call("tionharness__update_flow", args)`) {
		t.Fatalf("built-in wrapper must dispatch under tionharness__ namespace, got:\n%s", src)
	}
	if strings.Contains(string(src), "blocked_builtin") {
		t.Fatal("disallowed built-in leaked into bindings")
	}
}

// TestWriteBindingsBuiltinServerCollision covers the guard: an MCP server that
// sanitises to the reserved built-ins module name is renamed so it never clobbers
// the tionharness built-ins module.
func TestWriteBindingsBuiltinServerCollision(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mcp")
	entries := []mcp.CatalogEntry{entry(BuiltinServer, "alpha", "x")}
	builtins := []BuiltinDef{{Name: "beta", Description: "y", InputSchema: json.RawMessage(`{"type":"object"}`)}}
	modules, err := WriteBindings(dir, entries, builtins, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := modules[BuiltinServer]; !ok {
		t.Fatal("built-in module must keep the reserved name")
	}
	if _, ok := modules[BuiltinServer+"_server"]; !ok {
		t.Fatalf("colliding MCP server must be renamed to %q; modules=%v", BuiltinServer+"_server", modules)
	}
}

func TestPyIdent(t *testing.T) {
	cases := map[string]string{
		"list_issues": "list_issues",
		"my-server":   "my_server",
		"import":      "import_",
		"9lives":      "_9lives",
		"":            "_",
	}
	for in, want := range cases {
		if got := pyIdent(in); got != want {
			t.Errorf("pyIdent(%q) = %q, want %q", in, got, want)
		}
	}
}
