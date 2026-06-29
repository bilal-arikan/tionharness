package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/agent"
	"github.com/bilal-arikan/swarmgo/internal/interaction"
	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// askTimeout bounds a blocking ask_user call so a never-answering user can't pin
// the CLI tool call forever; on timeout the tool returns an error and the model
// proceeds on its own.
const askTimeout = 15 * time.Minute

// interactionBackend adapts the in-flight chat-run registry to the Interaction
// MCP server: it resolves a per-run Bearer token to its chatRun and dispatches
// ask_user / todo_write against that turn (emitting the same UI steps the native
// tool path emits). See _Docs/11-INTERACTION-MCP.md.
type interactionBackend struct {
	runs *chatRuns
	tun  *agent.Tunables // gates self-manage tools (spawn_session) on the CLI path
}

// Valid implements interaction.Backend.
func (b *interactionBackend) Valid(token string) bool {
	return b.runs.byToken(token) != nil
}

// coreInteractionTools is the eager tier of Interaction MCP tools: the ones the
// CLI MCP-config advertises on the alwaysLoad `swarmgo_interaction` server so they
// are NEVER deferred by the CLI's tool search (Bash, ask_user, the artifact/skill
// path, working-memory edits, permission_prompt). Everything else (notify,
// focus_view, the session-lifecycle setters, schedule_wake, spawn_session) plus the
// entire bridged self-management suite is the EXTENDED tier → a separate
// `swarmgo_extended` server subject to ToolSearch deferral. This mirrors the native
// registry's eager-vs-lazy split (see _Docs/19) and is the single source for both
// the tier filter below and the per-tier CLI allowlist.
var coreInteractionTools = map[string]bool{
	"Bash":                 true,
	"ask_user":             true,
	"request_confirmation": true,
	"todo_write":           true,
	"create_artifact":      true,
	"update_artifact":      true,
	"use_skill":            true,
	"skill_search":         true,
	"run_subagent":         true,
	"core_memory_replace":  true,
	"core_memory_append":   true,
	"permission_prompt":    true,
}

// interactionTier classifies a bare tool name into "core" or "extended".
func interactionTier(name string) string {
	if coreInteractionTools[name] {
		return "core"
	}
	return "extended"
}

// Tools implements interaction.Backend. The specs come from the single tool
// definitions in the tools package — the schema is never re-declared here, so the
// native and CLI paths advertise the identical contract. tier filters the result
// so each CLI MCP server entry (alwaysLoad core / deferred extended) gets its own
// subset; tier "" returns the full set.
func (b *interactionBackend) Tools(token, tier string) []interaction.ToolSpec {
	// Resolve the run first so the advertised set matches the turn's mode: an
	// autonomous turn drops the interactive (ask_user/request_confirmation) tools.
	run := b.runs.byToken(token)
	specs := interactionToolSpecs(b.tun, run != nil && run.autonomous)
	// Append the run's bridged self-management tools (CLI-3), deduped by name
	// against the static set (spawn_session is advertised by both paths).
	if run != nil {
		if defs := run.bridgeDefsFor(); len(defs) > 0 {
			seen := make(map[string]bool, len(specs))
			for _, s := range specs {
				seen[s.Name] = true
			}
			for _, d := range defs {
				if seen[d.Name] {
					continue
				}
				specs = append(specs, interaction.ToolSpec{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema})
			}
		}
	}
	if tier == "" {
		return specs
	}
	filtered := make([]interaction.ToolSpec, 0, len(specs))
	for _, s := range specs {
		if interactionTier(s.Name) == tier {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// interactionAdvertisedNames returns the bare tool names the Interaction MCP
// server advertises for a turn (gated by self-manage exactly like the specs).
// The CLI MCP-config writer consumes this via InteractionEndpoint.ToolNames so
// the advertised set and the CLI allowlist share ONE source — no second list.
func interactionAdvertisedNames(tun *agent.Tunables, autonomous bool) []string {
	specs := interactionToolSpecs(tun, autonomous)
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	return names
}

// interactiveOnlyTools need a live user to answer and therefore CANNOT work on an
// autonomous (scheduler/spawn/flow) turn: ask_user/request_confirmation block for
// a human and, with none present, return an is_error "proceed on your own" result
// — which a claude-cli child can mishandle into an exit-1 failure (the cause of
// flow parallel-node crashes). So they are omitted from the advertised set on
// autonomous turns: the agent never sees them and simply proceeds on its own.
var interactiveOnlyTools = map[string]bool{
	"ask_user":             true,
	"request_confirmation": true,
}

// splitInteractionTiers returns the static interaction tool names plus the bridged
// self-management tool names (CLI-3), deduped and split into the core (eager,
// alwaysLoad) and extended (deferred) tiers — the CLI allowlist source. Mirrors
// what Tools(token, tier) advertises so allowlist and tools/list agree per tier.
func splitInteractionTiers(static []string, bridge []providers.ToolDef) (core, extended []string) {
	seen := make(map[string]bool, len(static)+len(bridge))
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		if interactionTier(name) == "core" {
			core = append(core, name)
		} else {
			extended = append(extended, name)
		}
	}
	for _, n := range static {
		add(n)
	}
	for _, d := range bridge {
		add(d.Name)
	}
	return core, extended
}

// interactionToolSpecs builds the Interaction MCP tool specs for a turn. Single
// source for both Tools() (advertisement) and interactionAdvertisedNames (CLI
// allowlist). tun may be nil (then self-manage tools are omitted).
func interactionToolSpecs(tun *agent.Tunables, autonomous bool) []interaction.ToolSpec {
	defs := []providers.ToolDef{
		tools.NewAskUserTool().Def(),
		tools.NewTodoWriteTool().Def(),
		tools.NewRequestConfirmationTool().Def(),
		tools.NewCreateArtifactTool().Def(),
		tools.NewUpdateArtifactTool().Def(),
		// notify raises a non-blocking desktop notification so a CLI agent can get
		// the user's attention (job done, attention needed). Stays advertised on
		// autonomous turns too — it never blocks for a live user.
		tools.NewNotifyTool().Def(),
		// focus_view drives the user's UI to a view/entity to direct attention.
		// Non-blocking; advertised on autonomous turns too (no-op with no open window).
		tools.NewFocusViewTool().Def(),
		// set_session_goal / complete_goal write THIS session's persistent objective
		// (the same db.Session.Goal the user edits). Non-blocking; advertised on
		// autonomous turns too (a scheduled run can set/complete its own goal).
		tools.NewSetSessionGoalTool().Def(),
		tools.NewCompleteGoalTool().Def(),
		// set_session_title / set_working_dir / archive_session mutate THIS session's
		// own metadata. Non-blocking; advertised on autonomous turns too.
		tools.NewSetSessionTitleTool().Def(),
		tools.NewSetWorkingDirTool().Def(),
		tools.NewArchiveSessionTool().Def(),
		// schedule_wake replaces the CLI's native ScheduleWakeup (which SwarmGo
		// disallows): the CLI runs one-shot, so its built-in wake never fires —
		// ours arms a real SwarmGo timer that re-delivers into this session.
		tools.NewScheduleWakeTool().Def(),
		// use_skill loads a SwarmGo skill body on demand. The CLI sees the skill
		// catalog in its appended system prompt but has no native way to load a
		// body; this bridge gives it the same lazy-load path native agents use.
		tools.NewUseSkillTool(nil).Def(),
		// skill_search lets a claude-cli agent discover on-demand/conditional skills
		// not advertised in its appended catalog, mirroring the native path. (SK-2)
		tools.NewSkillSearchTool(nil).Def(),
	}
	// shell is bridged only when enabled, mirroring the native tool loop's shell
	// gate. It lets a claude-cli agent run commands through SwarmGo's sandboxed
	// shell (PowerShell on Windows) instead of the CLI's native POSIX Bash — so
	// the CLI's Bash can be safely disallowed and shell behaviour stays consistent.
	if tun != nil && tun.ShellEnabled() {
		defs = append(defs, tools.NewShellTool(tools.Sandbox{}).Def())
	}
	// spawn_session is a self-management capability: advertise it on the CLI path
	// only when self-manage is enabled, mirroring the native tool loop's gating.
	// The per-turn spawn tool is installed on each run by the stream handler.
	if tun != nil && tun.SelfManageEnabled() {
		defs = append(defs, tools.NewSpawnSessionTool("", 0, nil).Def())
	}
	// run_subagent is bridged only when delegation is enabled, mirroring the native
	// tool loop's gate. Unlike spawn_session (fire-and-forget into a separate
	// session), it runs a subagent synchronously and returns its answer into THIS
	// turn — the CLI agent's "ask another agent and get the result back now" path.
	if tun != nil && tun.DelegationEnabled() {
		defs = append(defs, tools.NewRunSubagentTool().Def())
	}
	specs := make([]interaction.ToolSpec, 0, len(defs)+1)
	for _, d := range defs {
		// On autonomous turns, omit the interactive tools that need a live user —
		// the agent can't reach one, so advertising them only invites an is_error
		// "proceed on your own" result that can crash a claude-cli child.
		if autonomous && interactiveOnlyTools[d.Name] {
			continue
		}
		specs = append(specs, interaction.ToolSpec{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema})
	}
	// permission_prompt is CLI-only (the claude CLI calls it via
	// --permission-prompt-tool before running a tool that needs permission). It
	// has no native built-in counterpart, so its spec is declared inline.
	specs = append(specs, interaction.ToolSpec{
		Name:        "permission_prompt",
		Description: "Internal permission handler: the CLI calls this before running a tool that requires approval; it returns an allow/deny decision.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"tool_name":{"type":"string"},"input":{"type":"object"}},"required":["tool_name"]}`),
	})
	return specs
}

// sinkTool describes a sink-bound interaction tool: a shared builtin handler that
// runs once the per-run sink is injected as a context value. newTool builds the
// handler; attach pulls the run's sink and returns the augmented ctx (ok=false when
// a required sink is missing → the model gets `missing` as a graceful error).
type sinkTool struct {
	newTool func() tools.Tool
	attach  func(ctx context.Context, run *chatRun) (context.Context, bool)
	missing string
}

// Shared attach closures (one per sink kind). Artifact/notify/focus deliberately
// run on a fresh background ctx (fire-and-forget, not tied to turn cancellation);
// goal/session edits use the call ctx.
var (
	artifactAttach = func(_ context.Context, run *chatRun) (context.Context, bool) {
		s := run.artifactSink()
		if s == nil {
			return nil, false
		}
		return tools.WithArtifacts(context.Background(), s), true
	}
	goalAttach = func(ctx context.Context, run *chatRun) (context.Context, bool) {
		s := run.goalSinkFor()
		if s == nil {
			return nil, false
		}
		return tools.WithGoal(ctx, s), true
	}
	sessionAttach = func(ctx context.Context, run *chatRun) (context.Context, bool) {
		s := run.sessionSinkFor()
		if s == nil {
			return nil, false
		}
		return tools.WithSession(ctx, s), true
	}
)

// sinkToolTable maps a bare tool name to its sink-bound handler. todo_write is the
// one optional-sink case (it persists best-effort and always runs).
var sinkToolTable = map[string]sinkTool{
	"todo_write": {
		newTool: func() tools.Tool { return tools.NewTodoWriteTool() },
		attach: func(_ context.Context, run *chatRun) (context.Context, bool) {
			if s := run.todoSink(); s != nil {
				return tools.WithTodoSink(context.Background(), s), true
			}
			return context.Background(), true // sink optional: still publish the checklist
		},
	},
	"create_artifact": {newTool: func() tools.Tool { return tools.NewCreateArtifactTool() }, attach: artifactAttach, missing: "artifacts are not available for this turn"},
	"update_artifact": {newTool: func() tools.Tool { return tools.NewUpdateArtifactTool() }, attach: artifactAttach, missing: "artifacts are not available for this turn"},
	"notify": {newTool: func() tools.Tool { return tools.NewNotifyTool() }, attach: func(_ context.Context, run *chatRun) (context.Context, bool) {
		s := run.notifySink()
		if s == nil {
			return nil, false
		}
		return tools.WithNotify(context.Background(), s), true
	}, missing: "no notification channel is available for this turn"},
	"focus_view": {newTool: func() tools.Tool { return tools.NewFocusViewTool() }, attach: func(_ context.Context, run *chatRun) (context.Context, bool) {
		s := run.navSink()
		if s == nil {
			return nil, false
		}
		return tools.WithNavigate(context.Background(), s), true
	}, missing: "no UI is available to navigate for this turn"},
	"set_session_goal":  {newTool: func() tools.Tool { return tools.NewSetSessionGoalTool() }, attach: goalAttach, missing: "no session goal is available for this turn"},
	"complete_goal":     {newTool: func() tools.Tool { return tools.NewCompleteGoalTool() }, attach: goalAttach, missing: "no session goal is available for this turn"},
	"set_session_title": {newTool: func() tools.Tool { return tools.NewSetSessionTitleTool() }, attach: sessionAttach, missing: "no session is available to edit for this turn"},
	"set_working_dir":   {newTool: func() tools.Tool { return tools.NewSetWorkingDirTool() }, attach: sessionAttach, missing: "no session is available to edit for this turn"},
	"archive_session":   {newTool: func() tools.Tool { return tools.NewArchiveSessionTool() }, attach: sessionAttach, missing: "no session is available to edit for this turn"},
}

// callViaSink runs a sink-bound tool: attach the per-run sink, then call the shared
// builtin. Replaces the former per-tool callTodo/callArtifact/callNotify/callFocus/
// callGoal/callSessionEdit methods (one shape, one place).
func (b *interactionBackend) callViaSink(ctx context.Context, run *chatRun, st sinkTool, args json.RawMessage) (interaction.CallResult, error) {
	cctx, ok := st.attach(ctx, run)
	if !ok {
		return interaction.CallResult{Text: st.missing, IsError: true}, nil
	}
	text, err := st.newTool().Call(cctx, args)
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	return interaction.CallResult{Text: text}, nil
}

// bareToolName strips the Interaction MCP namespace so dispatch matches whether
// the CLI sends a namespaced name (core: mcp__swarmgo_interaction__ask_user,
// extended: mcp__swarmgo_extended__create_agent) or the bare name.
func bareToolName(name string) string {
	if s := strings.TrimPrefix(name, "mcp__swarmgo_interaction__"); s != name {
		return s
	}
	return strings.TrimPrefix(name, "mcp__swarmgo_extended__")
}

// Call implements interaction.Backend.
func (b *interactionBackend) Call(ctx context.Context, token, name string, args json.RawMessage) (interaction.CallResult, error) {
	run := b.runs.byToken(token)
	if run == nil {
		return interaction.CallResult{}, errors.New("no live turn for token")
	}
	bare := bareToolName(name)
	// Sink-bound tools (todo/artifact/notify/focus/goal/session-edit) all share one
	// shape: pull the per-run sink, inject it as a context value, call the shared
	// builtin handler. Dispatched through a single table+helper instead of one
	// near-identical method each (callViaSink / sinkToolTable).
	if st, ok := sinkToolTable[bare]; ok {
		return b.callViaSink(ctx, run, st, args)
	}
	switch bare {
	case "ask_user":
		return b.callAsk(ctx, run, args)
	case "request_confirmation":
		return b.callConfirm(ctx, run, args)
	case "permission_prompt":
		return b.callPermission(ctx, run, args)
	case "schedule_wake":
		return b.callWake(ctx, run, args)
	case "spawn_session":
		return b.callSpawn(ctx, run, args)
	case "use_skill":
		return b.callUseSkill(run, args)
	case "skill_search":
		return b.callSkillSearch(run, args)
	case "Bash":
		return b.callShell(ctx, run, args)
	case "run_subagent":
		return b.callRunSubagent(ctx, run, args)
	default:
		// CLI-3: dispatch a bridged self-management tool through the run's native
		// registry. The advertised catalog (and the per-agent tool filter) gates
		// what the CLI can name here.
		if call := run.bridgeCallFor(); call != nil {
			out, err := call(ctx, bareToolName(name), args)
			if err != nil {
				return interaction.CallResult{Text: err.Error(), IsError: true}, nil
			}
			return interaction.CallResult{Text: out}, nil
		}
		return interaction.CallResult{Text: "unknown tool: " + name, IsError: true}, nil
	}
}

// callAsk emits a transient ask step and blocks until the user answers (via
// POST /api/chat/control {action:"answer"}), the turn ends, or the timeout fires.
func (b *interactionBackend) callAsk(ctx context.Context, run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	// Shared tolerant parser: accepts SwarmGo's {question, options} and claude-cli's
	// native AskUserQuestion shapes (option objects + questions[] wrapper) so a model
	// trained on the native tool no longer errors with a schema mismatch (SES73).
	question, options, err := tools.ParseAskInput(args)
	if err != nil {
		return interaction.CallResult{Text: "invalid ask_user input: " + err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(question) == "" {
		return interaction.CallResult{Text: "question is required", IsError: true}, nil
	}
	return b.blockForAnswer(ctx, run, question, options, func(a string) string { return a })
}

// callConfirm blocks for a yes/no decision on a risky action and normalises it.
func (b *interactionBackend) callConfirm(ctx context.Context, run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	var in struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return interaction.CallResult{Text: "invalid request_confirmation input: " + err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.Question) == "" {
		return interaction.CallResult{Text: "question is required", IsError: true}, nil
	}
	return b.blockForAnswer(ctx, run, in.Question, tools.ConfirmOptions, tools.NormalizeConfirmation)
}

// blockForAnswer emits an ask step (question + clickable options) and blocks until
// the user answers, the turn ends, the request is cancelled, or the timeout fires.
// normalize maps the raw answer to the tool's result text.
func (b *interactionBackend) blockForAnswer(ctx context.Context, run *chatRun, question string, options []string, normalize func(string) string) (interaction.CallResult, error) {
	// Headless turn (scheduler/spawn): no live user can answer, so bail
	// at once rather than pinning the call until the 15-minute timeout.
	if run.autonomous {
		return interaction.CallResult{Text: "no interactive session is available (autonomous run); proceed on your own", IsError: true}, nil
	}
	run.emit("step", agent.TurnStep{Kind: agent.StepAsk, Text: question, Options: options})
	select {
	case ans := <-run.answer:
		return interaction.CallResult{Text: normalize(ans)}, nil
	case <-run.done:
		return interaction.CallResult{Text: "the turn ended before the user answered; proceed without the answer", IsError: true}, nil
	case <-ctx.Done():
		return interaction.CallResult{}, ctx.Err()
	case <-time.After(askTimeout):
		return interaction.CallResult{Text: "no answer within the time limit; proceed on your own", IsError: true}, nil
	}
}

// callPermission implements the claude CLI's --permission-prompt-tool contract:
// the CLI calls it before running a tool that needs approval. It classifies the
// tool by risk, auto-allows reads and already-granted tools, and otherwise emits
// a StepPermission card and blocks for the user's decision. It returns the JSON
// the CLI expects: {"behavior":"allow","updatedInput":..} or {"behavior":"deny"}.
// Only wired in "ask" mode (read-only uses CLI plan mode, auto uses bypass).
func (b *interactionBackend) callPermission(ctx context.Context, run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	var in struct {
		ToolName string          `json:"tool_name"`
		Input    json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return interaction.CallResult{Text: permDecision(false, in.Input, "invalid permission request: "+err.Error())}, nil
	}
	// ExitPlanMode is the CLI's plan-mode exit: the model presents a plan and asks
	// to leave plan mode. Surface it as a dedicated plan-approval card instead of a
	// generic permission prompt so the user sees the full plan before deciding.
	if in.ToolName == "ExitPlanMode" {
		return b.callExitPlan(ctx, run, in.Input)
	}
	risk := tools.Classify(in.ToolName)
	// Argument-aware grants (B2): honour a standing rule (e.g. Bash(git *)) that
	// already covers this command without re-prompting.
	arg := tools.RepresentativeArg(in.ToolName, in.Input)
	if risk == tools.RiskRead || run.grantStore().Matches(in.ToolName, arg) {
		return interaction.CallResult{Text: permDecision(true, in.Input, "")}, nil
	}
	run.emit("step", agent.TurnStep{Kind: agent.StepPermission, Tool: in.ToolName, Reason: string(risk), Text: arg, Options: tools.PermissionOptions})
	select {
	case ans := <-run.answer:
		switch tools.NormalizePermission(ans) {
		case "always":
			run.grantStore().GrantRule(tools.DeriveGrantRule(in.ToolName, arg))
			return interaction.CallResult{Text: permDecision(true, in.Input, "")}, nil
		case "allow":
			return interaction.CallResult{Text: permDecision(true, in.Input, "")}, nil
		default:
			return interaction.CallResult{Text: permDecision(false, in.Input, "denied by the user")}, nil
		}
	case <-run.done:
		return interaction.CallResult{Text: permDecision(false, in.Input, "the turn ended before approval")}, nil
	case <-ctx.Done():
		return interaction.CallResult{}, ctx.Err()
	case <-time.After(askTimeout):
		return interaction.CallResult{Text: permDecision(false, in.Input, "no approval within the time limit")}, nil
	}
}

// callExitPlan handles the claude-cli ExitPlanMode tool via the permission-prompt
// contract: it surfaces the proposed plan as a dedicated approval card (StepPlan)
// and blocks for the user's decision. Approve → allow (the CLI leaves plan mode
// and proceeds); reject → deny carrying the user's feedback so the model revises.
// On an autonomous turn (no live user) the plan is auto-approved so the run can
// continue unattended. The CLI expects the same allow/deny JSON as any prompt.
func (b *interactionBackend) callExitPlan(ctx context.Context, run *chatRun, input json.RawMessage) (interaction.CallResult, error) {
	var in struct {
		Plan string `json:"plan"`
	}
	_ = json.Unmarshal(input, &in)
	plan := strings.TrimSpace(in.Plan)
	if plan == "" {
		plan = "(boş plan)"
	}
	// Headless turn (scheduler/spawn): no live user can approve, so let the plan
	// stand and proceed rather than pinning the call until the timeout.
	if run.autonomous {
		b.capturePlanArtifact(run, plan)
		return interaction.CallResult{Text: permDecision(true, input, "")}, nil
	}
	run.emit("step", agent.TurnStep{Kind: agent.StepPlan, Text: plan, Options: tools.PlanOptions})
	select {
	case ans := <-run.answer:
		if tools.PlanApproved(ans) {
			b.capturePlanArtifact(run, plan)
			return interaction.CallResult{Text: permDecision(true, input, "")}, nil
		}
		return interaction.CallResult{Text: permDecision(false, input, "the user rejected the plan and asked to revise it: "+ans)}, nil
	case <-run.done:
		return interaction.CallResult{Text: permDecision(false, input, "the turn ended before the plan was approved")}, nil
	case <-ctx.Done():
		return interaction.CallResult{}, ctx.Err()
	case <-time.After(askTimeout):
		return interaction.CallResult{Text: permDecision(false, input, "no plan approval within the time limit")}, nil
	}
}

// capturePlanArtifact best-effort records an approved plan into the session's
// single rolling plan artifact (one per session — the accumulation guard). It is
// a silent no-op when artifacts are unavailable for the turn or the installed sink
// lacks the capability, so a capture failure never blocks the plan from proceeding.
func (b *interactionBackend) capturePlanArtifact(run *chatRun, plan string) {
	sink := run.artifactSink()
	if sink == nil {
		return
	}
	if appender, ok := sink.(interface {
		AppendPlanArtifact(context.Context, string) (tools.ArtifactRef, error)
	}); ok {
		_, _ = appender.AppendPlanArtifact(context.Background(), plan)
	}
}

// permDecision builds the JSON result the claude CLI permission-prompt tool must
// return. allow echoes the (unchanged) input as updatedInput; deny carries a
// message the model sees.
func permDecision(allow bool, input json.RawMessage, message string) string {
	if allow {
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		b, _ := json.Marshal(map[string]any{"behavior": "allow", "updatedInput": input})
		return string(b)
	}
	b, _ := json.Marshal(map[string]any{"behavior": "deny", "message": message})
	return string(b)
}

// callTodo validates the checklist (reusing the canonical tool) and returns the
// confirmation text. No live emit — the CLI's stream-json trace surfaces the
// todo_write call, which traceStepToTurnStep promotes to a checklist card. When
// the run carries a todo sink, the checklist is also persisted to the project's
// progress file so it survives across sessions (same as the native path).
// callWake arms a one-shot self-wake for the responding agent (CLI path). It
// reaches the wake scheduler installed on the run by the stream handler, which
// knows the session + responding agent. No blocking — returns immediately.
func (b *interactionBackend) callWake(ctx context.Context, run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	fn := run.wakeScheduler()
	if fn == nil {
		return interaction.CallResult{Text: "schedule_wake is not available for this turn", IsError: true}, nil
	}
	var in struct {
		DelaySeconds int    `json:"delaySeconds"`
		Prompt       string `json:"prompt"`
		Reason       string `json:"reason"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return interaction.CallResult{Text: "invalid schedule_wake input: " + err.Error(), IsError: true}, nil
	}
	out, err := fn(ctx, in.DelaySeconds, in.Prompt, in.Reason)
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	return interaction.CallResult{Text: out}, nil
}

// callSpawn launches a new independent session through the per-agent spawn tool
// installed on the run by the stream handler (CLI path). Returns immediately —
// the spawned session runs in the background and appears in the activity feed.
func (b *interactionBackend) callSpawn(ctx context.Context, run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	tool := run.spawnTool()
	if tool == nil {
		return interaction.CallResult{Text: "spawn_session is not available for this turn (self-management is off)", IsError: true}, nil
	}
	text, err := tool.Call(ctx, args)
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	return interaction.CallResult{Text: text}, nil
}

// callUseSkill loads a skill body through the run's per-agent skill loader (CLI
// path), enforcing the same allowlist as the native use_skill tool. The output
// matches the native tool's shape so both provider paths read identically.
func (b *interactionBackend) callUseSkill(run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	load := run.skillLoaderFor()
	if load == nil {
		return interaction.CallResult{Text: "skills are not available for this turn", IsError: true}, nil
	}
	var in struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return interaction.CallResult{Text: "invalid use_skill input: " + err.Error(), IsError: true}, nil
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		return interaction.CallResult{Text: "slug is required", IsError: true}, nil
	}
	body, err := load(slug)
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	// SK-3 parity: auto-grant the skill's declared allowed-tools to the session,
	// mirroring the native use_skill tool, so the CLI agent runs them without a
	// permission re-prompt.
	note := b.grantSkillToolsCLI(run, slug)
	if strings.TrimSpace(body) == "" {
		return interaction.CallResult{Text: "Skill \"" + slug + "\" has no instructions."}, nil
	}
	return interaction.CallResult{Text: "# Skill: " + slug + "\n\n" + body + note}, nil
}

// grantSkillToolsCLI registers the skill's declared allowed-tools as session
// grants (CLI path SK-3) and returns a transparency note for the tool output.
func (b *interactionBackend) grantSkillToolsCLI(run *chatRun, slug string) string {
	lookup := run.skillAllowedFor()
	g := run.grantStore()
	if lookup == nil || g == nil {
		return ""
	}
	var granted []string
	for _, p := range lookup(slug) {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		g.GrantRule(tools.ParsePermRule(p))
		granted = append(granted, p)
	}
	if len(granted) == 0 {
		return ""
	}
	return "\n\n---\n_Tools auto-allowed for this session by this skill: " + strings.Join(granted, ", ") + "._"
}

// callSkillSearch runs the skill_search tool over the run's per-agent searcher
// (CLI path), enforcing the same allowlist as the native skill_search tool and
// formatting results identically. (SK-2)
func (b *interactionBackend) callSkillSearch(run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	search := run.skillSearcherFor()
	if search == nil {
		return interaction.CallResult{Text: "skills are not available for this turn", IsError: true}, nil
	}
	var in struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return interaction.CallResult{Text: "invalid skill_search input: " + err.Error(), IsError: true}, nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	hits := search(strings.TrimSpace(in.Query), limit)
	if len(hits) == 0 {
		return interaction.CallResult{Text: "No skills match \"" + in.Query + "\"."}, nil
	}
	var sb strings.Builder
	sb.WriteString("Found skill(s). Load one with use_skill <slug>:\n")
	for _, h := range hits {
		sb.WriteString("- `" + h.Slug + "` — " + h.Description)
		if h.WhenToUse != "" {
			sb.WriteString(" (when: " + h.WhenToUse + ")")
		}
		sb.WriteString("\n")
	}
	return interaction.CallResult{Text: strings.TrimSpace(sb.String())}, nil
}

// callShell runs a shell command through the run's per-agent shell runner (CLI
// path), which is bound to the workspace sandbox and uses SwarmGo's own shell
// (PowerShell on Windows). Returns a graceful error result when shell is not
// available for this turn (disabled or no sandbox).
func (b *interactionBackend) callShell(ctx context.Context, run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	runFn := run.shellRunnerFor()
	if runFn == nil {
		return interaction.CallResult{Text: "shell is not available for this turn", IsError: true}, nil
	}
	out, err := runFn(ctx, args)
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	return interaction.CallResult{Text: out}, nil
}

// callRunSubagent dispatches a bridged run_subagent call: it runs a subagent
// synchronously through the runner installed on the run and returns its final
// result, so a claude-cli agent can delegate and get the answer back in this turn.
func (b *interactionBackend) callRunSubagent(ctx context.Context, run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	runFn := run.runAgentFor()
	if runFn == nil {
		return interaction.CallResult{Text: "run_subagent is not available for this turn (delegation disabled)", IsError: true}, nil
	}
	out, err := runFn(ctx, args)
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	return interaction.CallResult{Text: out}, nil
}

// interactionURL builds the loopback URL a CLI subprocess uses to reach this
// server's /mcp/interaction endpoint, or "" when the base URL is unknown.
func (s *Server) interactionURL() string {
	if s.selfURL == "" {
		return ""
	}
	return strings.TrimRight(s.selfURL, "/") + "/mcp/interaction"
}
