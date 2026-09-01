package agent

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/google/uuid"
)

// defaultMaxToolIters bounds the native agentic loop so a misbehaving model can't
// spin forever calling tools. Set to 500 to give multi-step tool workflows
// (and schedule_wake-driven async flows) room to finish before the loop cap ends the turn.
const defaultMaxToolIters = 500

// maxToolIters is the loop bound, defaulting to defaultMaxToolIters and overridable
// ONLY via TIONHARNESS_MAX_TOOL_ITERS (positive integer) for power users who want
// longer or shorter native tool loops without a rebuild. It is resolved once at
// package init: there is deliberately no Settings field for it, so it does not
// change mid-process. (A Tunables setter/getter pair used to exist here, claiming
// the loop re-read it every iteration — it never did, and was removed.)
var maxToolIters = resolveMaxToolIters()

// resolveMaxToolIters reads the env override once at package init, falling back to
// the default for an unset, empty, non-numeric or non-positive value.
func resolveMaxToolIters() int {
	if v := os.Getenv("TIONHARNESS_MAX_TOOL_ITERS"); v != "" {
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
	case "ultra":
		return 131072
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
//   - other provider + MCP    → TionHarness's own agentic loop drives the tools via
//     the unified registry (built-ins + MCP).
//
// The agent is passed by value — the turn captures its model at the start. If
// the agent's model is changed mid-turn (via UpdateAgent), the in-flight turn
// completes with the old model; the NEXT turn picks up the new one. This is
// deliberate: switching models mid-turn would confuse the provider's prompt
// cache and the turn's own tool-use state.
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
	// The turn state and its phases live in toolloop_phases.go. ctx and req are
	// FIELDS there, not parameters: the setup phase rebinds ctx and mutates req,
	// and every later phase must see those changes.
	t := &toolLoopTurn{r: r, ctx: ctx, agent: agent, provider: provider, req: req, autonomous: autonomous, onStep: onStep}
	prepCleanup, err := t.prepare()
	if err != nil {
		return nil, nil, err
	}
	defer prepCleanup()

	if !agent.MCPEnabled && !t.cliMCP {
		return t.runPlain()
	}

	// Keyless delegation path: the claude CLI owns the tool loop. The config and
	// settings temp files must outlive the provider call, so their cleanup is
	// deferred HERE — same lifetime the original two stacked defers had.
	if t.cliMCP {
		defer t.configureCLIMCP()()
		return t.completeAndTraceCLI()
	}

	if resp, steps, err, done := t.prepareNativeLoop(); done {
		return resp, steps, err
	}

	// Wire the generic subagent runner for this turn: the run_subagent tool reads
	// it (and the shared loop guards) from the context. &t.req lets an inherited-
	// context subagent see the conversation as it stands when the tool fires.
	t.ctx = r.withRunAgent(t.ctx, agent, &t.req, autonomous)

	t.emit = serializeStepEmitter(func(s TurnStep) {
		if onStep != nil {
			onStep(s)
		}
	})
	// Keep the current batch reachable by panic cleanup. Tool implementations and
	// streaming callbacks may panic after several parallel cards have opened.
	defer cancelLiveCardsOnPanic(&t.activeLiveCards)

	t.initLoopState()
	return t.runNativeLoop()
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
		if st.Kind == "compaction" && st.Source == "cli-native" {
			// OnCLICompaction is the sole DebugCompaction producer for native CLI
			// success. The durable trace remains for UI/boundary detection only.
			continue
		}
		if st.Kind != "tool" {
			continue
		}
		if st.Tool == "collab_tool_call" {
			r.emitDebug(ctx, collabDebugEvent(agent.ID, st))
			continue
		}
		r.emitDebug(ctx, db.DebugEvent{
			Type:     db.DebugTool,
			AgentID:  agent.ID,
			Name:     st.Tool,
			DurMs:    st.DurMs,
			OutBytes: len(st.Output),
			Err:      st.IsError,
			Error:    debugToolError(st.Output, st.IsError),
			Args:     debugToolArgs(st.Input, st.IsError),
		})
	}
}

func collabDebugEvent(agentID string, st providers.TraceStep) db.DebugEvent {
	operation := st.Operation
	if operation == "" {
		operation = "collab_tool_call"
	}
	detail := st.Summary
	if detail == "" {
		parts := []string{operation}
		if len(st.Target) > 0 {
			parts = append(parts, "receivers: "+strings.Join(st.Target, ", "))
		}
		if st.Status != "" {
			parts = append(parts, st.Status)
		}
		detail = strings.Join(parts, " · ")
	}
	return db.DebugEvent{
		Type:    db.DebugTool,
		AgentID: agentID,
		Name:    operation,
		Detail:  detail,
		DurMs:   st.DurationMs,
		Err:     st.IsError,
	}
}

func debugToolError(output string, isError bool) string {
	if !isError {
		return ""
	}
	return debugSummary(output, 500)
}

func debugToolArgs(input []byte, isError bool) string {
	if !isError {
		return ""
	}
	return debugSummary(string(input), 200)
}

// recordFailedUsage bills a turn that ended in an error. A failure at or after
// the request (a rate-limit or auth rejection in the result envelope, a crash on
// the last internal round-trip) still consumed the input tokens it sent, and the
// provider carries that usage out on the error itself (providers.UsageError) —
// recording usage only on the success path under-reported exactly the turns that
// cost the most. Errors without usage (nothing was ever sent) record nothing.
func (r *Runtime) recordFailedUsage(ctx context.Context, agent db.Agent, req providers.Request, err error) {
	ue, ok := providers.UsageFromError(err)
	if !ok {
		return
	}
	model := ue.Model
	if model == "" {
		model = req.Model
	}
	r.RecordUsage(ctx, agent, model, ue.Usage, ue.ProviderCalls)
	// RecordUsage's llm_call event is shaped exactly like a successful one, so a
	// turn that produced NOTHING while still paying for its prompt reads as
	// ordinary spend. Name that case: no output tokens, but input/cache were
	// billed. Errors that spent nothing are not "billed" and stay unnamed.
	spent := ue.Usage.InputTokens + ue.Usage.CacheReadTokens + ue.Usage.CacheWriteTokens
	if ue.Usage.OutputTokens == 0 && spent > 0 {
		r.emitDebug(ctx, db.DebugEvent{
			Type:       db.DebugLLMCall,
			AgentID:    agent.ID,
			Name:       "failed_turn_billed",
			Model:      model,
			In:         ue.Usage.InputTokens,
			Out:        ue.Usage.OutputTokens,
			CacheRead:  ue.Usage.CacheReadTokens,
			CacheWrite: ue.Usage.CacheWriteTokens,
			Calls:      ue.ProviderCalls,
			Err:        true,
		})
	}
}

func (r *Runtime) recordedComplete(ctx context.Context, agent db.Agent, provider providers.Provider, req providers.Request) (*providers.Response, error) {
	// Opaque completions cannot prove liveness with heartbeat. Give the operation
	// its own bounded lease; successful completion is semantic progress.
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
	// Deliberately a concrete claude assertion, not providers.AsCLI: r.cliSessions
	// is a pool of warm claude-cli processes (stream-json protocol, claude session
	// ids) and the setting gating it is claude-specific. Another CLI transport must
	// fall through to the one-shot Complete below.
	if cli, ok := provider.(*providers.ClaudeCLI); ok && r.cliSessions != nil && r.tun.ClaudePersistentSession() {
		if sid := SessionIDFrom(ctx); sid != "" {
			key := sid + "|" + agent.ID
			resp, perr := runProviderOperation(ctx, req.OnEvent == nil, func(opCtx context.Context) (*providers.Response, error) {
				return r.cliSessions.Turn(opCtx, key, cli, req, req.OnEvent)
			})
			if perr == nil {
				if tracker := ActivityTrackerFrom(ctx); tracker != nil {
					tracker.Progress("provider_complete")
				}
				preserveOrDeriveThinkingTokens(resp)
				r.RecordUsage(ctx, agent, resp.Model, resp.Usage, resp.ProviderCalls)
				r.noteCacheOutcome(ctx, agent, req, resp.Model, resp.Usage)
				r.noteResolvedModel(ctx, agent, req.Model, resp.Model)
				return resp, nil
			} else {
				if errors.Is(perr, ErrOperationLeaseTimeout) {
					r.recordFailedUsage(ctx, agent, req, perr)
					return nil, perr
				}
				// The failed persistent turn still spent whatever it spent before dying;
				// the fallback one-shot below bills separately, so skipping this would
				// silently drop a whole turn's tokens.
				r.recordFailedUsage(ctx, agent, req, perr)
				r.logger.Warn("persistent cli session failed; falling back to one-shot complete",
					"agent", agent.ID, "session", sid, "error", perr)
			}
		}
	}
	resp, err := runProviderOperation(ctx, req.OnEvent == nil, func(opCtx context.Context) (*providers.Response, error) {
		return provider.Complete(opCtx, req)
	})
	if err != nil {
		r.recordFailedUsage(ctx, agent, req, err)
		r.logger.Warn("provider complete failed",
			"agent", agent.ID, "provider", agent.Provider, "model", req.Model,
			"callKind", callKindFrom(ctx), "error", err)
		return nil, err
	}
	if tracker := ActivityTrackerFrom(ctx); tracker != nil {
		tracker.Progress("provider_complete")
	}
	preserveOrDeriveThinkingTokens(resp)
	r.RecordUsage(ctx, agent, resp.Model, resp.Usage, resp.ProviderCalls)
	r.noteCacheOutcome(ctx, agent, req, resp.Model, resp.Usage)
	r.noteResolvedModel(ctx, agent, req.Model, resp.Model)
	return resp, nil
}

func runProviderOperation(ctx context.Context, leased bool, operation func(context.Context) (*providers.Response, error)) (*providers.Response, error) {
	if !leased {
		return operation(ctx)
	}
	return RunWithOperationLease(ctx, operation)
}

// recordedStream streams a completion, forwarding each chunk as a live step, and
// records usage. Text chunks become transient StepDelta (live UI only; the full
// text is on resp.Text); thinking chunks become a merged live StepThinking. The
// returned Response also carries the full thinking as a TraceStep, which the
// caller persists so the reasoning block survives reload.
func (r *Runtime) recordedStream(ctx context.Context, agent db.Agent, sm providers.Streamer, req providers.Request, onStep func(TurnStep)) (*providers.Response, error) {
	req = r.withMaxOutput(agent.Provider, req)
	var thinking *liveCard
	resp, err := sm.Stream(ctx, req, func(d providers.StreamDelta) {
		switch d.Kind {
		case providers.DeltaThinking:
			if thinking == nil {
				thinking = openLive(onStep, "thinking-"+uuid.NewString(), TurnStep{Kind: StepThinking})
			}
			thinking.Chunk(d.Text)
		default:
			onStep(TurnStep{Kind: StepDelta, Text: d.Text})
		}
	})
	if err != nil {
		r.recordFailedUsage(ctx, agent, req, err)
		return nil, err
	}
	preserveOrDeriveThinkingTokens(resp)
	r.RecordUsage(ctx, agent, resp.Model, resp.Usage, resp.ProviderCalls)
	r.noteCacheOutcome(ctx, agent, req, resp.Model, resp.Usage)
	r.noteResolvedModel(ctx, agent, req.Model, resp.Model)
	return resp, nil
}
