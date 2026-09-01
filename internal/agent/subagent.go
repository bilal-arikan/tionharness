package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/prompts"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
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
// run_subagent target. Each profile's system prompt lives in the central prompt
// registry (internal/prompts, key "subagent-<id>") — resolve one through
// Runtime.subagentProfile so a workspace override is honored. Allowlists stay
// in code (they are a safety contract, not prose) and intersect with the
// caller's own effective tools. An exact lower-case profile id always selects
// this contract; use a differently-cased workspace-agent name when an agent and
// profile intentionally share a name.
//
// The "config" profile is the mini-agent analog (the external agent project
// getMiniAgentSystemPrompt): a cheap, tightly-scoped editor for a workspace's
// config/ files. It is sandboxed to the config tools (which are themselves
// rooted at config/), so it can never wander the filesystem or grow the change.
var defaultSubagentProfiles = map[string]SubagentProfile{
	"explore": {ID: "explore", AllowedTools: []string{"Read", "LS", "Glob", "Grep", "WebFetch"}},
	// "planner" turns a task into an executable plan: it reads the code (like
	// explore) but its product is an ordered set of change sites, not findings. It
	// is read-only for the same reason validator is — a planner that "just fixes
	// this one thing" produces a plan that no longer matches the tree.
	"planner":  {ID: "planner", AllowedTools: []string{"Read", "LS", "Glob", "Grep", "WebFetch"}},
	"coder":    {ID: "coder", AllowedTools: []string{"Read", "LS", "Glob", "Grep", "Write", "Edit", "Bash"}},
	"reviewer": {ID: "reviewer", AllowedTools: []string{"Read", "LS", "Glob", "Grep"}},
	// "validator" proves another worker's change actually works: it may run the
	// codebase (tests, typecheck, build, git), but it does NOT edit source — its
	// verdict must reflect the code as written, not a fix it quietly slipped in.
	// Its prompt (subagent-validator) enforces a compact PASS/FAIL verdict so the
	// coordinator reads a decision, not raw logs.
	//
	// It has NO browser: driving one needs the playwright MCP server, and no
	// profile allowlist names a browser tool (the codebase-memory graph is the one
	// exemption — allowlistExemptServer — and it is not a browser), so e2e is
	// out of scope here — the
	// prompt says "(when available) a browser" and reports "e2e: n/a", which is
	// the honest answer for every profile worker. This comment once claimed the
	// opposite; on the native path that was never true, and on the CLI path it
	// only appeared true while external MCP servers bypassed the agent's tool
	// restriction entirely (fixed — see mcpservergate.go). Widening a profile to
	// an MCP server is a deliberate policy change, not a comment edit.
	"validator": {ID: "validator", AllowedTools: []string{"Read", "LS", "Glob", "Grep", "Bash", "unity-mcp__*"}},
	"config":    {ID: "config", AllowedTools: []string{"list_config", "read_config", "write_config", "config_validate"}},
}

// subagentProfile resolves a built-in profile by target name, filling its
// system prompt from the registry (workspace override → embedded default).
func (r *Runtime) subagentProfile(target string) (SubagentProfile, bool) {
	p, ok := defaultSubagentProfiles[strings.ToLower(target)]
	if !ok {
		return SubagentProfile{}, false
	}
	systemKey := "subagent-" + p.ID
	if systemAgent, _, err := r.ResolveSystemAgent(systemKey); err == nil {
		p.SystemPrompt = systemAgent.Soul
	} else {
		r.logger.Warn("subagent system agent resolution failed; using prompt registry fallback", "systemKey", systemKey, "error", err)
		p.SystemPrompt = r.readPrompt(systemKey)
	}
	return p, true
}

// SubagentProfiles returns the built-in profiles with their DEFAULT prompts
// (sorted-free; for display). Live resolution goes through subagentProfile.
func SubagentProfiles() []SubagentProfile {
	out := make([]SubagentProfile, 0, len(defaultSubagentProfiles))
	for _, p := range defaultSubagentProfiles {
		p.SystemPrompt = prompts.Default("subagent-" + p.ID)
		out = append(out, p)
	}
	return out
}

// subStepSink collects a subagent's nested activity trace so the parent tool loop
// can promote the run_subagent tool row into a StepSubagent carrying SubSteps.
// When a live emitter is bound (bindLive), every nested step ALSO republishes a
// partial StepSubagent card keyed by the parent call id, so the delegation is
// visible in the chat while it runs instead of only after it returns. The mutex
// is not decorative: parallel fan-out runs several subagents concurrently and the
// parent loop reads the collected steps from another goroutine.
type subStepSink struct {
	mu    sync.Mutex
	steps []TurnStep
	live  *liveCard
}

type subStepSinkKey struct{}

// bindLive attaches the parent turn's step emitter and the run_subagent call id
// this sink belongs to. Without it the sink only collects (legacy behaviour).
func (s *subStepSink) bindLive(card *liveCard) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live = card
}

// collected returns a copy of the nested steps gathered so far.
func (s *subStepSink) collected() []TurnStep {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]TurnStep(nil), s.steps...)
}

// setSteps replaces the collected trace (used by the parallel path, which runs
// the subagent under its own sink and hands the finished trace back).
func (s *subStepSink) setSteps(steps []TurnStep) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = steps
}

// addSteps appends to the collected trace.
func (s *subStepSink) addSteps(steps ...TurnStep) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = append(s.steps, steps...)
}

// emitLive publishes a partial StepSubagent card carrying everything gathered so
// far. No-op when no live emitter is bound. The card reuses the parent call id so
// the final step (same id) replaces it in the UI.
func (s *subStepSink) emitLive(input json.RawMessage) {
	s.mu.Lock()
	live := s.live
	steps := append([]TurnStep(nil), s.steps...)
	s.mu.Unlock()
	if live == nil {
		return
	}
	live.Update(func(st *TurnStep) {
		st.Kind = StepSubagent
		st.Tool = "run_subagent"
		st.Input = input
		st.SubSteps = steps
	})
}

// withSubStepSink attaches a fresh sink to ctx and returns it; the parent loop
// reads sink.collected() after the run_subagent call to nest the subagent's trace.
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
// ephemeral profile worker or an existing agent), then runs it to completion in
// an isolated or inherited context and returns its final reply.
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
	// Every subagent run is persisted as a child of the calling session, so resolve
	// the parent once here — failing now keeps a context without a session from
	// spending budget or resolving a provider first.
	parentSessionID := SessionIDFrom(ctx)
	if parentSessionID == "" {
		return tools.RunAgentResult{}, fmt.Errorf("subagent persistence requires a parent session")
	}
	// Guard 5 — retry lineage. Checked before the budget spend so a bad reference
	// costs the caller nothing and can simply be corrected.
	retryOfID, attempt, err := r.resolveRetryLineage(ctx, parentSessionID, spec.RetryOf)
	if err != nil {
		return tools.RunAgentResult{}, err
	}
	if m := strings.TrimSpace(spec.Model); m != "" {
		agent.Model = m
	}
	provider, err := r.providers.Get(agent.ProviderRef())
	if err != nil {
		return tools.RunAgentResult{}, fmt.Errorf("target %q provider unavailable: %w", agent.Name, err)
	}

	// Spend one unit of the shared per-turn budget (atomic so concurrent fan-out
	// never exceeds the cap; refund and refuse if this call would push us over).
	if !cur.spend(maxCalls) {
		return tools.RunAgentResult{}, fmt.Errorf("subagent budget (%d per turn) exhausted; do the rest yourself", maxCalls)
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
	childMeta := subagentSessionMeta(parentSessionID, agent, ephemeral, spec)
	stampRetryLineage(&childMeta, retryOfID, attempt)
	child, err := r.db.CreateChildSession(ctx, childMeta)
	if err != nil {
		// No child row, no turn: the unit was never used.
		cur.refund()
		return tools.RunAgentResult{}, err
	}
	childCtx = WithSessionID(childCtx, child.ID)
	// Re-bind the session-scoped sinks to the CHILD before the run. They are
	// inherited from the caller's context, and the tool loop only installs a
	// fallback when none is present — so without this an artifact the subagent
	// creates is filed under the CALLER's session and agent, and the delegated work
	// loses its provenance exactly where it matters most (a file produced by a
	// subagent looks like the parent wrote it).
	childCtx = tools.WithCurrentSession(childCtx, child.ID)
	childCtx = tools.WithArtifacts(childCtx, r.NewArtifactSink(child.ID, agent.ID))
	if err := r.initializeChildSession(ctx, child.ID, func() error {
		_, err := r.db.AddMessage(ctx, db.Message{SessionID: child.ID, Role: "user", Text: strings.TrimSpace(spec.Task)})
		return err
	}); err != nil {
		// The child row exists but is stamped failed and no turn ran — refund.
		cur.refund()
		return tools.RunAgentResult{}, err
	}

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
	sys := r.autonomousSystemPrompt(ctx, agent)
	if contract := delegationContract(spec); contract != "" {
		sys = strings.TrimSpace(sys + "\n\n" + contract)
	}
	req := providers.Request{
		Model:  agent.Model,
		System: sys,
		// Volatile turn-start clock (+ lessons when ctx carries a session)
		// rides the dynamic suffix, keeping the static prefix cacheable.
		SystemDynamic: r.autonomousDynamicSuffix(ctx, agent),
		Messages:      msgs,
	}

	r.logger.Info("subagent run",
		"from", caller.ID, "target", agent.Name, "ephemeral", ephemeral,
		"depth", cur.depth+1, "context", orDefault(spec.Context, "isolated"))

	// Live card: publish the delegation the moment the target is resolved (before
	// the first token), then republish it on every nested step. Without this the
	// chat shows nothing at all until the subagent's final reply lands. The card
	// carries the resolved target + task so the header reads like the final one.
	sink := subStepSinkFrom(ctx)
	liveInput := subagentCardInput(agent.Name, task)
	var onStep func(TurnStep)
	if sink != nil {
		sink.emitLive(liveInput)
		onStep = func(st TurnStep) {
			sink.addSteps(st)
			sink.emitLive(liveInput)
		}
	}

	// Run in an isolated trace. autonomous=true keeps it headless (interactive
	// tools like ask_user no-op) and enforces the caller's daily budget.
	//
	// Past this point the run is real: no failure below refunds the budget unit —
	// provider tokens were spent and the cap exists to bound exactly those.
	started := time.Now()
	resp, steps, err := r.completeTraced(WithCallKind(childCtx, KindSubagent), agent, provider, req, true, onStep)
	if err != nil {
		state := "failed"
		if errors.Is(err, context.Canceled) {
			state = "killed"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			state = "timeout"
		}
		summary := "subagent failed (" + string(classifyProviderError(err)) + ")"
		if persistErr := r.recordChildAssistantMessage(ctx, child.ID, agent.ID, summary, steps, nil, time.Since(started).Milliseconds(), state); persistErr != nil {
			return tools.RunAgentResult{}, fmt.Errorf("subagent %q failed: %w (persist transcript: %v)", agent.Name, err, persistErr)
		}
		return tools.RunAgentResult{}, fmt.Errorf("subagent %q failed: %w", agent.Name, err)
	}
	meta := &turnMeta{}
	meta.capture(resp)
	state := "completed"
	if resp.StopReason == providers.StopMaxTok {
		state = "incomplete"
	}
	if err := r.recordChildAssistantMessage(ctx, child.ID, agent.ID, resp.Text, steps, meta, time.Since(started).Milliseconds(), state); err != nil {
		return tools.RunAgentResult{}, err
	}
	// Hand the subagent's nested trace to the parent loop (when a sink is wired) so
	// it renders as a collapsible StepSubagent. The returned trace is authoritative
	// and supersedes what the live emitter accumulated.
	if sink != nil {
		sink.setSteps(steps)
	}
	// SubagentStop lifecycle hook (Claude Code parity): a delegated subagent
	// finished. Fire-and-forget audit; its injected context (if any) is folded
	// onto the subagent's returned trace so the parent still sees it.
	if sub := r.RunLifecycleHooks(ctx, "", db.HookSubagentStop, LifecycleExtras{}); len(sub.Steps) > 0 {
		if sink != nil {
			sink.addSteps(sub.Steps...)
		}
	}
	return tools.RunAgentResult{
		AgentName: agent.Name,
		Reply:     resp.Text,
		Artifacts: r.collectChildArtifacts(ctx, child.ID),
	}, nil
}

func subagentSessionMeta(parentID string, agent db.Agent, ephemeral bool, spec tools.RunAgentSpec) db.Session {
	contextMode := strings.TrimSpace(spec.Context)
	if contextMode == "" {
		contextMode = db.ContextIsolated
	}
	s := db.Session{Kind: subagentSessionKind, ParentSessionID: parentID, ExecutionType: db.ExecutionSubagent, Category: db.CategorySubagent, ContextMode: contextMode, Visibility: db.VisibilityInternal}
	if ephemeral {
		s.TargetProfile = strings.TrimSpace(spec.Target)
	} else {
		s.TargetAgentID = agent.ID
	}
	return s
}

// initializeChildSession commits the opening user turn before exposing the child
// as running. A failure after CreateChildSession must leave a terminal, inspectable
// row instead of an internal session that looks live forever.
func (r *Runtime) initializeChildSession(ctx context.Context, sessionID string, addOpeningMessage func() error) error {
	if err := addOpeningMessage(); err != nil {
		return r.failChildInitialization(ctx, sessionID, "persist opening user message", err)
	}
	if err := r.db.SetSessionRunState(ctx, sessionID, runStateRunning, time.Now().Unix()); err != nil {
		return r.failChildInitialization(ctx, sessionID, "persist running state", err)
	}
	return nil
}

func (r *Runtime) failChildInitialization(ctx context.Context, sessionID, operation string, cause error) error {
	stateErr := r.db.SetSessionRunState(ctx, sessionID, turnStatusFailed, time.Now().Unix())
	if stateErr != nil {
		err := fmt.Errorf("initialize child session: %s: %w (persist failed run state: %v)", operation, cause, stateErr)
		r.logger.Error("subagent child initialization failed", "session", sessionID, "operation", operation, "error", cause, "stateError", stateErr)
		return err
	}
	r.logger.Error("subagent child initialization failed", "session", sessionID, "operation", operation, "error", cause)
	return fmt.Errorf("initialize child session: %s: %w", operation, cause)
}

// childTranscriptSteps keeps execution trace shape while dropping raw tool
// arguments. Arguments may contain credentials or environment secrets and are
// not needed to understand the delegated run after completion.
func childTranscriptSteps(steps []TurnStep) []TurnStep {
	out := append([]TurnStep(nil), steps...)
	for i := range out {
		if out[i].Kind == StepTool {
			out[i].Input = nil
		}
		if len(out[i].SubSteps) > 0 {
			out[i].SubSteps = childTranscriptSteps(out[i].SubSteps)
		}
	}
	return out
}

// resolveSubagentTarget maps a run_subagent target to a runnable agent. An exact
// lower-case built-in profile id yields an EPHEMERAL agent cloned from the caller
// and cannot be shadowed by persisted state. Otherwise an existing workspace
// agent is preferred (e.g. "Reviewer" selects a real agent while `reviewer`
// selects the profile). If no real agent matches, case-insensitive profile
// resolution remains as a compatibility fallback. Ephemeral profiles inherit
// provider, model, daily limits and permission mode, usage attributed to the
// caller) but reshaped with the profile's system prompt and tool allowlist.
func (r *Runtime) resolveSubagentTarget(ctx context.Context, caller db.Agent, target string) (db.Agent, bool, error) {
	// An exact lower-case profile id is an explicit request for the built-in
	// contract. Do not let a persisted agent with the same display name shadow it:
	// that bypasses the profile allowlist on fresh run_subagent calls. A differently
	// cased name (for example "Reviewer") remains an explicit workspace-agent
	// reference for backwards compatibility.
	if target == strings.ToLower(strings.TrimSpace(target)) {
		if p, ok := r.subagentProfile(target); ok {
			return r.ephemeralSubagent(caller, p), true, nil
		}
	}
	// Prefer an existing agent so a user-named agent wins over a same-named profile.
	if a, err := r.resolveAgent(ctx, target); err == nil {
		return a, false, nil
	}
	if p, ok := r.subagentProfile(target); ok {
		return r.ephemeralSubagent(caller, p), true, nil
	}
	return db.Agent{}, false, fmt.Errorf("unknown subagent target %q: not an existing agent and not a built-in profile (explore|coder|reviewer|validator|config)", target)
}

func (r *Runtime) ephemeralSubagent(caller db.Agent, p SubagentProfile) db.Agent {
	eph := caller // clone limits/provider/model/permission from the caller
	eph.Name = "subagent:" + p.ID
	eph.Soul = p.SystemPrompt
	eph.Identity = ""
	eph.Skills = nil
	eph.MCPEnabled = true
	if len(p.AllowedTools) == 0 {
		eph.AllowedTools = caller.AllowedTools
	} else {
		b, _ := json.Marshal(p.AllowedTools)
		eph.AllowedTools = string(b)
	}
	return eph
}

// subFuture is the pending result of a run_subagent call started concurrently by
// the parallel fan-out path. done is closed when res/steps are ready.
type subFuture struct {
	res   providers.ToolResult
	steps []TurnStep
	done  chan struct{}
	card  *liveCard
}

// subagentCardInput renders the {target, task} payload the subagent card header
// parses. It mirrors the run_subagent call input so a live (partial) card and the
// final one read identically; on the live path the target is already RESOLVED to
// the agent's display name.
func subagentCardInput(target, task string) json.RawMessage {
	b, err := json.Marshal(struct {
		Target string `json:"target"`
		Task   string `json:"task"`
	}{Target: target, Task: task})
	if err != nil {
		// Both fields are plain strings; a failure here means the encoder itself
		// broke, which must not be swallowed into a silently blank card.
		panic("subagent card input marshal: " + err.Error())
	}
	return b
}

// launchParallelSubagents starts every run_subagent call in a batch concurrently,
// returning a map keyed by call id so the tool loop can await each in turn. It
// returns nil when the batch has fewer than two run_subagent calls — there is no
// parallelism to win, so the loop runs them inline as usual. Each subagent runs
// in its own context with a private trace sink; the shared per-turn budget guard
// (atomic) bounds total fan-out. Hooks/permission still run sequentially in the
// loop before the result is consumed (a blocked call simply discards its future).
// emit (may be nil) is the parent turn's step emitter: each call's sink is bound
// to it under that call's id so every fanned-out subagent streams its own live
// StepSubagent card while it runs.
func (r *Runtime) launchParallelSubagents(ctx context.Context, reg *tools.Registry, calls []providers.ToolCall, emit func(TurnStep)) map[string]*subFuture {
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
		if emit != nil && call.ID != "" {
			f.card = openLive(emit, call.ID, TurnStep{
				Kind:  StepTool,
				Tool:  call.Name,
				Input: call.Input,
			})
		}
		futures[call.ID] = f
		wg.Add(1)
		go func(call providers.ToolCall, f *subFuture) {
			defer wg.Done()
			defer close(f.done)
			cctx, sink := withSubStepSink(ctx)
			if f.card != nil {
				sink.bindLive(f.card)
			}
			f.res = reg.Call(cctx, call)
			f.steps = sink.collected()
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
