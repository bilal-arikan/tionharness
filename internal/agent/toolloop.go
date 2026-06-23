package agent

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/conversation"
	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// defaultMaxToolIters bounds the native agentic loop so a misbehaving model can't
// spin forever calling tools. Tripled from the original 8 to 24 to give multi-step
// tool workflows (and schedule_wake-driven async flows) room to finish before the
// loop cap ends the turn.
const defaultMaxToolIters = 24

// maxToolIters is the live loop bound, defaulting to defaultMaxToolIters and
// overridable via SWARMGO_MAX_TOOL_ITERS (positive integer) for power users who
// want longer or shorter native tool loops without a rebuild.
var maxToolIters = resolveMaxToolIters()

// resolveMaxToolIters reads the env override once at package init, falling back to
// the default for an unset, empty, non-numeric or non-positive value.
func resolveMaxToolIters() int {
	if v := os.Getenv("SWARMGO_MAX_TOOL_ITERS"); v != "" {
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
// token budget (0 = off). Providers without thinking support ignore it.
func thinkingBudgetForLevel(level string) int {
	switch level {
	case "low":
		return 2048
	case "medium":
		return 8192
	case "high":
		return 16384
	default:
		return 0
	}
}

// resolveThinkingBudget is the model-class-aware resolver. It starts from the
// agent's ThinkingLevel, but for models that mandate always-on adaptive
// reasoning (Fable/Mythos 5 class — they reject thinking:disabled with a 400)
// it floors an "off"/"low" request to a minimal adaptive budget so the request
// stays valid. Opus/Sonnet/Haiku are unaffected.
func resolveThinkingBudget(model, level string) int {
	budget := thinkingBudgetForLevel(level)
	if providers.RequiresAdaptiveThinking(model) && budget < providers.MinAdaptiveThinkingBudget {
		return providers.MinAdaptiveThinkingBudget
	}
	return budget
}

// CompleteWithTools runs a completion that may use tools. Behaviour depends on
// the agent and provider:
//
//   - MCP disabled            → a single plain completion.
//   - claude-cli + MCP        → delegate: the CLI runs the tool loop itself
//     using a generated --mcp-config (keyless path).
//   - other provider + MCP    → SwarmGo's own agentic loop drives the tools via
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
func (r *Runtime) completeTraced(ctx context.Context, agent db.Agent, provider providers.Provider, req providers.Request, autonomous bool, onStep func(TurnStep)) (*providers.Response, []TurnStep, error) {
	if autonomous {
		if err := r.ensureBudget(ctx, agent); err != nil {
			return nil, nil, err
		}
	}

	// Carry the agent's permission mode so provider-driven loops (claude CLI) can
	// gate their tool use. Empty maps to "auto" downstream.
	req.PermissionMode = effectivePermissionMode(agent.PermissionMode, autonomous)

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

	// Artifacts everywhere: chat turns install a session-bound sink before calling
	// in; autonomous turns (scheduler/spawn/flow) don't, so create_artifact would
	// fail with "artifacts are not available for this turn". Install a fallback sink
	// here whenever the turn has a session but no sink yet — so artifacts can always
	// be created, regardless of turn type. (CLI turns get theirs via the Interaction
	// bridge run; this covers the native loop.)
	if sid := SessionIDFrom(ctx); sid != "" && !tools.HasArtifactSink(ctx) {
		ctx = tools.WithArtifacts(ctx, r.NewArtifactSink(sid, agent.ID))
	}

	// Provider-driven paths (claude CLI) surface their own trace via OnEvent.
	if onStep != nil {
		req.OnEvent = func(ts providers.TraceStep) { onStep(traceStepToTurnStep(ts)) }
	}

	// The claude CLI runs its own tool loop. Route it through the keyless MCP
	// delegation path when external MCP is enabled OR an Interaction MCP endpoint
	// is wired for this turn (so ask_user/todo_write work even with MCP off).
	cli, isCLI := provider.(*providers.ClaudeCLI)
	inter := tools.InteractionFrom(ctx)
	// Autonomous CLI turns (scheduler/spawn/flow) don't carry an
	// Interaction endpoint the way chat turns do, so a CLI agent there can't reach
	// the bridged use_skill/shell/self-manage tools and falls back to its native
	// (now-disallowed/foreign) ones — the cause of scheduled "Unknown skill" + the
	// POSIX-Bash mismatch. Wire one on demand for this turn so headless runs get
	// the same bridge chat agents do. Only when none is already present.
	if autonomous && isCLI && inter.URL == "" && r.autoInteract != nil {
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
		return resp, traceToSteps(resp.Trace), nil
	}

	// Keyless delegation path: let the claude CLI own the tool loop, wiring the
	// external MCP servers (when enabled) plus the Interaction MCP server (when an
	// endpoint is present) into a single generated --mcp-config.
	if cliMCP {
		path, allowed, disallowed, cleanup, err := r.writeCLIMCPConfig(ctx, agent.MCPEnabled, inter)
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
	reg := r.buildRegistry(ctx, agent)
	if reg.Empty() {
		resp, err := r.recordedComplete(ctx, agent, provider, req)
		return resp, nil, err
	}
	toolFilter := r.toolFilter(ctx, agent)
	req.Tools = reg.ActiveDefs(toolFilter, active.Snapshot())

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
	var steps []TurnStep
	// ls carries the single-shot recovery guards (A1) across iterations so a
	// stuck model can never spin forever inside one turn; cfg/keepRecent are the
	// resolved, settings-driven recovery policy for this turn.
	var ls loopState
	cfg := recoveryConfig{
		maxTokenLimit:   r.tun.MaxTokenRetries(),
		reactiveCompact: r.tun.ReactiveCompact(),
	}
	keepRecent := r.tun.ReactiveKeepRecent()
	// partial accumulates answer text across max-output-token resumes, so the
	// stitched full answer is returned even though it arrived in capped pieces.
	var partial strings.Builder
	// fail records a turn-level error as an inline step before the loop returns.
	fail := func(reason string, err error) {
		st := TurnStep{Kind: StepError, Reason: reason, Text: err.Error(), IsError: true}
		steps = append(steps, st)
		emit(st)
	}
	for i := 0; i < maxToolIters; i++ {
		if autonomous {
			if err := r.ensureBudget(ctx, agent); err != nil {
				fail(string(termBudget), err)
				return nil, steps, err
			}
		}
		// Live steering: fold any user guidance that arrived since the last
		// iteration into the conversation before the next model call.
		for _, m := range drainSteer(ctx) {
			req.Messages = append(req.Messages, providers.Message{Role: providers.RoleUser, Text: steerPrefix + m})
			st := TurnStep{Kind: StepSteer, Text: m}
			steps = append(steps, st)
			emit(st)
		}
		// Recompute the shipped tool schemas for this step: eager tools plus any
		// lazy tools activated so far. Cheap; reflects activate/deactivate calls
		// from the previous iteration.
		active.SetIter(i)
		req.Tools = reg.ActiveDefs(toolFilter, active.Snapshot())
		resp, err := r.recordedComplete(ctx, agent, provider, req)
		if err != nil {
			// A1: a context-overflow error is recoverable once per turn by
			// compacting the in-flight history and retrying; any other error
			// ends the turn. decideRecovery keeps this policy pure + testable.
			d := decideRecovery(nil, err, ls, cfg)
			if d.compact {
				folded, ok, cerr := conversation.CompactInFlightMessages(ctx, r.db, provider, agent, req.Messages, keepRecent)
				if cerr == nil && ok {
					req.Messages = folded
					ls.compacted = true
					ls.lastContinue = d.reason
					rec := TurnStep{Kind: StepRecovery, Reason: string(d.reason), Text: recoveryText(d.reason)}
					steps = append(steps, rec)
					emit(rec)
					continue
				}
			}
			fail(string(termProviderErr), err)
			return nil, steps, err
		}
		last = resp
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
				continue
			}
			// Stitch any earlier capped fragments onto the final answer.
			if partial.Len() > 0 {
				resp.Text = partial.String() + resp.Text
			}
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
		req.Messages = append(req.Messages, providers.Message{
			Role:      providers.RoleAssistant,
			Text:      resp.Text,
			ToolCalls: resp.ToolCalls,
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
				allowed, denyMsg = permGate(ctx, agent.PermissionMode, call)
			}
			if !allowed {
				r.logger.Info("tool blocked", "agent", agent.ID, "tool", call.Name, "mode", agent.PermissionMode)
				results = append(results, providers.ToolResult{CallID: call.ID, Content: denyMsg, IsError: true})
				st := TurnStep{Kind: StepError, Tool: call.Name, Reason: "permission_denied", Text: denyMsg, IsError: true}
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
		req.Messages = append(req.Messages, providers.Message{
			Role:        providers.RoleUser,
			ToolResults: results,
		})
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
func (r *Runtime) recordedComplete(ctx context.Context, agent db.Agent, provider providers.Provider, req providers.Request) (*providers.Response, error) {
	resp, err := provider.Complete(ctx, req)
	if err != nil {
		r.logger.Warn("provider complete failed",
			"agent", agent.ID, "provider", agent.Provider, "model", req.Model,
			"callKind", callKindFrom(ctx), "error", err)
		return nil, err
	}
	r.RecordUsage(ctx, agent, resp.Model, resp.Usage)
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
	r.RecordUsage(ctx, agent, resp.Model, resp.Usage)
	return resp, nil
}
