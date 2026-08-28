package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexConfigMinimal(t *testing.T) {
	got := renderCodexConfig(codexConfig{DeveloperInstructions: "Be terse.\nAnswer in English."})
	want := "developer_instructions = \"\"\"Be terse.\nAnswer in English.\"\"\"\n"
	if got != want {
		t.Fatalf("minimal config mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestCodexConfigEmptyRendersNothing(t *testing.T) {
	if got := renderCodexConfig(codexConfig{}); got != "" {
		t.Fatalf("empty config should render nothing, got %q", got)
	}
}

func TestCodexConfigReasoningEffortOmittedWhenEmpty(t *testing.T) {
	if got := renderCodexConfig(codexConfig{ReasoningEffort: ""}); strings.Contains(got, "model_reasoning_effort") {
		t.Fatalf("empty effort must be omitted, got %q", got)
	}
	got := renderCodexConfig(codexConfig{ReasoningEffort: "xhigh"})
	if got != "model_reasoning_effort = \"xhigh\"\n" {
		t.Fatalf("effort mismatch, got %q", got)
	}
}

func TestCodexConfigFull(t *testing.T) {
	cfg := codexConfig{
		DeveloperInstructions:   "Stay on task.",
		ReasoningEffort:         "high",
		DisableWebSearch:        true,
		DisableUpdatePlan:       true,
		DisableRequestUserInput: true,
		Servers: map[string]CLIMCPServer{
			// Deliberately declared out of order: output must still be sorted.
			"tionharness_interaction": {
				Transport: "http",
				URL:       "http://127.0.0.1:8731/core",
				Headers: map[string]string{
					"Authorization": "Bearer tok-123",
					"X-Session":     "SES579",
				},
				AlwaysLoad: true, // no codex equivalent — must not appear in output
			},
			"local_probe": {
				Command: "node",
				Args:    []string{"probe.js", "--port", "9000"},
				Env:     map[string]string{"TOKEN": "abc", "MODE": "test"},
			},
		},
	}

	want := `developer_instructions = """Stay on task."""
model_reasoning_effort = "high"

[tools]
web_search = false
update_plan = { enabled = false }
experimental_request_user_input = { enabled = false }

[mcp_servers.local_probe]
command = "node"
args = ["probe.js", "--port", "9000"]
env = { MODE = "test", TOKEN = "abc" }
startup_timeout_sec = 30
tool_timeout_sec = 900
default_tools_approval_mode = "approve"
required = true

[mcp_servers.tionharness_interaction]
url = "http://127.0.0.1:8731/core"
http_headers = { Authorization = "Bearer tok-123", X-Session = "SES579" }
startup_timeout_sec = 30
tool_timeout_sec = 900
default_tools_approval_mode = "approve"
required = true
`

	got := renderCodexConfig(cfg)
	if got != want {
		t.Fatalf("full config mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	// The approval mode is what keeps every MCP tool call from being cancelled;
	// assert it lands in BOTH server blocks, not merely somewhere in the file.
	if n := strings.Count(got, `default_tools_approval_mode = "approve"`); n != 2 {
		t.Fatalf("approval mode must appear once per server block, got %d", n)
	}
	if strings.Contains(got, "always_load") || strings.Contains(got, "AlwaysLoad") {
		t.Fatalf("AlwaysLoad has no codex equivalent and must not be rendered:\n%s", got)
	}
}

func TestRenderCodexConfigMarksServersRequired(t *testing.T) {
	got := renderCodexConfig(codexConfig{Servers: map[string]CLIMCPServer{
		"stdio_server": {Command: "node", Args: []string{"probe.js"}},
		"remote_server": {
			Transport: "http",
			URL:       "http://127.0.0.1:8731/core",
		},
	}})

	// Every mcp_servers block must carry required = true — codex-cli treats a
	// server without it as optional and drops its tools after a 1s startup
	// grace (tool_catalog.rs:35, 171-224), which for codex exec's per-turn
	// fresh process means the first turn always loses that server's tools.
	if n := strings.Count(got, "required = true"); n != 2 {
		t.Fatalf("expected required = true in both server blocks, got %d occurrences:\n%s", n, got)
	}
	stdioIdx := strings.Index(got, "[mcp_servers.stdio_server]")
	remoteIdx := strings.Index(got, "[mcp_servers.remote_server]")
	if stdioIdx < 0 || remoteIdx < 0 {
		t.Fatalf("missing expected server blocks:\n%s", got)
	}
	// "remote_server" sorts before "stdio_server" (r < s), so it renders first.
	remoteBlock := got[remoteIdx:stdioIdx]
	stdioBlock := got[stdioIdx:]
	if !strings.Contains(stdioBlock, "required = true") {
		t.Fatalf("stdio server block missing required = true:\n%s", stdioBlock)
	}
	if !strings.Contains(remoteBlock, "required = true") {
		t.Fatalf("remote server block missing required = true:\n%s", remoteBlock)
	}
}

func TestCodexConfigUpdatePlanIsInlineTableNotBool(t *testing.T) {
	got := renderCodexConfig(codexConfig{DisableUpdatePlan: true})
	if !strings.Contains(got, "update_plan = { enabled = false }") {
		t.Fatalf("update_plan must be an inline table, got %q", got)
	}
	// A bare bool is rejected by codex with
	// "invalid type: boolean `false`, expected struct UpdatePlanToolConfig".
	if strings.Contains(got, "update_plan = false") {
		t.Fatalf("update_plan must never render as a bare bool, got %q", got)
	}
	// web_search, by contrast, does accept a bare bool.
	web := renderCodexConfig(codexConfig{DisableWebSearch: true})
	if !strings.Contains(web, "web_search = false\n") {
		t.Fatalf("web_search must render as a bare bool, got %q", web)
	}
}

func TestCodexConfigToolsSectionOmittedWhenNoToggle(t *testing.T) {
	got := renderCodexConfig(codexConfig{DeveloperInstructions: "hi"})
	if strings.Contains(got, "[tools]") {
		t.Fatalf("[tools] must be omitted when no toggle is set, got %q", got)
	}
}

func TestCodexConfigNativeWebSearchRequest(t *testing.T) {
	c := &CodexCLI{}
	tests := []struct {
		name            string
		nativeWebSearch bool
		wantDisabled    bool
	}{
		{name: "enabled omits strict config key", nativeWebSearch: true},
		{name: "disabled renders false", nativeWebSearch: false, wantDisabled: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := renderCodexConfig(c.buildConfig(Request{NativeWebSearch: tc.nativeWebSearch}))
			if tc.wantDisabled {
				if !strings.Contains(got, "web_search = false\n") {
					t.Fatalf("disabled native web search must render false, got %q", got)
				}
				return
			}
			if strings.Contains(got, "web_search") {
				t.Fatalf("enabled native web search must omit the strict config key, got %q", got)
			}
		})
	}
}

func TestCodexConfigURLWithoutTransportIsRemote(t *testing.T) {
	got := renderCodexConfig(codexConfig{Servers: map[string]CLIMCPServer{
		"remote": {URL: "https://example.test/mcp"},
	}})
	if !strings.Contains(got, `url = "https://example.test/mcp"`) {
		t.Fatalf("non-empty URL must render a remote block, got %q", got)
	}
	if strings.Contains(got, "command =") {
		t.Fatalf("remote block must not render a command, got %q", got)
	}
}

func TestCodexConfigStdioServerWithoutArgs(t *testing.T) {
	got := renderCodexConfig(codexConfig{Servers: map[string]CLIMCPServer{
		"bare": {Command: "mcp-server"},
	}})
	if !strings.Contains(got, "args = []\n") {
		t.Fatalf("missing args must render as an empty array, got %q", got)
	}
	if strings.Contains(got, "env =") {
		t.Fatalf("empty env must be omitted, got %q", got)
	}
}

func TestTomlStringEscaping(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"windows path", `C:\Users\x\y.js`, `"C:\\Users\\x\\y.js"`},
		{"embedded quote", `say "hi"`, `"say \"hi\""`},
		{"backslash and quote", `a\"b`, `"a\\\"b"`},
		{"newline and tab", "a\nb\tc", `"a\nb\tc"`},
		{"carriage return", "a\r\nb", `"a\r\nb"`},
		{"control char", "a\x01b", `"a\u0001b"`},
		{"plain", "gpt-5.5", `"gpt-5.5"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tomlString(tc.in); got != tc.want {
				t.Fatalf("tomlString(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestCodexConfigWindowsPathInArgs(t *testing.T) {
	got := renderCodexConfig(codexConfig{Servers: map[string]CLIMCPServer{
		"win": {Command: `C:\Program Files\nodejs\node.exe`, Args: []string{`C:\Users\x\y.js`}},
	}})
	if !strings.Contains(got, `command = "C:\\Program Files\\nodejs\\node.exe"`) {
		t.Fatalf("windows command not escaped:\n%s", got)
	}
	if !strings.Contains(got, `args = ["C:\\Users\\x\\y.js"]`) {
		t.Fatalf("windows arg not escaped:\n%s", got)
	}
}

func TestTomlMultilineStringDoesNotBreakOnTripleQuote(t *testing.T) {
	// Prompt text containing a triple quote (a fenced snippet, a docstring) must
	// not be able to terminate developer_instructions early.
	got := renderCodexConfig(codexConfig{DeveloperInstructions: `use """ to fence`})
	body := strings.TrimSuffix(strings.TrimPrefix(got, "developer_instructions = "), "\n")
	if !strings.HasPrefix(body, `"""`) || !strings.HasSuffix(body, `"""`) {
		t.Fatalf("delimiters missing: %q", body)
	}
	inner := body[3 : len(body)-3]
	if strings.Contains(inner, `"""`) {
		t.Fatalf("unescaped triple quote left inside the value: %q", inner)
	}
	if inner != `use ""\" to fence` {
		t.Fatalf("unexpected escaping: %q", inner)
	}
}

func TestTomlMultilineStringTrailingQuote(t *testing.T) {
	got := renderCodexConfig(codexConfig{DeveloperInstructions: `ends with "`})
	// The trailing quote must be escaped so it cannot merge with the delimiter.
	if !strings.HasSuffix(got, "ends with \\\"\"\"\"\n") {
		t.Fatalf("trailing quote not escaped: %q", got)
	}
}

func TestTomlMultilineStringEscapesBackslash(t *testing.T) {
	got := renderCodexConfig(codexConfig{DeveloperInstructions: `path C:\tmp`})
	if !strings.Contains(got, `path C:\\tmp`) {
		t.Fatalf("backslash not escaped in multi-line string: %q", got)
	}
}

func TestTomlKeyQuotesUnsafeSegments(t *testing.T) {
	if got := tomlKey("plain_key-1"); got != "plain_key-1" {
		t.Fatalf("bare key must stay unquoted, got %s", got)
	}
	if got := tomlKey("has.dot"); got != `"has.dot"` {
		t.Fatalf("dotted key must be quoted, got %s", got)
	}
	got := renderCodexConfig(codexConfig{Servers: map[string]CLIMCPServer{
		"has.dot": {Command: "x"},
	}})
	if !strings.Contains(got, `[mcp_servers."has.dot"]`) {
		t.Fatalf("dotted server key must be quoted in the table path:\n%s", got)
	}
}

func TestCodexConfigDeterministicAcrossRenders(t *testing.T) {
	cfg := codexConfig{Servers: map[string]CLIMCPServer{
		"c": {URL: "http://c", Headers: map[string]string{"B": "2", "A": "1", "C": "3"}},
		"a": {Command: "a", Env: map[string]string{"Z": "z", "Y": "y"}},
		"b": {Command: "b"},
	}}
	first := renderCodexConfig(cfg)
	for i := 0; i < 20; i++ {
		if got := renderCodexConfig(cfg); got != first {
			t.Fatalf("render is not deterministic on iteration %d:\n%s\nvs\n%s", i, got, first)
		}
	}
	if a, b, c := strings.Index(first, "[mcp_servers.a]"), strings.Index(first, "[mcp_servers.b]"), strings.Index(first, "[mcp_servers.c]"); !(a < b && b < c) {
		t.Fatalf("server blocks are not in sorted order:\n%s", first)
	}
}

func TestWriteCodexConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := codexConfig{DeveloperInstructions: "hello", DisableUpdatePlan: true}

	path, dropped, cleanup, err := writeCodexConfig(context.Background(), dir, cfg, nil)
	if len(dropped) != 0 {
		t.Fatalf("no servers configured, nothing can be dropped: %v", dropped)
	}
	if err != nil {
		t.Fatalf("writeCodexConfig: %v", err)
	}
	if want := filepath.Join(dir, "config.toml"); path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != renderCodexConfig(cfg) {
		t.Fatalf("written content differs from the rendered config:\n%s", data)
	}

	// Cleanup is a no-op: config.toml is the CODEX_HOME's real config.
	cleanup()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cleanup must not remove the real home config: %v", err)
	}
}

func TestWriteCodexConfigEmptyDirErrors(t *testing.T) {
	if _, _, _, err := writeCodexConfig(context.Background(), "", codexConfig{}, nil); err == nil {
		t.Fatal("expected an error for an empty config dir")
	}
}

func TestWriteCodexConfigMissingDirErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir")
	if _, _, _, err := writeCodexConfig(context.Background(), missing, codexConfig{}, nil); err == nil {
		t.Fatal("expected an error when the config dir does not exist")
	}
}

// A remote MCP server that is down must not take the whole session with it:
// codex aborts with "required MCP servers failed to initialize" before any turn
// output, so the unreachable server is omitted while the healthy one keeps
// required = true.
func TestWriteCodexConfigDropsUnreachableRemoteServers(t *testing.T) {
	dir := t.TempDir()
	cfg := codexConfig{Servers: map[string]CLIMCPServer{
		"healthy": {Transport: "http", URL: "http://127.0.0.1:8731/core"},
		"down":    {Transport: "http", URL: "http://127.0.0.1:8080/mcp"},
	}}

	path, dropped, _, err := writeCodexConfig(context.Background(), dir, cfg, func(_ context.Context, rawURL string) bool {
		return rawURL != "http://127.0.0.1:8080/mcp"
	})
	if err != nil {
		t.Fatalf("writeCodexConfig: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(data)
	if strings.Contains(got, "[mcp_servers.down]") || strings.Contains(got, "8080") {
		t.Fatalf("unreachable server must be omitted entirely:\n%s", got)
	}
	if !strings.Contains(got, "[mcp_servers.healthy]") {
		t.Fatalf("reachable server must survive:\n%s", got)
	}
	// The surviving block keeps required = true — dropping is the fix, never
	// downgrading a server to optional.
	if n := strings.Count(got, "required = true"); n != 1 {
		t.Fatalf("expected exactly one required = true, got %d:\n%s", n, got)
	}
	if len(dropped) != 1 || dropped[0] != "down" {
		t.Fatalf("dropped = %v, want [down]", dropped)
	}
}

// stdio servers are spawned by codex itself — there is nothing to connect to
// before the turn starts, so they must never be probed or dropped.
func TestWriteCodexConfigNeverProbesStdioServers(t *testing.T) {
	dir := t.TempDir()
	cfg := codexConfig{Servers: map[string]CLIMCPServer{
		"stdio_server": {Command: "node", Args: []string{"probe.js"}},
	}}

	probed := 0
	path, dropped, _, err := writeCodexConfig(context.Background(), dir, cfg, func(context.Context, string) bool {
		probed++
		return false
	})
	if err != nil {
		t.Fatalf("writeCodexConfig: %v", err)
	}
	if probed != 0 {
		t.Fatalf("stdio server was probed %d time(s)", probed)
	}
	if len(dropped) != 0 {
		t.Fatalf("stdio server must never be dropped, got %v", dropped)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(data), "[mcp_servers.stdio_server]") {
		t.Fatalf("stdio server missing from config:\n%s", data)
	}
}

// Every dropped key must be reported so the caller can make the capability loss
// visible; a silent drop is the failure mode this whole path exists to avoid.
func TestWriteCodexConfigReportsAllDroppedKeys(t *testing.T) {
	dir := t.TempDir()
	cfg := codexConfig{Servers: map[string]CLIMCPServer{
		"alpha": {URL: "http://127.0.0.1:1/a"},
		"zulu":  {Transport: "sse", URL: "http://127.0.0.1:2/z"},
		"kept":  {URL: "http://127.0.0.1:3/k"},
		"stdio": {Command: "node"},
	}}

	_, dropped, _, err := writeCodexConfig(context.Background(), dir, cfg, func(_ context.Context, rawURL string) bool {
		return rawURL == "http://127.0.0.1:3/k"
	})
	if err != nil {
		t.Fatalf("writeCodexConfig: %v", err)
	}
	// Sorted, so the report is deterministic across runs.
	if strings.Join(dropped, ",") != "alpha,zulu" {
		t.Fatalf("dropped = %v, want [alpha zulu]", dropped)
	}
}

// Codex ships its native multi-agent/collab tools ON by default; they shadow
// TionHarness's spawn_worker delegation and cannot work under --ephemeral. Both
// keys must reach the file, and the block must sit after the top-level keys so
// they are not swallowed into [features].
func TestCodexConfigDisablesNativeMultiAgent(t *testing.T) {
	got := renderCodexConfig(codexConfig{
		ReasoningEffort:         "high",
		DisableNativeMultiAgent: true,
		DisableUpdatePlan:       true,
	})
	want := "model_reasoning_effort = \"high\"\n" +
		"\n[features]\nmulti_agent = false\nmulti_agent_v2 = false\n" +
		"\n[tools]\nupdate_plan = { enabled = false }\n"
	if got != want {
		t.Fatalf("features block mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestCodexConfigFeaturesOmittedWhenNotDisabled(t *testing.T) {
	if got := renderCodexConfig(codexConfig{ReasoningEffort: "low"}); strings.Contains(got, "[features]") {
		t.Fatalf("features block must be opt-in, got %q", got)
	}
}
