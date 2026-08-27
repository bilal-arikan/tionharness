package api

import (
	"context"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// autonomousInteraction builds the headless Interaction MCP setup for one runtime.
// It mirrors what the chat stream handler installs per agent turn — skill loader,
// shell runner, spawn tool, wake scheduler and the self-management bridge — but
// for a non-chat (scheduler/spawn/flow) turn that has no SSE client.
//
// The workspace manager installs the returned AutonomousInteraction on every
// runtime; the agent tool loop calls it for autonomous claude-cli turns so those
// agents reach the same bridged use_skill/shell/self-manage tools chat agents do
// (fixing scheduled "Unknown skill" and the native-Bash mismatch). It registers a
// run (so the Interaction backend can correlate the CLI's Bearer token) and
// returns the endpoint-carrying context plus a cleanup that unregisters the run.
func (s *Server) autonomousInteraction(rt *agent.Runtime) agent.AutonomousInteraction {
	return func(ctx context.Context, ag db.Agent, sessionID string) (context.Context, func()) {
		url := s.interactionURL()
		if url == "" {
			return ctx, func() {}
		}
		runID := uuid.NewString()
		// Make the turn cancellable so a viewer's "Durdur"/"Kes" (session control
		// stop → run.cancel) actually aborts an autonomous claude-cli turn instead of
		// being a no-op. The cancellable child ctx returned here threads into the CLI
		// provider call (autoInteract reassigns the turn ctx), so cancelling it ends
		// generation. cleanup cancels + unregisters (cancel after completion is a
		// harmless no-op, and satisfies the vet "cancel used on all paths" check).
		ctx, cancel := context.WithCancel(ctx)
		run := s.runs.register(runID, sessionID, rt.WorkspaceID(), cancel)
		run.autonomous = true
		// Label the run with the agent's provider on autonomous turns too. The chat
		// path does this for the Session Info panel, but the call-time activation gate
		// also reads it: a full-tier provider (codex-cli) is shown the complete
		// extended tier without the activate_tools meta-tools, so leaving the provider
		// empty here made every extended call answer "is not activated" with no fix.
		run.setProvider(ag.Provider)

		// Artifacts (CLI path): bind a session-scoped artifact sink so create_artifact
		// / update_artifact work on autonomous CLI turns too (otherwise the bridge
		// reports "artifacts are not available for this turn"). Mirrors the chat path's
		// setArtifacts. Only when we know the session to stamp artifacts with.
		if sessionID != "" {
			run.setArtifacts(rt.NewArtifactSink(sessionID, ag.ID))
			// notify (CLI path): bind a notify sink so an autonomous claude-cli agent
			// can raise a desktop notification (e.g. "long job finished"). Publishes an
			// "agent" event onto the workspace bus → SSE → OS toast when a window is open.
			nsink := newNotifySink(sessionID, ag.ID, rt.Emit)
			run.setNotify(nsink)
			// focus_view (CLI path): same sink drives the UI (no-op when no window open).
			run.setNav(nsink)
			// Session sink (CLI path): title + working dir + tags + archive,
			// all bound to this session via the single update_session tool
			// bound to the running session.
			ssink := rt.NewSessionSink(sessionID)
			run.setSession(ssink)
			// Persistent progress (CLI path): persist the todo_write checklist to the
			// project's progress file on autonomous turns too. Keyed to the session's
			// working dir; gated by ProgressPersist.
			if s.tun.ProgressPersist() {
				run.setTodoSink(rt.NewTodoSink(sessionID, ag.ID))
			}
		}

		// use_skill (CLI path): enforce the same per-agent allowlist as the native
		// built-in, so a restricted skill stays unreachable unless assigned/shared.
		run.setSkillLoader(func(slug string) (string, error) {
			return rt.LoadSkillForAgent(ag, slug)
		})
		// skill_search (CLI path): discover on-demand/conditional skills. (SK-2)
		run.setSkillSearcher(func(query string, limit int) []tools.SkillHit {
			return rt.SearchSkillsForAgent(ag, query, limit)
		})
		// SK-3 (CLI path): loading a skill auto-grants its declared allowed-tools.
		run.setSkillAllowed(func(slug string) []string {
			return rt.SkillAllowedToolsForAgent(ag, slug)
		})
		// shell (CLI path): sandboxed PowerShell shell so the CLI's POSIX Bash can be
		// disallowed; nil when shell is off (then native Bash stays available).
		run.setShellRunner(rt.NewShellRunner())

		// spawn_session (CLI path): self-management is always on now. A fresh instance
		// resets the per-turn spawn budget.
		run.setSpawnTool(tools.NewSpawnSessionTool(ag.ID, s.tun.SpawnMaxPerTurn(),
			func(sctx context.Context, target, prompt, modelOverride string) (tools.SpawnResult, error) {
				// Inherit the caller's working directory: a session spawned from an
				// agent working in repo A must not land in the workspace default.
				res, err := rt.SpawnSession(sctx, target, prompt, agent.SpawnOptions{
					ModelOverride: modelOverride,
					CreatedBy:     ag.ID,
					WorkingDir:    rt.SessionWorkdir(sessionID),
				})
				return tools.SpawnResult{SessionID: res.SessionID, AgentName: res.AgentName, Queued: res.Queued, QueuePosition: res.QueuePosition}, err
			}))

		// run_subagent (CLI path): synchronous delegation — hand a sub-task to another
		// agent and get the answer back in this turn. nil when delegation is off.
		run.setRunAgent(rt.RunSubagentRunner(ag, true))

		// schedule_wake (CLI path): only when we know the session to resume.
		if sessionID != "" {
			run.setWakeScheduler(func(wctx context.Context, delaySeconds int, prompt, reason string) (string, error) {
				return rt.ScheduleWake(wctx, sessionID, ag.ID, prompt, reason, delaySeconds)
			})
		}

		// Self-management bridge (CLI-3): advertise the agent's lazy self-management
		// tools and dispatch them through the same registry the native loop uses.
		// Stamp the session id so BridgeTools can detect a coordinator session and
		// bridge the coordinator tools (M2) for a claude-cli coordinator.
		if sessionID != "" {
			ctx = agent.WithSessionID(ctx, sessionID)
		}
		bridgeDefs, bridgeCall := rt.BridgeTools(ctx, ag)
		run.setBridge(bridgeDefs, bridgeCall)

		// Visibility-aware CLI wire split (see chat_stream): full→core, summary/
		// name-only→extended, hidden→neither. Installed on the run so tools/list
		// classifies identically to the allowlist.
		visOf := rt.ToolVisibilityFunc(ctx, ag)
		run.setTierVis(visOf)
		// Effective tool filter (workspace DisabledTools + agent denylist), so the
		// headless CLI bridge drops workspace-disabled tools exactly like the chat path.
		allowOf := rt.ToolAllowedFunc(ctx, ag)
		run.setToolAllowed(allowOf)
		names := filterAllowedNames(interactionAdvertisedNames(s.tun, true), allowOf)
		coreNames, extNames := splitInteractionTiers(names, bridgeDefs, visOf)
		// Stable per-(session,agent) Bearer token (see chat_stream): keeps the CLI
		// mcp-config byte-identical across turns so a persistent process stays warm
		// (Doc 52 §3-D). bindActive resolves it to this in-flight run.
		tok := s.runs.interactionToken(rt.WorkspaceID(), sessionID, ag.ID)
		s.runs.bindActive(tok, run)
		ctx = tools.WithInteractionEndpoint(ctx, url, tok, coreNames, extNames)
		return ctx, func() { cancel(); s.runs.unregister(runID) }
	}
}
