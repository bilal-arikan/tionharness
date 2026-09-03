package climcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// fakeHost is the in-memory Host every test in this package uses.
type fakeHost struct {
	servers  []mcp.ServerConfig
	gate     func(string) bool
	gateErr  error
	hooks    map[string][]db.Hook
	shell    bool
	cliHooks bool
	debug    []db.DebugEvent
}

func (f *fakeHost) EnabledServers(context.Context) ([]mcp.ServerConfig, error) { return f.servers, nil }
func (f *fakeHost) ServerGate(context.Context, db.Agent) (func(string) bool, error) {
	return f.gate, f.gateErr
}
func (f *fakeHost) EnabledHooks(_ context.Context, event string) ([]db.Hook, error) {
	return f.hooks[event], nil
}
func (f *fakeHost) ShellEnabled() bool    { return f.shell }
func (f *fakeHost) CLIHooksEnabled() bool { return f.cliHooks }
func (f *fakeHost) Logger() *slog.Logger  { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
func (f *fakeHost) EmitDebug(_ context.Context, ev db.DebugEvent) {
	f.debug = append(f.debug, ev)
}

func readConfig(t *testing.T, path string) Config {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v\n%s", err, data)
	}
	return cfg
}

func toSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, s := range list {
		out[s] = true
	}
	return out
}

var bridgedInter = tools.InteractionEndpoint{
	URL:               "http://127.0.0.1:8090/mcp/interaction",
	Token:             "tok-123",
	CoreToolNames:     []string{"Bash", "ask_user", "todo_write", "use_skill", "permission_prompt"},
	ExtendedToolNames: []string{"update_session", "create_agent"},
}

// TestWriteConfigTwoTierInteraction locks the claude-cli 2.1.x+ two-tier bridge:
// the Interaction MCP endpoint is emitted as two server entries — a core server
// marked alwaysLoad (eager, never deferred) and an extended server (deferred via
// the CLI's ToolSearch) — with the core tier allowlisted per tool and the
// extended tier through a single server-level wildcard.
func TestWriteConfigTwoTierInteraction(t *testing.T) {
	h := &fakeHost{shell: true}
	res, err := WriteConfig(context.Background(), h, false, db.Agent{}, bridgedInter, "ask")
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	defer res.Cleanup()
	if res.Path == "" {
		t.Fatal("expected a config path for a populated interaction endpoint")
	}
	cfg := readConfig(t, res.Path)

	core, ok := cfg.MCPServers[InteractionCoreKey]
	if !ok || !core.AlwaysLoad || !strings.HasSuffix(core.URL, "/core") {
		t.Fatalf("core entry = %+v ok=%v; want alwaysLoad + /core", core, ok)
	}
	ext, ok := cfg.MCPServers[InteractionExtendedKey]
	if !ok || ext.AlwaysLoad || !strings.HasSuffix(ext.URL, "/extended") {
		t.Fatalf("extended entry = %+v ok=%v; want deferred + /extended", ext, ok)
	}
	if core.Headers["Authorization"] != "Bearer tok-123" {
		t.Fatalf("core auth header = %q", core.Headers["Authorization"])
	}

	got := toSet(res.Allowed)
	for _, w := range []string{
		InteractionToolPrefix + "Bash", InteractionToolPrefix + "ask_user",
		InteractionToolPrefix + "permission_prompt", "mcp__" + InteractionExtendedKey,
	} {
		if !got[w] {
			t.Errorf("allowlist missing %q; got %v", w, res.Allowed)
		}
	}
	for _, a := range res.Allowed {
		if strings.HasPrefix(a, "mcp__"+InteractionExtendedKey+"__") {
			t.Errorf("extended tier should use a server-level wildcard, not per-tool entry %q", a)
		}
	}
	// Bridged families are suppressed; plan mode stays (ask mode can answer it).
	dis := toSet(res.Disallowed)
	for _, w := range []string{"AskUserQuestion", "TodoWrite", "TaskCreate", "Skill", "Task", "Agent", "SendMessage", "Bash", "TaskStop"} {
		if !dis[w] {
			t.Errorf("expected %q suppressed; got %v", w, res.Disallowed)
		}
	}
	if dis["EnterPlanMode"] {
		t.Error("ask mode must keep EnterPlanMode/ExitPlanMode")
	}
}

// TestWriteConfigNativeWebSearchWithoutServers pins that the agent-level web
// search opt-out travels even when nothing is mounted: no config file, but a
// disallow list the caller must still apply.
func TestWriteConfigNativeWebSearchWithoutServers(t *testing.T) {
	off := false
	res, err := WriteConfig(context.Background(), &fakeHost{}, false, db.Agent{NativeWebSearch: &off}, tools.InteractionEndpoint{}, "auto")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Cleanup()
	if res.Path != "" {
		t.Fatalf("no servers → no config file, got %q", res.Path)
	}
	dis := toSet(res.Disallowed)
	if !dis["WebSearch"] || !dis["WebFetch"] {
		t.Fatalf("web natives must be suppressed, got %v", res.Disallowed)
	}
}

// TestWriteConfigRequiredCoreInvariant is the WS17 rule: a native whose bridged
// replacement is NOT advertised this turn keeps its native fallback.
func TestWriteConfigRequiredCoreInvariant(t *testing.T) {
	bare := tools.InteractionEndpoint{URL: bridgedInter.URL, Token: "t", CoreToolNames: []string{"Bash"}}
	res, err := WriteConfig(context.Background(), &fakeHost{shell: true}, false, db.Agent{}, bare, "auto")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Cleanup()
	dis := toSet(res.Disallowed)
	for _, keep := range []string{"TodoWrite", "TaskCreate", "Skill"} {
		if dis[keep] {
			t.Errorf("%q must survive when its bridge is not advertised; got %v", keep, res.Disallowed)
		}
	}
	if !dis["EnterPlanMode"] || !dis["ExitPlanMode"] {
		t.Error("auto mode must suppress plan-mode tools")
	}
	if !dis["AskUserQuestion"] {
		t.Error("AskUserQuestion is always suppressed with a bridge present")
	}
}

// TestWriteConfigExternalServersAndGate covers the external-server path: gated
// servers are withheld, transports render to the right entry shape, and a
// malformed gate fails closed with a debug event.
func TestWriteConfigExternalServersAndGate(t *testing.T) {
	h := &fakeHost{
		servers: []mcp.ServerConfig{
			{Name: "codebase-memory-mcp", Transport: db.MCPTransportStdio, Command: "cbm.exe", Args: []string{"serve"}, Env: map[string]string{"X": "1"}},
			{Name: "remote", Transport: db.MCPTransportHTTP, URL: "http://h/mcp", Headers: map[string]string{"A": "b"}},
			{Name: "blocked", Transport: db.MCPTransportStdio, Command: "no.exe"},
		},
		gate: func(key string) bool { return key != "blocked" },
	}
	res, err := WriteConfig(context.Background(), h, true, db.Agent{ID: "A"}, tools.InteractionEndpoint{}, "auto")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Cleanup()
	cfg := readConfig(t, res.Path)
	if _, ok := cfg.MCPServers["blocked"]; ok {
		t.Error("gated server must not be mounted")
	}
	if s := cfg.MCPServers["codebase-memory-mcp"]; s.Command != "cbm.exe" || len(s.Args) != 1 || s.Env["X"] != "1" {
		t.Errorf("stdio entry = %+v", s)
	}
	if s := cfg.MCPServers["remote"]; s.Type != db.MCPTransportHTTP || s.URL != "http://h/mcp" || s.Headers["A"] != "b" {
		t.Errorf("http entry = %+v", s)
	}
	if got := toSet(res.Allowed); !got["mcp__codebase-memory-mcp"] || !got["mcp__remote"] || got["mcp__blocked"] {
		t.Errorf("allowlist = %v", res.Allowed)
	}

	h.gateErr = errors.New("bad allowlist json")
	if _, err := WriteConfig(context.Background(), h, true, db.Agent{ID: "A"}, tools.InteractionEndpoint{}, "auto"); err == nil {
		t.Fatal("malformed gate must fail closed")
	}
	if len(h.debug) != 1 || h.debug[0].Name != "mcp_server_gate_malformed" {
		t.Fatalf("expected one gate debug event, got %+v", h.debug)
	}
}

// TestWriteSettingsHooksAndEffort pins the settings file: deny list, hook
// passthrough only when enabled (matcher/command translated), effort clamped
// from max to xhigh, and nothing written when there is nothing to say.
func TestWriteSettingsHooksAndEffort(t *testing.T) {
	h := &fakeHost{cliHooks: true, hooks: map[string][]db.Hook{
		db.HookPreToolUse: {{Type: "command", Matcher: "Bash,PowerShell", Command: "sqz hook claude", TimeoutSec: 30}},
	}}
	path, cleanup, err := WriteSettings(context.Background(), h, []string{"Task"}, "max")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	data, _ := os.ReadFile(path)
	var set Settings
	if err := json.Unmarshal(data, &set); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, data)
	}
	if set.EffortLevel != "xhigh" {
		t.Errorf("effort = %q, want xhigh (max clamped)", set.EffortLevel)
	}
	if set.Permissions == nil || len(set.Permissions.Deny) != 1 {
		t.Errorf("permissions = %+v", set.Permissions)
	}
	rules := set.Hooks[db.HookPreToolUse]
	if len(rules) != 1 || rules[0].Matcher != MatcherRegex("Bash,PowerShell") || rules[0].Hooks[0].Command != HookCommand("sqz hook claude") {
		t.Errorf("hook rules = %+v", rules)
	}

	h.cliHooks = false
	path, cleanup, err = WriteSettings(context.Background(), h, nil, "high")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "hooks") {
		t.Errorf("hooks must be omitted when passthrough is off:\n%s", data)
	}

	if p, _, err := WriteSettings(context.Background(), h, nil, ""); err != nil || p != "" {
		t.Fatalf("nothing to say → no file; got %q, %v", p, err)
	}
}

func TestEffortLevel(t *testing.T) {
	cases := map[string]string{"": "high", "off": "high", "high": "high", "low": "low", "medium": "medium",
		"xhigh": "xhigh", "max": "max", "ultra": "ultra", "bogus": "bogus"}
	for in, want := range cases {
		if got := EffortLevel(in); got != want {
			t.Errorf("EffortLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPromptToolForMode(t *testing.T) {
	if got := PromptToolForMode("ask", bridgedInter); got != PermissionPromptToolID {
		t.Errorf("ask = %q", got)
	}
	if got := PromptToolForMode("read-only", bridgedInter); got != PermissionPromptToolID {
		t.Errorf("read-only = %q", got)
	}
	if got := PromptToolForMode("auto", bridgedInter); got != "" {
		t.Errorf("auto = %q", got)
	}
	if got := PromptToolForMode("ask", tools.InteractionEndpoint{}); got != "" {
		t.Errorf("ask without endpoint = %q", got)
	}
}
