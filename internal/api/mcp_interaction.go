package api

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
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
	"PowerShell":           true, // Windows-native shell (OS-default bridged shell)
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
	// gate. The CLI gets ONE shell: the OS-native one — PowerShell on Windows, Bash
	// on Unix — so a claude-cli agent runs commands through SwarmGo's sandboxed shell
	// (and the CLI's own native Bash can be safely disallowed). The runner that backs
	// it (NewShellRunner) picks the SAME shell, and callShell dispatches both names.
	if tun != nil && tun.ShellEnabled() {
		if runtime.GOOS == "windows" {
			defs = append(defs, tools.NewPowerShellTool(tools.Sandbox{}).Def())
		} else {
			defs = append(defs, tools.NewShellTool(tools.Sandbox{}).Def())
		}
	}
	// spawn_session is a self-management capability, always advertised now (the
	// self-manage master toggle was removed). The per-turn spawn tool is installed
	// on each run by the stream handler.
	defs = append(defs, tools.NewSpawnSessionTool("", 0, nil).Def())
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
	case "Bash", "PowerShell":
		// One bridged shell per OS (Bash on Unix, PowerShell on Windows); the runner
		// resolves which one. Accept both names so the dispatch never depends on OS.
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

// interactionURL builds the loopback URL a CLI subprocess uses to reach this
// server's /mcp/interaction endpoint, or "" when the base URL is unknown.
func (s *Server) interactionURL() string {
	if s.selfURL == "" {
		return ""
	}
	return strings.TrimRight(s.selfURL, "/") + "/mcp/interaction"
}
