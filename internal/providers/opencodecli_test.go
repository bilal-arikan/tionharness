package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Fixture lines captured live from `opencode run --format json` on 1.19.0.
const (
	ocStepStart  = `{"type":"step_start","timestamp":1,"sessionID":"ses_abc123","part":{"id":"prt_1","messageID":"msg_1","sessionID":"ses_abc123","type":"step-start"}}`
	ocToolRead   = `{"type":"tool_use","timestamp":2,"sessionID":"ses_abc123","part":{"type":"tool","tool":"read","callID":"call-1","state":{"status":"completed","input":{"filePath":"/tmp/note.txt"},"output":"hello from probe file","metadata":{}},"id":"prt_2","sessionID":"ses_abc123","messageID":"msg_1"}}`
	ocStepTools  = `{"type":"step_finish","timestamp":3,"sessionID":"ses_abc123","part":{"id":"prt_3","reason":"tool-calls","messageID":"msg_1","sessionID":"ses_abc123","type":"step-finish","tokens":{"total":6805,"input":6554,"output":68,"reasoning":80,"cache":{"write":0,"read":103}},"cost":0}}`
	ocText       = `{"type":"text","timestamp":4,"sessionID":"ses_abc123","part":{"id":"prt_4","messageID":"msg_2","sessionID":"ses_abc123","type":"text","text":"hello from probe file","time":{"start":1,"end":2}}}`
	ocStepStop   = `{"type":"step_finish","timestamp":5,"sessionID":"ses_abc123","part":{"id":"prt_5","reason":"stop","messageID":"msg_2","sessionID":"ses_abc123","type":"step-finish","tokens":{"total":6976,"input":152,"output":11,"reasoning":154,"cache":{"write":0,"read":6659}},"cost":0}}`
	ocToolDenied = `{"type":"tool_use","timestamp":6,"sessionID":"ses_abc123","part":{"type":"tool","tool":"write","callID":"call-2","state":{"status":"error","input":{"filePath":"/tmp/x"},"error":"The user rejected permission to use this specific tool call."},"id":"prt_6","sessionID":"ses_abc123","messageID":"msg_1"}}`
	ocMCPTool    = `{"type":"tool_use","timestamp":7,"sessionID":"ses_abc123","part":{"type":"tool","tool":"tionharness_interaction_todo_write","callID":"call-3","state":{"status":"completed","input":{"todos":[{"content":"a","status":"completed"}]},"output":"ok"},"id":"prt_7","sessionID":"ses_abc123","messageID":"msg_1"}}`
	ocError      = `{"type":"error","timestamp":8,"sessionID":"ses_abc123","error":{"name":"UnknownError","data":{"message":"Unexpected server error.","ref":"err_1"}}}`
)

func TestOpenCodeParserHappyTurn(t *testing.T) {
	var live []TraceStep
	p := newOpenCodeParser("opencode/big-pickle", []string{"tionharness_interaction"}, func(st TraceStep) { live = append(live, st) })
	for _, l := range []string{ocStepStart, ocToolRead, ocStepTools, ocMCPTool, ocStepStart, ocText, ocStepStop} {
		p.feed(l)
	}
	resp, err := p.finish()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "hello from probe file" || resp.SessionID != "ses_abc123" {
		t.Fatalf("text/session: %q %q", resp.Text, resp.SessionID)
	}
	if resp.Usage.InputTokens != 6554+152 || resp.Usage.CacheReadTokens != 103+6659 || resp.Usage.OutputTokens != 79 || resp.Usage.ThinkingTokens != 234 || !resp.Usage.ThinkingTokensMeasured {
		t.Fatalf("usage: %+v", resp.Usage)
	}
	if resp.ProviderCalls != 2 || resp.FirstCallPromptTokens != 6554+103 {
		t.Fatalf("calls/first prompt: %d %d", resp.ProviderCalls, resp.FirstCallPromptTokens)
	}
	if len(resp.Trace) != 2 || resp.Trace[0].Tool != "read" || resp.Trace[0].Output != "hello from probe file" {
		t.Fatalf("trace: %+v", resp.Trace)
	}
	if resp.Trace[1].Tool != "mcp__tionharness_interaction__todo_write" {
		t.Fatalf("mcp tool naming: %q", resp.Trace[1].Tool)
	}
	if len(live) != 2 || !p.ranTool() {
		t.Fatalf("live emits: %d", len(live))
	}
}

func TestOpenCodeParserDeniedToolAndError(t *testing.T) {
	p := newOpenCodeParser("m", nil, nil)
	p.feed(ocStepStart)
	p.feed(ocToolDenied)
	if len(p.resp.Trace) != 1 || !p.resp.Trace[0].IsError || !strings.Contains(p.resp.Trace[0].Output, "rejected") {
		t.Fatalf("denied tool: %+v", p.resp.Trace)
	}
	p.feed(ocError)
	if _, err := p.finish(); err == nil || !strings.Contains(err.Error(), "Unexpected server error") {
		t.Fatalf("error not surfaced: %v", err)
	}
	if p.salvage() != nil {
		t.Fatal("salvage must not mask a reported error")
	}
}

func TestOpenCodeParserTruncatedStreamSalvages(t *testing.T) {
	p := newOpenCodeParser("m", nil, nil)
	p.feed(ocStepStart)
	p.feed(ocText)
	if _, err := p.finish(); err == nil {
		t.Fatal("missing final step_finish must be an error")
	}
	if got := p.salvage(); got == nil || got.Text != "hello from probe file" {
		t.Fatalf("salvage: %+v", got)
	}
	if p.ranTool() {
		t.Fatal("no tool ran")
	}
}

func TestOpenCodeParserTodoWriteMapsToChecklist(t *testing.T) {
	p := newOpenCodeParser("m", nil, nil)
	p.feed(`{"type":"tool_use","sessionID":"ses_x","part":{"type":"tool","tool":"todowrite","callID":"c","state":{"status":"completed","input":{"todos":[{"content":"one","status":"completed"},{"content":"two","status":"pending"}]},"output":"ok"}}}`)
	st := p.resp.Trace[0]
	if st.Tool != "todo_list" || st.Output != "[x] one\n[ ] two" {
		t.Fatalf("todo mapping: %+v", st)
	}
}

func TestOpenCodeParserParseDropIsNoted(t *testing.T) {
	p := newOpenCodeParser("m", nil, nil)
	p.feed(`{"type":"text","part":`)
	p.feed(`{broken`)
	p.feed(ocStepStart)
	p.feed(ocText)
	p.feed(ocStepStop)
	resp, err := p.finish()
	if err != nil {
		t.Fatal(err)
	}
	var notes int
	for _, st := range resp.Trace {
		if strings.Contains(st.Text, "[opencode parse drop]") {
			notes++
		}
	}
	if notes != 2 {
		t.Fatalf("want one detailed note + one summary, got %d: %+v", notes, resp.Trace)
	}
}

func TestOpenCodeBuildArgs(t *testing.T) {
	c := NewOpenCodeCLI("/bin/opencode", "", t.TempDir())
	args := c.buildArgs(Request{WorkDir: t.TempDir(), ResumeSessionID: "ses_Abc9", CLIResumeScope: "scope"}, "anthropic/claude-sonnet-5-5")
	want := []string{"run", "--format", "json", "--agent", "tionharness", "-m", "anthropic/claude-sonnet-5-5"}
	if !slices.Equal(args[:len(want)], want) || args[len(args)-2] != "-s" || args[len(args)-1] != "ses_Abc9" || !slices.Contains(args, "--dir") {
		t.Fatalf("args: %v", args)
	}
	// An unsafe session id or a scope-less request never reaches the command line.
	if args := c.buildArgs(Request{ResumeSessionID: "ses_x; rm", CLIResumeScope: "scope"}, ""); slices.Contains(args, "-s") {
		t.Fatalf("unsafe id passed: %v", args)
	}
	if args := c.buildArgs(Request{ResumeSessionID: "ses_ok"}, ""); slices.Contains(args, "-s") || slices.Contains(args, "-m") {
		t.Fatalf("scope-less resume or empty model passed: %v", args)
	}
	// --variant only when the instance opted in AND the vendor vocabulary is known.
	if args := c.buildArgs(Request{CLIEffortLevel: "max"}, "openai/gpt-5.5"); slices.Contains(args, "--variant") {
		t.Fatalf("variant without opt-in: %v", args)
	}
	c.effortVariant = true
	if args := c.buildArgs(Request{CLIEffortLevel: "max"}, "openai/gpt-5.5"); !slices.Contains(args, "xhigh") {
		t.Fatalf("openai max→xhigh: %v", args)
	}
	if args := c.buildArgs(Request{CLIEffortLevel: "ultra"}, "anthropic/claude-opus-5-5"); !slices.Contains(args, "max") {
		t.Fatalf("anthropic ultra→max: %v", args)
	}
	if args := c.buildArgs(Request{CLIEffortLevel: "high"}, "opencode/big-pickle"); slices.Contains(args, "--variant") {
		t.Fatalf("unknown vendor must not get a variant: %v", args)
	}
	if args := c.buildArgs(Request{CLIEffortLevel: "high", DisableThinking: true}, "openai/gpt-5.5"); slices.Contains(args, "--variant") {
		t.Fatalf("thinking off must not get a variant: %v", args)
	}
}

func TestOpenCodeConfigRender(t *testing.T) {
	c := NewOpenCodeCLI("/bin/opencode", "", t.TempDir())
	c.ConfigureCLIMCP(CLIMCPSpec{DisableNativeShell: true, Servers: map[string]CLIMCPServer{
		"tionharness_interaction": {Transport: "http", URL: "http://127.0.0.1:1/core?full=1", Headers: map[string]string{"Authorization": "Bearer t"}, DisabledTools: []string{"noisy"}},
		"local":                   {Command: "srv", Args: []string{"--x"}, Env: map[string]string{"K": "V"}},
	}})
	cfg := c.buildConfig(Request{System: "persona", PermissionMode: "read-only", NativeWebSearch: false})
	dir := t.TempDir()
	path, err := writeOpenCodeConfig(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var doc opencodeConfigDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("rendered config is not JSON: %v\n%s", err, data)
	}
	if doc.Permission["edit"] != "deny" || doc.Permission["bash"] != "deny" || doc.Permission["*"] != "allow" {
		t.Fatalf("read-only permission: %+v", doc.Permission)
	}
	for _, tool := range []string{"task", "bash", "edit", "write", "patch", "websearch", "webfetch", "tionharness_interaction_noisy"} {
		if v, ok := doc.Tools[tool]; !ok || v {
			t.Fatalf("tool %q should be off: %+v", tool, doc.Tools)
		}
	}
	if _, off := doc.Tools["read"]; off {
		t.Fatalf("read must stay on in read-only mode: %+v", doc.Tools)
	}
	agent := doc.Agent[opencodeAgentName]
	if agent.Mode != "primary" || !strings.HasPrefix(agent.Prompt, "{file:") || !strings.HasSuffix(agent.Prompt, opencodeSystemFile+"}") {
		t.Fatalf("agent: %+v", agent)
	}
	if !doc.Agent["general"].Disable {
		t.Fatal("built-in general subagent must be disabled")
	}
	sys, _ := os.ReadFile(filepath.Join(dir, opencodeSystemFile))
	if !strings.HasPrefix(string(sys), "persona") || !strings.Contains(string(sys), "todo_write") {
		t.Fatalf("system prompt: %q", sys)
	}
	remote := doc.MCP["tionharness_interaction"]
	if remote.Type != "remote" || remote.URL == "" || remote.Headers["Authorization"] != "Bearer t" || !remote.Enabled {
		t.Fatalf("remote server: %+v", remote)
	}
	local := doc.MCP["local"]
	if local.Type != "local" || !slices.Equal(local.Command, []string{"srv", "--x"}) || local.Environment["K"] != "V" {
		t.Fatalf("local server: %+v", local)
	}
	if doc.Share != "disabled" || doc.Autoupdate {
		t.Fatalf("share/autoupdate: %+v", doc)
	}

	// Auxiliary call: every native tool off, no prompt file when System is empty.
	aux := c.buildConfig(Request{CLIRestrictNativeTools: true, NativeWebSearch: true})
	if !aux.DisableAllNativeTools {
		t.Fatal("aux must disable all natives")
	}
	raw, _ := renderOpenCodeConfig(aux, "/x/system.md")
	var auxDoc opencodeConfigDoc
	_ = json.Unmarshal(raw, &auxDoc)
	for _, tool := range opencodeNativeTools {
		if v, ok := auxDoc.Tools[tool]; !ok || v {
			t.Fatalf("aux tool %q should be off", tool)
		}
	}
	if auxDoc.Agent[opencodeAgentName].Prompt == "" {
		// interactionSystemNote is always appended, so a prompt file exists even
		// for an empty persona.
		t.Fatal("prompt expected")
	}
	if doc.Permission = opencodePermission("ask"); doc.Permission["edit"] != "ask" {
		t.Fatalf("ask mapping: %+v", doc.Permission)
	}
}

func TestOpenCodeScopeRecord(t *testing.T) {
	c := NewOpenCodeCLI("/bin/opencode", "", t.TempDir())
	if !c.ResumeScopeReady("scope-a") {
		t.Fatal("scope should be ready with a home")
	}
	if c.CanResumeScoped("scope-a", "ses_1") {
		t.Fatal("nothing recorded yet")
	}
	c.recordScopeSession("scope-a", "ses_1")
	if !c.CanResumeScoped("scope-a", "ses_1") || c.CanResumeScoped("scope-a", "ses_2") || c.CanResumeScoped("scope-b", "ses_1") {
		t.Fatal("scope record mismatch")
	}
	if c.CanResumeScoped("scope-a", "not-a-session") {
		t.Fatal("unsafe id accepted")
	}
	if NewOpenCodeCLI("/bin/opencode", "", "").ResumeScopeReady("scope-a") {
		t.Fatal("no home means no scoped resume")
	}
}

func TestBareModelForKind(t *testing.T) {
	cases := map[string]string{"anthropic/claude-sonnet-5-5": "claude-sonnet-5-5", "openrouter/anthropic/claude-opus-5-5": "claude-opus-5-5", "plain": "plain", "": ""}
	for in, want := range cases {
		if got := BareModelForKind("opencode-cli", in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	if got := BareModelForKind("codex-cli", "a/b"); got != "a/b" {
		t.Errorf("other kinds untouched: %q", got)
	}
	if ContextWindowFor("opencode-cli", "anthropic/claude-sonnet-5-5") != ContextWindowFor("anthropic", "claude-sonnet-5-5") {
		t.Error("context window must resolve through the bare model")
	}
	if p, ok := EstimateFor("opencode-cli", "anthropic/claude-sonnet-5-5"); !ok || p.InputPerMTok == 0 {
		t.Errorf("estimate via anthropic table: %+v %v", p, ok)
	}
	if _, ok := EstimateFor("opencode-cli", "opencode/big-pickle"); ok {
		t.Error("free zen model has no list price")
	}
}

// --- subprocess fixture: the test binary stands in for `opencode` ---

func TestOpenCodeCleanCrashRetriesOnce(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	counter := filepath.Join(home, "attempts")
	t.Setenv("OPENCODE_TEST_FIXTURE", "crash-then-ok")
	t.Setenv("OPENCODE_TEST_COUNTER", counter)
	c := &OpenCodeCLI{binPath: self, configDir: home}
	var live []TraceStep
	resp, err := c.completeWithArgs(context.Background(), []string{"-test.run=TestOpenCodeHelperFixture", "-test.v=false"}, "prompt", "m", Request{OnEvent: func(st TraceStep) { live = append(live, st) }, CLIResumeScope: "scope"})
	if err != nil {
		t.Fatalf("second attempt should succeed: %v", err)
	}
	if resp.Text != "hello from probe file" || resp.SessionID != "ses_abc123" {
		t.Fatalf("resp: %+v", resp)
	}
	data, _ := os.ReadFile(counter)
	if string(data) != "xx" {
		t.Fatalf("want exactly two attempts, got %q", data)
	}
	if !c.CanResumeScoped("scope", "ses_abc123") {
		t.Fatal("successful turn must record its session for the scope")
	}
	if len(live) != 1 || live[0].Tool != "read" {
		t.Fatalf("live steps: %+v", live)
	}
	// The turn dir is cleaned up; the home keeps data/config/scopes only.
	entries, _ := os.ReadDir(filepath.Join(home, "turns"))
	if len(entries) != 0 {
		t.Fatalf("turn dir leaked: %v", entries)
	}
}

func TestOpenCodeAuthErrorIsPermanentAndNotRetried(t *testing.T) {
	self, _ := os.Executable()
	home := t.TempDir()
	counter := filepath.Join(home, "attempts")
	t.Setenv("OPENCODE_TEST_FIXTURE", "auth-error")
	t.Setenv("OPENCODE_TEST_COUNTER", counter)
	c := &OpenCodeCLI{binPath: self, configDir: home}
	_, err := c.completeWithArgs(context.Background(), []string{"-test.run=TestOpenCodeHelperFixture", "-test.v=false"}, "prompt", "m", Request{})
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("auth classification: %v", err)
	}
	var perm *permanentProviderError
	if !asPermanent(err, &perm) {
		t.Fatalf("auth error must be permanent: %T %v", err, err)
	}
	if data, _ := os.ReadFile(counter); string(data) != "x" {
		t.Fatalf("auth failure must not retry, attempts=%q", data)
	}
}

func asPermanent(err error, target **permanentProviderError) bool {
	for err != nil {
		if p, ok := err.(*permanentProviderError); ok {
			*target = p
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestOpenCodeHelperFixture(t *testing.T) {
	fixture := os.Getenv("OPENCODE_TEST_FIXTURE")
	if fixture == "" {
		t.Skip("subprocess fixture only")
	}
	counter := os.Getenv("OPENCODE_TEST_COUNTER")
	prev, _ := os.ReadFile(counter)
	attempt := len(prev) + 1
	_ = os.WriteFile(counter, append(prev, 'x'), 0o600)
	if os.Getenv("OPENCODE_CONFIG") == "" {
		fmt.Fprintln(os.Stderr, "OPENCODE_CONFIG not exported")
		os.Exit(3)
	}
	switch fixture {
	case "crash-then-ok":
		if attempt == 1 {
			fmt.Fprintln(os.Stderr, "boom: died before any step")
			os.Exit(1)
		}
		for _, l := range []string{ocStepStart, ocToolRead, ocStepTools, ocStepStart, ocText, ocStepStop} {
			fmt.Println(l)
		}
		os.Exit(0)
	case "auth-error":
		fmt.Println(`{"type":"error","sessionID":"ses_e","error":{"name":"ProviderAuthError","data":{"message":"401 Unauthorized: invalid api key"}}}`)
		os.Exit(1)
	}
	os.Exit(0)
}
