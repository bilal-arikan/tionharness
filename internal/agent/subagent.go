package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// SubagentProfile is a built-in ephemeral subagent type: a system prompt plus a
// tool allowlist that shape a throwaway agent persona. A profile target spins up
// a one-shot worker that is never persisted — its usage is attributed to the
// caller (shared-infra principle), and it vanishes when its run returns.
type SubagentProfile struct {
	ID           string
	SystemPrompt string
	AllowedTools []string // empty = inherit the caller's allowlist
}

// defaultSubagentProfiles are the built-in worker types selectable as a
// run_subagent target. Kept in code for now (a later phase may move them to
// settings.json). Allowlists intersect with the caller's own effective tools.
var defaultSubagentProfiles = map[string]SubagentProfile{
	"explore": {
		ID: "explore",
		SystemPrompt: "You are an Explore subagent: a focused, read-only investigator. Search the " +
			"workspace, read the relevant files, and report precise findings (file:line, names, facts). " +
			"You never modify anything. Return a concise, structured answer — your reply is the only " +
			"thing the caller sees, so make it self-contained.",
		AllowedTools: []string{"Read", "LS", "Glob", "Grep", "WebFetch", "memory_recall"},
	},
	"coder": {
		ID: "coder",
		SystemPrompt: "You are a Coder subagent: you implement a well-scoped change. Read what you need, " +
			"write or edit the necessary files, and keep edits minimal and idiomatic. Report what you " +
			"changed (files + a one-line rationale each). Your reply is the only thing the caller sees.",
		AllowedTools: []string{"Read", "LS", "Glob", "Grep", "Write", "Edit", "Bash"},
	},
	"reviewer": {
		ID: "reviewer",
		SystemPrompt: "You are a Reviewer subagent: an independent, read-only critic. Examine the target " +
			"for bugs, races, security issues and unclear code. Report ONLY real, actionable findings " +
			"with file:line and a short why; say so plainly if it looks correct. You never modify " +
			"anything. Your reply is the only thing the caller sees.",
		AllowedTools: []string{"Read", "LS", "Glob", "Grep"},
	},
}

// SubagentProfiles returns the built-in profile ids (sorted-free; for display).
func SubagentProfiles() []SubagentProfile {
	out := make([]SubagentProfile, 0, len(defaultSubagentProfiles))
	for _, p := range defaultSubagentProfiles {
		out = append(out, p)
	}
	return out
}

// subStepSink collects a subagent's nested activity trace so the parent tool loop
// can promote the run_subagent tool row into a StepSubagent carrying SubSteps.
type subStepSink struct{ steps []TurnStep }

type subStepSinkKey struct{}

// withSubStepSink attaches a fresh sink to ctx and returns it; the parent loop
// reads sink.steps after the run_subagent call to nest the subagent's trace.
func withSubStepSink(ctx context.Context) (context.Context, *subStepSink) {
	s := &subStepSink{}
	return context.WithValue(ctx, subStepSinkKey{}, s), s
}

// subStepSinkFrom returns the sink on ctx, or nil when none is wired.
func subStepSinkFrom(ctx context.Context) *subStepSink {
	s, _ := ctx.Value(subStepSinkKey{}).(*subStepSink)
	return s
}

// withRunAgent wires the run_subagent tool for one completion. It seeds the
// per-turn call-graph position (top-level: depth 0, the caller marked visited, a
// fresh budget counter) or inherits it from a parent run, then attaches a runner
// that launches a subagent. reqPtr points at the live request so an inherited-
// context subagent sees the conversation exactly as it stands when the tool fires.
func (r *Runtime) withRunAgent(ctx context.Context, caller db.Agent, reqPtr *providers.Request, autonomous bool) context.Context {
	if _, ok := delegStateFrom(ctx); !ok {
		var n int32
		ctx = context.WithValue(ctx, delegStateKey{}, delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n})
	}
	fn := func(rctx context.Context, spec tools.RunAgentSpec) (tools.RunAgentResult, error) {
		return r.runAgent(rctx, caller, reqPtr, autonomous, spec)
	}
	return tools.WithRunAgent(ctx, fn)
}

// RunSubagentRunner returns a run_subagent runner for the CLI Interaction bridge,
// bound to the caller agent: it seeds the delegation call-graph + runner into ctx
// and executes the run_subagent tool, returning its formatted result. There is no
// live providers.Request on the CLI path, so inherited-context mode degrades to the
// prompt only (the caller passes any needed context in the task). run_subagent is
// always installed (2026-07-02: the delegation master toggle was removed; per-tool
// visibility handles disabling), so the runner is always returned.
func (r *Runtime) RunSubagentRunner(caller db.Agent, autonomous bool) func(ctx context.Context, args json.RawMessage) (string, error) {
	tool := tools.NewRunSubagentTool()
	return func(ctx context.Context, args json.RawMessage) (string, error) {
		ctx = r.withRunAgent(ctx, caller, nil, autonomous)
		return tool.Call(ctx, args)
	}
}

// runAgent is the single generic entry point behind run_subagent. It enforces the
// shared guards (depth / per-turn budget / cycle), resolves the target (an
// ephemeral profile worker or an existing agent), then runs it in one of two
// modes: sync (isolated or inherited context, returns the final reply) or async
// (detached background session via SpawnSession, returns a handle).
func (r *Runtime) runAgent(ctx context.Context, caller db.Agent, parentReq *providers.Request, autonomous bool, spec tools.RunAgentSpec) (tools.RunAgentResult, error) {
	cur, _ := delegStateFrom(ctx)

	// Guard 1 — depth.
	if cur.depth >= r.tun.DelegationMaxDepth() {
		return tools.RunAgentResult{}, fmt.Errorf("subagent depth limit (%d) reached; do the work yourself instead of nesting further", r.tun.DelegationMaxDepth())
	}
	// Guard 2 — per-turn budget (shared counter across the whole turn's tree).
	// Fast-fail check; the precise spend (atomic add-then-check) happens below so
	// parallel fan-out never overspends the cap.
	maxCalls := r.tun.DelegationMaxCalls()
	if cur.calls != nil && atomic.LoadInt32(cur.calls) >= int32(maxCalls) {
		return tools.RunAgentResult{}, fmt.Errorf("subagent budget (%d per turn) exhausted; do the rest yourself", maxCalls)
	}

	agent, ephemeral, err := r.resolveSubagentTarget(ctx, caller, spec.Target)
	if err != nil {
		return tools.RunAgentResult{}, err
	}
	// Guard 3 — cycle/self (only meaningful for a persistent agent target).
	if !ephemeral && cur.visited[agent.ID] {
		return tools.RunAgentResult{}, fmt.Errorf("agent %q is already part of this chain; pick a different target", agent.Name)
	}
	if m := strings.TrimSpace(spec.Model); m != "" {
		agent.Model = m
	}
	provider, err := r.providers.Get(agent.Provider)
	if err != nil {
		return tools.RunAgentResult{}, fmt.Errorf("target %q provider unavailable: %w", agent.Name, err)
	}

	// Spend one unit of the shared per-turn budget (atomic so concurrent fan-out
	// never exceeds the cap; refund and refuse if this call would push us over).
	if cur.calls != nil {
		if atomic.AddInt32(cur.calls, 1) > int32(maxCalls) {
			atomic.AddInt32(cur.calls, -1)
			return tools.RunAgentResult{}, fmt.Errorf("subagent budget (%d per turn) exhausted; do the rest yourself", maxCalls)
		}
	}

	// Async mode: detach into a persistent background session (fire-and-forget).
	// Only real agents can run async — an ephemeral profile has no persistent id.
	if spec.Wait == "async" {
		if ephemeral {
			return tools.RunAgentResult{}, fmt.Errorf("async subagents require an existing agent target, not a profile")
		}
		res, err := r.SpawnSession(ctx, agent.ID, strings.TrimSpace(spec.Task), SpawnOptions{ModelOverride: spec.Model, CreatedBy: caller.ID})
		if err != nil {
			return tools.RunAgentResult{}, err
		}
		return tools.RunAgentResult{AgentName: res.AgentName, SessionID: res.SessionID, Async: true}, nil
	}

	// Build the child chain position: one level deeper, the target added to the
	// visited-set (real agents only), sharing the per-turn budget counter.
	childVisited := make(map[string]bool, len(cur.visited)+1)
	for id := range cur.visited {
		childVisited[id] = true
	}
	if !ephemeral {
		childVisited[agent.ID] = true
	}
	childCtx := context.WithValue(ctx, delegStateKey{}, delegState{
		depth:   cur.depth + 1,
		visited: childVisited,
		calls:   cur.calls,
	})

	// Build the request: a clean isolated context (default) or the caller's
	// conversation (inherited). Isolation is the whole point — the subagent works
	// from just the task, and its tool output never re-enters the caller's history.
	task := strings.TrimSpace(spec.Task)
	var msgs []providers.Message
	if spec.Context == "inherited" && parentReq != nil {
		msgs = inheritedMessages(parentReq)
		msgs = append(msgs, providers.Message{Role: providers.RoleUser, Text: fmt.Sprintf("[Delegated by %q]\n\n%s", caller.Name, task)})
	} else {
		msgs = []providers.Message{{Role: providers.RoleUser, Text: task}}
	}
	sys := r.autonomousSystemPrompt(agent)
	if contract := delegationContract(spec); contract != "" {
		sys = strings.TrimSpace(sys + "\n\n" + contract)
	}
	req := providers.Request{
		Model:    agent.Model,
		System:   sys,
		Messages: msgs,
	}

	r.logger.Info("subagent run",
		"from", caller.ID, "target", agent.Name, "ephemeral", ephemeral,
		"depth", cur.depth+1, "context", orDefault(spec.Context, "isolated"))

	// Run in an isolated trace. autonomous=true keeps it headless (interactive
	// tools like ask_user no-op) and enforces the caller's daily budget.
	resp, steps, err := r.completeTraced(WithCallKind(childCtx, KindSubagent), agent, provider, req, true, nil)
	if err != nil {
		return tools.RunAgentResult{}, fmt.Errorf("subagent %q failed: %w", agent.Name, err)
	}
	// Hand the subagent's nested trace to the parent loop (when a sink is wired) so
	// it renders as a collapsible StepSubagent.
	if sink := subStepSinkFrom(ctx); sink != nil {
		sink.steps = steps
	}
	return tools.RunAgentResult{AgentName: agent.Name, Reply: resp.Text}, nil
}

// resolveSubagentTarget maps a run_subagent target to a runnable agent. A target
// matching a built-in profile id yields an EPHEMERAL agent cloned from the caller
// (so it inherits provider, model, daily limits and permission mode, and its
// usage attributes to the caller) but reshaped with the profile's system prompt
// and tool allowlist. Otherwise the target is resolved as an existing workspace
// agent.
func (r *Runtime) resolveSubagentTarget(ctx context.Context, caller db.Agent, target string) (db.Agent, bool, error) {
	if p, ok := defaultSubagentProfiles[strings.ToLower(target)]; ok {
		eph := caller // clone limits/provider/model/permission from the caller
		eph.Name = "subagent:" + p.ID
		eph.Soul = p.SystemPrompt
		eph.Identity = ""
		eph.Skills = nil
		eph.MCPEnabled = true
		allow := p.AllowedTools
		if len(allow) == 0 {
			eph.AllowedTools = caller.AllowedTools
		} else {
			b, _ := json.Marshal(allow)
			eph.AllowedTools = string(b)
		}
		return eph, true, nil
	}
	a, err := r.resolveAgent(ctx, target)
	if err != nil {
		return db.Agent{}, false, fmt.Errorf("unknown subagent target %q: not a profile (explore|coder|reviewer) and %w", target, err)
	}
	return a, false, nil
}

// subFuture is the pending result of a run_subagent call started concurrently by
// the parallel fan-out path. done is closed when res/steps are ready.
type subFuture struct {
	res   providers.ToolResult
	steps []TurnStep
	done  chan struct{}
}

// launchParallelSubagents starts every run_subagent call in a batch concurrently,
// returning a map keyed by call id so the tool loop can await each in turn. It
// returns nil when the batch has fewer than two run_subagent calls — there is no
// parallelism to win, so the loop runs them inline as usual. Each subagent runs
// in its own context with a private trace sink; the shared per-turn budget guard
// (atomic) bounds total fan-out. Hooks/permission still run sequentially in the
// loop before the result is consumed (a blocked call simply discards its future).
func (r *Runtime) launchParallelSubagents(ctx context.Context, reg *tools.Registry, calls []providers.ToolCall) map[string]*subFuture {
	n := 0
	for _, c := range calls {
		if c.Name == "run_subagent" {
			n++
		}
	}
	if n < 2 {
		return nil
	}
	futures := make(map[string]*subFuture, n)
	var wg sync.WaitGroup
	for _, call := range calls {
		if call.Name != "run_subagent" {
			continue
		}
		f := &subFuture{done: make(chan struct{})}
		futures[call.ID] = f
		wg.Add(1)
		go func(call providers.ToolCall, f *subFuture) {
			defer wg.Done()
			defer close(f.done)
			cctx, sink := withSubStepSink(ctx)
			f.res = reg.Call(cctx, call)
			f.steps = sink.steps
		}(call, f)
	}
	r.logger.Info("subagent parallel fan-out", "count", n)
	return futures
}

// delegationContract renders the optional structured task contract (objective /
// output format / boundaries) the caller attached to a run_subagent call into a
// system-prompt block. It gives the subagent the clear objective, required output
// shape and explicit scope limits that Anthropic's multi-agent guidance calls for
// to avoid duplicated work and gaps. Only set fields become lines; when none are
// set it returns "" so the legacy plain-task path stays byte-identical.
func delegationContract(spec tools.RunAgentSpec) string {
	var b strings.Builder
	if spec.Objective != "" {
		fmt.Fprintf(&b, "- Objective: %s\n", spec.Objective)
	}
	if spec.OutputFormat != "" {
		fmt.Fprintf(&b, "- Output format: %s\n", spec.OutputFormat)
	}
	if spec.Boundaries != "" {
		fmt.Fprintf(&b, "- Boundaries (do NOT exceed): %s\n", spec.Boundaries)
	}
	if b.Len() == 0 {
		return ""
	}
	return "## Task contract\n" +
		"This work was delegated to you. Honor every clause below; your single reply is all the caller sees.\n" +
		b.String() +
		"Stay strictly within the boundaries and return your result in exactly the requested output format."
}

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
