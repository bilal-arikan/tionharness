package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/agent"
	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/market"
	"github.com/bilal-arikan/swarmgo/internal/orchestration"
	"github.com/bilal-arikan/swarmgo/internal/workspace"
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
		if _, err := s.workspaces.UpdateSettings(wsNew.ID, patch); err != nil {
			s.logger.Warn("seed template settings failed", "workspace", wsNew.ID, "error", err)
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
		agent, err := wsNew.DB.CreateAgent(ctx, db.Agent{
			Name:            ta.Name,
			Soul:            ta.Soul,
			Identity:        ta.Identity,
			Avatar:          ta.Avatar,
			Color:           ta.Color,
			Provider:        ap,
			Model:           am,
			ThinkingLevel:   ta.ThinkingLevel,
			PermissionMode:  ta.PermissionMode,
			MCPEnabled:      mcpEnabled,
			AllowedTools:    ta.AllowedTools,
			BlockedTools:    ta.BlockedTools,
			Skills:          known,
		})
		if err != nil {
			s.logger.Warn("seed template agent failed", "workspace", wsNew.ID, "agent", ta.Name, "error", err)
			continue
		}
		ids[ta.Key] = agent.ID
	}

	// 3) Flows (linear or non-linear), each wired to the seeded agents.
	for _, tf := range wp.Flows {
		s.seedTemplateFlow(ctx, wsNew, tf, ids)
	}

	// 4) Starter schedules (always disabled).
	for _, ts := range wp.Schedules {
		agentID, ok := ids[ts.AgentKey]
		if !ok {
			continue
		}
		if _, err := wsNew.DB.CreateSchedule(ctx, db.Schedule{
			AgentID:  agentID,
			CronExpr: ts.CronExpr,
			Prompt:   ts.Prompt,
			Enabled:  false,
		}); err != nil {
			s.logger.Warn("seed template schedule failed", "workspace", wsNew.ID, "error", err)
		}
	}

	// 5) Editable config files: non-default runtime prompts + README.
	s.seedTemplateConfigFiles(wsNew, wp)
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
func (s *Server) seedTemplateFlow(ctx context.Context, wsNew *workspace.Workspace, tf market.WorkspaceTemplateFlow, ids map[string]string) {
	graph, ok := resolveTemplateFlowGraph(tf, ids)
	if !ok {
		s.logger.Warn("seed flow skipped: unresolved/invalid", "workspace", wsNew.ID, "flow", tf.Name)
		return
	}
	raw, err := json.Marshal(graph)
	if err != nil {
		s.logger.Warn("seed flow marshal failed", "workspace", wsNew.ID, "error", err)
		return
	}
	if _, err := wsNew.DB.CreateFlow(ctx, db.Flow{
		Name:        tf.Name,
		Description: tf.Description,
		Graph:       string(raw),
	}); err != nil {
		s.logger.Warn("seed flow create failed", "workspace", wsNew.ID, "error", err)
	}
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
	if graph.Validate() != nil {
		return orchestration.Graph{}, false
	}
	return graph, true
}

// defaultProviderModel resolves the provider/model for seeded agents:
// workspace override → app default → claude-cli last resort.
func (s *Server) defaultProviderModel(wsNew *workspace.Workspace) (provider, model string) {
	cfg := s.settings.Get()
	wsCfg := wsNew.Settings()

	provider = wsCfg.DefaultProvider
	if provider == "" {
		provider = cfg.DefaultProvider
	}
	if provider == "" {
		provider = "claude-cli"
	}
	model = wsCfg.DefaultModel
	if model == "" {
		model = cfg.DefaultModel
	}
	return provider, model
}
