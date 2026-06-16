package agent

import (
	"context"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
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

	// Provider-driven paths (claude CLI) surface their own trace via OnEvent.
	if onStep != nil {
		req.OnEvent = func(ts providers.TraceStep) { onStep(traceStepToTurnStep(ts)) }
	}

	if !agent.MCPEnabled {
		// Extended reasoning is applied only on the plain (non-tool) path: the
		// native tool loop would need to echo signed thinking blocks back, which
		// the provider abstraction doesn't preserve. Providers without thinking
		// support (claude-cli, minimax) ignore the budget.
		req.ThinkingBudget = thinkingBudgetForLevel(agent.ThinkingLevel)

		// Prefer first-class token streaming when a live sink is present and the
		// provider supports it (anthropic/minimax). claude-cli is not a Streamer;
		// it streams its own trace via req.OnEvent wired above.
		if onStep != nil {
			if sm, ok := provider.(providers.Streamer); ok {
				resp, err := r.recordedStream(ctx, agent, sm, req, onStep)
				if err != nil {
					return nil, nil, err
				}
				return resp, nil, nil // deltas are transient; full text is on resp
			}
		}
		resp, err := r.recordedComplete(ctx, agent, provider, req)
		if err != nil {
			return nil, nil, err
		}
		// claude-cli surfaces its own tool/thinking trace via stream-json.
		return resp, traceToSteps(resp.Trace), nil
	}

	// Keyless delegation path: let the claude CLI own the tool loop.
	if cli, ok := provider.(*providers.ClaudeCLI); ok {
		path, allowed, cleanup, err := r.writeCLIMCPConfig(ctx)
		if err != nil {
			r.logger.Warn("cli mcp config failed", "error", err)
		} else if path != "" {
			defer cleanup()
			cli.ConfigureMCP(path, allowed)
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
	req.Tools = reg.Defs(allowFunc(agent))

	emit := func(s TurnStep) {
		if onStep != nil {
			onStep(s)
		}
	}

	var last *providers.Response
	var steps []TurnStep
	for i := 0; i < maxToolIters; i++ {
		if autonomous {
			if err := r.ensureBudget(ctx, agent); err != nil {
				return nil, steps, err
			}
		}
		// Live steering: fold any user guidance that arrived since the last
		// iteration into the conversation before the next model call.
		for _, m := range drainSteer(ctx) {
			req.Messages = append(req.Messages, providers.Message{Role: providers.RoleUser, Text: steerPrefix + m})
			st := TurnStep{Kind: StepText, Text: "↪ Yönlendirme: " + m}
			steps = append(steps, st)
			emit(st)
		}
		resp, err := r.recordedComplete(ctx, agent, provider, req)
		if err != nil {
			return nil, steps, err
		}
		last = resp
		if resp.StopReason != providers.StopToolUse || len(resp.ToolCalls) == 0 {
			return resp, steps, nil
		}

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
			res := reg.Call(ctx, call)
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
		Reason: "max_tool_iterations",
		Text:   "Araç döngüsü iterasyon limitine ulaştı; tur burada sonlandırıldı.",
	}
	steps = append(steps, rec)
	emit(rec)
	return last, steps, nil
}

// recordedComplete calls the provider once and records token usage.
func (r *Runtime) recordedComplete(ctx context.Context, agent db.Agent, provider providers.Provider, req providers.Request) (*providers.Response, error) {
	resp, err := provider.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	r.RecordUsage(ctx, agent.ID, resp.Usage)
	return resp, nil
}

// recordedStream streams a completion, forwarding each text delta as a live
// StepDelta, and records usage. Deltas are transient (live UI only); the
// returned Response carries the full text the caller persists as the message.
func (r *Runtime) recordedStream(ctx context.Context, agent db.Agent, sm providers.Streamer, req providers.Request, onStep func(TurnStep)) (*providers.Response, error) {
	resp, err := sm.Stream(ctx, req, func(delta string) {
		onStep(TurnStep{Kind: StepDelta, Text: delta})
	})
	if err != nil {
		return nil, err
	}
	r.RecordUsage(ctx, agent.ID, resp.Usage)
	return resp, nil
}
