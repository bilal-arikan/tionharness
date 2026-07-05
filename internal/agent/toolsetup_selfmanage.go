package agent

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/skills"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// selfManageBuiltins builds the self-management tool suite: the tools that let an
// agent create/edit/delete agents, flows, schedules, tasks, hooks and MCP
// servers, manage artifacts, author skills, read/apply app settings, manage
// workspaces, add memories and read logs. Provenance is enforced downstream —
// agents only mutate agent-created entities.
//
// It is split out of buildRegistry so that method reads as its high-level shape
// (core tools → self-manage suite → visibility tiers → MCP) instead of a single
// 400-line body. The returned slice is appended as one contiguous run, and its
// start index in the registry is what buildRegistry marks as the HIDDEN tier.
// Gated members (vault/skills/settings/workspace bridges) are simply omitted when
// their dependency is absent — the visibility marks tolerate missing names.
func (r *Runtime) selfManageBuiltins(agent db.Agent) []tools.Tool {
	builtins := []tools.Tool{
		// Agents. New agents are seeded with the default TionSwarm skill set when
		// the caller passes none; caller-supplied slugs are validated against the
		// skill store.
		tools.NewCreateAgentTool(r.db, agent.ID, skills.DefaultSkillSlugs(), r.skillExists),
		tools.NewUpdateAgentTool(r.db, agent.ID),
		tools.NewDeleteAgentTool(r.db, agent.ID, r.reloadSchedules),
		tools.NewListAgentsTool(r.db, agent.ID),
		// Note: agent→agent work is unified under run_subagent (above) — async
		// background runs go through its wait:"async" mode (→ SpawnSession). The
		// old call_agent / spawn_session / send_agent_message tools were removed.
		// send_message is the "peer DM" complement: an addressed, sender-tagged
		// message into another agent's persistent inbox (Claude Code mailbox model).
		tools.NewSendMessageTool(agent.ID, func(ctx context.Context, to, summary, message string) (string, error) {
			return r.DeliverAgentMessage(ctx, agent.ID, to, summary, message)
		}),
		// Context reset: let the agent hand off to a fresh session when it nears
		// the context limit (Anthropic "context reset" pattern) — operates on the
		// CURRENT session (resolved from the context) and the running agent.
		tools.NewHandoffSessionTool(func(ctx context.Context, reason string) (tools.HandoffResult, error) {
			sid := SessionIDFrom(ctx)
			if sid == "" {
				return tools.HandoffResult{}, fmt.Errorf("no active session to hand off")
			}
			sess, err := r.db.GetSession(ctx, sid)
			if err != nil {
				return tools.HandoffResult{}, err
			}
			res, err := r.HandoffSession(ctx, sess, agent, HandoffOptions{Reason: HandoffReasonAgent, CreatedBy: agent.ID})
			if err != nil {
				return tools.HandoffResult{}, err
			}
			return tools.HandoffResult{NewSessionID: res.NewSessionID, AgentName: res.AgentName, ArtifactID: res.ArtifactID}, nil
		}),
		// Flows.
		tools.NewCreateFlowTool(r.db, agent.ID),
		tools.NewUpdateFlowTool(r.db, agent.ID),
		tools.NewDeleteFlowTool(r.db, agent.ID),
		tools.NewListFlowsTool(r.db, agent.ID),
		tools.NewGetFlowTool(r.db, agent.ID),
		// run_flow drives a flow to completion (autonomous, budget-gated) and
		// records it in the executions feed, like run_task.
		tools.NewRunFlowTool(r.db, agent.ID, func(ctx context.Context, flowID, input string) (db.FlowRun, error) {
			run, _, err := r.RunFlowRecorded(ctx, flowID, input, true, nil)
			return run, err
		}),
		// Schedules (routines).
		tools.NewCreateScheduleTool(r.db, agent.ID, r.reloadSchedules),
		tools.NewUpdateScheduleTool(r.db, agent.ID, r.reloadSchedules),
		tools.NewDeleteScheduleTool(r.db, agent.ID, r.reloadSchedules),
		tools.NewListSchedulesTool(r.db, agent.ID),
		// Manually fire a schedule now (the "Run now" trigger).
		tools.NewRunScheduleTool(r.db, r.runScheduleNow),
		// Tag-triggered automations (event-driven loops). Provenance-enforced.
		tools.NewCreateAutomationTool(r.db, agent.ID),
		tools.NewUpdateAutomationTool(r.db, agent.ID),
		tools.NewDeleteAutomationTool(r.db, agent.ID),
		tools.NewListAutomationsTool(r.db, agent.ID),
		// Flow/schedule tags are edited via the `tags` field on update_flow /
		// update_schedule; session tags via update_session. No separate tag tools.
		// Tasks (kanban board). Read/create/edit/move on any task; delete only
		// agent-created (provenance). The board is passive — no run tool.
		tools.NewListTasksTool(r.db, agent.ID),
		tools.NewCreateTaskTool(r.db, agent.ID),
		tools.NewUpdateTaskTool(r.db, agent.ID),
		tools.NewMoveTaskTool(r.db, agent.ID),
		tools.NewDeleteTaskTool(r.db, agent.ID),
		// Hooks (PreToolUse/PostToolUse). List/create on any; delete only
		// agent-created (provenance).
		tools.NewListHooksTool(r.db, agent.ID),
		tools.NewCreateHookTool(r.db, agent.ID),
		tools.NewDeleteHookTool(r.db, agent.ID),
		// MCP servers. List/create/toggle on any; delete only agent-created
		// (provenance). New/enabled servers are picked up next turn.
		tools.NewListMCPServersTool(r.db, agent.ID),
		tools.NewCreateMCPServerTool(r.db, agent.ID),
		tools.NewToggleMCPServerTool(r.db, agent.ID),
		tools.NewDeleteMCPServerTool(r.db, agent.ID),
		// Artifacts (create/update already provided via the per-turn sink).
		tools.NewDeleteArtifactTool(r.db, agent.ID),
		tools.NewListArtifactsTool(r.db, agent.ID),
		tools.NewReadArtifactTool(r.db, agent.ID),
		// Memory (recall already provided above) + logs.
		tools.NewMemoryAddTool(r.mem, agent.ID),
		tools.NewReadLogsTool(r.logs),
	}
	// Secret vault: list/get/set/delete are unified in the single `secret` tool
	// registered in buildRegistry (always-on when a vault exists) — no separate
	// write tools here.
	// Skill authoring: create/update/delete/import reusable workspace skills.
	if r.skills != nil {
		builtins = append(builtins,
			tools.NewCreateSkillTool(agentSkillWriter{store: r.skills}),
			tools.NewUpdateSkillTool(agentSkillWriter{store: r.skills}),
			tools.NewDeleteSkillTool(agentSkillWriter{store: r.skills, db: r.db}),
			tools.NewImportSkillTool(agentSkillWriter{store: r.skills}), // SK-IMP
		)
	}
	// Application-wide settings: read + live-apply the settings.json document
	// behind the Settings screen. Only offered when the bridge is wired (the
	// api server provides it), since changes affect every workspace.
	if r.settingsBridge != nil {
		builtins = append(builtins,
			tools.NewGetSettingsTool(r.settingsBridge),
			tools.NewUpdateSettingsTool(r.settingsBridge),
		)
	}
	// Workspaces: list/create/rename across all workspaces; delete only
	// agent-created ones (and never the current or last). Cross-workspace, so
	// it goes through the bridge the manager wires from the api server. Only
	// offered when that bridge is present.
	if r.workspaceBridge != nil {
		builtins = append(builtins,
			tools.NewListWorkspacesTool(r.workspaceBridge, agent.ID, r.wsID),
			tools.NewCreateWorkspaceTool(r.workspaceBridge, agent.ID, r.wsID),
			tools.NewRenameWorkspaceTool(r.workspaceBridge, agent.ID, r.wsID),
			tools.NewDeleteWorkspaceTool(r.workspaceBridge, agent.ID, r.wsID),
		)
	}
	return builtins
}
