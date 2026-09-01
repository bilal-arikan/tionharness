package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// toolLoopTurn carries the mutable state of ONE completeTracedInner turn across
// its named phase methods. This is a purely structural split of what used to be a
// single ~1100-line function: every phase below is a verbatim block of that body.
//
// The rule that makes the split safe: anything the original function REASSIGNED
// mid-body is a field here, not a parameter. ctx is the load-bearing case — the
// setup phase rebinds it repeatedly (sandbox root, session id, artifact/todo
// sinks, interaction endpoint, active tools, coordination, MCP failure sink,
// subagent runner) and every later phase must observe the wrapped value. Passing
// ctx as a parameter would silently confine each wrap to its own phase.
type toolLoopTurn struct {
	r          *Runtime
	ctx        context.Context
	agent      db.Agent
	provider   providers.Provider
	req        providers.Request
	autonomous bool
	onStep     func(TurnStep)

	// Resolved by prepare.
	cli    providers.CLIProvider
	isCLI  bool
	inter  tools.InteractionEndpoint
	cliMCP bool

	// Resolved by prepareNativeLoop.
	reg     *tools.Registry
	active  *tools.ActiveTools
	shipFor func() []providers.ToolDef
	rawEcho func(json.RawMessage) json.RawMessage
	mcpNote *TurnStep

	// Native-loop state.
	emit            func(TurnStep)
	activeLiveCards map[string]*liveCard
	last            *providers.Response
	turnUsage       providers.Usage
	steps           []TurnStep
	ls              loopState
	cfg             recoveryConfig
	keepRecent      int
	guard           *toolGuard
	guardHaltReason string
	repair          *mcpRepair
	partial         strings.Builder
	steerRole       string
	batchSeq        int
	// pendingProgrammatic defers steering while a programmatic tool batch awaits
	// its results: that request's trailing message must stay PURE tool_results,
	// so queued guidance is delivered on the next ordinary iteration instead.
	pendingProgrammatic bool
}

// toolBatch is the per-iteration state of one provider response's tool calls,
// shared between runNativeLoop and runToolCall.
type toolBatch struct {
	resp       *providers.Response
	subFutures map[string]*subFuture
	openCards  map[string]*liveCard
	batch      int
	results    []providers.ToolResult
}

// prepare wires the per-turn request fields, context sinks and provider homes
// that every downstream path (plain completion, CLI delegation, native loop)
// depends on. It returns a cleanup func the caller MUST defer: the headless
// interaction endpoint installed here has to stay live for the whole turn, which
// is exactly the lifetime of the original function's `defer done()`.
func (t *toolLoopTurn) prepare() (func(), error) {
	noop := func() {}
	// Carry the agent's permission mode so provider-driven loops (claude CLI) can
	// gate their tool use. Empty maps to "auto" downstream.
	t.req.PermissionMode = effectivePermissionMode(t.agent.PermissionMode, t.autonomous)
	if t.agent.ThinkingLevel != "" {
		if err := providers.ValidateThinkingLevelForProvider(t.agent.Provider, t.agent.Model, t.agent.ThinkingLevel); err != nil {
			return noop, err
		}
	}

	// Thinking parity for provider-driven loops: a ThinkingLevel that resolves to
	// no budget ("Kapalı"/"off") now reaches the claude-cli subprocess as
	// MAX_THINKING_TOKENS=0 instead of silently falling back to the CLI's own
	// adaptive-thinking default. Besides honouring the setting, this restores
	// PARALLEL tool batching on claude-code ≥2.1.203, which refuses parallel tool
	// calls while thinking is active ("think XOR batch" — the AlgoBench cost
	// regression, _Docs/05 2026-07-10). Native providers ignore the flag
	// (ThinkingBudget==0 already means off there).
	t.req.DisableThinking = t.agent.ThinkingLevel == "off"

	// Provider-native web search is opt-OUT per agent: on unless the agent stores
	// an explicit false (NativeWebSearchEnabled resolves the nil = enabled
	// default). On leaves the native tool available and its call surfaces as a
	// trace step; off tells the CLI providers to switch their own search off so
	// the bridged WebSearch/WebFetch tools stay the single path.
	t.req.NativeWebSearch = t.agent.NativeWebSearchEnabled()

	// Announce the configured task budget to autonomous turns (API-native
	// output_config.task_budget): the model sees a running countdown for the
	// whole loop and paces itself. Providers/models without support ignore it;
	// interactive chat turns stay un-budgeted (a human is watching).
	if t.autonomous {
		t.req.TaskBudgetTokens = t.r.tun.AutonomousTaskBudget()
	}

	// Resolve this turn's working directory: the session's WorkingDir override
	// (else the workspace default). The result roots the fs/shell sandbox
	// (carried via ctx into buildRegistry) and is the cwd for provider-driven CLI
	// subprocesses (claude-cli) so relative paths — e.g. an attachment's
	// "uploads/<sid>/<file>" — resolve there. Native providers ignore req.WorkDir.
	workDir := t.r.effectiveWorkDir(t.ctx)
	t.req.WorkDir = workDir
	t.ctx = withResolvedWorkDir(t.ctx, workDir, t.autonomous)

	// Expose the current session id to session-scoped tools (read_session_debug)
	// so they default to the running session without an explicit arg.
	if sid := SessionIDFrom(t.ctx); sid != "" {
		t.ctx = tools.WithCurrentSession(t.ctx, sid)
	}

	// Artifacts everywhere: chat turns install a session-bound sink before calling
	// in; autonomous turns (scheduler/spawn/flow) don't, so create_artifact would
	// fail with "artifacts are not available for this turn". Install a fallback sink
	// here whenever the turn has a session but no sink yet — so artifacts can always
	// be created, regardless of turn type. (CLI turns get theirs via the Interaction
	// bridge run; this covers the native loop.)
	if sid := SessionIDFrom(t.ctx); sid != "" && !tools.HasArtifactSink(t.ctx) {
		t.ctx = tools.WithArtifacts(t.ctx, t.r.NewArtifactSink(sid, t.agent.ID))
	}

	// Persistent progress: when todo_write runs, persist the checklist to the
	// project's progress file so it survives across sessions (Claude Code's
	// claude-progress convention). Install a fallback sink whenever the turn has a
	// session but no sink yet — covering native chat + autonomous (scheduler/spawn/
	// flow) turns. (CLI turns get theirs via the Interaction bridge run.) Gated by
	// the ProgressPersist setting (default on); workDir was resolved just above.
	if sid := SessionIDFrom(t.ctx); sid != "" && t.r.tun.ProgressPersist() && !tools.HasTodoSink(t.ctx) {
		t.ctx = tools.WithTodoSink(t.ctx, t.r.NewTodoSink(sid, t.agent.ID))
	}

	// The claude CLI runs its own tool loop. Route it through the keyless MCP
	// delegation path when external MCP is enabled OR an Interaction MCP endpoint
	// is wired for this turn (so ask_user/todo_write work even with MCP off).
	t.cli, t.isCLI = providers.AsCLI(t.provider)
	// Point the CLI at a config home so it reads the right skills/settings/login.
	// K1 (_Docs/71-SAGLAYICI-ORNEKLERI-PLANI.md): an instance whose own configDir
	// field is set (ConfigDir() already non-empty, baked in at construction from
	// the provider instance's config) keeps that dedicated home untouched here —
	// only an instance with NO configDir of its own falls back to the APP-GLOBAL
	// home (<dataDir>/claude-home or <dataDir>/codex-home), so one login serves
	// every workspace. No-op when the data dir is unknown (keeps the provider's
	// own default). This is the single per-turn seam every CLI turn passes through.
	if t.isCLI {
		// Both CLI transports consume the product's resolved reasoning tier.
		// Provider-specific encoding stays below the Request boundary.
		t.req.CLIEffortLevel = cliEffortLevel(t.agent.ThinkingLevel)
		// The config home and the credential heal below are claude-specific: both
		// name a claude-home and the CLI's own .credentials.json. A second
		// CLI transport must NOT inherit them, so they stay behind a narrow concrete
		// assertion while the generic wiring (MCP delegation) goes through the
		// interface.
		if cc, ok := t.provider.(*providers.ClaudeCLI); ok {
			home, err := t.r.PinClaudeHome(cc)
			if err != nil {
				return noop, err
			}
			ensureClaudeHomeEffortLevel(home)
		}
		// codex-cli's sibling of the block above, shared with guardedComplete so
		// both entry points pin the same home (see PinCodexHome / PinCLIHome).
		if err := t.r.PinCodexHome(t.provider); err != nil {
			return noop, err
		}
	}
	t.inter = tools.InteractionFrom(t.ctx)
	// CLI turns that arrive without an Interaction endpoint — autonomous ones
	// (scheduler/spawn/flow) AND the non-stream /api/chat path (only /api/chat/stream
	// registers a run) — can't reach the bridged use_skill/shell/self-manage/
	// coordination tools and fall back to native (now-disallowed/foreign) ones: the
	// cause of scheduled "Unknown skill", the POSIX-Bash mismatch, and a coordinator
	// fanning out via the CLI's own Agent tool instead of spawn_worker. Wire the
	// headless endpoint on demand for ANY such CLI turn; skipped whenever one is
	// already present (the stream path installs its own).
	cleanup := noop
	if t.isCLI && t.inter.URL == "" && t.r.autoInteract != nil {
		var done func()
		t.ctx, done = t.r.autoInteract(t.ctx, t.agent, SessionIDFrom(t.ctx))
		cleanup = done
		t.inter = tools.InteractionFrom(t.ctx)
	}
	t.cliMCP = t.isCLI && (t.agent.MCPEnabled || t.inter.URL != "")

	// Provider-driven paths (claude CLI) surface their own trace via OnEvent. This
	// is wired AFTER the interaction endpoint is resolved because the dead-tool
	// repair below needs that turn's Bearer token to activate against; the provider
	// call itself happens further down, so the ordering is free.
	//
	// deadTools is nil on every path that cannot hit the failure (native loop, no
	// activator wired), and repair leaves every unrelated step byte-identical.
	if t.onStep != nil {
		deadTools := t.r.deadToolRepairFor(t.agent, t.inter)
		t.req.OnEvent = func(ts providers.TraceStep) {
			st := t.r.traceStepToTurnStep(ts)
			deadTools.repair(t.ctx, &st)
			t.onStep(st)
		}
	}
	t.req.OnCLICompaction = func(ev providers.CLICompactionEvent) {
		if err := t.r.db.AppendCLICompactionEvent(SessionIDFrom(t.ctx), t.agent.ID, ev); err != nil {
			t.r.logger.Error("persist cli compaction lifecycle failed", "error", err)
		}
	}

	// A CLI provider that kills a wedged subprocess reports it here so the kill
	// lands in debug.jsonl with its stdout tail instead of surviving only as a turn
	// error string. Providers cannot reach the db layer (they sit below it), so the
	// runtime supplies the sink — same shape as OnEvent above, but kept for the
	// native path too since it is diagnostics, not streaming.
	t.req.OnWatchdog = func(kill providers.WatchdogKill) {
		t.r.emitDebug(t.ctx, db.DebugEvent{
			Type:   db.DebugError,
			Name:   kill.Provider + "_watchdog_" + kill.Reason,
			Model:  kill.Model,
			DurMs:  kill.Window.Milliseconds(),
			Err:    true,
			Detail: debugSummary(kill.Detail, 400),
		})
	}
	return cleanup, nil
}

// completeAndTraceCLI is the shared tail of both provider-driven paths (plain
// completion and CLI MCP delegation): one completion, then the CLI's own
// stream-json trace becomes this turn's steps.
func (t *toolLoopTurn) completeAndTraceCLI() (*providers.Response, []TurnStep, error) {
	resp, err := t.r.recordedComplete(t.ctx, t.agent, t.provider, t.req)
	if err != nil {
		return nil, nil, err
	}
	// claude-cli surfaces its own tool/thinking trace via stream-json.
	t.r.emitCLIToolDebug(t.ctx, t.agent, resp.Trace)
	// Guardrail visibility parity: flag looping CLI turns post-hoc.
	t.r.analyzeCLIGuardrail(t.ctx, t.agent, resp.Trace)
	return resp, t.r.traceToSteps(resp.Trace), nil
}

// runPlain handles the no-tools path: a single completion, streamed when a live
// sink and a Streamer provider are both present.
func (t *toolLoopTurn) runPlain() (*providers.Response, []TurnStep, error) {
	// Extended reasoning is applied only on the plain (non-tool) path: the
	// native tool loop would need to echo signed thinking blocks back, which
	// the provider abstraction doesn't preserve. Providers without thinking
	// support (claude-cli, minimax) ignore the budget.
	t.req.ThinkingBudget = resolveThinkingBudget(t.agent.Model, t.agent.ThinkingLevel)

	// Prefer first-class token streaming when a live sink is present and the
	// provider supports it (anthropic/minimax). claude-cli is not a Streamer;
	// it streams its own trace via req.OnEvent wired above.
	if t.onStep != nil {
		if sm, ok := t.provider.(providers.Streamer); ok {
			resp, err := t.r.recordedStream(t.ctx, t.agent, sm, t.req, t.onStep)
			if err != nil {
				return nil, nil, err
			}
			// Text deltas are transient (recovered from resp.Text); the
			// thinking trace, if any, is persisted so the reasoning block
			// survives reload.
			return resp, t.r.traceToSteps(resp.Trace), nil
		}
	}
	return t.completeAndTraceCLI()
}

// configureCLIMCP builds the keyless delegation config: the claude CLI owns the
// tool loop, wiring the external MCP servers (when enabled) plus the Interaction
// MCP server (when an endpoint is present) into a single generated --mcp-config.
// The returned func MUST be deferred by the caller so the temp config/settings
// files outlive the provider call, exactly as the original's two stacked defers
// did (LIFO: settings first, then the mcp config).
func (t *toolLoopTurn) configureCLIMCP() func() {
	noop := func() {}
	if _, ok := t.provider.(*providers.CodexCLI); ok {
		// codex has no --mcp-config file or --settings file; its MCP delegation is
		// expressed entirely through the CLIMCPSpec.Servers map, rendered straight
		// into config.toml by ConfigureCLIMCP. Building that map is asymmetric
		// enough from the claude path (see codexmcp.go's file comment) that it gets
		// its own builder rather than reusing writeCLIMCPConfig/writeCLISettings.
		spec, err := t.r.codexMCPSpec(t.ctx, t.agent.MCPEnabled, t.agent, t.inter)
		if err != nil {
			t.r.logger.Warn("codex mcp spec failed", "error", err)
		} else if len(spec.Servers) > 0 {
			t.cli.ConfigureCLIMCP(spec)
		}
		return noop
	}
	path, allowed, disallowed, cleanup, err := t.r.writeCLIMCPConfig(t.ctx, t.agent.MCPEnabled, t.agent, t.inter, t.agent.PermissionMode)
	if err != nil {
		t.r.logger.Warn("cli mcp config failed", "error", err)
		return noop
	}
	if path == "" && len(disallowed) == 0 {
		return noop
	}
	// A turn with no MCP servers still has something to say when the
	// disallow list is non-empty (agent-level native-tool suppression, e.g.
	// web search off): the spec then carries only --disallowedTools +
	// --settings, no --mcp-config.
	//
	// Per-turn --settings: permission deny-list (mirrors disallowed) plus the
	// workspace's PreToolUse/PostToolUse hooks, so the CLI's own loop honours
	// the same blocks/hooks the native loop does. "" when there is nothing.
	settingsPath, settingsCleanup, serr := t.r.writeCLISettings(t.ctx, disallowed, cliEffortLevel(t.agent.ThinkingLevel))
	if serr != nil {
		t.r.logger.Warn("cli settings write failed", "error", serr)
	}
	// In "ask" mode route risky CLI tools through the Interaction MCP
	// permission-prompt tool (real per-tool approval) instead of acceptEdits.
	t.cli.ConfigureCLIMCP(providers.CLIMCPSpec{
		ConfigPath:       path,
		AllowedTools:     allowed,
		DisallowedTools:  disallowed,
		PermissionPrompt: promptToolForMode(t.agent.PermissionMode, t.inter),
		SettingsPath:     settingsPath,
	})
	return func() {
		settingsCleanup()
		cleanup()
	}
}

// prepareNativeLoop builds the tool registry and resolves everything the native
// agentic loop ships on every iteration (active set, server-tool modes, frozen
// prompt-epoch defs, the raw-echo policy). done=true means the turn is already
// finished — an empty registry degrades to a plain completion.
func (t *toolLoopTurn) prepareNativeLoop() (resp *providers.Response, steps []TurnStep, err error, done bool) {
	// Native agentic loop (providers that return structured tool_use). OnEvent
	// is not used here — we emit each step ourselves as the loop progresses.
	t.req.OnEvent = nil
	// Lazy tool loading: a per-turn active set tracks which on-demand (lazy) tools
	// the model has activated. buildRegistry wires the activate_tools meta-tools to
	// this same set (via ctx); req.Tools is recomputed each iteration so a freshly
	// activated tool's schema is shipped on the next step.
	t.active = tools.NewActiveTools()
	t.ctx = withActiveTools(t.ctx, t.active)
	// Coordinator/worker tools (M2): wired only when this turn runs on a coordinator
	// session. Injected BEFORE buildRegistry so the registry can gate their
	// registration on the runner's presence (so ordinary/worker sessions never see
	// them). No-op on every other turn.
	t.ctx = t.r.withCoordination(t.ctx, t.agent)
	// Collect per-server MCP catalog failures during the build so the turn can
	// report them once (mcpnotice.go); without this they are log-only and the
	// missing tools look like they never existed.
	ctx, mcpFailures := withMCPFailures(t.ctx)
	t.ctx = ctx
	t.reg = t.r.buildRegistry(t.ctx, t.agent)
	// One card per turn (not per tool call) naming every MCP server that failed
	// its catalog build, with the reason — so a missing tool reads as "the server
	// is down" instead of "that tool does not exist".
	if note := formatMCPFailureNote(mcpFailures.list()); note != "" {
		st := TurnStep{Kind: StepRecovery, Reason: mcpFailureReason, Text: note}
		t.mcpNote = &st
		t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugError, AgentID: t.agent.ID, Detail: note, Err: true})
	}
	if t.reg.Empty() {
		resp, err := t.r.recordedComplete(t.ctx, t.agent, t.provider, t.req)
		if t.mcpNote != nil {
			if t.onStep != nil {
				t.onStep(*t.mcpNote)
			}
			return resp, []TurnStep{*t.mcpNote}, err, true
		}
		return resp, nil, err, true
	}
	toolFilter := t.r.toolFilter(t.ctx, t.agent)
	t.reg.ConfigureAutoActivation(t.active, toolFilter)
	// Native (server-side) tool search — first-party anthropic only: the full
	// catalog ships with lazy tools marked defer_loading + the search server
	// tool, so discovery needs no activate_tools round-trip and the tools block
	// stays byte-stable across iterations. Anthropic-protocol lookalikes
	// (minimax-anthropic, custom endpoints) would reject the server tool type,
	// hence the exact-name gate. Everything else keeps the activation flow.
	nativeSearch := t.r.tun.NativeToolSearch() && t.provider.Name() == "anthropic"
	// Programmatic tool calling (PTC): the code-execution server tool lets the
	// model call code-callable tools from Python inside Anthropic's container —
	// intermediate results never enter context. First-party anthropic only.
	ptcMode := t.r.tun.ProgrammaticTools() && t.provider.Name() == "anthropic"
	// Server-side web search + fetch (first-party anthropic only): declared in
	// the tools array, executed on Anthropic's infrastructure, results ride the
	// same response with citations.
	webMode := t.r.tun.WebTools() && t.provider.Name() == "anthropic"
	// API-native compaction (beta, applied provider-side): the loop only needs
	// to know so the verbatim echo below preserves compaction blocks.
	serverCompact := t.r.tun.ServerCompaction() && t.provider.Name() == "anthropic"
	shipDefs := func() []providers.ToolDef {
		var defs []providers.ToolDef
		if nativeSearch {
			defs = t.reg.DeferredDefs(toolFilter, t.active.Snapshot())
		} else {
			defs = t.reg.ActiveDefs(toolFilter, t.active.Snapshot())
		}
		if ptcMode {
			for i := range defs {
				// MCP tools are incompatible with PTC (namespaced server__tool);
				// interactive/recursive builtins are excluded by the same policy
				// code mode uses (they would hang or recurse inside a script).
				if strings.Contains(defs[i].Name, "__") || !tools.CodeModeEligible(defs[i].Name) {
					continue
				}
				defs[i].CodeCallable = true
			}
		}
		return defs
	}
	// Prompt epoch: the shipped tool defs are the FROZEN session-start snapshot
	// (promptepoch.go) so passive catalog drift — a skill install, an MCP
	// tools/list_changed, a visibility edit — cannot bust the tools block at the
	// very front of the cache prefix. The agent's own in-turn activations are the
	// deliberate exception: shipFor merges them live (a chosen one-time re-write).
	// Execution always uses the LIVE registry, so a tool disabled mid-session
	// fails closed even while its frozen schema is still advertised.
	frozenDefs, toolsStale := t.r.EpochToolDefs(t.ctx, SessionIDFrom(t.ctx), t.agent, shipDefs)
	t.shipFor = func() []providers.ToolDef {
		if frozenDefs == nil { // epoch off / no session / no snapshot: live defs
			return shipDefs()
		}
		return mergeFrozenToolDefs(frozenDefs, shipDefs(), t.active.Snapshot())
	}
	if toolsStale && !strings.Contains(t.req.SystemDynamic, suffixNoteMarker) {
		if note := t.r.PromptEpochContextNote(SessionIDFrom(t.ctx), t.agent.ID); note != "" {
			t.req.SystemDynamic = strings.TrimSpace(t.req.SystemDynamic + "\n\n" + note)
		}
	}
	t.req.Tools = t.shipFor()
	t.req.ProgrammaticTools = ptcMode
	t.req.WebTools = webMode
	// rawEcho gates the verbatim assistant-content echo on the modes whose
	// responses carry server blocks (tool-search results, code-execution runs,
	// web tool results, compaction blocks) that MUST ride back exactly;
	// everywhere else the portable Text+ToolCalls echo stays, so
	// inherited-context subagents on other providers see no behaviour change.
	//
	// Always-on-thinking models (Fable/Mythos 5) are ALWAYS raw-echoed: they
	// think on every response — tool loop included — and the API requires those
	// thinking blocks back exactly as received on the same model; the
	// constructed Text+ToolCalls echo would drop them and break the turn.
	t.rawEcho = func(raw json.RawMessage) json.RawMessage {
		if nativeSearch || ptcMode || webMode || serverCompact || providers.AlwaysOnThinking(t.agent.Model) {
			return raw
		}
		return nil
	}
	return nil, nil, nil, false
}

// initLoopState resolves the settings-driven recovery/guardrail policy for this
// turn and seeds the trace. Runs after the emitter and the panic-cleanup defer
// are in place, so a step emitted here already reaches the live sink.
func (t *toolLoopTurn) initLoopState() {
	// MCP catalog failures (collected during buildRegistry above) lead the trace:
	// they explain a capability gap that applies to the whole turn.
	if t.mcpNote != nil {
		t.steps = append(t.steps, *t.mcpNote)
		t.emit(*t.mcpNote)
	}
	// ls carries the single-shot recovery guards (A1) across iterations so a
	// stuck model can never spin forever inside one turn; cfg/keepRecent are the
	// resolved, settings-driven recovery policy for this turn.
	t.cfg = recoveryConfig{
		maxTokenLimit:      t.r.tun.MaxTokenRetries(),
		reactiveCompact:    t.r.tun.ReactiveCompact(),
		maxProviderRetries: t.r.tun.ProviderRetryMax(),
	}
	t.keepRecent = t.r.tun.ReactiveKeepRecent()
	// Tool-loop guardrail (self-healing Faz B): per-turn loop detection. Warnings
	// ride the failing tool results; block/halt fire only when the hard stop is
	// enabled in settings.
	gew, geb, gsw, gsh, gnw, gnb := t.r.tun.ToolGuardThresholds()
	t.guard = newToolGuard(toolGuardConfig{
		warnings:       t.r.tun.ToolGuardWarnings(),
		hardStop:       t.r.tun.ToolGuardHardStop(),
		exactWarn:      gew,
		exactBlock:     geb,
		sameToolWarn:   gsw,
		sameToolHalt:   gsh,
		noProgressWarn: gnw,
		noProgressBlck: gnb,
	})
	// MCP not-indexed repair (self-healing): a codebase-memory (or peer) MCP call
	// whose `project` is unindexed fails with a body the model does not act on, so
	// it loops. This breaks the loop on the first repeat — independent of the loop
	// guardrail's hard-stop setting. Per-turn, isolated to mcprepair.go.
	t.repair = newMCPRepair()
	// Steer messages ride the operator channel ({"role":"system"} in messages)
	// on models that support it — cache-safe, non-spoofable, and valid between a
	// tool_result user turn and the next assistant turn. Elsewhere they stay
	// user-role (the provider folds them to keep alternation intact).
	t.steerRole = providers.RoleUser
	if t.provider.Name() == "anthropic" && providers.SupportsSystemInMessages(t.agent.Model) {
		t.steerRole = providers.RoleSystem
	}
}

// fail records a turn-level error as an inline step before the loop returns.
// A usage/rate-limit, overload or billing terminal error surfaces as a
// cryptic provider string ("anthropic HTTP 429: …"); replace it with a clear,
// actionable message and retag the step with the specific limit class so the
// UI renders a dedicated "hit limit" card + retry hint. The raw detail is kept
// below the explanation. Classification is conservative, so a guardrail/
// max-iters failure never matches and keeps its original reason + text.
func (t *toolLoopTurn) fail(reason string, err error) {
	text := err.Error()
	if cls := classifyProviderError(err); limitErrorText(cls) != "" {
		reason = string(cls)
		text = limitErrorText(cls) + "\n\n" + text
	}
	st := TurnStep{Kind: StepError, Reason: reason, Text: text, IsError: true}
	t.steps = append(t.steps, st)
	t.emit(st)
	t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugError, AgentID: t.agent.ID, Detail: reason + ": " + err.Error(), Err: true})
}

// compactAndRetry folds the in-flight history once per turn and reports it as a
// recovery + compaction step pair. ok=false means the fold did not happen and
// the caller must fall through to its normal (terminal) handling.
func (t *toolLoopTurn) compactAndRetry(reason contReason) bool {
	cctx := conversation.WithCompactPrompt(t.ctx, t.r.CompactPromptTemplate())
	folded, fold, ok, cerr := conversation.CompactInFlightMessages(cctx, t.r.db, t.provider, t.agent, t.req.Messages, t.keepRecent)
	if cerr != nil || !ok {
		return false
	}
	t.req.Messages = folded
	// A CLI resume target still owns the pre-fold history. Clear it before
	// retrying so this request starts a fresh CLI session from the in-flight
	// summary + recent tail and captures a new session/thread id.
	t.req.ResumeSessionID = ""
	t.ls.compacted = true
	t.ls.lastContinue = reason
	// Signal the autonomous caller that this turn hit the context
	// limit, so it can decide on an automatic context-reset handoff.
	markContextOverflow(t.ctx)
	// Two steps, two jobs: the recovery card says WHY the turn was
	// retried, the compaction card says WHAT the fold cost.
	rec := TurnStep{Kind: StepRecovery, Reason: string(reason), Text: recoveryText(reason)}
	t.steps = append(t.steps, rec)
	t.emit(rec)
	cst := reactiveCompactionStep(fold, t.provider)
	t.steps = append(t.steps, cst)
	t.emit(cst)
	t.r.emitDebug(t.ctx, reactiveCompactionEvent(t.agent.ID, string(reason), fold))
	return true
}

// runNativeLoop is TionHarness's own agentic loop: call the provider, recover
// from a recoverable failure, execute the tool batch it asked for, answer with
// tool_results, repeat until the model stops asking or a bound is hit.
func (t *toolLoopTurn) runNativeLoop() (*providers.Response, []TurnStep, error) {
	for i := 0; i < maxToolIters; i++ {
		// Live steering: fold any user guidance that arrived since the last
		// iteration into the conversation before the next model call.
		if !t.pendingProgrammatic {
			for _, m := range drainSteer(t.ctx) {
				t.req.Messages = append(t.req.Messages, providers.Message{Role: t.steerRole, Text: steerPrefix + m})
				st := TurnStep{Kind: StepSteer, Text: m}
				t.steps = append(t.steps, st)
				t.emit(st)
			}
		}
		// Recompute the shipped tool schemas for this step: eager tools plus any
		// lazy tools activated so far (native-search mode: full deferred catalog,
		// byte-stable apart from activations). Cheap; reflects activate/deactivate
		// calls from the previous iteration.
		t.active.SetIter(i)
		t.req.Tools = t.shipFor()
		// Self-healing: enforce the tool_use↔tool_result pairing invariants on
		// the in-flight history before every provider call. A well-formed slice
		// passes through untouched; a healed one is logged + journaled (never
		// silent), instead of surfacing as an opaque provider 400.
		if repaired, notes := conversation.RepairSequence(t.req.Messages); len(notes) > 0 {
			t.req.Messages = repaired
			for _, n := range notes {
				t.r.logger.Warn("message sequence repaired", "agent", t.agent.ID, "rule", n.Rule, "detail", n.Detail)
				t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugRepair, AgentID: t.agent.ID, Name: n.Rule, Detail: n.Detail})
			}
		}
		resp, err := t.r.recordedComplete(t.ctx, t.agent, t.provider, t.req)
		if err != nil {
			// A1: a context-overflow error is recoverable once per turn by
			// compacting the in-flight history and retrying; a transient
			// provider fault (429/5xx/timeout) is retried after a backoff,
			// bounded by the budget; any other error ends the turn.
			// decideRecovery keeps this policy pure + testable.
			d := decideRecovery(nil, err, t.ls, t.cfg)
			if d.cont {
				t.ls.providerRetries++
				t.ls.lastContinue = d.reason
				rec := TurnStep{Kind: StepRecovery, Reason: string(d.reason), Text: recoveryText(d.reason)}
				t.steps = append(t.steps, rec)
				t.emit(rec)
				t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugRecovery, AgentID: t.agent.ID, Detail: string(d.reason) + ": " + err.Error()})
				// Backoff is ctx-aware: a user stop during the wait ends the
				// turn instead of firing one more doomed request.
				if !sleepCtx(t.ctx, d.backoff) {
					t.fail(string(termCancelled), t.ctx.Err())
					return nil, t.steps, t.ctx.Err()
				}
				continue
			}
			if d.compact {
				if t.compactAndRetry(d.reason) {
					continue
				}
			}
			t.fail(string(d.term), err)
			return nil, t.steps, err
		}
		t.last = resp
		t.turnUsage = sumUsage(t.turnUsage, resp.Usage)
		// The request that carried pending programmatic results has completed;
		// steering may resume (a new programmatic batch re-defers it below).
		t.pendingProgrammatic = false
		// PTC container chaining: while code execution is live, every follow-up
		// request of this turn must name the container (the API rejects a
		// continuation with pending programmatic calls but no container id).
		if resp.ContainerID != "" {
			t.req.ContainerID = resp.ContainerID
		}
		// Surface SERVER-executed steps (native tool-search discovery rides
		// resp.Trace as "tool" entries) so the chat UI shows them like any other
		// tool card. Client tool calls are traced by the loop itself below;
		// thinking entries can't occur here (thinking is off on the tool path).
		for _, ts := range resp.Trace {
			if ts.Kind != "tool" {
				continue
			}
			st := TurnStep{Kind: StepTool, Tool: ts.Tool, Input: ts.Input, Output: ts.Output}
			t.steps = append(t.steps, st)
			t.emit(st)
		}
		// pause_turn: the SERVER-side tool loop (native tool search) hit its
		// internal limit mid-turn. Echo the assistant content verbatim and
		// immediately re-request — the server detects the trailing server-tool
		// block and resumes where it left off (no extra user message). Bounded by
		// the surrounding iteration cap.
		if resp.StopReason == providers.StopPauseTurn && len(resp.ToolCalls) == 0 {
			t.req.Messages = append(t.req.Messages, providers.Message{
				Role:       providers.RoleAssistant,
				Text:       resp.Text,
				RawContent: t.rawEcho(resp.RawContent),
			})
			rec := TurnStep{Kind: StepRecovery, Reason: "pause_turn", Text: "Server-side tool run paused mid-turn; resuming automatically."}
			t.steps = append(t.steps, rec)
			t.emit(rec)
			continue
		}
		if resp.StopReason != providers.StopToolUse || len(resp.ToolCalls) == 0 {
			if done, fresp, ferr := t.finishTurn(resp); done {
				return fresp, t.steps, ferr
			}
			continue
		}
		t.ls.lastContinue = contToolUse

		// Capture the narration the model produced alongside this tool turn.
		if resp.Text != "" {
			st := TurnStep{Kind: StepText, Text: resp.Text}
			t.steps = append(t.steps, st)
			t.emit(st)
		}

		if stop, sresp, serr := t.runToolBatch(resp); stop {
			return sresp, t.steps, serr
		}
		// Phase 3: drop lazy tools activated but left unused for a while, so a long
		// turn does not keep shipping schemas the model is no longer reaching for.
		if pruned := t.active.Prune(activeToolMaxIdle); len(pruned) > 0 {
			t.r.logger.Info("pruned idle lazy tools", "agent", t.agent.ID, "tools", pruned)
		}
		t.activeLiveCards = nil
	}
	t.r.logger.Warn("tool loop hit iteration cap", "agent", t.agent.ID)
	rec := TurnStep{
		Kind:   StepRecovery,
		Reason: string(termMaxIters),
		Text:   "Araç döngüsü iterasyon limitine ulaştı; tur burada sonlandırıldı.",
	}
	t.steps = append(t.steps, rec)
	t.emit(rec)
	return t.last, t.steps, nil
}

// finishTurn handles a response that asked for no tools: either it is resumable
// (output cap / context overflow, done=false → the loop iterates again) or it is
// this turn's final answer, stitched and usage-stamped.
func (t *toolLoopTurn) finishTurn(resp *providers.Response) (done bool, out *providers.Response, err error) {
	// A1: resume an answer cut off by the output-token cap (bounded by
	// the guard) so the full reply is produced across capped calls.
	d := decideRecovery(resp, nil, t.ls, t.cfg)
	if d.cont {
		if resp.Text != "" {
			t.partial.WriteString(resp.Text)
			t.req.Messages = append(t.req.Messages, providers.Message{Role: providers.RoleAssistant, Text: resp.Text})
		}
		if d.inject != nil {
			t.req.Messages = append(t.req.Messages, *d.inject)
		}
		t.ls.maxTokenRetries++
		t.ls.lastContinue = d.reason
		rec := TurnStep{Kind: StepRecovery, Reason: string(d.reason), Text: recoveryText(d.reason)}
		t.steps = append(t.steps, rec)
		t.emit(rec)
		t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugRecovery, AgentID: t.agent.ID, Detail: string(d.reason)})
		return false, nil, nil
	}
	// Context window hit as a STOP REASON (Claude 4.5+): same one-shot
	// compact-and-retry as the error-shaped overflow above.
	if d.compact {
		if t.compactAndRetry(d.reason) {
			return false, nil, nil
		}
	}
	// Safety refusal (Fable-class classifiers; HTTP 200 + empty/partial
	// content): surface WHAT happened instead of a silent empty bubble.
	// With the server-side fallback enabled this only fires when the
	// fallback chain itself declined.
	if resp.StopReason == providers.StopRefusal {
		txt := "Güvenlik sınıflandırıcıları bu isteği reddetti; yanıt üretilmedi."
		if resp.StopDetails != nil && resp.StopDetails.Category != "" {
			txt += " Kategori: " + resp.StopDetails.Category + "."
		}
		if resp.StopDetails != nil && resp.StopDetails.Explanation != "" {
			txt += " " + resp.StopDetails.Explanation
		}
		st := TurnStep{Kind: StepError, Reason: string(providers.StopRefusal), Text: txt, IsError: true}
		t.steps = append(t.steps, st)
		t.emit(st)
		t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugError, AgentID: t.agent.ID, Detail: "refusal: " + txt, Err: true})
	}
	// A truncated reply (context window exhausted after compaction) must
	// say so in the trace rather than masquerading as a completed turn.
	if d.term == termContextExhausted {
		rec := TurnStep{Kind: StepRecovery, Reason: string(termContextExhausted), Text: "Bağlam penceresi doldu; yanıt bu noktada kesildi (sıkıştırma hakkı tükendi)."}
		t.steps = append(t.steps, rec)
		t.emit(rec)
	}
	// Stitch any earlier capped fragments onto the final answer.
	if t.partial.Len() > 0 {
		resp.Text = t.partial.String() + resp.Text
	}
	// Report the whole turn's token usage on the returned response (the bubble
	// + autonomous turn-meta read it), not just this final call's.
	resp.Usage = t.turnUsage
	return true, resp, nil
}

// runToolBatch records the assistant's tool-call turn, executes every call in it
// and appends the answering tool_result message. stop=true means the turn ends
// here (durable-ask suspend, cancellation, or a guardrail halt).
func (t *toolLoopTurn) runToolBatch(resp *providers.Response) (stop bool, out *providers.Response, err error) {
	// Record the assistant's tool-call turn, then execute and answer each.
	// RawContent carries the response's exact content array so server-side
	// blocks (native tool-search results) survive the echo; providers without
	// raw support render Text+ToolCalls instead.
	t.req.Messages = append(t.req.Messages, providers.Message{
		Role:       providers.RoleAssistant,
		Text:       resp.Text,
		ToolCalls:  resp.ToolCalls,
		RawContent: t.rawEcho(resp.RawContent),
	})
	// Parallel fan-out: when this batch holds multiple run_subagent calls, start
	// them concurrently up front; the loop below awaits each future in place
	// (results stay in tool_use order). nil when there is nothing to parallelise.
	// Live subagent cards are emitted from worker goroutines while the loop can
	// emit post-tool steps. Every path in this batch must share one serialized
	// emitter; wrapping either path again would risk double locking.
	// emit is serialized once for the whole turn, so every synchronous loop
	// step and every worker callback shares the same lock.
	safeEmit := t.emit
	b := &toolBatch{resp: resp}
	b.subFutures = t.r.launchParallelSubagents(t.ctx, t.reg, resp.ToolCalls, safeEmit)
	b.openCards = make(map[string]*liveCard, len(resp.ToolCalls))
	t.activeLiveCards = b.openCards
	for id, future := range b.subFutures {
		if future.card != nil {
			b.openCards[id] = future.card
		}
	}
	b.results = make([]providers.ToolResult, 0, len(resp.ToolCalls))
	// One multi-call response = one parallel batch: allocate its group id so
	// every step below (tool cards, permission/guardrail errors) carries it.
	if len(resp.ToolCalls) > 1 {
		t.batchSeq++
		b.batch = t.batchSeq
	}
	for _, call := range resp.ToolCalls {
		if s, sresp, serr := t.runToolCall(b, call); s {
			return true, sresp, serr
		}
	}
	// A programmatic batch (calls made from Claude's code) constrains the
	// answering message to PURE tool_result blocks and defers steering until
	// the code run has consumed the results.
	progBatch := false
	for _, c := range resp.ToolCalls {
		if c.Programmatic() {
			progBatch = true
			break
		}
	}
	t.req.Messages = append(t.req.Messages, providers.Message{
		Role:            providers.RoleUser,
		ToolResults:     b.results,
		OnlyToolResults: progBatch,
	})
	t.pendingProgrammatic = progBatch
	// Guardrail halt: the batch's results are all recorded (pairing holds),
	// so this is a clean, controlled turn end — not an error return. The
	// recovery step tells the transcript (and Faz D's stuck counter) why.
	if t.guardHaltReason != "" {
		t.r.logger.Warn("turn halted by loop guardrail", "agent", t.agent.ID, "reason", t.guardHaltReason)
		rec := TurnStep{
			Kind:   StepRecovery,
			Reason: string(termGuardrailHalt),
			Text:   "Araç döngüsü guardrail tarafından durduruldu: " + t.guardHaltReason,
		}
		t.steps = append(t.steps, rec)
		safeEmit(rec)
		return true, t.last, nil
	}
	return false, nil, nil
}

// runToolCall runs ONE tool call of a batch: hooks, permission gate, MCP repair /
// schema gate, loop guardrail, execution, post-hooks and the resulting step card.
// stop=true ends the whole turn (durable-ask suspend or cancellation); otherwise
// the call's result is recorded on b.results and the caller moves to the next
// call — the direct translation of this block's `continue` statements.
func (t *toolLoopTurn) runToolCall(b *toolBatch, call providers.ToolCall) (stop bool, out *providers.Response, err error) {
	resp := b.resp
	safeEmit := t.emit
	// Debug, not Info: a busy multi-turn session makes 100+ tool calls and
	// would otherwise dominate the 2000-entry ring buffer. Blocks/denials
	// below stay at Info — those are the actionable events.
	t.r.logger.Debug("tool call", "agent", t.agent.ID, "tool", call.Name)
	t.active.MarkUsed(call.Name) // reset idle age for pruning (Phase 3)

	// Durable Ask (MVP): at a CLEAN suspend point, a native ask_user call is
	// parked to disk and the turn returns an *askSuspend sentinel instead of
	// blocking a goroutine on the interactive asker (see ask_suspend.go). The
	// caller persists the state + opens a durable card; the answer endpoint
	// re-drives the loop via ResumeAsk. Gated by WithDurableAsk (only the
	// interactive chat turn runner sets it) so every other path keeps today's
	// blocking behavior. "Clean" = this ask is the sole call in its batch, no
	// parallel subagents are in flight, and no code-execution container is
	// active — so the resumable state is exactly the message history already
	// appended above (the assistant tool_use turn). Anything else falls through
	// to the ordinary (blocking) asker path below.
	cleanAskPoint := durableAskEnabled(t.ctx) && len(resp.ToolCalls) == 1 && b.subFutures == nil && t.req.ContainerID == ""
	if cleanAskPoint && call.Name == askUserToolName {
		return true, nil, &askSuspend{Kind: "ask", CallID: call.ID, Call: call, Payload: call.Input, Messages: t.req.Messages}
	}

	// PreToolUse hooks (Faz P4): user-defined commands may rewrite the
	// tool input, auto-approve the call (bypassing the permission gate) or
	// block it. A blocked call becomes an error result fed back to the
	// model. Runs before the permission gate so a hook can veto first.
	pre := t.r.runPreToolHooks(t.ctx, "", call)
	for _, st := range pre.steps {
		t.steps = append(t.steps, st)
		safeEmit(st)
	}
	if len(pre.input) > 0 {
		call.Input = pre.input
	}
	if pre.block {
		t.r.logger.Info("tool blocked by hook", "agent", t.agent.ID, "tool", call.Name)
		b.results = append(b.results, providers.ToolResult{CallID: call.ID, Content: pre.denyMsg, IsError: true})
		return false, nil, nil
	}

	// Permission gate: under read-only/ask the call may be blocked or need
	// user approval before it runs. A blocked call becomes an error result
	// fed back to the model (so it can adapt) instead of executing. A hook
	// that explicitly approved the call short-circuits the gate.
	allowed, denyMsg := true, ""
	if !pre.autoAllow {
		// Durable Ask (permission): at a clean point, a write/exec call that
		// would block on the approval prompter is parked to disk instead — the
		// permission analog of the ask_user suspend above. On approval the
		// resume EXECUTES this call (see resolveResumeResult). Non-clean points
		// and no-prompter (autonomous) runs fall through to the blocking gate.
		if cleanAskPoint && wouldPromptPermission(t.ctx, t.agent.PermissionMode, call) {
			return true, nil, &askSuspend{Kind: "permission", CallID: call.ID, Call: call, Payload: permissionCardPayload(call), Messages: t.req.Messages}
		}
		// Carry the logger so approvals / "always allow" grants leave an
		// audit trail in the Logs screen (denials are already logged below).
		allowed, denyMsg = permGate(withPermLogger(t.ctx, t.r.logger), t.agent.PermissionMode, call)
	}
	if !allowed {
		t.r.logger.Info("tool blocked", "agent", t.agent.ID, "tool", call.Name, "mode", t.agent.PermissionMode)
		b.results = append(b.results, providers.ToolResult{CallID: call.ID, Content: denyMsg, IsError: true})
		st := TurnStep{Kind: StepError, Tool: call.Name, Reason: "permission_denied", Text: denyMsg, IsError: true, Batch: b.batch}
		t.steps = append(t.steps, st)
		safeEmit(st)
		return false, nil, nil
	}

	// MCP not-indexed repair (pre-execution): refuse an identical repeat of
	// a call that already failed this turn because its `project` is unindexed.
	// Hitting the server again would return the same error; instead feed the
	// recovery instruction (call list_projects, copy an exact project id).
	if blocked, msg := t.repair.precheck(call); blocked {
		t.r.logger.Info("mcp call blocked by not-indexed repair", "agent", t.agent.ID, "tool", call.Name)
		t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugGuardrail, AgentID: t.agent.ID, Name: "mcp_repair_block", Detail: call.Name, Err: true})
		b.results = append(b.results, providers.ToolResult{CallID: call.ID, Content: msg, IsError: true})
		st := TurnStep{Kind: StepError, Tool: call.Name, Reason: "mcp_repair", Text: msg, IsError: true, Batch: b.batch}
		t.steps = append(t.steps, st)
		safeEmit(st)
		return false, nil, nil
	}

	// MCP schema gate (pre-execution): a call that omits a required argument
	// is either completed from context TionHarness already holds (the `project`
	// of a codebase-memory tool is the session's own repo) or refused here
	// with an accurate message. Letting it through means the model reads the
	// server's inference about an incomplete call, which for this server
	// reports a missing argument as an unindexed project.
	if missing := missingRequiredArgs(t.reg.MCPSchema(call.Name), call.Input); len(missing) > 0 {
		if fixed, ok := prefillMCPArgs(call, missing, t.r.sessionCwd(t.ctx)); ok {
			t.r.logger.Info("mcp call prefilled", "agent", t.agent.ID, "tool", call.Name, "args", strings.Join(missing, ","))
			t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugGuardrail, AgentID: t.agent.ID, Name: "mcp_prefill", Detail: call.Name})
			call = fixed
			missing = missingRequiredArgs(t.reg.MCPSchema(call.Name), call.Input)
		}
		if len(missing) > 0 {
			msg := missingArgsMessage(call.Name, missing)
			t.r.logger.Info("mcp call missing required args", "agent", t.agent.ID, "tool", call.Name, "args", strings.Join(missing, ","))
			t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugGuardrail, AgentID: t.agent.ID, Name: "mcp_args_block", Detail: call.Name, Err: true})
			b.results = append(b.results, providers.ToolResult{CallID: call.ID, Content: msg, IsError: true})
			st := TurnStep{Kind: StepError, Tool: call.Name, Reason: "mcp_args", Text: msg, IsError: true, Batch: b.batch}
			t.steps = append(t.steps, st)
			safeEmit(st)
			return false, nil, nil
		}
	}

	// Loop guardrail (pre-execution): a call past a block threshold is
	// refused with a synthetic error result (pairing invariant holds);
	// past the halt threshold the whole turn ends after this batch.
	// Hook/permission denials above intentionally never reach the
	// guardrail counters — only real executions are observed.
	if verdict, greason := t.guard.check(call); verdict != guardAllow {
		name := "block"
		if verdict == guardHalt {
			name = "halt"
			t.guardHaltReason = greason
		}
		t.r.logger.Warn("tool call blocked by loop guardrail", "agent", t.agent.ID, "tool", call.Name, "verdict", name, "reason", greason)
		t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugGuardrail, AgentID: t.agent.ID, Name: name, Detail: call.Name + ": " + greason, Err: true})
		denyMsg := blockedResultMsg(greason)
		b.results = append(b.results, providers.ToolResult{CallID: call.ID, Content: denyMsg, IsError: true})
		st := TurnStep{Kind: StepError, Tool: call.Name, Reason: "guardrail_" + name, Text: denyMsg, IsError: true, Batch: b.batch}
		t.steps = append(t.steps, st)
		safeEmit(st)
		return false, nil, nil
	}

	// Attach a per-call diff sink so file-mutating built-ins (write_file /
	// edit_file) can surface a structured diff for the UI card below. A
	// per-call subagent sink lets run_subagent hand back its nested trace so
	// the row is promoted to a collapsible StepSubagent.
	var card *liveCard
	if f := b.subFutures[call.ID]; f != nil && f.card != nil {
		card = f.card
		card.Update(func(st *TurnStep) {
			st.Input = call.Input
			st.Batch = b.batch
		})
	} else {
		card = openLive(safeEmit, call.ID, TurnStep{
			Kind:  StepTool,
			Tool:  call.Name,
			Input: call.Input,
			Batch: b.batch,
		})
	}
	b.openCards[call.ID] = card
	callCtx, diffs := tools.WithDiffSink(t.ctx)
	callCtx, subs := withSubStepSink(callCtx)
	// Sequential run_subagent: stream the delegation's nested steps live under
	// this call's id, so the card appears immediately and grows while it runs.
	if call.Name == "run_subagent" && call.ID != "" && b.subFutures[call.ID] == nil {
		subs.bindLive(card)
	}
	// Per-call optimizer sink: a shell tool whose output was shrunk by sqz
	// (or whose command was rtk-wrapped) reports it here, so the card can
	// show what the model actually received.
	callCtx, opts := tools.WithOptimizerSink(callCtx)

	// Stream long-running tool output into the already-open live card when
	// the tool and live sink support it.
	toolStart := time.Now()
	var res providers.ToolResult
	if f := b.subFutures[call.ID]; f != nil {
		// Parallel run_subagent: the runner was launched before the loop; wait
		// for it and adopt its result + nested trace (promoted to StepSubagent
		// below via the per-call sink). Verified nested steps feed the parent
		// tracker; synthetic heartbeat is deliberately absent.
		<-f.done
		res = f.res
		subs.setSteps(f.steps)
	} else if t.onStep != nil && call.ID != "" && t.reg.CanStream(call.Name) {
		// Streaming tool: its chunks touch the watchdog on every
		// chunk, so a stall is still caught on idle — no heartbeat here.
		res = t.reg.CallStream(callCtx, call, func(chunk string) {
			card.Chunk(chunk)
		})
	} else {
		// Non-streaming tool: bound the opaque call with an operation lease.
		var callErr error
		res, callErr = RunWithOperationLease(callCtx, func(leaseCtx context.Context) (providers.ToolResult, error) {
			return t.reg.Call(leaseCtx, call), nil
		})
		if callErr != nil {
			res.IsError = true
			res.Content = callErr.Error()
		}
	}
	// Debug journal: record this tool's latency, output size and outcome
	// (pre-compaction size, the true tool output) for optimisation.
	t.r.emitDebug(t.ctx, db.DebugEvent{
		Type:     db.DebugTool,
		AgentID:  t.agent.ID,
		Name:     call.Name,
		DurMs:    time.Since(toolStart).Milliseconds(),
		OutBytes: len(res.Content),
		Err:      res.IsError,
		Error:    debugToolError(res.Content, res.IsError),
		Args:     debugToolArgs(call.Input, res.IsError),
	})
	// Cancellation mid-tool (user stop / timeout): record it and end the
	// turn cleanly instead of feeding a half-result back to the model.
	// A3 (cancellation hierarchy): the assistant's tool_use turn was already
	// appended with every call in this batch, but only the calls processed
	// so far have results. Synthesize a 'cancelled' tool_result for the
	// interrupted call and any not-yet-run calls, then append the user turn,
	// so the in-flight history never carries a dangling tool_use (which the
	// provider rejects on any later replay / reactive compaction).
	if t.ctx.Err() != nil {
		cancelLiveCards(b.openCards)
		b.results = fillCancelledResults(b.results, resp.ToolCalls)
		t.req.Messages = append(t.req.Messages, providers.Message{
			Role:        providers.RoleUser,
			ToolResults: b.results,
		})
		t.fail(string(termCancelled), t.ctx.Err())
		return true, t.last, t.ctx.Err()
	}

	// Token optimization: there is NO built-in tool-output compaction
	// anymore (the deterministic "System A" was removed 2026-07-10). Shrinking
	// large outputs is delegated entirely to external tools — a PostToolUse
	// hook (e.g. sqz) below, or the agent invoking a wrapper like rtk at the
	// command layer. A raw result re-enters context untouched unless a hook
	// rewrites it.
	//
	// PostToolUse hooks (Faz P4): user-defined commands may rewrite the
	// output (e.g. external compression like sqz), append extra context, or
	// block the result — the only tool-output shrink path now.
	post := t.r.runPostToolHooks(t.ctx, "", call, res)
	for _, st := range post.steps {
		t.steps = append(t.steps, st)
		safeEmit(st)
	}
	if post.output != nil {
		res.Content = *post.output
	}
	if post.block {
		res.IsError = true
		if post.denyMsg != "" {
			res.Content = post.denyMsg
		}
	}
	if post.extra != "" {
		res.Content = strings.TrimSpace(res.Content + "\n\n" + post.extra)
	}

	// Loop guardrail (post-execution): update the counters and, when a
	// warning threshold was crossed, append the recovery guidance right
	// onto this result so the model reads it where the failure happened.
	if hint := t.guard.observe(call, res); hint != "" {
		res.Content += hint
		t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugGuardrail, AgentID: t.agent.ID, Name: "warn", Detail: call.Name})
	}
	// MCP not-indexed repair (post-execution). Three outcomes, cheapest first:
	// re-run the call with a corrected `project` (the model never pays a turn
	// for it), start a background index of the session's repo, or append the
	// recovery instruction and remember the call so an identical repeat is
	// refused above before it re-hits the server.
	if plan, ok := t.repair.repair(call, res, t.r.sessionCwd(t.ctx)); ok {
		switch {
		case plan.Fixed != nil:
			t.r.logger.Info("mcp call auto-repaired", "agent", t.agent.ID, "tool", call.Name, "project", callProjectArg(*plan.Fixed))
			t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugGuardrail, AgentID: t.agent.ID, Name: "mcp_repair_retry", Detail: call.Name})
			// Surface the otherwise-silent fix-up in the chat trace too,
			// not only in the debug journal.
			rec := mcpRepairStep(reasonMCPRepairRetry, call.Name, b.batch)
			t.steps = append(t.steps, rec)
			safeEmit(rec)
			retryStart := time.Now()
			res = t.reg.Call(callCtx, *plan.Fixed)
			t.r.emitDebug(t.ctx, db.DebugEvent{
				Type:     db.DebugTool,
				AgentID:  t.agent.ID,
				Name:     call.Name,
				DurMs:    time.Since(retryStart).Milliseconds(),
				OutBytes: len(res.Content),
				Err:      res.IsError,
			})
			// The corrected arguments become the recorded ones: the step card and
			// the model's history must show the call that actually produced this
			// result, not the one that failed.
			call.Input = plan.Fixed.Input
			// A retry that failed again gets the normal treatment (hint + poison),
			// so a broken repair degrades to the old behaviour instead of hiding.
			if plan2, ok2 := t.repair.repair(call, res, t.r.sessionCwd(t.ctx)); ok2 {
				res.Content += plan2.Hint
				if plan2.IndexPath != "" {
					t.r.EnsureCodebaseIndexed(t.ctx, plan2.IndexPath)
				}
			}
		default:
			res.Content += plan.Hint
			if plan.IndexPath != "" {
				t.r.EnsureCodebaseIndexed(t.ctx, plan.IndexPath)
				t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugGuardrail, AgentID: t.agent.ID, Name: "mcp_repair_index", Detail: plan.IndexPath})
				rec := mcpRepairStep(reasonMCPRepairIndex, plan.IndexPath, b.batch)
				t.steps = append(t.steps, rec)
				safeEmit(rec)
			}
			t.r.emitDebug(t.ctx, db.DebugEvent{Type: db.DebugGuardrail, AgentID: t.agent.ID, Name: "mcp_repair", Detail: call.Name, Err: true})
		}
	}

	b.results = append(b.results, res)
	st := TurnStep{
		Kind:    StepTool,
		Tool:    call.Name,
		Input:   call.Input,
		Output:  res.Content,
		IsError: res.IsError,
		Batch:   b.batch,
	}
	// Token-optimizer chip: recorded even on an errored shell call, since the
	// compression happened regardless and the user should see why the output
	// reads abbreviated.
	st.Optimizer = opts.Take()
	// The working checklist is a first-class step, not a generic tool row.
	// `set` updates carry the merged list in the result, not the input.
	if call.Name == "todo_write" && !res.IsError {
		if todos := todoStepItems(call.Input, res.Content); len(todos) > 0 {
			st.Kind = StepTodo
			st.Todos = todos
		}
	}
	// A file mutation renders as a diff card instead of a generic tool row.
	if d := diffs.Take(); d != nil && !res.IsError {
		st.Kind = StepDiff
		st.Tool = call.Name
		st.Path = d.Path
		st.Added = d.Added
		st.Removed = d.Removed
		st.Patch = d.Patch
		st.Created = d.Created
	}
	// A subagent run renders as a collapsible nested-agent card carrying the
	// subagent's own trace (input=target+task, output=its final reply).
	if subSteps := subs.collected(); len(subSteps) > 0 && !res.IsError {
		st.Kind = StepSubagent
		st.SubSteps = subSteps
	}
	st.ID = call.ID
	t.steps = append(t.steps, st)
	card.Close(st)
	delete(b.openCards, call.ID)
	return false, nil, nil
}
