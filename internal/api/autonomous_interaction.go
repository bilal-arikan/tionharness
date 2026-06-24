package api

import (
	"context"

	"github.com/google/uuid"

	"github.com/bilal-arikan/swarmgo/internal/agent"
	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/tools"
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
		// cancel is a no-op: an autonomous turn is driven by the runtime, not by the
		// /chat/control endpoint, so there is nothing for it to cancel.
		run := s.runs.register(runID, sessionID, func() {})
		run.autonomous = true

		// Artifacts (CLI path): bind a session-scoped artifact sink so create_artifact
		// / update_artifact work on autonomous CLI turns too (otherwise the bridge
		// reports "artifacts are not available for this turn"). Mirrors the chat path's
		// setArtifacts. Only when we know the session to stamp artifacts with.
		if sessionID != "" {
			run.setArtifacts(rt.NewArtifactSink(sessionID, ag.ID))
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

		// spawn_session (CLI path): only when self-manage is on, mirroring the native
		// gate. A fresh instance resets the per-turn spawn budget.
		if s.tun.SelfManageEnabled() {
			run.setSpawnTool(tools.NewSpawnSessionTool(ag.ID, s.tun.SpawnMaxPerTurn(),
				func(sctx context.Context, target, prompt, modelOverride string) (tools.SpawnResult, error) {
					res, err := rt.SpawnSession(sctx, target, prompt, agent.SpawnOptions{ModelOverride: modelOverride, CreatedBy: ag.ID})
					return tools.SpawnResult{SessionID: res.SessionID, AgentName: res.AgentName}, err
				}))
		}

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
		bridgeDefs, bridgeCall := rt.BridgeTools(ctx, ag)
		run.setBridge(bridgeDefs, bridgeCall)

		ctx = tools.WithInteractionEndpoint(ctx, url, run.token,
			mergeInteractionToolNames(interactionAdvertisedNames(s.tun, true), bridgeDefs))
		return ctx, func() { s.runs.unregister(runID) }
	}
}
