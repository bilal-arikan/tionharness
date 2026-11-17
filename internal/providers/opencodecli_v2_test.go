package providers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Fixture lines captured live from `opencode run --format json` on 2.2.5
// (metadata trimmed). A tool part arrives once, completed; "id" is the call id.
const (
	oc2StepStart = `{"type":"step_start","timestamp":1,"sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","part":{"id":"prt_1eb45b5eb001nt024UGBER9ZiZ","sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","messageID":"msg_1","type":"step-start"}}`
	oc2Shell     = `{"type":"tool_use","timestamp":2,"sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","part":{"partID":"prt_1eb45babc001QQc3Gsc3m7IDWc","sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","messageID":"msg_1","type":"tool","id":"chatcmpl-tool-b2cd2a11ea44bebc","tool":"shell","state":{"status":"completed","input":{"command":"echo hello-from-shell"},"output":"hello-from-shell\n","title":"shell","metadata":{"metadata":{"status":"completed","truncated":false,"exit":0},"content":[{"type":"text","text":"hello-from-shell\n"}]},"time":{"start":1,"end":2}}}}`
	oc2StepTools = `{"type":"step_finish","timestamp":3,"sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","part":{"id":"prt_1eb45bc1f001azvjFoMcx7XiV0","sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","messageID":"msg_1","type":"step-finish","reason":"tool-calls","cost":0,"tokens":{"input":8343,"output":114,"reasoning":190,"cache":{"read":1792,"write":0}}}}`
	oc2Execute   = `{"type":"tool_use","timestamp":4,"sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","part":{"partID":"prt_1eb48a161001kVJM0wOuoJH5cX","sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","messageID":"msg_2","type":"tool","id":"chatcmpl-tool-8dd8e68b7c9e4f94","tool":"execute","state":{"status":"completed","input":{"code":"return await tools.tionharness_interaction.ping_tool({ msg: \"hi\" });"},"output":"pong:hi","title":"execute","metadata":{"metadata":{"toolCalls":[{"tool":"tionharness_interaction.ping_tool","status":"completed","input":{"msg":"hi"}}],"truncated":false},"content":[{"type":"text","text":"pong:hi"}]},"time":{"start":1,"end":2}}}}`
	oc2ExecInner = `{"type":"tool_use","timestamp":5,"sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","part":{"partID":"prt_x","type":"tool","id":"chatcmpl-tool-823ff0e2f764f106","tool":"execute","state":{"status":"completed","input":{"code":"return await tools.opencode.list_mcp_resources({});"},"output":"{}","title":"execute","metadata":{"metadata":{"toolCalls":[{"tool":"opencode.list_mcp_resources","status":"completed"}],"truncated":false}}}}}`
	oc2Text      = `{"type":"text","timestamp":6,"sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","part":{"id":"prt_1eb45bc260014C7MTDnEP98ie2_text-0","sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","messageID":"msg_3","type":"text","text":"Output: hello-from-shell","time":{"start":1,"end":2}}}`
	oc2StepStop  = `{"type":"step_finish","timestamp":7,"sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","part":{"id":"prt_1eb45bc260014C7MTDnEP98ie2_step-finish","sessionID":"ses_e14ba5521ffeDZDGu4FX2RFzSa","messageID":"msg_3","type":"step-finish","reason":"stop","cost":0,"tokens":{"input":467,"output":9,"reasoning":0,"cache":{"read":9984,"write":0}}}}`
	oc2Rejected  = `{"type":"tool_use","timestamp":8,"sessionID":"ses_e14b7e1ebffe2pvHehN7lt2SY4","part":{"partID":"prt_r","type":"tool","id":"chatcmpl-tool-83f70530d264a1de","tool":"shell","state":{"status":"error","input":{"command":"echo asked-ok"},"error":"This non-interactive run cannot ask the user for permission, so the request was rejected. Continue without this action.","metadata":{"providerCall":{"executed":false}}}}}`
)

func TestOpenCodeParserV2Turn(t *testing.T) {
	var live []TraceStep
	p := newOpenCodeParser("opencode/big-pickle", []string{"tionharness_interaction"}, func(st TraceStep) { live = append(live, st) })
	p.silentTools = true
	for _, l := range []string{oc2StepStart, oc2Shell, oc2Execute, oc2ExecInner, oc2StepTools, oc2StepStart, oc2Text, oc2StepStop} {
		p.feed(l)
	}
	resp, err := p.finish()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "Output: hello-from-shell" || resp.SessionID != "ses_e14ba5521ffeDZDGu4FX2RFzSa" {
		t.Fatalf("text/session: %q %q", resp.Text, resp.SessionID)
	}
	if resp.ProviderCalls != 2 || resp.Usage.InputTokens != 8343+467 || resp.Usage.CacheReadTokens != 1792+9984 {
		t.Fatalf("usage: %+v calls=%d", resp.Usage, resp.ProviderCalls)
	}
	if len(resp.Trace) != 3 || len(live) != 3 {
		t.Fatalf("trace: %+v (live %d)", resp.Trace, len(live))
	}
	if st := resp.Trace[0]; st.Tool != "shell" || st.Output != "hello-from-shell\n" || st.IsError {
		t.Fatalf("shell step: %+v", st)
	}
	// A single Code Mode call into a bridged server renders as that tool.
	if st := resp.Trace[1]; st.Tool != "mcp__tionharness_interaction__ping_tool" || string(st.Input) != `{"msg":"hi"}` || st.Output != "pong:hi" {
		t.Fatalf("code mode unwrap: %+v", st)
	}
	// opencode's own Code Mode namespace is not a bridged server: stays execute.
	if st := resp.Trace[2]; st.Tool != "execute" {
		t.Fatalf("opencode-internal execute: %+v", st)
	}
}

func TestOpenCodeParserV2RejectedTool(t *testing.T) {
	p := newOpenCodeParser("m", nil, nil)
	p.silentTools = true
	p.feed(oc2StepStart)
	p.feed(oc2Rejected)
	if len(p.resp.Trace) != 1 || !p.resp.Trace[0].IsError || !strings.Contains(p.resp.Trace[0].Output, "cannot ask the user") {
		t.Fatalf("rejected tool: %+v", p.resp.Trace)
	}
}

func TestOpenCodeParserV2CodeModeEdgeCases(t *testing.T) {
	p := newOpenCodeParser("m", []string{"my.srv"}, nil)
	// Two calls in one execute stay a single execute step.
	p.feed(`{"type":"tool_use","part":{"type":"tool","id":"c1","tool":"execute","state":{"status":"completed","input":{"code":"x"},"output":"o","metadata":{"metadata":{"toolCalls":[{"tool":"my_srv.a","status":"completed"},{"tool":"my_srv.b","status":"completed"}]}}}}}`)
	// A server key with characters opencode sanitises still maps back, and an
	// inner failure marks the step as an error.
	p.feed(`{"type":"tool_use","part":{"type":"tool","id":"c2","tool":"execute","state":{"status":"completed","input":{"code":"y"},"output":"bad","metadata":{"metadata":{"toolCalls":[{"tool":"my_srv.run","status":"error","input":{"k":1}}]}}}}}`)
	// An unexpected metadata shape must not drop the line.
	p.feed(`{"type":"tool_use","part":{"type":"tool","id":"c3","tool":"execute","state":{"status":"completed","input":{"code":"z"},"output":"ok","metadata":"weird"}}}`)
	// Direct (non Code Mode) MCP tool ids use the sanitised server key too.
	p.feed(`{"type":"tool_use","part":{"type":"tool","id":"c4","tool":"my_srv_direct","state":{"status":"completed","input":{},"output":"ok"}}}`)
	tr := p.resp.Trace
	if len(tr) != 4 || tr[0].Tool != "execute" || tr[2].Tool != "execute" || p.parseDropCount != 0 {
		t.Fatalf("trace: %+v drops=%d", tr, p.parseDropCount)
	}
	if tr[1].Tool != "mcp__my_srv__run" || !tr[1].IsError || string(tr[1].Input) != `{"k":1}` {
		t.Fatalf("sanitised key unwrap: %+v", tr[1])
	}
	if tr[3].Tool != "mcp__my_srv__direct" {
		t.Fatalf("direct mcp naming: %+v", tr[3])
	}
}

// TestOpenCodeParserV2OpenStepIsBusy: 2.x never streams a running tool, so the
// stdout watchdog must read an open step as busy (a long tool), and only a
// closed one as idle. A 1.x stream keeps the per-tool signal.
func TestOpenCodeParserV2OpenStepIsBusy(t *testing.T) {
	p := newOpenCodeParser("m", nil, nil)
	p.silentTools = true
	if p.toolInFlight() {
		t.Fatal("nothing started yet")
	}
	p.feed(oc2StepStart)
	if !p.toolInFlight() {
		t.Fatal("an open 2.x step must count as in flight")
	}
	p.feed(oc2Shell)
	if !p.toolInFlight() {
		t.Fatal("the step is still open after a tool completed")
	}
	p.feed(oc2StepTools)
	if p.toolInFlight() {
		t.Fatal("a finished step is not in flight")
	}

	v1 := newOpenCodeParser("m", nil, nil)
	v1.feed(ocStepStart)
	if v1.toolInFlight() {
		t.Fatal("1.x: an open step alone is not a running tool")
	}
}

func TestOpenCodeBuildArgsV2(t *testing.T) {
	c := NewOpenCodeCLI("/bin/opencode", "", t.TempDir())
	args := c.buildArgs(Request{WorkDir: t.TempDir(), ResumeSessionID: "ses_Abc9", CLIResumeScope: "scope"}, "anthropic/claude-sonnet-5-5", 2)
	want := []string{"run", "--format", "json", "--agent", "tionharness", "--standalone", "-m", "anthropic/claude-sonnet-5-5", "-s", "ses_Abc9"}
	if !slices.Equal(args, want) {
		t.Fatalf("args: %v", args)
	}
	c.effortVariant = true
	args = c.buildArgs(Request{CLIEffortLevel: "max"}, "openai/gpt-5.5", 2)
	if !slices.Contains(args, "openai/gpt-5.5#xhigh") || slices.Contains(args, "--variant") {
		t.Fatalf("2.x variant rides the model id: %v", args)
	}
	if args := c.buildArgs(Request{CLIEffortLevel: "high"}, "opencode/big-pickle", 2); !slices.Contains(args, "opencode/big-pickle") {
		t.Fatalf("unknown vendor keeps the bare model: %v", args)
	}
	if args := c.buildArgs(Request{CLIEffortLevel: "high"}, "", 2); slices.Contains(args, "-m") {
		t.Fatalf("empty model must not pass -m: %v", args)
	}
}

// opencodeV2Effect evaluates rules the way opencode 2.x does: the last rule
// whose action matches (exactly or "*") wins.
func opencodeV2Effect(rules []opencodeV2Rule, action string) string {
	effect := ""
	for _, r := range rules {
		if r.Action == "*" || r.Action == action {
			effect = r.Effect
		}
	}
	return effect
}

func renderV2(t *testing.T, cfg opencodeConfig) opencodeV2Doc {
	t.Helper()
	raw, err := renderOpenCodeConfig(cfg, "/x/system.md")
	if err != nil {
		t.Fatal(err)
	}
	var doc opencodeV2Doc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("rendered config is not JSON: %v\n%s", err, raw)
	}
	// The legacy keys must not be emitted: 2.x would migrate them ahead of the
	// rules and they would be overridden.
	var generic map[string]json.RawMessage
	_ = json.Unmarshal(raw, &generic)
	for _, legacy := range []string{"tools", "permission", "agent", "autoupdate"} {
		if _, ok := generic[legacy]; ok {
			t.Fatalf("legacy key %q in a 2.x document:\n%s", legacy, raw)
		}
	}
	return doc
}

func TestOpenCodeConfigRenderV2(t *testing.T) {
	c := NewOpenCodeCLI("/bin/opencode", "", t.TempDir())
	c.ConfigureCLIMCP(CLIMCPSpec{Servers: map[string]CLIMCPServer{
		"tionharness_interaction": {Transport: "http", URL: "http://127.0.0.1:1/core?full=1", Headers: map[string]string{"Authorization": "Bearer t"}, DisabledTools: []string{"noisy"}},
		"local":                   {Command: "srv", Args: []string{"--x"}, Env: map[string]string{"K": "V"}},
	}})

	// Default turn: everything allowed except the native subagent launcher.
	doc := renderV2(t, c.buildConfig(Request{System: "persona", NativeWebSearch: true}, 2))
	for action, want := range map[string]string{"shell": "allow", "edit": "allow", "read": "allow", "webfetch": "allow", "execute": "allow", "subagent": "deny", "tionharness_interaction_noisy": "deny", "tionharness_interaction_other": "allow"} {
		if got := opencodeV2Effect(doc.Permissions, action); got != want {
			t.Fatalf("default %s: got %q want %q (%+v)", action, got, want, doc.Permissions)
		}
	}
	agent := doc.Agents[opencodeAgentName]
	if agent.Mode != "primary" || agent.System != "{file:/x/system.md}" || !doc.Agents["general"].Disabled {
		t.Fatalf("agents: %+v", doc.Agents)
	}
	if doc.Update != "disable" || doc.Share != "disabled" {
		t.Fatalf("update/share: %+v", doc)
	}
	remote, local := doc.MCP.Servers["tionharness_interaction"], doc.MCP.Servers["local"]
	if remote.Type != "remote" || remote.URL == "" || remote.Headers["Authorization"] != "Bearer t" || remote.Disabled {
		t.Fatalf("remote server: %+v", remote)
	}
	if local.Type != "local" || !slices.Equal(local.Command, []string{"srv", "--x"}) || local.Environment["K"] != "V" || local.Disabled {
		t.Fatalf("local server: %+v", local)
	}

	// Read-only + bridged web only.
	doc = renderV2(t, c.buildConfig(Request{PermissionMode: "read-only", NativeWebSearch: false}, 2))
	for action, want := range map[string]string{"shell": "deny", "edit": "deny", "webfetch": "deny", "websearch": "deny", "read": "allow", "subagent": "deny"} {
		if got := opencodeV2Effect(doc.Permissions, action); got != want {
			t.Fatalf("read-only %s: got %q want %q", action, got, want)
		}
	}
	// Ask: natives that mutate ask (and `run` auto-rejects them).
	doc = renderV2(t, c.buildConfig(Request{PermissionMode: "ask", NativeWebSearch: true}, 2))
	if opencodeV2Effect(doc.Permissions, "shell") != "ask" || opencodeV2Effect(doc.Permissions, "edit") != "ask" || opencodeV2Effect(doc.Permissions, "subagent") != "deny" {
		t.Fatalf("ask mode: %+v", doc.Permissions)
	}
	// Auxiliary call: an empty menu, including tools a later release adds.
	doc = renderV2(t, c.buildConfig(Request{CLIRestrictNativeTools: true, NativeWebSearch: true}, 2))
	for _, action := range []string{"shell", "read", "subagent", "execute", "some_future_tool"} {
		if got := opencodeV2Effect(doc.Permissions, action); got != "deny" {
			t.Fatalf("aux %s: got %q", action, got)
		}
	}
}

func TestOpenCodeWriteConfigV2(t *testing.T) {
	dir := t.TempDir()
	path, err := writeOpenCodeConfig(dir, opencodeConfig{SystemPrompt: "persona", APIMajor: 2})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"permissions"`) || !strings.Contains(string(raw), filepath.ToSlash(filepath.Join(dir, opencodeSystemFile))) {
		t.Fatalf("written 2.x config: %s", raw)
	}
}

// TestOpenCodeParserV2ErrorShape: 2.x error events carry type/message/status
// instead of name/data.message (captured live on 2.2.5, body trimmed).
func TestOpenCodeParserV2ErrorShape(t *testing.T) {
	p := newOpenCodeParser("m", nil, nil)
	p.silentTools = true
	p.feed(`{"type":"error","timestamp":1,"sessionID":"ses_e14a7ea12ffe9VSbzf3dUlUy86","error":{"type":"provider.free-limit","message":"Free usage limit reached. Connect a provider or subscribe to OpenCode Go to keep going, or switch models.","status":429,"retryAfterMs":10218000}}`)
	if !strings.HasPrefix(p.errText, "Free usage limit reached") {
		t.Fatalf("error text: %q", p.errText)
	}
	if cls := classifyOpenCodeError(p.errName, p.errText); cls != opencodeFailureQuota {
		t.Fatalf("free-limit must classify as quota, got %v (name %q)", cls, p.errName)
	}
	if _, err := p.finish(); err == nil || !strings.Contains(err.Error(), "Free usage limit") {
		t.Fatalf("finish: %v", err)
	}
}

func TestParseOpenCodeMajor(t *testing.T) {
	for out, want := range map[string]int{"opencode v2.2.5\n": 2, "1.19.0\n": 1, "v10.0.1": 10, "garbage": 0} {
		got, ok := parseOpenCodeMajor(out)
		if got != want || ok != (want != 0) {
			t.Fatalf("%q: got %d %v", out, got, ok)
		}
	}
}

func TestOpenCodeDesktopCLIsNewestFirst(t *testing.T) {
	root := t.TempDir()
	for _, v := range []string{"2.2.5-x64", "2.10.0-x64", "1.19.0-x64"} {
		if err := os.MkdirAll(filepath.Join(root, v), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(root, "stray.txt"), nil, 0o600)
	got := opencodeDesktopCLIs(root, "opencode-cli.exe")
	want := []string{
		filepath.Join(root, "2.10.0-x64", "opencode-cli.exe"),
		filepath.Join(root, "2.2.5-x64", "opencode-cli.exe"),
		filepath.Join(root, "1.19.0-x64", "opencode-cli.exe"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
	if opencodeDesktopCLIs(filepath.Join(root, "missing"), "x") != nil {
		t.Fatal("missing root must yield nothing")
	}
}

func TestOpenCodeCandidatePathsIncludeDesktopCLI(t *testing.T) {
	appData := t.TempDir()
	exe := filepath.Join(appData, opencodeDesktopID, "cli", "2.2.5-x64", "opencode-cli.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDATA", appData)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	got := opencodeCandidatePaths("windows")
	if len(got) == 0 || got[len(got)-1] != exe {
		t.Fatalf("desktop CLI must be the last-resort candidate: %v", got)
	}
}
