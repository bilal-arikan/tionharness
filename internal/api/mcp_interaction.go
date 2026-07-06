package api

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/interaction"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
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
	// srv is the streaming server, used to PUSH tools/list_changed when activate_tools
	// grows a session's extended surface (Doc 52 Faz 1-b). Set after server construction
	// (setServer); nil-safe — without it activation still mutates state, the client just
	// converges on its next tools/list instead of via an immediate push.
	srv *interaction.Server
	// mu guards activated. activated maps a session token to the set of extended tool
	// names the model has turned on this session (gateway dynamic surface). Only
	// consulted when tun.GatewayDynamicExtended() is on; otherwise the extended tier
	// advertises its full set and activated is unused.
	mu        sync.Mutex
	activated map[string]map[string]bool
}

// setServer wires the streaming server so the backend can push tools/list_changed.
func (b *interactionBackend) setServer(s *interaction.Server) { b.srv = s }

// dynamicExtended reports whether the gateway dynamic extended surface is on.
func (b *interactionBackend) dynamicExtended() bool {
	return b.tun != nil && b.tun.GatewayDynamicExtended()
}

// isActivated reports whether name is in the session's activated extended set.
func (b *interactionBackend) isActivated(token, name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	set := b.activated[token]
	return set != nil && set[name]
}

// activateExtended turns names on for a session and returns the names newly added
// (already-active names are skipped). Caller filters names to the real extended set.
func (b *interactionBackend) activateExtended(token string, names []string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.activated == nil {
		b.activated = map[string]map[string]bool{}
	}
	set := b.activated[token]
	if set == nil {
		set = map[string]bool{}
		b.activated[token] = set
	}
	var added []string
	for _, n := range names {
		if !set[n] {
			set[n] = true
			added = append(added, n)
		}
	}
	return added
}

// deactivateExtended turns names off for a session and returns the names removed.
func (b *interactionBackend) deactivateExtended(token string, names []string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	set := b.activated[token]
	if set == nil {
		return nil
	}
	var removed []string
	for _, n := range names {
		if set[n] {
			delete(set, n)
			removed = append(removed, n)
		}
	}
	return removed
}

// activeExtended returns a sorted snapshot of the session's activated extended tools.
func (b *interactionBackend) activeExtended(token string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	set := b.activated[token]
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Valid implements interaction.Backend.
func (b *interactionBackend) Valid(token string) bool {
	return b.runs.byToken(token) != nil
}

// coreInteractionTools is the eager tier of Interaction MCP tools: the ones the
// CLI MCP-config advertises on the alwaysLoad `tionswarm_interaction` server so they
// are NEVER deferred by the CLI's tool search (Bash, ask_user, the artifact/skill
// path, working-memory edits, permission_prompt). Everything else (notify,
// focus_view, the session-lifecycle setters, schedule_wake, spawn_session) plus the
// entire bridged self-management suite is the EXTENDED tier → a separate
// `tionswarm_extended` server subject to ToolSearch deferral. This mirrors the native
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
	"permission_prompt":    true,
	// Gateway meta-tools (Doc 52 Faz 1-b/2): activate/deactivate/list the extended
	// surface. Always core (eager) so the model can always grow the surface without a
	// discovery round-trip. Only advertised when GatewayDynamicExtended is on (see
	// interactionToolSpecs).
	"activate_tools":   true,
	"deactivate_tools": true,
	"active_tools":     true,
}

// cliTier classifies a bare tool name into the claude-cli wire tier: "core"
// (eager — advertised on the alwaysLoad tionswarm_interaction server, never deferred),
// "extended" (deferred — the tionswarm_extended server, discovered via the CLI's
// ToolSearch), or "hidden" (advertised on NEITHER server this turn).
//
// This is the projection of TionSwarm's 4-tier visibility model onto claude-cli's
// own two-state model (alwaysLoad vs tool-search). visOf reports a tool's effective
// visibility (from the per-agent registry); nil reproduces the historical static
// split (core set eager, everything else deferred, nothing hidden):
//   - coreInteractionTools members are eager regardless of visibility — they are the
//     behavioral/required primitives (permission_prompt, the shell, ask_user, the
//     artifact/skill/todo path) that must not pay a discovery round-trip.
//   - full        → core     (matches the native path's "schema shipped every turn")
//   - summary     → extended  (deferred; claude-cli cannot distinguish these two, so
//   - name-only   → extended   both project to a single deferred state)
//   - hidden      → hidden    (not advertised; re-allowlisted on a later turn if the
//     user raises the tool's visibility — the CLI analogue of native activate_tools)
func cliTier(name string, visOf func(string) string) string {
	if coreInteractionTools[name] {
		return "core"
	}
	if visOf == nil {
		return "extended"
	}
	switch visOf(name) {
	case tools.VisibilityFull:
		return "core"
	case tools.VisibilityHidden:
		return "hidden"
	default: // summary, name-only
		return "extended"
	}
}

// interactionTier is the visibility-agnostic classifier (static core set eager,
// the rest deferred). Retained for the context-preview projection, which has no
// live per-agent registry to consult.
func interactionTier(name string) string { return cliTier(name, nil) }

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
	// Classify with the run's per-agent visibility so tools/list agrees with the
	// allowlist splitInteractionTiers produced (same cliTier + same visOf). Without
	// a run (token unresolved) visOf is nil → the static split. A tool whose tier is
	// "hidden" matches neither "core" nor "extended", so it is advertised on neither
	// server this turn — the CLI analogue of the native hidden tier.
	var visOf func(string) string
	if run != nil {
		visOf = run.tierVisFor()
	}
	// Gateway dynamic surface (Doc 52 Faz 1-b): when on, the EXTENDED tier advertises
	// only the tools the model has activated this session — it starts empty and grows
	// via activate_tools + tools/list_changed. Core is unaffected (always eager). When
	// off, the extended tier advertises its full set (historical behaviour).
	dynExtended := tier == "extended" && b.dynamicExtended()
	filtered := make([]interaction.ToolSpec, 0, len(specs))
	for _, s := range specs {
		if cliTier(s.Name, visOf) != tier {
			continue
		}
		if dynExtended && !b.isActivated(token, s.Name) {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
}

// extendedCandidates returns the bare names that WOULD be advertised on the extended
// tier for this run (ignoring activation) — the valid targets for activate_tools. Used
// to validate activate/deactivate names so a typo is reported instead of silently
// registering a phantom tool.
func (b *interactionBackend) extendedCandidates(run *chatRun) map[string]bool {
	specs := interactionToolSpecs(b.tun, run != nil && run.autonomous)
	var bridge []providers.ToolDef
	var visOf func(string) string
	if run != nil {
		bridge = run.bridgeDefsFor()
		visOf = run.tierVisFor()
	}
	names := make([]string, 0, len(specs)+len(bridge))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	for _, d := range bridge {
		names = append(names, d.Name)
	}
	out := map[string]bool{}
	for _, n := range names {
		if cliTier(n, visOf) == "extended" {
			out[n] = true
		}
	}
	return out
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
// visOf reports each tool's effective visibility so the split honors the 4-tier
// model (full→core, summary/name-only→extended, hidden→neither); nil reproduces the
// historical static split. A hidden tool lands on neither list — it is not
// advertised on the wire this turn.
func splitInteractionTiers(static []string, bridge []providers.ToolDef, visOf func(string) string) (core, extended []string) {
	seen := make(map[string]bool, len(static)+len(bridge))
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		switch cliTier(name, visOf) {
		case "core":
			core = append(core, name)
		case "extended":
			extended = append(extended, name)
			// "hidden" → advertised on neither tier this turn.
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
		// update_session mutates THIS session's own metadata (title, working dir,
		// persistent goal + completion, tags, archive) in one call — the same
		// db.Session fields the user edits. Non-blocking; advertised on autonomous
		// turns too (a scheduled run can set/complete its own goal or retag itself).
		tools.NewUpdateSessionTool().Def(),
		// schedule_wake replaces the CLI's native ScheduleWakeup (which TionSwarm
		// disallows): the CLI runs one-shot, so its built-in wake never fires —
		// ours arms a real TionSwarm timer that re-delivers into this session.
		tools.NewScheduleWakeTool().Def(),
		// use_skill loads a TionSwarm skill body on demand. The CLI sees the skill
		// catalog in its appended system prompt but has no native way to load a
		// body; this bridge gives it the same lazy-load path native agents use.
		tools.NewUseSkillTool(nil).Def(),
		// skill_search lets a claude-cli agent discover on-demand/conditional skills
		// not advertised in its appended catalog, mirroring the native path. (SK-2)
		tools.NewSkillSearchTool(nil).Def(),
	}
	// shell is bridged only when enabled, mirroring the native tool loop's shell
	// gate. The CLI gets ONE shell: the OS-native one — PowerShell on Windows, Bash
	// on Unix — so a claude-cli agent runs commands through TionSwarm's sandboxed shell
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
	// run_subagent is always bridged now (2026-07-02: the delegation master toggle
	// was removed; per-tool visibility handles disabling), mirroring the native tool
	// loop. Unlike spawn_session (fire-and-forget into a separate session), it runs a
	// subagent synchronously and returns its answer into THIS turn — the CLI agent's
	// "ask another agent and get the result back now" path.
	defs = append(defs, tools.NewRunSubagentTool().Def())
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
	// Gateway meta-tools (Doc 52 Faz 1-b): only advertised when the dynamic extended
	// surface is on. With it on, the extended tier starts empty and the model grows it
	// by calling activate_tools; the backend registers the tool and pushes
	// tools/list_changed so the CLI re-lists and can call it the same turn.
	if tun != nil && tun.GatewayDynamicExtended() {
		specs = append(specs,
			interaction.ToolSpec{
				Name:        "activate_tools",
				Description: "Load one or more on-demand tools (from the 'Available Tools' catalog) into this session so you can call them. After activating, the tool becomes callable immediately. Pass tool names in `tools`.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"tools":{"type":"array","items":{"type":"string"},"description":"On-demand tool names to activate"}},"required":["tools"]}`),
			},
			interaction.ToolSpec{
				Name:        "deactivate_tools",
				Description: "Unload previously activated on-demand tools from this session to free context. Pass tool names in `tools`.",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"tools":{"type":"array","items":{"type":"string"},"description":"Tool names to deactivate"}},"required":["tools"]}`),
			},
			interaction.ToolSpec{
				Name:        "active_tools",
				Description: "List the on-demand tools you have activated in this session (the ones you can call now, besides the always-loaded core tools).",
				InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			},
		)
	}
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
	// update_session reads the SessionSink (a superset of GoalSink), so a single
	// sessionAttach covers title/working-dir/goal/tags/archive — no separate
	// goalAttach is needed any more.
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
	// update_session covers title/working-dir/goal/tags/archive; it reads the
	// SessionSink (a superset of GoalSink), so sessionAttach alone suffices.
	"update_session": {newTool: func() tools.Tool { return tools.NewUpdateSessionTool() }, attach: sessionAttach, missing: "no session is available to edit for this turn"},
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
// the CLI sends a namespaced name (core: mcp__tionswarm_interaction__ask_user,
// extended: mcp__tionswarm_extended__create_agent) or the bare name.
func bareToolName(name string) string {
	if s := strings.TrimPrefix(name, "mcp__tionswarm_interaction__"); s != name {
		return s
	}
	return strings.TrimPrefix(name, "mcp__tionswarm_extended__")
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
	case "activate_tools":
		return b.callActivate(token, run, args, true)
	case "deactivate_tools":
		return b.callActivate(token, run, args, false)
	case "active_tools":
		return b.callActiveTools(token), nil
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

// callActivate handles the gateway activate_tools / deactivate_tools meta-tools
// (Doc 52 Faz 1-b): it mutates the session's activated extended set and pushes
// tools/list_changed so the CLI re-lists. activate=true adds, false removes. Names
// are validated against the run's real extended candidates so a typo is reported
// rather than silently registering a phantom tool.
func (b *interactionBackend) callActivate(token string, run *chatRun, args json.RawMessage, activate bool) (interaction.CallResult, error) {
	if !b.dynamicExtended() {
		// Meta-tools are only advertised when the dynamic surface is on; a stray call
		// with it off is a no-op the model can recover from.
		return interaction.CallResult{Text: "dynamic tool activation is not enabled", IsError: true}, nil
	}
	var in struct {
		Tools []string `json:"tools"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return interaction.CallResult{Text: "invalid input: " + err.Error(), IsError: true}, nil
	}
	if len(in.Tools) == 0 {
		return interaction.CallResult{Text: "no tool names given", IsError: true}, nil
	}
	// Accept either the bare name ("notify") or the namespaced form the catalog shows
	// ("mcp__tionswarm_extended__notify") — the model may echo either. Normalise to bare.
	for i, n := range in.Tools {
		in.Tools[i] = bareToolName(n)
	}

	if activate {
		// Split requested names into valid extended candidates vs unknown, so the model
		// gets clear feedback instead of a silent partial success.
		candidates := b.extendedCandidates(run)
		var valid, unknown []string
		for _, n := range in.Tools {
			if candidates[n] {
				valid = append(valid, n)
			} else {
				unknown = append(unknown, n)
			}
		}
		added := b.activateExtended(token, valid)
		// Push list_changed so the CLI re-fetches tools/list and the newly registered
		// tools become callable this turn. Pushed BEFORE returning so, by the time the
		// model reads this result, the notification is already queued on the SSE stream
		// (ordering mitigation, Doc 52 YENI-D). Nil-safe: without a live stream the
		// client still converges on its next tools/list.
		pushed := false
		if len(added) > 0 && b.srv != nil {
			pushed = b.srv.PushToolsChanged(token)
		}
		msg := "activated: " + strings.Join(added, ", ")
		if len(added) == 0 {
			msg = "no new tools activated (already active or none valid)"
		}
		if len(unknown) > 0 {
			msg += "; unknown (not in the on-demand catalog): " + strings.Join(unknown, ", ")
		}
		if len(added) > 0 && !pushed {
			msg += "\n(note: tools registered; they will appear on your next tool list)"
		}
		return interaction.CallResult{Text: msg, IsError: len(added) == 0 && len(unknown) > 0}, nil
	}

	removed := b.deactivateExtended(token, in.Tools)
	if len(removed) > 0 && b.srv != nil {
		b.srv.PushToolsChanged(token)
	}
	if len(removed) == 0 {
		return interaction.CallResult{Text: "no tools deactivated (none were active)", IsError: false}, nil
	}
	return interaction.CallResult{Text: "deactivated: " + strings.Join(removed, ", ")}, nil
}

// callActiveTools implements the active_tools meta-tool: it lists the extended tools
// the model has activated this session (the gateway analogue of the TS gateway's
// active_tools). Config-level MCP server management (list/enable/disable) is a separate
// concern served by the self-management suite (list_mcp_servers / toggle_mcp_server).
func (b *interactionBackend) callActiveTools(token string) interaction.CallResult {
	if !b.dynamicExtended() {
		return interaction.CallResult{Text: "dynamic tool activation is not enabled", IsError: true}
	}
	active := b.activeExtended(token)
	if len(active) == 0 {
		return interaction.CallResult{Text: "No on-demand tools activated. Use activate_tools to load one from the 'Available Tools' catalog."}
	}
	return interaction.CallResult{Text: "Activated tools (callable now):\n- " + strings.Join(active, "\n- ")}
}

// interactionURL builds the loopback URL a CLI subprocess uses to reach this
// server's /mcp/interaction endpoint, or "" when the base URL is unknown.
func (s *Server) interactionURL() string {
	if s.selfURL == "" {
		return ""
	}
	return strings.TrimRight(s.selfURL, "/") + "/mcp/interaction"
}
