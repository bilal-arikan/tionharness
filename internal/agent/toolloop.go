package agent

import (
	"context"
	"strings"

	"github.com/bilal/swarmgo/internal/conversation"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
)

// maxToolIters bounds the native agentic loop so a misbehaving model can't spin
// forever calling tools.
const maxToolIters = 8

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
	req.PermissionMode = agent.PermissionMode

	// Run provider-driven CLI subprocesses (claude-cli) inside the workspace
	// sandbox root so relative paths — e.g. an attachment's "uploads/<sid>/<file>"
	// — resolve there instead of the backend's launch directory. Native providers
	// ignore this. Empty when no sandbox is configured.
	req.WorkDir = r.workDir

	// Provider-driven paths (claude CLI) surface their own trace via OnEvent.
	if onStep != nil {
		req.OnEvent = func(ts providers.TraceStep) { onStep(traceStepToTurnStep(ts)) }
	}

	// The claude CLI runs its own tool loop. Route it through the keyless MCP
	// delegation path when external MCP is enabled OR an Interaction MCP endpoint
	// is wired for this turn (so ask_user/todo_write work even with MCP off).
	cli, isCLI := provider.(*providers.ClaudeCLI)
	inter := tools.InteractionFrom(ctx)
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
			// In "ask" mode route risky CLI tools through the Interaction MCP
			// permission-prompt tool (real per-tool approval) instead of acceptEdits.
			cli.ConfigureMCP(path, allowed, disallowed, promptToolForMode(agent.PermissionMode, inter))
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
	reg := r.buildRegistry(ctx, agent)
	if reg.Empty() {
		resp, err := r.recordedComplete(ctx, agent, provider, req)
		return resp, nil, err
	}
	req.Tools = reg.Defs(r.toolFilter(ctx, agent))

	// Wire agent→agent delegation for this turn: the call_agent tool reads the
	// runner (and its loop guards) from the context. &req lets a summoned agent
	// inherit the conversation exactly as it stands when the tool fires.
	ctx = r.withDelegation(ctx, agent, &req, autonomous)

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
		results := make([]providers.ToolResult, 0, len(resp.ToolCalls))
		for _, call := range resp.ToolCalls {
			r.logger.Info("tool call", "agent", agent.ID, "tool", call.Name)

			// Permission gate: under read-only/ask the call may be blocked or need
			// user approval before it runs. A blocked call becomes an error result
			// fed back to the model (so it can adapt) instead of executing.
			if allowed, denyMsg := permGate(ctx, agent.PermissionMode, call); !allowed {
				r.logger.Info("tool blocked", "agent", agent.ID, "tool", call.Name, "mode", agent.PermissionMode)
				results = append(results, providers.ToolResult{CallID: call.ID, Content: denyMsg, IsError: true})
				st := TurnStep{Kind: StepError, Tool: call.Name, Reason: "permission_denied", Text: denyMsg, IsError: true}
				steps = append(steps, st)
				emit(st)
				continue
			}

			// Attach a per-call diff sink so file-mutating built-ins (write_file /
			// edit_file) can surface a structured diff for the UI card below.
			callCtx, diffs := tools.WithDiffSink(ctx)

			// Stream long-running tool output live as tool_delta chunks (keyed by
			// the call id) when the tool and the live sink both support it.
			var res providers.ToolResult
			streamed := false
			if onStep != nil && call.ID != "" && reg.CanStream(call.Name) {
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
			if ctx.Err() != nil {
				fail(string(termCancelled), ctx.Err())
				return last, steps, ctx.Err()
			}

			// Token optimization: shrink the result before it re-enters context
			// (and the persisted step) via the two independent compaction systems.
			res = r.compactToolResult(ctx, agent, call.Name, call.Input, res)

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
			steps = append(steps, st)
			emit(st)
		}
		req.Messages = append(req.Messages, providers.Message{
			Role:        providers.RoleUser,
			ToolResults: results,
		})
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

// recordedComplete calls the provider once and records token usage. Every
// tool-loop path (native, claude-cli, streaming fallback) funnels its provider
// call through here, so logging the failure once at this choke point guarantees
// a provider error is recorded regardless of which caller (chat, task,
// schedule, heartbeat) triggered it.
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
