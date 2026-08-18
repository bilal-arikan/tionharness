package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/market"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// Workspace templates seed a freshly created workspace with a curated set of
// agents, an orchestration flow connecting them, and optional schedules — so a
// new workspace arrives ready for a specific kind of work (research, software
// development, daily routine) instead of an empty roster.
//
// Templates now live in the MARKET as workspace-kind packs (bundled tier +
// global dir + remote registries), not in a hard-coded Go list. The
// create-workspace picker lists them via the market store; seeding reads the
// chosen pack's WorkspacePayload. This keeps templates shareable/installable like
// any other pack while still shipping a default set embedded in the binary.

// blankTemplateID is the id of the embedded "blank" template — the safe default
// when no template is chosen or a requested id is unknown.
const blankTemplateID = "workspace-blank"

// templateListItem is the catalog view sent to the create-workspace picker. The
// shape is unchanged from the legacy in-code template list so the frontend modal
// works without modification.
type templateListItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	AgentCount  int    `json:"agentCount"`
	HasFlow     bool   `json:"hasFlow"`
	// CoordinatorCount / AutomationCount describe how the team is WIRED, which the
	// agent count alone cannot: "2 agents" reads the same for a chat pair and for a
	// delegation chain. Both 0 for an ordinary template, so the picker only
	// mentions them when they exist.
	CoordinatorCount int `json:"coordinatorCount"`
	AutomationCount  int `json:"automationCount"`
}

// handleListWorkspaceTemplates returns the available workspace templates, sourced
// from the market's workspace-kind packs (bundled + global + remote). The blank
// default is pinned first; the rest keep the market's name-sorted order.
func (s *Server) handleListWorkspaceTemplates(w http.ResponseWriter, _ *http.Request) {
	packs := s.market.ListKind(market.KindWorkspace)
	out := make([]templateListItem, 0, len(packs))
	for _, meta := range packs {
		item := templateListItem{ID: meta.ID, Name: meta.Name, Description: meta.Description, Icon: meta.Icon}
		// Counts need the payload (manifests omit it); Get reads it lazily.
		if full, ok := s.market.Get(meta.ID); ok && full.Payload.Workspace != nil {
			wp := full.Payload.Workspace
			item.AgentCount = len(wp.Agents)
			item.HasFlow = len(wp.Flows) > 0
			item.AutomationCount = len(wp.Automations)
			for _, a := range wp.Agents {
				if a.CoordinatorMode {
					item.CoordinatorCount++
				}
			}
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		// Pin blank first; otherwise preserve the incoming (name-sorted) order.
		return out[i].ID == blankTemplateID && out[j].ID != blankTemplateID
	})
	writeJSON(w, http.StatusOK, out)
}

// resolveTemplatePayload returns the WorkspacePayload for a template id, falling
// back to the embedded blank template when the id is empty/unknown. The second
// return is false only when even the blank fallback cannot be resolved (no
// bundled templates — should not happen in a normal build).
func (s *Server) resolveTemplatePayload(id string) (market.WorkspacePayload, bool) {
	if id != "" {
		if p, ok := s.market.Get(id); ok && p.Kind == market.KindWorkspace && p.Payload.Workspace != nil {
			return *p.Payload.Workspace, true
		}
	}
	if id != blankTemplateID {
		if p, ok := s.market.Get(blankTemplateID); ok && p.Payload.Workspace != nil {
			return *p.Payload.Workspace, true
		}
	}
	return market.WorkspacePayload{}, false
}

// seedWorkspaceFromTemplate populates a freshly created workspace from a chosen
// template id: it resolves the market pack and seeds its starter team. Identity
// (icon/color) is NOT applied here — the create-workspace flow lets the user pick
// those in the modal; only template content (instructions/board/agents/flow/
// schedules) is seeded. Unknown ids fall back to the blank template.
func (s *Server) seedWorkspaceFromTemplate(ctx context.Context, wsNew *workspace.Workspace, templateID string) {
	wp, ok := s.resolveTemplatePayload(templateID)
	if !ok {
		return // no templates available — leave the workspace empty
	}
	// Apply the template's instructions / board layout (icon/color stay the
	// user's choice from the create modal).
	patch := workspace.WSSettingsPatch{}
	if wp.Instructions != "" {
		patch.Instructions = &wp.Instructions
	}
	if len(wp.Columns) > 0 {
		cols := toBoardColumnDefs(wp.Columns)
		patch.BoardColumns = &cols
	}
	if patch.Instructions != nil || patch.BoardColumns != nil {
		if updated, err := s.workspaces.UpdateSettings(wsNew.ID, patch); err != nil {
			s.logger.Warn("seed template settings failed", "workspace", wsNew.ID, "error", err)
		} else if patch.BoardColumns != nil {
			// Mirror handleUpdateWorkspaceSettings / market_install: a freshly
			// created workspace seeded from a template still needs the live-mode
			// Network anchors + any open TaskBoard to see the new columns.
			publishEntityChange(updated, "board", "Boards sütunları güncellendi", "",
				map[string]string{"view": "board", "op": "columns_changed"})
		}
	}
	s.seedWorkspaceTeam(ctx, wsNew, wp)
}

// seedWorkspaceTeam seeds a template's full starter ecosystem into a workspace:
// bundled skills, a richly-configured agent team, one or more flows (linear or
// non-linear) wiring them, and disabled starter schedules. Seed order matters —
// skills first (so agent skill assignments resolve), then agents, then flows and
// schedules (which reference agents by key). Shared by the create-from-template
// flow and the market workspace-pack install. All failures are logged but
// non-fatal so the workspace is still usable.
func (s *Server) seedWorkspaceTeam(ctx context.Context, wsNew *workspace.Workspace, wp market.WorkspacePayload) {
	// 1) Bundled skills → workspace skills dir, then reload so they are known to
	// the catalog before agents reference them.
	s.seedTemplateSkills(wsNew, wp.Skills)

	provider, model := s.defaultProviderModel(wsNew)
	skillStore := wsNew.Runtime.Skills()

	// 2) Create the (rich) agents, recording key → real ID for flow/schedule
	// wiring. Empty provider/model fall back to the workspace/app default; skill
	// references are filtered to ones that actually resolved.
	ids := make(map[string]string, len(wp.Agents))
	for _, ta := range wp.Agents {
		ap, am := ta.Provider, ta.Model
		if ap == "" {
			ap = provider
		}
		if am == "" {
			am = model
		}
		known := make([]string, 0, len(ta.Skills))
		for _, slug := range ta.Skills {
			if _, ok := skillStore.Get(slug); ok {
				known = append(known, slug)
			}
		}
		// Default-on: workspace templates that omit mcpEnabled (or ship it
		// false because Go's bool zero value is false) seed agents with tools
		// enabled, so the starter team is ready to use every workspace-active
		// tool without the user toggling the master switch in each agent's
		// detail panel. To turn tools off for a chat-only seeded agent, use
		// UpdateAgentTools after seeding — see db.Agent.MCPEnabled.
		mcpEnabled := ta.MCPEnabled
		if !mcpEnabled {
			mcpEnabled = true
		}
		// A pinned coordinator recipe is only carried over when it resolves against
		// this workspace's skills (the pack normally bundles it — seeded in step 1
		// above, before agents, precisely so this works).
		workflow := resolvableRecipe(skillStore, ta.CoordinatorWorkflow)
		if workflow == "" && strings.TrimSpace(ta.CoordinatorWorkflow) != "" {
			s.logger.Warn("seed template agent: unknown coordinator recipe, ignoring",
				"workspace", wsNew.ID, "agent", ta.Name, "workflow", ta.CoordinatorWorkflow)
		}
		// ap is a template's kind id, accepted as a provider INSTANCE id here too
		// (default-instance-id-equals-kind-id convention, _Docs/71 §3/§5). A
		// template referencing a kind that no longer exists is logged and
		// skipped, same as any other seed failure for this agent — not a silent
		// claude-cli fallback.
		providerKind, providerInstanceID, perr := agent.SyncProviderFields(s.providers, ap)
		if perr != nil {
			s.logger.Warn("seed template agent: unresolvable provider, skipping", "workspace", wsNew.ID, "agent", ta.Name, "provider", ap, "error", perr)
			continue
		}
		created, err := wsNew.DB.CreateAgent(ctx, db.Agent{
			Name:                ta.Name,
			Soul:                ta.Soul,
			Identity:            ta.Identity,
			Avatar:              ta.Avatar,
			Color:               ta.Color,
			Provider:            providerKind,
			ProviderInstanceID:  providerInstanceID,
			Model:               am,
			ThinkingLevel:       ta.ThinkingLevel,
			PermissionMode:      ta.PermissionMode,
			MCPEnabled:          mcpEnabled,
			AllowedTools:        ta.AllowedTools,
			BlockedTools:        ta.BlockedTools,
			ToolOverrides:       ta.ToolOverrides,
			Skills:              known,
			CoordinatorMode:     ta.CoordinatorMode,
			CoordinatorWorkflow: workflow,
			CoordinatorPrompt:   ta.CoordinatorPrompt,
		})
		if err != nil {
			s.logger.Warn("seed template agent failed", "workspace", wsNew.ID, "agent", ta.Name, "error", err)
			continue
		}
		ids[ta.Key] = created.ID
	}

	// 3) Flows (linear or non-linear), each wired to the seeded agents. The
	// name → id map feeds flow-backed automations in step 5.
	flowIDs := make(map[string]string, len(wp.Flows))
	for _, tf := range wp.Flows {
		if id := s.seedTemplateFlow(ctx, wsNew, tf, ids); id != "" {
			flowIDs[tf.Name] = id
		}
	}

	// 4) Starter schedules (always disabled).
	for _, ts := range wp.Schedules {
		agentID, ok := ids[ts.AgentKey]
		if !ok {
			continue
		}
		if _, err := wsNew.DB.CreateSchedule(ctx, db.Schedule{
			Name:     ts.Name,
			AgentID:  agentID,
			CronExpr: ts.CronExpr,
			Prompt:   ts.Prompt,
			Enabled:  false,
		}); err != nil {
			s.logger.Warn("seed template schedule failed", "workspace", wsNew.ID, "error", err)
		}
	}

	// 5) Starter automations (always disabled), wired to the seeded team.
	s.seedTemplateAutomations(ctx, wsNew, wp.Automations, ids, flowIDs)

	// 6) Editable config files: non-default runtime prompts + README.
	s.seedTemplateConfigFiles(wsNew, wp)
}

// seedTemplateAutomations creates a template's starter automation rules, resolving
// each rule's agent key / flow name against the team seeded above. A rule whose
// target does not resolve is SKIPPED, not seeded with an empty target: a board
// rule with no agent fires and then fails on every card move, which is worse than
// a rule that is simply absent. (This is also why the built-in board defaults —
// seeded at workspace-open time, before any template agents exist — cannot serve a
// template team: their target is empty by construction.)
//
// Every rule is seeded DISABLED, matching starter schedules and the built-in board
// defaults: the wiring ships, the spending does not.
func (s *Server) seedTemplateAutomations(ctx context.Context, wsNew *workspace.Workspace, autos []market.WorkspaceTemplateAutomation, agentIDs, flowIDs map[string]string) {
	for _, ta := range autos {
		agentID, flowID := "", ""
		if ta.AgentKey != "" {
			id, ok := agentIDs[ta.AgentKey]
			if !ok {
				s.logger.Warn("seed template automation skipped: unknown agent key",
					"workspace", wsNew.ID, "automation", ta.Name, "agentKey", ta.AgentKey)
				continue
			}
			agentID = id
		}
		if ta.FlowName != "" {
			id, ok := flowIDs[ta.FlowName]
			if !ok {
				s.logger.Warn("seed template automation skipped: unknown flow",
					"workspace", wsNew.ID, "automation", ta.Name, "flow", ta.FlowName)
				continue
			}
			flowID = id
		}
		// A spawn-action rule with neither target would fire into the void.
		// Archive-action board rules legitimately have no target (no LLM call).
		if agentID == "" && flowID == "" && ta.BoardAction != db.BoardActionArchive {
			s.logger.Warn("seed template automation skipped: no agent or flow target",
				"workspace", wsNew.ID, "automation", ta.Name)
			continue
		}
		maxIter := ta.MaxIterations
		if maxIter <= 0 {
			maxIter = db.MaxIterationsHardCap
		}
		if _, err := wsNew.DB.CreateAutomation(ctx, db.Automation{
			Name:            ta.Name,
			TriggerKind:     ta.TriggerKind,
			TriggerTag:      ta.TriggerTag,
			BoardOp:         ta.BoardOp,
			BoardFromState:  ta.BoardFromState,
			BoardToState:    ta.BoardToState,
			BoardPriority:   ta.BoardPriority,
			BoardExclusive:  ta.BoardExclusive,
			BoardAction:     ta.BoardAction,
			TokenScope:      ta.TokenScope,
			TokenThreshold:  ta.TokenThreshold,
			CounterMetric:   ta.CounterMetric,
			CounterScope:    ta.CounterScope,
			CounterInterval: ta.CounterInterval,
			TargetAgentID:   agentID,
			FlowID:          flowID,
			SessionMode:     ta.SessionMode,
			PromptTemplate:  ta.PromptTemplate,
			SpawnTags:       ta.SpawnTags,
			MaxIterations:   maxIter,
			CooldownSec:     ta.CooldownSec,
			Enabled:         false,
		}); err != nil {
			s.logger.Warn("seed template automation failed", "workspace", wsNew.ID, "automation", ta.Name, "error", err)
		}
	}
}

// seedTemplateConfigFiles writes a template's non-default runtime prompt overrides
// and README into the workspace config dir (<workspace>/config/). Only the keys
// the publisher actually customised are present in wp.Prompts, so untouched keys
// keep the running build's compiled-in defaults. All failures are logged but
// non-fatal.
func (s *Server) seedTemplateConfigFiles(wsNew *workspace.Workspace, wp market.WorkspacePayload) {
	wsDir := wsNew.DataDir
	if len(wp.Prompts) > 0 {
		valid := map[string]bool{}
		for _, k := range agent.PromptKeys {
			valid[k] = true
		}
		if err := os.MkdirAll(filepath.Join(agent.WorkspaceConfigDir(wsDir), "prompts"), 0o755); err != nil {
			s.logger.Warn("seed template prompts mkdir failed", "workspace", wsNew.ID, "error", err)
		} else {
			for key, content := range wp.Prompts {
				if !valid[key] || content == "" {
					continue // unknown key or empty override — nothing to write
				}
				if err := os.WriteFile(agent.PromptFilePath(wsDir, key), []byte(content), 0o644); err != nil {
					s.logger.Warn("seed template prompt failed", "workspace", wsNew.ID, "prompt", key, "error", err)
				}
			}
		}
	}
	if strings.TrimSpace(wp.Readme) != "" {
		if err := os.MkdirAll(agent.WorkspaceConfigDir(wsDir), 0o755); err != nil {
			s.logger.Warn("seed template readme mkdir failed", "workspace", wsNew.ID, "error", err)
		} else if err := os.WriteFile(agent.ReadmeFilePath(wsDir), []byte(wp.Readme), 0o644); err != nil {
			s.logger.Warn("seed template readme failed", "workspace", wsNew.ID, "error", err)
		}
	}
}

// seedTemplateSkills writes a template's bundled skills into the workspace skills
// dir and reloads the catalog so agents can reference them by slug. Reuses the
// market skill installer (slug-safety, nested files) via a synthetic pack.
func (s *Server) seedTemplateSkills(wsNew *workspace.Workspace, skills []market.WorkspaceTemplateSkill) {
	if len(skills) == 0 {
		return
	}
	skillsDir := wsNew.Runtime.WorkspaceSkillsDir()
	for _, sk := range skills {
		if sk.Slug == "" || sk.Body == "" {
			continue
		}
		p := market.Pack{
			Kind:    market.KindSkill,
			Payload: market.Payload{Skill: &market.SkillPayload{Slug: sk.Slug, Body: sk.Body}},
			Files:   sk.Files,
		}
		if _, err := market.InstallSkill(p, skillsDir, true); err != nil {
			s.logger.Warn("seed template skill failed", "workspace", wsNew.ID, "skill", sk.Slug, "error", err)
		}
	}
	wsNew.Runtime.Skills().Reload()
}

// seedTemplateFlow resolves a template flow's graph (linear steps OR a full
// orchestration graph with branch/parallel/delay/transform), wiring agent keys
// to real IDs, and persists it. A flow referencing a missing agent is skipped.
// Returns the created flow's id ("" when the flow was skipped) so a flow-backed
// starter automation can bind to it by name.
func (s *Server) seedTemplateFlow(ctx context.Context, wsNew *workspace.Workspace, tf market.WorkspaceTemplateFlow, ids map[string]string) string {
	graph, ok := resolveTemplateFlowGraph(tf, ids)
	if !ok {
		s.logger.Warn("seed flow skipped: unresolved/invalid", "workspace", wsNew.ID, "flow", tf.Name)
		return ""
	}
	raw, err := json.Marshal(graph)
	if err != nil {
		s.logger.Warn("seed flow marshal failed", "workspace", wsNew.ID, "error", err)
		return ""
	}
	created, err := wsNew.DB.CreateFlow(ctx, db.Flow{
		Name:  tf.Name,
		Graph: string(raw),
	})
	if err != nil {
		s.logger.Warn("seed flow create failed", "workspace", wsNew.ID, "error", err)
		return ""
	}
	return created.ID
}

// resolveTemplateFlowGraph builds the runnable orchestration graph for a template
// flow, resolving agent keys to the real agent ids created for this workspace. A
// flow with a Graph uses it directly (agent nodes' agentId "tmpl:<key>" are
// substituted, so branch/parallel/delay/transform all work); otherwise the linear
// Steps are assembled. Returns false when a referenced agent key is missing or the
// resulting graph is empty/invalid. Pure (no side effects) so it is unit-testable.
func resolveTemplateFlowGraph(tf market.WorkspaceTemplateFlow, ids map[string]string) (orchestration.Graph, bool) {
	if g := strings.TrimSpace(tf.Graph); g != "" {
		graph, err := orchestration.ParseGraph(g)
		if err != nil {
			return orchestration.Graph{}, false
		}
		for i := range graph.Nodes {
			n := &graph.Nodes[i]
			if n.Type == orchestration.NodeAgent && strings.HasPrefix(n.AgentID, market.TemplateAgentKeyPrefix) {
				real, ok := ids[strings.TrimPrefix(n.AgentID, market.TemplateAgentKeyPrefix)]
				if !ok {
					return orchestration.Graph{}, false
				}
				n.AgentID = real
			}
		}
		graph, _ = orchestration.MigrateAddStart(graph) // ensure the required start node
		if graph.Validate() != nil {
			return orchestration.Graph{}, false
		}
		return graph, true
	}

	// Linear steps → a sequential agent graph.
	if len(tf.Steps) == 0 {
		return orchestration.Graph{}, false
	}
	nodes := make([]orchestration.Node, 0, len(tf.Steps))
	for i, st := range tf.Steps {
		agentID, ok := ids[st.AgentKey]
		if !ok {
			return orchestration.Graph{}, false
		}
		next := ""
		if i+1 < len(tf.Steps) {
			next = tf.Steps[i+1].ID
		}
		nodes = append(nodes, orchestration.Node{
			ID:      st.ID,
			Type:    orchestration.NodeAgent,
			Title:   st.Title,
			AgentID: agentID,
			Prompt:  st.Prompt,
			Next:    next,
		})
	}
	graph := orchestration.Graph{Start: tf.Steps[0].ID, Nodes: nodes}
	graph, _ = orchestration.MigrateAddStart(graph) // ensure the required start node
	if graph.Validate() != nil {
		return orchestration.Graph{}, false
	}
	return graph, true
}

// defaultProviderModel resolves the provider/model for seeded agents. A
// freshly-created workspace has no agents yet and there is no abstract default
// provider/model, so seeded agents that omit both start on the keyless local
// claude-cli with the provider's own default model (empty).
//
// Not switched to codex-cli: this is THE keyless final fallback every fresh
// workspace lands on before the user picks anything, so it must be the provider
// most likely to already be logged in / configured out of the box. claude-cli
// stays that default; codex-cli is opt-in like every other provider.
func (s *Server) defaultProviderModel(_ *workspace.Workspace) (provider, model string) {
	return "claude-cli", ""
}
