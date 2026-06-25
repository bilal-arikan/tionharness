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

// Tools implements interaction.Backend. The specs come from the single tool
// definitions in the tools package — the schema is never re-declared here, so the
// native and CLI paths advertise the identical contract.
func (b *interactionBackend) Tools(token string) []interaction.ToolSpec {
	// Resolve the run first so the advertised set matches the turn's mode: an
	// autonomous turn drops the interactive (ask_user/request_confirmation) tools.
	run := b.runs.byToken(token)
	specs := interactionToolSpecs(b.tun, run != nil && run.autonomous)
	// Append the run's bridged self-management tools (CLI-3), deduped by name
	// against the static set (spawn_session is advertised by both paths).
	if run == nil {
		return specs
	}
	defs := run.bridgeDefsFor()
	if len(defs) == 0 {
		return specs
	}
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
	return specs
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

// mergeInteractionToolNames returns the static interaction tool names plus the
// bridged self-management tool names (CLI-3), deduped — the CLI allowlist source.
// Mirrors what Tools(token) advertises so allowlist and tools/list agree.
func mergeInteractionToolNames(static []string, bridge []providers.ToolDef) []string {
	seen := make(map[string]bool, len(static)+len(bridge))
	out := make([]string, 0, len(static)+len(bridge))
	for _, n := range static {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, d := range bridge {
		if !seen[d.Name] {
			seen[d.Name] = true
			out = append(out, d.Name)
		}
	}
	return out
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

// bareToolName strips the Interaction MCP namespace so dispatch matches whether
// the CLI sends the namespaced (mcp__swarmgo_interaction__ask_user) or bare name.
func bareToolName(name string) string {
	return strings.TrimPrefix(name, "mcp__swarmgo_interaction__")
}

// Call implements interaction.Backend.
func (b *interactionBackend) Call(ctx context.Context, token, name string, args json.RawMessage) (interaction.CallResult, error) {
	run := b.runs.byToken(token)
	if run == nil {
		return interaction.CallResult{}, errors.New("no live turn for token")
	}
	switch bareToolName(name) {
	case "ask_user":
		return b.callAsk(ctx, run, args)
	case "request_confirmation":
		return b.callConfirm(ctx, run, args)
	case "permission_prompt":
		return b.callPermission(ctx, run, args)
	case "todo_write":
		return b.callTodo(run, args)
	case "schedule_wake":
		return b.callWake(ctx, run, args)
	case "create_artifact", "update_artifact":
		return b.callArtifact(run, bareToolName(name), args)
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
	var in struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return interaction.CallResult{Text: "invalid ask_user input: " + err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(in.Question) == "" {
		return interaction.CallResult{Text: "question is required", IsError: true}, nil
	}
	return b.blockForAnswer(ctx, run, in.Question, in.Options, func(a string) string { return a })
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
func (b *interactionBackend) callTodo(run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	ctx := context.Background()
	if sink := run.todoSink(); sink != nil {
		ctx = tools.WithTodoSink(ctx, sink)
	}
	text, err := tools.NewTodoWriteTool().Call(ctx, args)
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	return interaction.CallResult{Text: text}, nil
}

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

// callArtifact creates or updates a versioned artifact through the run's sink. The
// CLI's stream-json trace surfaces the call as an artifact card (no live emit).
func (b *interactionBackend) callArtifact(run *chatRun, name string, args json.RawMessage) (interaction.CallResult, error) {
	sink := run.artifactSink()
	if sink == nil {
		return interaction.CallResult{Text: "artifacts are not available for this turn", IsError: true}, nil
	}
	actx := tools.WithArtifacts(context.Background(), sink)
	var (
		text string
		err  error
	)
	if name == "create_artifact" {
		text, err = tools.NewCreateArtifactTool().Call(actx, args)
	} else {
		text, err = tools.NewUpdateArtifactTool().Call(actx, args)
	}
	if err != nil {
		return interaction.CallResult{Text: err.Error(), IsError: true}, nil
	}
	return interaction.CallResult{Text: text}, nil
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
