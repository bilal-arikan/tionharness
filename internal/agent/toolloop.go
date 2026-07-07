package agent

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// defaultMaxToolIters bounds the native agentic loop so a misbehaving model can't
// spin forever calling tools. Set to 500 to give multi-step tool workflows
// (and schedule_wake-driven async flows) room to finish before the loop cap ends the turn.
const defaultMaxToolIters = 500

// maxToolIters is the live loop bound, defaulting to defaultMaxToolIters and
// overridable via TIONSWARM_MAX_TOOL_ITERS (positive integer) for power users who
// want longer or shorter native tool loops without a rebuild.
var maxToolIters = resolveMaxToolIters()

// resolveMaxToolIters reads the env override once at package init, falling back to
// the default for an unset, empty, non-numeric or non-positive value.
func resolveMaxToolIters() int {
	if v := os.Getenv("TIONSWARM_MAX_TOOL_ITERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxToolIters
}

// activeToolMaxIdle is how many iterations a lazily-activated tool may go unused
// before it is pruned from the shipped schema set (Phase 3 of lazy tool loading).
const activeToolMaxIdle = 3

// thinkingBudgetForLevel maps an agent's ThinkingLevel to a provider thinking
// token budget (0 = off). Providers without thinking support ignore it. The
// xhigh/max tiers exist as effort levels on adaptive-class models only; the
// legacy enabled+budget wire format clamps them down provider-side.
func thinkingBudgetForLevel(level string) int {
	switch level {
	case "low":
		return 2048
	case "medium":
		return 8192
	case "high":
		return 16384
	case "xhigh":
		return 32768
	case "max":
		return 65536
	default:
		return 0
	}
}

// resolveThinkingBudget resolves the agent's ThinkingLevel to a token budget.
// Model-class translation (adaptive vs legacy wire format, always-on models)
// now lives in the provider (providers.thinkingFor), so the budget passes
// through unchanged: 0 means "off" for every model class — on always-on models
// (Fable/Mythos 5) the provider simply omits the thinking field.
func resolveThinkingBudget(model, level string) int {
	_ = model // kept for call-site/test stability; translation moved provider-side
	return thinkingBudgetForLevel(level)
}

// CompleteWithTools runs a completion that may use tools. Behaviour depends on
// the agent and provider:
//
//   - MCP disabled            → a single plain completion.
//   - claude-cli + MCP        → delegate: the CLI runs the tool loop itself
//     using a generated --mcp-config (keyless path).
//   - other provider + MCP    → TionSwarm's own agentic loop drives the tools via
//     the unified registry (built-ins + MCP).
//
// autonomous gates the daily budget; usage is always recorded.
func (r *Runtime) CompleteWithTools(ctx context.Context, agent db.Agent, provider providers.Provider, req providers.Request, autonomous bool) (*providers.Response, error) {
	resp, _, err := r.CompleteWithToolsTraced(ctx, agent, provider, req, autonomous)
	return resp, err
}

// CompleteWithToolsTraced is CompleteWithTools plus an ordered activity trace
// (intermediate text + tool calls/results) for the rich chat turn renderer.
func (r *Runtime) CompleteWithToolsTraced(ctx context.Context, agent db.Agent, provider providers.Provider, req providers.Request, autonomous bool) (*providers.Response, []TurnStep, error) {
	return r.completeTraced(ctx, agent, provider, req, autonomous, nil)
}

// CompleteWithToolsStream is CompleteWithToolsTraced that additionally delivers
// each step to onStep the moment it becomes available — for SSE streaming the
// chat turn to the UI step-by-step. The returned slice is the full trace (for
// persistence). onStep is called from the calling goroutine.
func (r *Runtime) CompleteWithToolsStream(ctx context.Context, agent db.Agent, provider providers.Provider, req providers.Request, autonomous bool, onStep func(TurnStep)) (*providers.Response, []TurnStep, error) {
	return r.completeTraced(ctx, agent, provider, req, autonomous, onStep)
}

// effectivePermissionMode resolves the permission mode a turn actually runs under.
// On a chat turn it is the agent's own mode. On an autonomous turn (scheduler/
// spawn/flow) there is no live user to answer a permission prompt, so "ask" mode
// would stall or fail — the CLI's permission-prompt tool returns an is_error "no
// interactive session", which a claude-cli child can turn into an exit-1 crash.
// So autonomous turns are forced to unattended "auto" (risky tools auto-approved,
// nothing blocks on a human), EXCEPT "read-only", which is non-interactive and a
// deliberate no-mutation guard worth preserving.
func effectivePermissionMode(mode string, autonomous bool) string {
	if autonomous && mode != "read-only" {
		return "auto"
	}
	return mode
}

// completeTraced is the shared implementation. When onStep is non-nil, steps are
// emitted live: provider-driven paths (claude CLI) wire it through req.OnEvent;
// the native loop emits as it appends.
// completeTraced wraps the turn machinery with a debug-journal turn boundary: it
// times the whole turn and appends one "turn" event (duration + stop reason +
// error) to the session's debug.jsonl. It covers every path (native loop,
// claude-cli, plain completion) since they all funnel through completeTracedInner.
func (r *Runtime) completeTraced(ctx context.Context, agent db.Agent, provider providers.Provider, req providers.Request, autonomous bool, onStep func(TurnStep)) (*providers.Response, []TurnStep, error) {
	start := time.Now()
	resp, steps, err := r.completeTracedInner(ctx, agent, provider, req, autonomous, onStep)
	ev := db.DebugEvent{Type: db.DebugTurn, AgentID: agent.ID, DurMs: time.Since(start).Milliseconds()}
	if resp != nil {
		ev.Stop = string(resp.StopReason)
	}
	if err != nil {
		ev.Err = true
		ev.Detail = err.Error()
	}
	r.emitDebug(ctx, ev)
	// Autonomous callers install a turn-meta sink on ctx; record this completion's
	// model/stop-reason/usage so they can stamp it onto the persisted assistant
	// message (chat paths read it from the returned response directly instead).
	if err == nil {
		turnMetaFrom(ctx).capture(resp)
	}
	return resp, steps, err
}

func (r *Runtime) completeTracedInner(ctx context.Context, agent db.Agent, provider providers.Provider, req providers.Request, autonomous bool, onStep func(TurnStep)) (*providers.Response, []TurnStep, error) {
	// Stuck-session gate (self-healing Faz D): a session whose consecutive
	// bad-turn counter crossed the threshold gets no further AUTONOMOUS turns —
	// unattended retries of a failing session only burn budget. Manual chat is
	// never gated (a human driving the session is exactly how it recovers), and
	// a clean manual turn resets the counter via AutoTagTurn.
	if autonomous {
		if err := r.stuckGate(ctx); err != nil {
			return nil, nil, err
		}
	}
	// Carry the agent's permission mode so provider-driven loops (claude CLI) can
	// gate their tool use. Empty maps to "auto" downstream.
	req.PermissionMode = effectivePermissionMode(agent.PermissionMode, autonomous)

	// Announce the configured task budget to autonomous turns (API-native
	// output_config.task_budget): the model sees a running countdown for the
	// whole loop and paces itself. Providers/models without support ignore it;
	// interactive chat turns stay un-budgeted (a human is watching).
	if autonomous {
		req.TaskBudgetTokens = r.tun.AutonomousTaskBudget()
	}

	// Resolve this turn's working directory: the session's WorkingDir override
	// (else the workspace default). Autonomous turns may additionally get an
	// isolated per-session git worktree. The result roots the fs/shell sandbox
	// (carried via ctx into buildRegistry) and is the cwd for provider-driven CLI
	// subprocesses (claude-cli) so relative paths — e.g. an attachment's
	// "uploads/<sid>/<file>" — resolve there. Native providers ignore req.WorkDir.
	workDir := r.effectiveWorkDir(ctx)
	if autonomous && r.tun.GitWorktreeIsolation() {
		workDir = r.ensureWorktree(ctx, workDir, SessionIDFrom(ctx))
	}
	req.WorkDir = workDir
	ctx = withResolvedWorkDir(ctx, workDir, autonomous)

	// Expose the current session id to session-scoped tools (read_session_debug)
	// so they default to the running session without an explicit arg.
	if sid := SessionIDFrom(ctx); sid != "" {
		ctx = tools.WithCurrentSession(ctx, sid)
	}

	// Artifacts everywhere: chat turns install a session-bound sink before calling
	// in; autonomous turns (scheduler/spawn/flow) don't, so create_artifact would
	// fail with "artifacts are not available for this turn". Install a fallback sink
	// here whenever the turn has a session but no sink yet — so artifacts can always
	// be created, regardless of turn type. (CLI turns get theirs via the Interaction
	// bridge run; this covers the native loop.)
	if sid := SessionIDFrom(ctx); sid != "" && !tools.HasArtifactSink(ctx) {
		ctx = tools.WithArtifacts(ctx, r.NewArtifactSink(sid, agent.ID))
	}

	// Persistent progress: when todo_write runs, persist the checklist to the
	// project's progress file so it survives across sessions (Claude Code's
	// claude-progress convention). Install a fallback sink whenever the turn has a
	// session but no sink yet — covering native chat + autonomous (scheduler/spawn/
	// flow) turns. (CLI turns get theirs via the Interaction bridge run.) Gated by
	// the ProgressPersist setting (default on); workDir was resolved just above.
	if sid := SessionIDFrom(ctx); sid != "" && r.tun.ProgressPersist() && !tools.HasTodoSink(ctx) {
		ctx = tools.WithTodoSink(ctx, r.NewTodoSink(sid, agent.ID))
	}

	// Provider-driven paths (claude CLI) surface their own trace via OnEvent.
	if onStep != nil {
		req.OnEvent = func(ts providers.TraceStep) { onStep(traceStepToTurnStep(ts)) }
	}

	// The claude CLI runs its own tool loop. Route it through the keyless MCP
	// delegation path when external MCP is enabled OR an Interaction MCP endpoint
	// is wired for this turn (so ask_user/todo_write work even with MCP off).
	cli, isCLI := provider.(*providers.ClaudeCLI)
	// Point the CLI at THIS workspace's config home (<workspace>/claude-home) so it
	// reads the same skills/settings/login as the workspace instead of one global
	// home. No-op when the workspace dir is unknown (keeps the provider's global
	// default). This is the single per-turn seam every CLI turn passes through.
	if isCLI {
		cli.SetConfigDir(r.claudeHomeDir())
	}
	inter := tools.InteractionFrom(ctx)
	// CLI turns that arrive without an Interaction endpoint — autonomous ones
	// (scheduler/spawn/flow) AND the non-stream /api/chat path (only /api/chat/stream
	// registers a run) — can't reach the bridged use_skill/shell/self-manage/
	// coordination tools and fall back to native (now-disallowed/foreign) ones: the
	// cause of scheduled "Unknown skill", the POSIX-Bash mismatch, and a coordinator
	// fanning out via the CLI's own Agent tool instead of spawn_worker. Wire the
	// headless endpoint on demand for ANY such CLI turn; skipped whenever one is
	// already present (the stream path installs its own).
	if isCLI && inter.URL == "" && r.autoInteract != nil {
		var done func()
		ctx, done = r.autoInteract(ctx, agent, SessionIDFrom(ctx))
		defer done()
		inter = tools.InteractionFrom(ctx)
	}
	cliMCP := isCLI && (agent.MCPEnabled || inter.URL != "")

	if !agent.MCPEnabled && !cliMCP {
		// Extended reasoning is applied only on the plain (non-tool) path: the
		// native tool loop would need to echo signed thinking blocks back, which
		// the provider abstraction doesn't preserve. Providers without thinking
		// support (claude-cli, minimax) ignore the budget.
		req.ThinkingBudget = resolveThinkingBudget(agent.Model, agent.ThinkingLevel)

		// Prefer first-class token streaming when a live sink is present and the
		// provider supports it (anthropic/minimax). claude-cli is not a Streamer;
		// it streams its own trace via req.OnEvent wired above.
		if onStep != nil {
			if sm, ok := provider.(providers.Streamer); ok {
				resp, err := r.recordedStream(ctx, agent, sm, req, onStep)
				if err != nil {
					return nil, nil, err
				}
				// Text deltas are transient (recovered from resp.Text); the
				// thinking trace, if any, is persisted so the reasoning block
				// survives reload.
				return resp, traceToSteps(resp.Trace), nil
			}
		}
		resp, err := r.recordedComplete(ctx, agent, provider, req)
		if err != nil {
			return nil, nil, err
		}
		// claude-cli surfaces its own tool/thinking trace via stream-json.
		r.emitCLIToolDebug(ctx, agent, resp.Trace)
		// Guardrail visibility parity: flag looping CLI turns post-hoc.
		r.analyzeCLIGuardrail(ctx, agent, resp.Trace)
		return resp, traceToSteps(resp.Trace), nil
	}

	// Keyless delegation path: let the claude CLI own the tool loop, wiring the
	// external MCP servers (when enabled) plus the Interaction MCP server (when an
	// endpoint is present) into a single generated --mcp-config.
	if cliMCP {
		path, allowed, disallowed, cleanup, err := r.writeCLIMCPConfig(ctx, agent.MCPEnabled, inter, agent.PermissionMode)
		if err != nil {
			r.logger.Warn("cli mcp config failed", "error", err)
		} else if path != "" {
			defer cleanup()
			// Per-turn --settings: permission deny-list (mirrors disallowed) plus the
			// workspace's PreToolUse/PostToolUse hooks, so the CLI's own loop honours
			// the same blocks/hooks the native loop does. "" when there is nothing.
			settingsPath, settingsCleanup, serr := r.writeCLISettings(ctx, disallowed)
			if serr != nil {
				r.logger.Warn("cli settings write failed", "error", serr)
			}
			defer settingsCleanup()
			// In "ask" mode route risky CLI tools through the Interaction MCP
			// permission-prompt tool (real per-tool approval) instead of acceptEdits.
			cli.ConfigureMCP(path, allowed, disallowed, promptToolForMode(agent.PermissionMode, inter), settingsPath)
		}
		resp, err := r.recordedComplete(ctx, agent, provider, req)
		if err != nil {
			return nil, nil, err
		}
		// The CLI runs the loop itself; its stream-json trace becomes our steps.
		r.emitCLIToolDebug(ctx, agent, resp.Trace)
		// Guardrail visibility parity: flag looping CLI turns post-hoc.
		r.analyzeCLIGuardrail(ctx, agent, resp.Trace)
		return resp, traceToSteps(resp.Trace), nil
	}

	// Native agentic loop (providers that return structured tool_use). OnEvent
	// is not used here — we emit each step ourselves as the loop progresses.
	req.OnEvent = nil
	// Lazy tool loading: a per-turn active set tracks which on-demand (lazy) tools
	// the model has activated. buildRegistry wires the activate_tools meta-tools to
	// this same set (via ctx); req.Tools is recomputed each iteration so a freshly
	// activated tool's schema is shipped on the next step.
	active := tools.NewActiveTools()
	ctx = withActiveTools(ctx, active)
	// Coordinator/worker tools (M2): wired only when this turn runs on a coordinator
	// session. Injected BEFORE buildRegistry so the registry can gate their
	// registration on the runner's presence (so ordinary/worker sessions never see
	// them). No-op on every other turn.
	ctx = r.withCoordination(ctx, agent)
	reg := r.buildRegistry(ctx, agent)
	if reg.Empty() {
		resp, err := r.recordedComplete(ctx, agent, provider, req)
		return resp, nil, err
	}
	toolFilter := r.toolFilter(ctx, agent)
	// Native (server-side) tool search — first-party anthropic only: the full
	// catalog ships with lazy tools marked defer_loading + the search server
	// tool, so discovery needs no activate_tools round-trip and the tools block
	// stays byte-stable across iterations. Anthropic-protocol lookalikes
	// (minimax-anthropic, custom endpoints) would reject the server tool type,
	// hence the exact-name gate. Everything else keeps the activation flow.
	nativeSearch := r.tun.NativeToolSearch() && provider.Name() == "anthropic"
	// Programmatic tool calling (PTC): the code-execution server tool lets the
	// model call code-callable tools from Python inside Anthropic's container —
	// intermediate results never enter context. First-party anthropic only.
	ptcMode := r.tun.ProgrammaticTools() && provider.Name() == "anthropic"
	// Server-side web search + fetch (first-party anthropic only): declared in
	// the tools array, executed on Anthropic's infrastructure, results ride the
	// same response with citations.
	webMode := r.tun.WebTools() && provider.Name() == "anthropic"
	// API-native compaction (beta, applied provider-side): the loop only needs
	// to know so the verbatim echo below preserves compaction blocks.
	serverCompact := r.tun.ServerCompaction() && provider.Name() == "anthropic"
	shipDefs := func() []providers.ToolDef {
		var defs []providers.ToolDef
		if nativeSearch {
			defs = reg.DeferredDefs(toolFilter, active.Snapshot())
		} else {
			defs = reg.ActiveDefs(toolFilter, active.Snapshot())
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
	req.Tools = shipDefs()
	req.ProgrammaticTools = ptcMode
	req.WebTools = webMode
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
	rawEcho := func(raw json.RawMessage) json.RawMessage {
		if nativeSearch || ptcMode || webMode || serverCompact || providers.AlwaysOnThinking(agent.Model) {
			return raw
		}
		return nil
	}

	// Wire the generic subagent runner for this turn: the run_subagent tool reads
	// it (and the shared loop guards) from the context. &req lets an inherited-
	// context subagent see the conversation as it stands when the tool fires.
	ctx = r.withRunAgent(ctx, agent, &req, autonomous)

	emit := func(s TurnStep) {
		if onStep != nil {
			onStep(s)
		}
	}

	var last *providers.Response
	// turnUsage sums the token usage of EVERY provider call this turn makes (each
	// tool-loop iteration + every max-output resume) so the returned response — and
	// thus the persisted assistant bubble — reflects the whole turn, not just the
	// final call. RecordUsage already sums per-call into the daily/session rollups;
	// this keeps the per-message figure consistent with those totals (otherwise a
	// multi-step native turn showed only the last iteration's tokens, so the bubbles
	// summed to less than the session lifetime total). The claude-cli path doesn't
	// reach here (one Complete; its usage is already the turn aggregate).
	var turnUsage providers.Usage
	var steps []TurnStep
	// ls carries the single-shot recovery guards (A1) across iterations so a
	// stuck model can never spin forever inside one turn; cfg/keepRecent are the
	// resolved, settings-driven recovery policy for this turn.
	var ls loopState
	cfg := recoveryConfig{
		maxTokenLimit:      r.tun.MaxTokenRetries(),
		reactiveCompact:    r.tun.ReactiveCompact(),
		maxProviderRetries: r.tun.ProviderRetryMax(),
	}
	keepRecent := r.tun.ReactiveKeepRecent()
	// Tool-loop guardrail (self-healing Faz B): per-turn loop detection. Warnings
	// ride the failing tool results; block/halt fire only when the hard stop is
	// enabled in settings.
	gew, geb, gsw, gsh, gnw, gnb := r.tun.ToolGuardThresholds()
	guard := newToolGuard(toolGuardConfig{
		warnings:       r.tun.ToolGuardWarnings(),
		hardStop:       r.tun.ToolGuardHardStop(),
		exactWarn:      gew,
		exactBlock:     geb,
		sameToolWarn:   gsw,
		sameToolHalt:   gsh,
		noProgressWarn: gnw,
		noProgressBlck: gnb,
	})
	guardHaltReason := ""
	// partial accumulates answer text across max-output-token resumes, so the
	// stitched full answer is returned even though it arrived in capped pieces.
	var partial strings.Builder
	// fail records a turn-level error as an inline step before the loop returns.
	fail := func(reason string, err error) {
		st := TurnStep{Kind: StepError, Reason: reason, Text: err.Error(), IsError: true}
		steps = append(steps, st)
		emit(st)
		r.emitDebug(ctx, db.DebugEvent{Type: db.DebugError, AgentID: agent.ID, Detail: reason + ": " + err.Error(), Err: true})
	}
	// Steer messages ride the operator channel ({"role":"system"} in messages)
	// on models that support it — cache-safe, non-spoofable, and valid between a
	// tool_result user turn and the next assistant turn. Elsewhere they stay
	// user-role (the provider folds them to keep alternation intact).
	steerRole := providers.RoleUser
	if provider.Name() == "anthropic" && providers.SupportsSystemInMessages(agent.Model) {
		steerRole = providers.RoleSystem
	}
	// pendingProgrammatic defers steering while a programmatic tool batch awaits
	// its results: that request's trailing message must stay PURE tool_results,
	// so queued guidance is delivered on the next ordinary iteration instead.
	pendingProgrammatic := false
	for i := 0; i < maxToolIters; i++ {
		// Live steering: fold any user guidance that arrived since the last
		// iteration into the conversation before the next model call.
		if !pendingProgrammatic {
			for _, m := range drainSteer(ctx) {
				req.Messages = append(req.Messages, providers.Message{Role: steerRole, Text: steerPrefix + m})
				st := TurnStep{Kind: StepSteer, Text: m}
				steps = append(steps, st)
				emit(st)
			}
		}
		// Recompute the shipped tool schemas for this step: eager tools plus any
		// lazy tools activated so far (native-search mode: full deferred catalog,
		// byte-stable apart from activations). Cheap; reflects activate/deactivate
		// calls from the previous iteration.
		active.SetIter(i)
		req.Tools = shipDefs()
		// Self-healing: enforce the tool_use↔tool_result pairing invariants on
		// the in-flight history before every provider call. A well-formed slice
		// passes through untouched; a healed one is logged + journaled (never
		// silent), instead of surfacing as an opaque provider 400.
		if repaired, notes := conversation.RepairSequence(req.Messages); len(notes) > 0 {
			req.Messages = repaired
			for _, n := range notes {
				r.logger.Warn("message sequence repaired", "agent", agent.ID, "rule", n.Rule, "detail", n.Detail)
				r.emitDebug(ctx, db.DebugEvent{Type: db.DebugRepair, AgentID: agent.ID, Name: n.Rule, Detail: n.Detail})
			}
		}
		resp, err := r.recordedComplete(ctx, agent, provider, req)
		if err != nil {
			// A1: a context-overflow error is recoverable once per turn by
			// compacting the in-flight history and retrying; a transient
			// provider fault (429/5xx/timeout) is retried after a backoff,
			// bounded by the budget; any other error ends the turn.
			// decideRecovery keeps this policy pure + testable.
			d := decideRecovery(nil, err, ls, cfg)
			if d.cont {
				ls.providerRetries++
				ls.lastContinue = d.reason
				rec := TurnStep{Kind: StepRecovery, Reason: string(d.reason), Text: recoveryText(d.reason)}
				steps = append(steps, rec)
				emit(rec)
				r.emitDebug(ctx, db.DebugEvent{Type: db.DebugRecovery, AgentID: agent.ID, Detail: string(d.reason) + ": " + err.Error()})
				// Backoff is ctx-aware: a user stop during the wait ends the
				// turn instead of firing one more doomed request.
				if !sleepCtx(ctx, d.backoff) {
					fail(string(termCancelled), ctx.Err())
					return nil, steps, ctx.Err()
				}
				continue
			}
			if d.compact {
				cctx := conversation.WithCompactPrompt(ctx, r.CompactPromptTemplate())
				folded, ok, cerr := conversation.CompactInFlightMessages(cctx, r.db, provider, agent, req.Messages, keepRecent)
				if cerr == nil && ok {
					req.Messages = folded
					ls.compacted = true
					ls.lastContinue = d.reason
					// Signal the autonomous caller that this turn hit the context
					// limit, so it can decide on an automatic context-reset handoff.
					markContextOverflow(ctx)
					rec := TurnStep{Kind: StepRecovery, Reason: string(d.reason), Text: recoveryText(d.reason)}
					steps = append(steps, rec)
					emit(rec)
					r.emitDebug(ctx, db.DebugEvent{Type: db.DebugCompaction, AgentID: agent.ID, Detail: string(d.reason)})
					continue
				}
			}
			fail(string(d.term), err)
			return nil, steps, err
		}
		last = resp
		turnUsage = sumUsage(turnUsage, resp.Usage)
		// The request that carried pending programmatic results has completed;
		// steering may resume (a new programmatic batch re-defers it below).
		pendingProgrammatic = false
		// PTC container chaining: while code execution is live, every follow-up
		// request of this turn must name the container (the API rejects a
		// continuation with pending programmatic calls but no container id).
		if resp.ContainerID != "" {
			req.ContainerID = resp.ContainerID
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
			steps = append(steps, st)
			emit(st)
		}
		// pause_turn: the SERVER-side tool loop (native tool search) hit its
		// internal limit mid-turn. Echo the assistant content verbatim and
		// immediately re-request — the server detects the trailing server-tool
		// block and resumes where it left off (no extra user message). Bounded by
		// the surrounding iteration cap.
		if resp.StopReason == providers.StopPauseTurn && len(resp.ToolCalls) == 0 {
			req.Messages = append(req.Messages, providers.Message{
				Role:       providers.RoleAssistant,
				Text:       resp.Text,
				RawContent: rawEcho(resp.RawContent),
			})
			rec := TurnStep{Kind: StepRecovery, Reason: "pause_turn", Text: "Server-side tool run paused mid-turn; resuming automatically."}
			steps = append(steps, rec)
			emit(rec)
			continue
		}
		if resp.StopReason != providers.StopToolUse || len(resp.ToolCalls) == 0 {
			// A1: resume an answer cut off by the output-token cap (bounded by
			// the guard) so the full reply is produced across capped calls.
			d := decideRecovery(resp, nil, ls, cfg)
			if d.cont {
				if resp.Text != "" {
					partial.WriteString(resp.Text)
					req.Messages = append(req.Messages, providers.Message{Role: providers.RoleAssistant, Text: resp.Text})
				}
				if d.inject != nil {
					req.Messages = append(req.Messages, *d.inject)
				}
				ls.maxTokenRetries++
				ls.lastContinue = d.reason
				rec := TurnStep{Kind: StepRecovery, Reason: string(d.reason), Text: recoveryText(d.reason)}
				steps = append(steps, rec)
				emit(rec)
				r.emitDebug(ctx, db.DebugEvent{Type: db.DebugRecovery, AgentID: agent.ID, Detail: string(d.reason)})
				continue
			}
			// Context window hit as a STOP REASON (Claude 4.5+): same one-shot
			// compact-and-retry as the error-shaped overflow above.
			if d.compact {
				cctx := conversation.WithCompactPrompt(ctx, r.CompactPromptTemplate())
				folded, ok, cerr := conversation.CompactInFlightMessages(cctx, r.db, provider, agent, req.Messages, keepRecent)
				if cerr == nil && ok {
					req.Messages = folded
					ls.compacted = true
					ls.lastContinue = d.reason
					markContextOverflow(ctx)
					rec := TurnStep{Kind: StepRecovery, Reason: string(d.reason), Text: recoveryText(d.reason)}
					steps = append(steps, rec)
					emit(rec)
					r.emitDebug(ctx, db.DebugEvent{Type: db.DebugCompaction, AgentID: agent.ID, Detail: string(d.reason)})
					continue
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
				steps = append(steps, st)
				emit(st)
				r.emitDebug(ctx, db.DebugEvent{Type: db.DebugError, AgentID: agent.ID, Detail: "refusal: " + txt, Err: true})
			}
			// A truncated reply (context window exhausted after compaction) must
			// say so in the trace rather than masquerading as a completed turn.
			if d.term == termContextExhausted {
				rec := TurnStep{Kind: StepRecovery, Reason: string(termContextExhausted), Text: "Bağlam penceresi doldu; yanıt bu noktada kesildi (sıkıştırma hakkı tükendi)."}
				steps = append(steps, rec)
				emit(rec)
			}
			// Stitch any earlier capped fragments onto the final answer.
			if partial.Len() > 0 {
				resp.Text = partial.String() + resp.Text
			}
			// Report the whole turn's token usage on the returned response (the bubble
			// + autonomous turn-meta read it), not just this final call's.
			resp.Usage = turnUsage
			return resp, steps, nil
		}
		ls.lastContinue = contToolUse

		// Capture the narration the model produced alongside this tool turn.
		if resp.Text != "" {
			st := TurnStep{Kind: StepText, Text: resp.Text}
			steps = append(steps, st)
			emit(st)
		}

		// Record the assistant's tool-call turn, then execute and answer each.
		// RawContent carries the response's exact content array so server-side
		// blocks (native tool-search results) survive the echo; providers without
		// raw support render Text+ToolCalls instead.
		req.Messages = append(req.Messages, providers.Message{
			Role:       providers.RoleAssistant,
			Text:       resp.Text,
			ToolCalls:  resp.ToolCalls,
			RawContent: rawEcho(resp.RawContent),
		})
		// Parallel fan-out: when this batch holds multiple run_subagent calls, start
		// them concurrently up front; the loop below awaits each future in place
		// (results stay in tool_use order). nil when there is nothing to parallelise.
		subFutures := r.launchParallelSubagents(ctx, reg, resp.ToolCalls)
		results := make([]providers.ToolResult, 0, len(resp.ToolCalls))
		for _, call := range resp.ToolCalls {
			r.logger.Info("tool call", "agent", agent.ID, "tool", call.Name)
			active.MarkUsed(call.Name) // reset idle age for pruning (Phase 3)

			// PreToolUse hooks (Faz P4): user-defined commands may rewrite the
			// tool input, auto-approve the call (bypassing the permission gate) or
			// block it. A blocked call becomes an error result fed back to the
			// model. Runs before the permission gate so a hook can veto first.
			pre := r.runPreToolHooks(ctx, "", call)
			for _, st := range pre.steps {
				steps = append(steps, st)
				emit(st)
			}
			if len(pre.input) > 0 {
				call.Input = pre.input
			}
			if pre.block {
				r.logger.Info("tool blocked by hook", "agent", agent.ID, "tool", call.Name)
				results = append(results, providers.ToolResult{CallID: call.ID, Content: pre.denyMsg, IsError: true})
				continue
			}

			// Permission gate: under read-only/ask the call may be blocked or need
			// user approval before it runs. A blocked call becomes an error result
			// fed back to the model (so it can adapt) instead of executing. A hook
			// that explicitly approved the call short-circuits the gate.
			allowed, denyMsg := true, ""
			if !pre.autoAllow {
				// Carry the logger so approvals / "always allow" grants leave an
				// audit trail in the Logs screen (denials are already logged below).
				allowed, denyMsg = permGate(withPermLogger(ctx, r.logger), agent.PermissionMode, call)
			}
			if !allowed {
				r.logger.Info("tool blocked", "agent", agent.ID, "tool", call.Name, "mode", agent.PermissionMode)
				results = append(results, providers.ToolResult{CallID: call.ID, Content: denyMsg, IsError: true})
				st := TurnStep{Kind: StepError, Tool: call.Name, Reason: "permission_denied", Text: denyMsg, IsError: true}
				steps = append(steps, st)
				emit(st)
				continue
			}

			// Loop guardrail (pre-execution): a call past a block threshold is
			// refused with a synthetic error result (pairing invariant holds);
			// past the halt threshold the whole turn ends after this batch.
			// Hook/permission denials above intentionally never reach the
			// guardrail counters — only real executions are observed.
			if verdict, greason := guard.check(call); verdict != guardAllow {
				name := "block"
				if verdict == guardHalt {
					name = "halt"
					guardHaltReason = greason
				}
				r.logger.Warn("tool call blocked by loop guardrail", "agent", agent.ID, "tool", call.Name, "verdict", name, "reason", greason)
				r.emitDebug(ctx, db.DebugEvent{Type: db.DebugGuardrail, AgentID: agent.ID, Name: name, Detail: call.Name + ": " + greason, Err: true})
				denyMsg := blockedResultMsg(greason)
				results = append(results, providers.ToolResult{CallID: call.ID, Content: denyMsg, IsError: true})
				st := TurnStep{Kind: StepError, Tool: call.Name, Reason: "guardrail_" + name, Text: denyMsg, IsError: true}
				steps = append(steps, st)
				emit(st)
				continue
			}

			// Attach a per-call diff sink so file-mutating built-ins (write_file /
			// edit_file) can surface a structured diff for the UI card below. A
			// per-call subagent sink lets run_subagent hand back its nested trace so
			// the row is promoted to a collapsible StepSubagent.
			callCtx, diffs := tools.WithDiffSink(ctx)
			callCtx, subs := withSubStepSink(callCtx)

			// Stream long-running tool output live as tool_delta chunks (keyed by
			// the call id) when the tool and the live sink both support it.
			toolStart := time.Now()
			var res providers.ToolResult
			streamed := false
			if f := subFutures[call.ID]; f != nil {
				// Parallel run_subagent: the runner was launched before the loop; wait
				// for it and adopt its result + nested trace (promoted to StepSubagent
				// below via the per-call sink).
				<-f.done
				res = f.res
				subs.steps = f.steps
			} else if onStep != nil && call.ID != "" && reg.CanStream(call.Name) {
				res = reg.CallStream(callCtx, call, func(chunk string) {
					streamed = true
					emit(TurnStep{Kind: StepToolDelta, ID: call.ID, Tool: call.Name, Output: chunk})
				})
			} else {
				res = reg.Call(callCtx, call)
			}
			// Retract the live streaming placeholder; the final card (or the
			// cancellation error below) takes its place.
			if streamed {
				emit(TurnStep{Kind: StepTombstone, Ref: call.ID})
			}
			// Debug journal: record this tool's latency, output size and outcome
			// (pre-compaction size, the true tool output) for optimisation.
			r.emitDebug(ctx, db.DebugEvent{
				Type:     db.DebugTool,
				AgentID:  agent.ID,
				Name:     call.Name,
				DurMs:    time.Since(toolStart).Milliseconds(),
				OutBytes: len(res.Content),
				Err:      res.IsError,
			})
			// Cancellation mid-tool (user stop / timeout): record it and end the
			// turn cleanly instead of feeding a half-result back to the model.
			// A3 (cancellation hierarchy): the assistant's tool_use turn was already
			// appended with every call in this batch, but only the calls processed
			// so far have results. Synthesize a 'cancelled' tool_result for the
			// interrupted call and any not-yet-run calls, then append the user turn,
			// so the in-flight history never carries a dangling tool_use (which the
			// provider rejects on any later replay / reactive compaction).
			if ctx.Err() != nil {
				results = fillCancelledResults(results, resp.ToolCalls)
				req.Messages = append(req.Messages, providers.Message{
					Role:        providers.RoleUser,
					ToolResults: results,
				})
				fail(string(termCancelled), ctx.Err())
				return last, steps, ctx.Err()
			}

			// Token optimization: shrink the result before it re-enters context
			// (and the persisted step) via the two independent compaction systems.
			res = r.compactToolResult(ctx, agent, call.Name, call.Input, res)

			// PostToolUse hooks (Faz P4): user-defined commands may rewrite the
			// output (e.g. external compression like sqz), append extra context, or
			// block the result. Runs after compaction so a hook sees the final text.
			post := r.runPostToolHooks(ctx, "", call, res)
			for _, st := range post.steps {
				steps = append(steps, st)
				emit(st)
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
			if hint := guard.observe(call, res); hint != "" {
				res.Content += hint
				r.emitDebug(ctx, db.DebugEvent{Type: db.DebugGuardrail, AgentID: agent.ID, Name: "warn", Detail: call.Name})
			}

			results = append(results, res)
			st := TurnStep{
				Kind:    StepTool,
				Tool:    call.Name,
				Input:   call.Input,
				Output:  res.Content,
				IsError: res.IsError,
			}
			// The working checklist is a first-class step, not a generic tool row.
			if call.Name == "todo_write" && !res.IsError {
				if todos := parseTodos(call.Input); len(todos) > 0 {
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
			if len(subs.steps) > 0 && !res.IsError {
				st.Kind = StepSubagent
				st.SubSteps = subs.steps
			}
			steps = append(steps, st)
			emit(st)
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
		req.Messages = append(req.Messages, providers.Message{
			Role:            providers.RoleUser,
			ToolResults:     results,
			OnlyToolResults: progBatch,
		})
		pendingProgrammatic = progBatch
		// Guardrail halt: the batch's results are all recorded (pairing holds),
		// so this is a clean, controlled turn end — not an error return. The
		// recovery step tells the transcript (and Faz D's stuck counter) why.
		if guardHaltReason != "" {
			r.logger.Warn("turn halted by loop guardrail", "agent", agent.ID, "reason", guardHaltReason)
			rec := TurnStep{
				Kind:   StepRecovery,
				Reason: string(termGuardrailHalt),
				Text:   "Araç döngüsü guardrail tarafından durduruldu: " + guardHaltReason,
			}
			steps = append(steps, rec)
			emit(rec)
			return last, steps, nil
		}
		// Phase 3: drop lazy tools activated but left unused for a while, so a long
		// turn does not keep shipping schemas the model is no longer reaching for.
		if pruned := active.Prune(activeToolMaxIdle); len(pruned) > 0 {
			r.logger.Info("pruned idle lazy tools", "agent", agent.ID, "tools", pruned)
		}
	}
	r.logger.Warn("tool loop hit iteration cap", "agent", agent.ID)
	rec := TurnStep{
		Kind:   StepRecovery,
		Reason: string(termMaxIters),
		Text:   "Araç döngüsü iterasyon limitine ulaştı; tur burada sonlandırıldı.",
	}
	steps = append(steps, rec)
	emit(rec)
	return last, steps, nil
}

// cancelledToolMsg is the synthetic tool_result content fed back for a tool call
// abandoned by cancellation (user stop / timeout), so the model — and the
// provider's tool_use/tool_result pairing rule — sees a terminal answer.
const cancelledToolMsg = "tool call cancelled: the turn was stopped before this tool finished"

// fillCancelledResults appends a synthetic 'cancelled' tool_result for every
// call in calls that does not already have a result, preserving the provider
// invariant that each tool_use block is answered by exactly one tool_result.
// A3 (cancellation hierarchy): used when a turn ends mid-batch so no tool_use is
// left dangling in the in-flight history.
func fillCancelledResults(results []providers.ToolResult, calls []providers.ToolCall) []providers.ToolResult {
	have := make(map[string]bool, len(results))
	for _, r := range results {
		have[r.CallID] = true
	}
	for _, c := range calls {
		if !have[c.ID] {
			results = append(results, providers.ToolResult{CallID: c.ID, Content: cancelledToolMsg, IsError: true})
		}
	}
	return results
}

// recordedComplete calls the provider once and records token usage. Every
// tool-loop path (native, claude-cli, streaming fallback) funnels its provider
// call through here, so logging the failure once at this choke point guarantees
// a provider error is recorded regardless of which caller (chat, task,
// schedule) triggered it.
// emitCLIToolDebug records one DebugTool event per tool the claude-cli ran during
// its OWN (delegated) loop, so the per-message debug panel lists the CLI agent's
// tool calls too. DurMs is the CLI-side wall-clock latency the stream parser
// measured (time between the tool_use event and its tool_result on the live
// stream; 0 when the stream was not consumed in real time). OutBytes is the tool
// result size and Err the tool's error flag. Native-loop tools already emit their
// own (latency-bearing) DebugTool events inside the loop, so this is CLI-only.
func (r *Runtime) emitCLIToolDebug(ctx context.Context, agent db.Agent, trace []providers.TraceStep) {
	for _, st := range trace {
		if st.Kind != "tool" {
			continue
		}
		r.emitDebug(ctx, db.DebugEvent{
			Type:     db.DebugTool,
			AgentID:  agent.ID,
			Name:     st.Tool,
			DurMs:    st.DurMs,
			OutBytes: len(st.Output),
			Err:      st.IsError,
		})
	}
}

func (r *Runtime) recordedComplete(ctx context.Context, agent db.Agent, provider providers.Provider, req providers.Request) (*providers.Response, error) {
	req = r.withMaxOutput(agent.Provider, req)
	// How claude-cli receives its appended system prompt (inline vs temp file). Set
	// on every path (one-shot + persistent) since both flow through here. Ignored by
	// non-CLI providers. See providers.ClaudeCLI / _Docs/17.
	req.SysPromptFile = r.tun.ClaudeSysPromptFile()
	// Persistent claude-cli session (opt-in): route the turn through the warm
	// long-lived process keyed by SESSION + AGENT, so each agent in a multi-agent
	// session keeps its OWN warm process (with its own system prompt) instead of
	// thrashing one process cold on every agent switch. Any failure falls back to a
	// one-shot Complete, so the feature can never wedge a turn.
	if cli, ok := provider.(*providers.ClaudeCLI); ok && r.cliSessions != nil && r.tun.ClaudePersistentSession() {
		if sid := SessionIDFrom(ctx); sid != "" {
			key := sid + "|" + agent.ID
			if resp, perr := r.cliSessions.Turn(ctx, key, cli, req, req.OnEvent); perr == nil {
				r.RecordUsage(ctx, agent, resp.Model, resp.Usage, resp.ProviderCalls)
				r.noteCacheOutcome(ctx, agent, req, resp.Model, resp.Usage)
				return resp, nil
			} else {
				r.logger.Warn("persistent cli session failed; falling back to one-shot complete",
					"agent", agent.ID, "session", sid, "error", perr)
			}
		}
	}
	resp, err := provider.Complete(ctx, req)
	if err != nil {
		r.logger.Warn("provider complete failed",
			"agent", agent.ID, "provider", agent.Provider, "model", req.Model,
			"callKind", callKindFrom(ctx), "error", err)
		return nil, err
	}
	r.RecordUsage(ctx, agent, resp.Model, resp.Usage, resp.ProviderCalls)
	r.noteCacheOutcome(ctx, agent, req, resp.Model, resp.Usage)
	return resp, nil
}

// liveThinkingID keys the live thinking chunks so the UI merges the streamed
// reasoning deltas into a single growing thinking block (same pattern as
// tool_delta merging) rather than rendering one card per chunk.
const liveThinkingID = "thinking-stream"

// recordedStream streams a completion, forwarding each chunk as a live step, and
// records usage. Text chunks become transient StepDelta (live UI only; the full
// text is on resp.Text); thinking chunks become a merged live StepThinking. The
// returned Response also carries the full thinking as a TraceStep, which the
// caller persists so the reasoning block survives reload.
func (r *Runtime) recordedStream(ctx context.Context, agent db.Agent, sm providers.Streamer, req providers.Request, onStep func(TurnStep)) (*providers.Response, error) {
	req = r.withMaxOutput(agent.Provider, req)
	resp, err := sm.Stream(ctx, req, func(d providers.StreamDelta) {
		switch d.Kind {
		case providers.DeltaThinking:
			onStep(TurnStep{Kind: StepThinking, Text: d.Text, ID: liveThinkingID})
		default:
			onStep(TurnStep{Kind: StepDelta, Text: d.Text})
		}
	})
	if err != nil {
		return nil, err
	}
	r.RecordUsage(ctx, agent, resp.Model, resp.Usage, resp.ProviderCalls)
	r.noteCacheOutcome(ctx, agent, req, resp.Model, resp.Usage)
	return resp, nil
}
