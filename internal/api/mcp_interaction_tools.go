package api

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/interaction"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// callAsk emits a transient ask step and blocks until the user answers (via
// POST /api/chat/control {action:"answer"}), the turn ends, or the timeout fires.
func (b *interactionBackend) callAsk(ctx context.Context, run *chatRun, args json.RawMessage) (interaction.CallResult, error) {
	// Shared tolerant parser: accepts TionSwarm's {question, options} and claude-cli's
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
	// Strip the Interaction MCP namespace so a bridged/activated tool is classified by
	// its REAL bare name (mcp__tionswarm_extended__create_agent -> create_agent). Without
	// this, a namespaced name misses the risk table and defaults to RiskWrite — safe
	// (never wrongly auto-allows) but it would needlessly prompt for a read-only tool and
	// skip matching standing grants. Gateway-activated extended tools (Doc 52 Faz 1-b)
	// reach the CLI's permission prompt under their namespaced name, so this matters
	// exactly for them (YENI-A). Non-interaction names (native Bash, external mcp__x__y)
	// are returned unchanged and keep their existing classification.
	toolName := bareToolName(in.ToolName)
	risk := tools.Classify(toolName)
	// Argument-aware grants (B2): honour a standing rule (e.g. Bash(git *)) that
	// already covers this command without re-prompting.
	arg := tools.RepresentativeArg(toolName, in.Input)
	if risk == tools.RiskRead || run.grantStore().Matches(toolName, arg) {
		return interaction.CallResult{Text: permDecision(true, in.Input, "")}, nil
	}
	run.emit("step", agent.TurnStep{Kind: agent.StepPermission, Tool: toolName, Reason: string(risk), Text: arg, Options: tools.PermissionOptions})
	select {
	case ans := <-run.answer:
		switch tools.NormalizePermission(ans) {
		case "always":
			// Derive the standing rule from the bare name so it matches future calls (we
			// now match grants by the stripped name above).
			run.grantStore().GrantRule(tools.DeriveGrantRule(toolName, arg))
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
// path), which is bound to the workspace sandbox and uses TionSwarm's own shell
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
