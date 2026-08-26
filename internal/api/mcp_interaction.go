package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/interaction"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
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
	// apiSrv is the owning HTTP server, so the CLI human-in-the-loop tools
	// (ask_user/permission/plan) route through the session interaction store +
	// hub (resolve-once CAS, broadcast to every window) instead of the old
	// owner-only SSE + run.answer channel. Set in NewServer.
	apiSrv *Server
	// srv is the streaming server, used to PUSH tools/list_changed when activate_tools
	// grows a session's extended surface (Doc 52 Faz 1-b). Set after server construction
	// (setServer); nil-safe — without it activation still mutates state, the client just
	// converges on its next tools/list instead of via an immediate push.
	srv *interaction.Server
	// mu guards activated. activated maps a session token to the set of extended tool
	// names the model has turned on this session — the gateway dynamic surface: the
	// claude-cli extended tier starts EMPTY and grows only as the model activates tools.
	mu        sync.Mutex
	activated map[string]map[string]bool
}

// setServer wires the streaming server so the backend can push tools/list_changed.
func (b *interactionBackend) setServer(s *interaction.Server) { b.srv = s }

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
// CLI MCP-config advertises on the alwaysLoad `tionharness_interaction` server so they
// are NEVER deferred by the CLI's tool search (Bash, ask_user, the artifact/skill
// path, working-memory edits, permission_prompt). Everything else (notify,
// focus_view, the session-lifecycle setters, schedule_wake, spawn_session) plus the
// entire bridged self-management suite is the EXTENDED tier → a separate
// `tionharness_extended` server subject to ToolSearch deferral. This mirrors the native
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
	// discovery round-trip.
	"activate_tools":   true,
	"deactivate_tools": true,
	"active_tools":     true,
	"tool_search":      true,
}

// cliTier classifies a bare tool name into the claude-cli wire tier: "core"
// (eager — advertised on the alwaysLoad tionharness_interaction server, never deferred),
// "extended" (deferred — the tionharness_extended server, discovered via the CLI's
// ToolSearch), or "hidden" (advertised on NEITHER server this turn).
//
// This is the projection of TionHarness's 4-tier visibility model onto claude-cli's
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

// gatewayMetaTools are the lazy-activation meta-tools (Doc 52): they only do
// something useful when the client watches tools/list_changed and re-lists on
// push. codex-cli logs the notification and never re-fetches (see
// _Docs/69-CODEX-CLI-SAGLAYICI.md, "lazy tool loading" gap) — advertising these
// to it just invites a call→no-op→retry loop (measured live: 7+3 wasted calls
// before the model gave up). The full-tier request (below) omits this set.
var gatewayMetaTools = map[string]bool{
	"activate_tools":   true,
	"deactivate_tools": true,
	"active_tools":     true,
	"tool_search":      true,
}

// Tools implements interaction.Backend. The specs come from the single tool
// definitions in the tools package — the schema is never re-declared here, so the
// native and CLI paths advertise the identical contract. tier filters the result
// so each CLI MCP server entry (alwaysLoad core / deferred extended) gets its own
// subset; tier "" returns the full set.
//
// tier may carry a "-full" suffix ("core-full" / "extended-full"): this is the
// codex-cli variant of the "core" / "extended" request (see fullTierQueryParam in
// package interaction). codex-cli cannot use the lazy gateway model — its client
// never re-lists on tools/list_changed — so for a "-full" request the extended
// half returns EVERY extended tool unconditionally (bypassing the per-session
// activated-set gate) and the gateway meta-tools (activate_tools/deactivate_tools/
// active_tools/tool_search) are dropped from BOTH halves, since activating a tool
// that is already fully advertised is a no-op the model would only waste calls
// discovering. The claude-cli request never carries the suffix, so its behavior
// (gated extended tier, meta-tools present) is byte-for-byte unchanged.
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
	// Drop tools the agent's effective filter rejects (workspace DisabledTools or the
	// agent denylist). Without this the claude-cli bridge advertised a tool the native
	// ToolCatalog filters out — e.g. a workspace that disables PowerShell to force Bash
	// (so the sqz/rtk token-optimizer, which only rewrites Bash, always applies) still
	// saw PowerShell here and could call it. nil predicate → no restriction.
	if run != nil {
		if allow := run.toolAllowedFor(); allow != nil {
			specs = filterAllowedSpecs(specs, allow)
		}
	}
	if tier == "" {
		return specs
	}
	// Peel the "-full" suffix (codex-cli's no-gate request) off the base tier name
	// so the switch below still matches "core"/"extended" as before; full tracks
	// whether the meta-tools should be dropped and the extended gate bypassed.
	full := strings.HasSuffix(tier, "-full")
	if full {
		tier = strings.TrimSuffix(tier, "-full")
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
	// Gateway dynamic surface (Doc 52): the EXTENDED tier advertises only the tools the
	// model has activated this session — it starts empty and grows via activate_tools +
	// tools/list_changed. The deferred/activatable tier is now everything non-core
	// (extended AND hidden): with nothing advertised until activated, hidden tools cost
	// no tokens up front, so they become discoverable (tool_search) + activatable in-turn
	// — the CLI analogue of native hidden tools (§7-15). Core is unaffected (always eager).
	filtered := make([]interaction.ToolSpec, 0, len(specs))
	for _, s := range specs {
		if full && gatewayMetaTools[s.Name] {
			continue // codex can never activate anything, so hide the mechanism entirely
		}
		t := cliTier(s.Name, visOf)
		if tier == "extended" {
			if t == "core" {
				continue
			}
			// full: every non-core (extended ∪ hidden) tool advertised unconditionally —
			// codex has no activation round-trip to gate. Otherwise: gated by the
			// session's activated set, as before.
			if !full && !b.isActivated(token, s.Name) {
				continue
			}
			filtered = append(filtered, s)
			continue
		}
		if t == tier {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// extendedCandidates returns the bare names activate_tools may turn on for this run —
// every NON-CORE tool (the deferred tier = extended + hidden). Used to validate
// activate/deactivate names so a typo is reported instead of registering a phantom tool.
// Hidden tools qualify: with the gateway advertising nothing until activated, they are
// bridged for free and become activatable in-turn (the CLI analogue of native hidden).
func (b *interactionBackend) extendedCandidates(run *chatRun) map[string]bool {
	out := map[string]bool{}
	for name := range b.candidateDefs(run) {
		out[name] = true
	}
	return out
}

// candidateDefs returns name→description for every NON-CORE (activatable) tool this run
// exposes: the static interaction specs plus the bridged self-management/hidden defs.
// Shared by extendedCandidates (validation) and tool_search (discovery).
func (b *interactionBackend) candidateDefs(run *chatRun) map[string]string {
	specs := interactionToolSpecs(b.tun, run != nil && run.autonomous)
	var bridge []providers.ToolDef
	var visOf func(string) string
	var allow func(string) bool
	if run != nil {
		bridge = run.bridgeDefsFor()
		visOf = run.tierVisFor()
		allow = run.toolAllowedFor()
	}
	out := map[string]string{}
	for _, s := range specs {
		if allow != nil && !allow(s.Name) {
			continue // workspace-disabled / agent-blocked: not activatable either
		}
		if cliTier(s.Name, visOf) != "core" {
			out[s.Name] = s.Description
		}
	}
	for _, d := range bridge {
		if allow != nil && !allow(d.Name) {
			continue
		}
		if cliTier(d.Name, visOf) != "core" {
			out[d.Name] = d.Description
		}
	}
	return out
}

// filterAllowedSpecs drops interaction tool specs the agent's effective filter
// rejects (workspace DisabledTools or the agent denylist), keeping the claude-cli
// bridge's advertised set aligned with the native ToolCatalog. allow nil → returned
// unchanged.
func filterAllowedSpecs(specs []interaction.ToolSpec, allow func(string) bool) []interaction.ToolSpec {
	if allow == nil {
		return specs
	}
	kept := specs[:0]
	for _, s := range specs {
		if allow(s.Name) {
			kept = append(kept, s)
		}
	}
	return kept
}

// filterAllowedNames drops the bare tool names the agent's effective filter rejects,
// so the CLI allowlist never advertises a tool the bridge's tools/list will refuse
// (advertised set and allowlist share one source — see interactionAdvertisedNames).
// allow nil → returned unchanged.
func filterAllowedNames(names []string, allow func(string) bool) []string {
	if allow == nil {
		return names
	}
	kept := make([]string, 0, len(names))
	for _, n := range names {
		if allow(n) {
			kept = append(kept, n)
		}
	}
	return kept
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
		// tags, archive) in one call — the same
		// db.Session fields the user edits. Non-blocking; advertised on autonomous
		// turns too (a scheduled run can rename or retag itself).
		tools.NewUpdateSessionTool().Def(),
		// schedule_wake replaces the CLI's native ScheduleWakeup (which TionHarness
		// disallows): the CLI runs one-shot, so its built-in wake never fires —
		// ours arms a real TionHarness timer that re-delivers into this session.
		tools.NewScheduleWakeTool().Def(),
		// use_skill loads a TionHarness skill body on demand. The CLI sees the skill
		// catalog in its appended system prompt but has no native way to load a
		// body; this bridge gives it the same lazy-load path native agents use.
		tools.NewUseSkillTool(nil).Def(),
		// skill_search lets a claude-cli agent discover on-demand/conditional skills
		// not advertised in its appended catalog, mirroring the native path. (SK-2)
		tools.NewSkillSearchTool(nil).Def(),
	}
	// shell is bridged only when enabled, mirroring the native tool loop's shell
	// gate. Bash-preferred: whenever a POSIX shell backs it (always on Unix; on
	// Windows only when bash.exe — Git Bash / WSL — is on PATH) the CLI gets Bash.
	// On Windows, PowerShell is ALSO advertised alongside Bash for Windows-native
	// tasks (cmdlets, registry, $env:); when no bash.exe is present it is the sole
	// fallback shell. tools.ShellToolNames() is the single source of what is
	// available, so the bridge can never advertise a shell that is not registered.
	// The runner that backs it (NewShellRunner) dispatches per tool name (callShell
	// passes it), and the CLI's own native Bash can be safely disallowed.
	if tun != nil && tun.ShellEnabled() {
		for _, name := range tools.ShellToolNames() {
			switch name {
			// AdvertiseOptimizerFlag: the schema is built here from a bare tool, but
			// the real output filter is installed per turn by NewShellRunner — so
			// no_compress IS supported at call time and must be declared, or the
			// schema's "additionalProperties": false forbids the very escape hatch
			// the optimizer's degraded note tells the agent to use.
			case "Bash":
				defs = append(defs, tools.NewShellTool(tools.Sandbox{}).AdvertiseOptimizerFlag().Def())
			case "PowerShell":
				defs = append(defs, tools.NewPowerShellTool(tools.Sandbox{}).AdvertiseOptimizerFlag().Def())
			}
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
	// Gateway meta-tools (Doc 52 Faz 1-b): the extended tier starts empty and the model
	// grows it by calling activate_tools; the backend registers the tool and pushes
	// tools/list_changed so the CLI re-lists and can call it the same turn. Always
	// advertised on the core (eager) tier.
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
		interaction.ToolSpec{
			Name:        "tool_search",
			Description: "Search ALL on-demand tools by keyword — including ones not shown in the 'Available Tools' catalog (hidden tier). Returns matching names to load with activate_tools. Use when you need a capability you don't see listed.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Keywords to match against tool names + descriptions"}},"required":["query"]}`),
		},
	)
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
// session edits use the call ctx.
var (
	artifactAttach = func(_ context.Context, run *chatRun) (context.Context, bool) {
		s := run.artifactSink()
		if s == nil {
			return nil, false
		}
		return tools.WithArtifacts(context.Background(), s), true
	}
	// update_session reads the SessionSink, so a single
	// sessionAttach covers title/working-dir/tags/archive.
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
	// update_session covers title/working-dir/tags/archive via the SessionSink.
	"update_session": {newTool: func() tools.Tool { return tools.NewUpdateSessionTool() }, attach: sessionAttach, missing: "no session is available to edit for this turn"},
}

// callViaSink runs a sink-bound tool: attach the per-run sink, then call the shared
// builtin. Replaces the former per-tool callTodo/callArtifact/callNotify/callFocus/
// callSessionEdit method (one shape, one place).
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

// extendedNSPrefix is the claude-cli namespace every deferred (extended-tier) tool
// is reachable under: the CLI advertises MCP tools as mcp__<server>__<tool>, so a
// tool activated via activate_tools is callable ONLY as mcp__tionharness_extended__<name>,
// never by its bare name. Used to report the exact callable name back to the model.
const extendedNSPrefix = "mcp__tionharness_extended__"

// activateRelistTimeout bounds how long callActivate waits for the CLI to re-fetch
// tools/list after an activate push (PushToolsChangedAndWait). The live probe saw
// claude-cli re-list concurrently in ~10-16ms, so the wait almost always returns far
// under this ceiling; the generous cap only guards a client that never re-lists
// (then the historical retry path takes over).
const activateRelistTimeout = 1 * time.Second

// bareToolName strips the Interaction MCP namespace so dispatch matches whether
// the CLI sends a namespaced name (core: mcp__tionharness_interaction__ask_user,
// extended: mcp__tionharness_extended__create_agent) or the bare name.
func bareToolName(name string) string {
	if s := strings.TrimPrefix(name, "mcp__tionharness_interaction__"); s != name {
		return s
	}
	return strings.TrimPrefix(name, extendedNSPrefix)
}

// fullTierProvider reports whether a provider mounts the Interaction MCP with the
// "-full" tier variant (?full=1, see fullTierQueryParam): codex-cli never re-lists
// tools on tools/list_changed, so its extended tier is advertised in full and the
// activate_tools meta-tools are hidden from it (see Tools and interactionServers).
// Such a run can never populate the session's activated set, so the call-time
// activation gate must not apply to it either — otherwise every extended tool it
// was shown answers "is not activated" with no way to fix it.
func fullTierProvider(provider string) bool { return provider == "codex-cli" }

// toolCallError distinguishes an unloaded on-demand tool from a policy-blocked or
// genuinely unknown name before dispatch. Claude can submit either a bare or MCP-
// namespaced name, but activation guidance always reports the exact extended name.
func (b *interactionBackend) toolCallError(token, name string, run *chatRun) string {
	bare := bareToolName(name)
	defs := interactionToolSpecs(b.tun, run != nil && run.autonomous)
	var visOf func(string) string
	if run != nil {
		visOf = run.tierVisFor()
	}
	known := make(map[string]bool, len(defs))
	for _, def := range defs {
		known[def.Name] = true
	}
	if run != nil {
		for _, def := range run.bridgeDefsFor() {
			known[def.Name] = true
		}
	}

	if known[bare] {
		if allow := run.toolAllowedFor(); allow != nil && !allow(bare) {
			return fmt.Sprintf("Tool %s exists but is blocked by the current workspace or agent policy; it cannot be activated in this context.", callableToolName(bare, visOf))
		}
		if cliTier(bare, visOf) != "core" && !fullTierProvider(run.providerOf()) && !b.isActivated(token, bare) {
			callable := extendedNSPrefix + bare
			return fmt.Sprintf("Tool %s exists in the on-demand catalog but is not activated. Activate it with activate_tools({\"tools\":[%q]}). It will become visible on the next turn, not the current turn.", callable, callable)
		}
		return ""
	}

	msg := "No such tool available: " + name
	if suggestion := closestToolName(bare, known); suggestion != "" {
		msg += ". Did you mean " + callableToolName(suggestion, visOf) + "?"
	}
	return msg + ". Use tool_search({\"query\":\"<keywords>\"}) to find an on-demand tool."
}

func callableToolName(name string, visOf func(string) string) string {
	if cliTier(name, visOf) == "core" {
		return "mcp__tionharness_interaction__" + name
	}
	return extendedNSPrefix + name
}

func closestToolName(want string, known map[string]bool) string {
	best, bestDistance := "", len(want)/3+1
	for name := range known {
		distance := toolNameDistance(want, name)
		if distance < bestDistance || distance == bestDistance && (best == "" || name < best) {
			best, bestDistance = name, distance
		}
	}
	return best
}

func toolNameDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			current[j] = min(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

// Call implements interaction.Backend.
func (b *interactionBackend) Call(ctx context.Context, token, name string, args json.RawMessage) (interaction.CallResult, error) {
	run := b.runs.byToken(token)
	if run == nil {
		return interaction.CallResult{}, errors.New("no live turn for token")
	}
	bare := bareToolName(name)
	if text := b.toolCallError(token, name, run); text != "" {
		return interaction.CallResult{Text: text, IsError: true}, nil
	}
	// Sink-bound tools (todo/artifact/notify/focus/session-edit) all share one
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
		// Both shells can be bridged (Bash-preferred; PowerShell for Windows-native
		// tasks). Pass the tool name so the runner dispatches to the right interpreter
		// instead of guessing from OS.
		return b.callShell(ctx, run, bare, args)
	case "run_subagent":
		return b.callRunSubagent(ctx, run, args)
	case "activate_tools":
		return b.callActivate(token, run, args, true)
	case "deactivate_tools":
		return b.callActivate(token, run, args, false)
	case "active_tools":
		return b.callActiveTools(token), nil
	case "tool_search":
		return b.callToolSearch(run, args), nil
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
	// ("mcp__tionharness_extended__notify") — the model may echo either. Normalise to bare.
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
		// Push list_changed AND wait for the CLI to re-fetch tools/list before returning,
		// so the just-activated tools are already in the CLI's registry by the time the
		// model reads this result and calls one. This closes the activate→call race that
		// otherwise surfaced as "No such tool available: mcp__tionharness_extended__<name>"
		// on a same-turn call (SES125). A live probe (probe_relist_test) measured claude-cli
		// re-listing concurrently in ~10-16ms while activate is pending, so this returns
		// almost immediately; the bounded timeout means a client that fails to re-list can
		// never wedge the call (worst case: the historical retry behavior).
		pushed := false
		if len(added) > 0 && b.srv != nil {
			pushed = b.srv.PushToolsChangedAndWait(token, activateRelistTimeout)
		}
		// Report the NAMESPACED callable names (mcp__tionharness_extended__<name>), not
		// the bare ones: in the claude-cli path a deferred tool is reachable ONLY under
		// its namespaced name, so echoing the bare name led the model to call e.g.
		// `list_agents` and hit "No such tool available: list_agents" before retrying
		// with the correct name — a wasted round-trip (and, before the autotag fix, a
		// spurious tool-error). Naming the exact callable form removes that.
		callable := make([]string, len(added))
		for i, n := range added {
			callable[i] = extendedNSPrefix + n
		}
		msg := "activated: " + strings.Join(callable, ", ")
		if len(added) == 0 {
			msg = "no new tools activated (already active or none valid)"
		} else {
			msg += "\nCall each by this exact (namespaced) name."
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
	active := b.activeExtended(token)
	if len(active) == 0 {
		return interaction.CallResult{Text: "No on-demand tools activated. Use activate_tools to load one from the 'Available Tools' catalog."}
	}
	// Namespaced callable names (mcp__tionharness_extended__<name>): these are the exact
	// forms the model must call — the bare name is not a valid tool in the CLI path
	// (see extendedNSPrefix / the activate_tools note).
	callable := make([]string, len(active))
	for i, n := range active {
		callable[i] = extendedNSPrefix + bareToolName(n)
	}
	return interaction.CallResult{Text: "Activated tools (call by these exact names):\n- " + strings.Join(callable, "\n- ")}
}

// callToolSearch implements the tool_search meta-tool: it keyword-searches EVERY
// activatable (non-core) tool — including hidden-tier ones not shown in the catalog — so
// the model can discover a capability it doesn't see listed, then load it with
// activate_tools. Mirrors the native tool_search over the load-on-demand catalog.
func (b *interactionBackend) callToolSearch(run *chatRun, args json.RawMessage) interaction.CallResult {
	var in struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return interaction.CallResult{Text: "invalid input: " + err.Error(), IsError: true}
	}
	q := strings.ToLower(strings.TrimSpace(in.Query))
	if q == "" {
		return interaction.CallResult{Text: "empty query", IsError: true}
	}
	terms := strings.Fields(q)
	cands := b.candidateDefs(run)
	names := make([]string, 0, len(cands))
	for name := range cands {
		names = append(names, name)
	}
	sort.Strings(names)
	// Term-scoring (OR + rank), not strict AND — mirrors the native builtin
	// tool_search (internal/tools/builtin_activate.go): a tool matches when it
	// contains AT LEAST ONE query term, ranked by how many distinct terms it hits
	// (a name hit outweighs a description hit). Strict AND used to return empty
	// when the model passed several full tool names in one query, since no single
	// tool contains every term. Names are pre-sorted, so equal-score ties stay
	// alphabetical.
	type scored struct {
		name     string
		terms    int
		nameHits int
	}
	var ranked []scored
	for _, name := range names {
		lname := strings.ToLower(name)
		hay := lname + " " + strings.ToLower(cands[name])
		var termHits, nameHits int
		for _, term := range terms {
			if strings.Contains(hay, term) {
				termHits++
				if strings.Contains(lname, term) {
					nameHits++
				}
			}
		}
		if termHits > 0 {
			ranked = append(ranked, scored{name: name, terms: termHits, nameHits: nameHits})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].terms != ranked[j].terms {
			return ranked[i].terms > ranked[j].terms
		}
		return ranked[i].nameHits > ranked[j].nameHits
	})
	var matches []string
	for _, r := range ranked {
		desc := cands[r.name]
		if len(desc) > 100 {
			desc = desc[:100] + "…"
		}
		matches = append(matches, "- "+r.name+" — "+desc)
	}
	if len(matches) == 0 {
		return interaction.CallResult{Text: fmt.Sprintf("No on-demand tools match %q.", in.Query)}
	}
	const max = 30
	more := ""
	if len(matches) > max {
		more = fmt.Sprintf("\n…and %d more; refine the query.", len(matches)-max)
		matches = matches[:max]
	}
	return interaction.CallResult{Text: "Matching tools (load with activate_tools):\n" + strings.Join(matches, "\n") + more}
}

// interactionURL builds the loopback URL a CLI subprocess uses to reach this
// server's /mcp/interaction endpoint, or "" when the base URL is unknown.
func (s *Server) interactionURL() string {
	if s.selfURL == "" {
		return ""
	}
	return strings.TrimRight(s.selfURL, "/") + "/mcp/interaction"
}
