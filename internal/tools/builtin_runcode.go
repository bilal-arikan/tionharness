package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/codemode"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/proc"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

const (
	// runCodeBindingsDir is where the generated Python MCP bindings live, under
	// the turn's working directory — a sibling of .tionharness/progress.json and
	// handoff.md. Regenerated (from scratch) on every run_code call.
	runCodeBindingsDir = ".tionharness/mcp"
	// runCodeDefaultTimeout / runCodeMaxTimeout bound a script's wall-clock
	// runtime. Wider than transform_data's fixed 30s because a script may chain
	// several (slow) MCP calls.
	runCodeDefaultTimeout = 60 * time.Second
	runCodeMaxTimeout     = 300 * time.Second
	// runCodeMaxScriptBytes / runCodeMaxLogBytes mirror transform_data's caps:
	// the DATA stays in the execution environment; only logs return to context.
	runCodeMaxScriptBytes = 256 * 1024
	runCodeMaxLogBytes    = 16 * 1024
)

// RunCodeTool is the code-execution-with-MCP entry point (_Docs/44): instead of
// shipping every MCP tool's schema to the model, the enabled MCP catalog is
// rendered as a generated Python module tree under .tionharness/mcp/ and the model
// CALLS tools by writing code. Intermediate results stay in the script's
// variables/files; only stdout/stderr (capped) plus a per-execution MCP call
// summary return to the conversation.
//
// Safety mirrors transform_data: stripped env (no host secrets), bounded
// wall-clock, capped logs, RiskExec classification behind the shell gate. The
// loopback bridge additionally enforces the agent's own tool filter, so code
// mode grants NO tool the agent could not already call directly. The bridge
// URL + per-execution random token travel to the subprocess via env vars that
// exist only for that one execution.
//
// Faz 2 (_Docs/44 §5): every in-script MCP call is additionally permission-
// gated (RunCodeGate — the agent layer binds the SAME permGate the native tool
// loop runs, so "ask" mode prompts per call and grants short-circuit) and
// observed (RunCodeObserver — one debug-journal tool event per call, restoring
// the per-call observability that folding N calls into one run_code loses).
type RunCodeTool struct {
	sb       Sandbox
	entries  []mcp.CatalogEntry
	caller   MCPCaller
	builtins []codemode.BuiltinDef // TionHarness built-in tools exposed as the `tionharness` module
	callBI   MCPCaller             // dispatches a built-in by bare name (nil = built-ins off)
	allow    func(string) bool     // agent tool filter (nil = allow all)
	gate     RunCodeGate           // per-call permission (nil = allow all)
	observe  RunCodeObserver       // per-call observability (nil = none)
}

// RunCodeGate decides one in-script MCP call under the agent's permission mode.
// ctx is the run_code tool call's context (it carries the permission prompter,
// session grants and audit logger on interactive turns).
type RunCodeGate func(ctx context.Context, tool string, args json.RawMessage) (allowed bool, denial string)

// RunCodeObserver receives one record per in-script MCP call (dispatched or
// denied) — the agent layer writes each as a debug-journal tool event.
type RunCodeObserver func(ctx context.Context, ob codemode.CallObservation)

// NewRunCodeTool binds the tool to the turn's working-dir sandbox, the MCP
// catalog to expose, the pool-backed MCP dispatcher, the built-in tools to expose
// (as the `tionharness` module) with their dispatcher, the agent's tool filter and the
// per-call permission/observability hooks. entries/caller may be empty (built-ins
// only); builtins/callBI may be nil (MCP only); gate/observe may be nil.
func NewRunCodeTool(sb Sandbox, entries []mcp.CatalogEntry, caller MCPCaller, builtins []codemode.BuiltinDef, callBI MCPCaller, allow func(string) bool, gate RunCodeGate, observe RunCodeObserver) RunCodeTool {
	return RunCodeTool{sb: sb, entries: entries, caller: caller, builtins: builtins, callBI: callBI, allow: allow, gate: gate, observe: observe}
}

// codeModeExcludedBuiltins are the built-in tools NEVER exposed as run_code
// bindings, regardless of the agent's tool filter. They fall into four groups,
// each nonsensical or unsafe to drive from a script:
//   - interactive (block for a live human → would hang the bridge for the whole run)
//   - other execution engines / code-in-code (no added value, extra risk)
//   - delegation, recursion and meta tool-loading (run_code inside run_code, etc.)
//   - coordinator worker controls (only valid on a coordinator turn; recursion-ish)
//
// Everything else that is a registered built-in AND passes the agent's tool filter
// gets a binding — so code mode grants no capability the agent lacks directly.
var codeModeExcludedBuiltins = map[string]bool{
	// Interactive — block for a human.
	"ask_user": true, "request_confirmation": true,
	// UI-sink fire-and-forget; meaningless when scripted.
	"notify": true, "focus_view": true,
	// Other execution engines / code-in-code.
	"Bash": true, "PowerShell": true, "transform_data": true, "run_code": true,
	"shell_manage": true,
	// Delegation / recursion / meta tool-loading.
	"run_subagent": true, "use_skill": true, "skill_search": true,
	"activate_tools": true, "deactivate_tools": true, "tool_search": true,
	"spawn_session": true,
	// Coordinator worker controls.
	"spawn_worker": true, "send_to_worker": true, "stop_worker": true, "list_workers": true,
}

// CodeModeEligible reports whether a built-in tool NAME may be exposed as a
// run_code binding (i.e. it is not on the hard-exclude list above). The agent's
// own tool filter is applied on top of this, at binding-generation time.
func CodeModeEligible(name string) bool { return !codeModeExcludedBuiltins[name] }

func (RunCodeTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "run_code",
		// Deliberately short — run_code ships EAGERLY every turn, so the def
		// carries only what is needed to decide to call it. Usage detail (keyword
		// args, error type, ask-mode timeout, sampling accuracy) is returned by
		// the discovery call (renderBindingListing) exactly when it is needed.
		Description: "Run a Python script that calls this agent's tools (MCP + built-ins) as ordinary functions — " +
			"orchestrate a multi-tool workflow (list → filter → act) in ONE call instead of many round-trips. " +
			"Call with NO script first: bindings are regenerated under " + runCodeBindingsDir + "/ and the " +
			"module/function listing is returned; Read a module file for docstrings + schemas. Only print() " +
			"output (capped 16KB) returns to the conversation — keep large intermediates in variables/files. " +
			"Isolated subprocess, stripped env (no secrets), bounded timeout.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"script":{"type":"string","description":"Python script source. Omit (or empty) to just regenerate the bindings and get the module/function listing."},
				"timeout_sec":{"type":"integer","description":"Wall-clock timeout in seconds (default 60, max 300)"}
			},
			"additionalProperties":false
		}`),
	}
}

func (t RunCodeTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Script     string `json:"script"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErr(err)
	}
	if len(args.Script) > runCodeMaxScriptBytes {
		return "", fmt.Errorf("script is too large (%d bytes, max %d)", len(args.Script), runCodeMaxScriptBytes)
	}
	if !t.sb.Ready() {
		return "", fmt.Errorf("no working directory is configured for run_code")
	}
	if len(t.entries) == 0 && len(t.builtins) == 0 {
		return "", fmt.Errorf("no tools are available to expose as bindings")
	}

	bindDir, err := t.sb.Resolve(runCodeBindingsDir)
	if err != nil {
		return "", fmt.Errorf("bindings dir: %w", err)
	}
	// Stateless regeneration on every call: the bindings always mirror the
	// current catalog (MCP servers + the `tionharness` built-ins module), so a
	// changed/removed tool can never serve stale stubs.
	modules, err := codemode.WriteBindings(bindDir, t.entries, t.builtins, t.allow)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(args.Script) == "" {
		return renderBindingListing(modules), nil
	}
	if len(t.entries) > 0 && t.caller == nil {
		return "", fmt.Errorf("run_code has no MCP dispatcher wired (workspace MCP pool unavailable)")
	}

	interp, scriptExt, err := resolveInterpreter("python3")
	if err != nil {
		return "", err
	}
	scriptPath, cleanup, err := writeTempScript(args.Script, scriptExt)
	if err != nil {
		return "", err
	}
	defer cleanup()

	// Bind this call's ctx into the bridge hooks: the gate needs the turn's
	// permission prompter/grants, the observer needs the session/turn ids — both
	// ride on ctx, which the HTTP handler does not have.
	cfg := codemode.Config{Allow: t.allow}
	if t.caller != nil {
		cfg.Call = codemode.CallFunc(t.caller)
	} else {
		// Built-ins only (no MCP servers): the bridge still needs a non-nil MCP
		// dispatcher to Start; a stray MCP call (none should occur — no MCP module
		// was generated) fails loudly rather than panicking.
		cfg.Call = func(context.Context, string, json.RawMessage) (mcp.CallToolResult, error) {
			return mcp.CallToolResult{}, fmt.Errorf("no MCP servers are available in this run_code execution")
		}
	}
	if t.callBI != nil {
		// Built-ins dispatch on the TURN's ctx (session id + sinks), NOT the bridge's
		// per-call ctx, so a sink-based built-in behaves exactly like a direct call.
		// Mirrors the gate/observe closures below, which also capture the turn ctx.
		cfg.Builtin = func(_ context.Context, name string, in json.RawMessage) (mcp.CallToolResult, error) {
			return t.callBI(ctx, name, in)
		}
	}
	if t.gate != nil {
		cfg.Gate = func(tool string, in json.RawMessage) (bool, string) { return t.gate(ctx, tool, in) }
	}
	if t.observe != nil {
		cfg.Observe = func(ob codemode.CallObservation) { t.observe(ctx, ob) }
	}
	bridge, err := codemode.Start(cfg)
	if err != nil {
		return "", err
	}
	defer bridge.Close()

	timeout := runCodeDefaultTimeout
	if args.TimeoutSec > 0 {
		timeout = time.Duration(args.TimeoutSec) * time.Second
		if timeout > runCodeMaxTimeout {
			timeout = runCodeMaxTimeout
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := proc.CommandContext(runCtx, interp, scriptPath)
	// The script may itself spawn processes; on timeout they must die with it,
	// otherwise a survivor keeps the output pipes open and this call never ends.
	proc.TreeKill(cmd)
	cmd.Dir = t.sb.Root
	// Stripped env (transform_data's allowlist) + the code-mode extras: the
	// bindings dir on PYTHONPATH and the per-execution bridge address/token.
	// The token is NOT a host secret — it is random, scoped to this single
	// execution's bridge, and dies with it.
	cmd.Env = append(minimalScriptEnv(),
		"PYTHONPATH="+bindDir,
		"TIONHARNESS_MCP_BRIDGE_URL="+bridge.URL(),
		"TIONHARNESS_MCP_BRIDGE_TOKEN="+bridge.Token(),
	)
	var logBuf bytes.Buffer
	w := &capWriter{buf: &logBuf, max: runCodeMaxLogBytes}
	cmd.Stdout = w
	cmd.Stderr = w
	runErr := cmd.Run()

	logs := strings.TrimSpace(logBuf.String())
	if w.truncated {
		logs += "\n[logs truncated at 16KB]"
	}
	summary := bridge.Summary()

	if runCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("run_code timed out after %s\n%s\n%s", timeout, summary, logs)
	}
	if runErr != nil {
		// A failed script must fail loudly (with its own traceback) so the model
		// can fix it — never a silent success.
		return "", fmt.Errorf("run_code script failed (%v)\n%s\n%s", runErr, summary, logs)
	}

	var b strings.Builder
	if logs == "" {
		b.WriteString("(no output — print() what you need back in the conversation)")
	} else {
		b.WriteString(logs)
	}
	if summary != "" {
		b.WriteString("\n\n--- mcp calls ---\n")
		b.WriteString(summary)
	}
	return b.String(), nil
}

// renderBindingListing builds the discovery-mode result: which modules exist,
// which functions each exposes, and how to proceed. Names only — schemas stay
// on disk until the model Reads them (progressive disclosure).
func renderBindingListing(modules map[string][]string) string {
	names := make([]string, 0, len(modules))
	for m := range modules {
		names = append(names, m)
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprintf(&b, "Tool bindings regenerated under %s/ (on PYTHONPATH for run_code scripts).\n"+
		"Modules: one per MCP server, plus `tionharness` for TionHarness's built-in tools.\nModules:\n", runCodeBindingsDir)
	for _, m := range names {
		fmt.Fprintf(&b, "- %s: %s\n", m, strings.Join(modules[m], ", "))
	}
	b.WriteString("\nRead " + runCodeBindingsDir + "/<module>.py for each function's docstring + input schema, " +
		"then call run_code with a script (e.g. `from <module> import <function>`).\n" +
		"Notes: pass arguments as keywords; functions return the tool's result (JSON-decoded when possible) and " +
		"raise _bridge.MCPError on failure. Under the 'ask' permission mode each in-script call may pause for " +
		"user approval — the wait counts against the script's timeout, so raise timeout_sec for scripts expected " +
		"to prompt. ACCURACY: tool results are strings in a format you have NOT seen — on first use of a tool, " +
		"print a small sample (e.g. repr(result)[:200]) to verify the format before aggregating.")
	return b.String()
}
