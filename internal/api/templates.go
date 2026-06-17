package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/orchestration"
	"github.com/bilal/swarmgo/internal/workspace"
)

// Workspace templates seed a freshly created workspace with a curated set of
// agents, an orchestration flow connecting them, and optional schedules — so a
// new workspace arrives ready for a specific kind of work (research, software
// development, daily routine) instead of an empty roster.

// tmplAgent is a template agent definition. Key is a local reference used to
// wire flow nodes to the agent's real ID after creation.
type tmplAgent struct {
	Key    string
	Name   string
	Soul   string
	Avatar string
	Color  string
}

// tmplStep is one node of a linear flow pipeline. AgentKey points at a
// tmplAgent.Key; Prompt is an orchestration template ({{input}}, {{last}}).
type tmplStep struct {
	ID       string
	Title    string
	AgentKey string
	Prompt   string
}

// tmplFlow is a linear (sequential) flow built from steps.
type tmplFlow struct {
	Name        string
	Description string
	Steps       []tmplStep
}

// tmplSchedule is a starter cron schedule. It is always seeded DISABLED so it
// never fires until the user opts in via the Schedules screen.
type tmplSchedule struct {
	AgentKey string
	CronExpr string
	Prompt   string
}

// workspaceTemplate is a complete workspace blueprint.
type workspaceTemplate struct {
	ID          string
	Name        string
	Description string
	Icon        string
	Agents      []tmplAgent
	Flow        *tmplFlow
	Schedules   []tmplSchedule
}

// workspaceTemplates is the registry of available templates, in display order.
// The first entry ("blank") reproduces the legacy single-assistant default.
var workspaceTemplates = []workspaceTemplate{
	{
		ID:          "blank",
		Name:        "Boş",
		Description: "Tek bir genel asistan ile başla. Kendi ajanlarını ve akışlarını sıfırdan kur.",
		Icon:        "⬡",
		Agents: []tmplAgent{
			{Key: "assistant", Name: "Asistan", Soul: "You are a helpful assistant."},
		},
		Schedules: []tmplSchedule{
			{AgentKey: "assistant", CronExpr: "0 * * * *", Prompt: "Review the task board for unfinished or stuck tasks and send a notification summarizing them."},
		},
	},
	{
		ID:          "research",
		Name:        "Bilimsel Araştırma",
		Description: "Literatür taraması, metodoloji, analiz ve hakem değerlendirmesi ajanları + uçtan uca araştırma akışı.",
		Icon:        "🔬",
		Agents: []tmplAgent{
			{Key: "scout", Name: "Literatür Tarayıcı", Avatar: "📚", Color: "#6366f1",
				Soul: "You are a research literature scout. Given a research question, you find, gather and summarize the most relevant prior work, papers and sources. You cite sources, extract key findings, and surface gaps in the existing literature."},
			{Key: "method", Name: "Metodolog", Avatar: "🧪", Color: "#0ea5e9",
				Soul: "You are a research methodologist. You design rigorous methodology: clear hypotheses, variables, experiment or analysis design, sampling, and threats to validity. You favor reproducible, well-justified designs."},
			{Key: "analyst", Name: "Analist", Avatar: "📊", Color: "#10b981",
				Soul: "You are a data and results analyst. You analyze findings, choose appropriate statistical or qualitative methods, interpret results honestly, and draw evidence-based conclusions while stating uncertainty."},
			{Key: "critic", Name: "Hakem", Avatar: "🧐", Color: "#f59e0b",
				Soul: "You are a peer reviewer. You critically evaluate research for rigor, bias, reproducibility and gaps. You give constructive, specific feedback and concrete suggestions for improvement."},
		},
		Flow: &tmplFlow{
			Name:        "Araştırma Akışı",
			Description: "Tarama → Metodoloji → Analiz → Hakem değerlendirmesi",
			Steps: []tmplStep{
				{ID: "scan", Title: "Literatür Tarama", AgentKey: "scout",
					Prompt: "Research question:\n{{input}}\n\nFind and summarize the most relevant prior work and sources. List key findings, cite them, and note open gaps."},
				{ID: "design", Title: "Metodoloji", AgentKey: "method",
					Prompt: "Research question:\n{{input}}\n\nPrior work summary:\n{{last}}\n\nDesign a rigorous methodology to investigate this question."},
				{ID: "analyze", Title: "Analiz", AgentKey: "analyst",
					Prompt: "Given the research context and methodology below, outline the analysis plan and interpret what the expected results would mean.\n\n{{last}}"},
				{ID: "review", Title: "Hakem Değerlendirmesi", AgentKey: "critic",
					Prompt: "Critically review the following research plan and analysis. Identify weaknesses, biases and gaps, and propose concrete improvements.\n\n{{last}}"},
			},
		},
	},
	{
		ID:          "software",
		Name:        "Yazılım Geliştirme",
		Description: "Search · Plan · Execute · Verify ajanları ve bu adımları sırayla yürüten geliştirme akışı.",
		Icon:        "🛠",
		Agents: []tmplAgent{
			{Key: "search", Name: "Araştırmacı (Search)", Avatar: "🔍", Color: "#6366f1",
				Soul: "You explore the codebase and gather context before any change: relevant files, existing patterns, conventions and constraints. You output a concise, well-organized findings report and never edit code."},
			{Key: "plan", Name: "Planlayıcı (Plan)", Avatar: "🗺", Color: "#8b5cf6",
				Soul: "You turn findings into a concrete, step-by-step implementation plan: which files to change, in what order, the approach, and the risks. You do not write the final code — you produce an actionable plan."},
			{Key: "execute", Name: "Geliştirici (Execute)", Avatar: "⚙", Color: "#10b981",
				Soul: "You implement the plan with clean, idiomatic code and small, atomic changes. You follow existing conventions and keep edits focused on the plan."},
			{Key: "verify", Name: "Doğrulayıcı (Verify)", Avatar: "✅", Color: "#f59e0b",
				Soul: "You verify the implementation: run or inspect tests, check that the original goal is met, and report any remaining issues or regressions clearly."},
		},
		Flow: &tmplFlow{
			Name:        "Search → Plan → Execute → Verify",
			Description: "Yazılım görevlerini dört aşamada yürüten akış",
			Steps: []tmplStep{
				{ID: "search", Title: "Search", AgentKey: "search",
					Prompt: "Task:\n{{input}}\n\nExplore the relevant code and context. Produce a concise findings report."},
				{ID: "plan", Title: "Plan", AgentKey: "plan",
					Prompt: "Task:\n{{input}}\n\nFindings:\n{{last}}\n\nProduce a concrete step-by-step implementation plan."},
				{ID: "execute", Title: "Execute", AgentKey: "execute",
					Prompt: "Implement the following plan with clean, atomic changes.\n\n{{last}}"},
				{ID: "verify", Title: "Verify", AgentKey: "verify",
					Prompt: "Verify the implementation below against the original task. Run/inspect tests and report remaining issues.\n\nTask:\n{{input}}\n\nImplementation:\n{{last}}"},
			},
		},
	},
	{
		ID:          "daily",
		Name:        "Günlük Rutin",
		Description: "Planlayıcı, koç ve hatırlatıcı ajanlarla günlük rutinleri optimize et. Sabah planı + akşam değerlendirmesi zamanlamaları hazır gelir.",
		Icon:        "🌙",
		Agents: []tmplAgent{
			{Key: "planner", Name: "Planlayıcı", Avatar: "🗓", Color: "#6366f1",
				Soul: "You are a daily planner. You help organize the day: prioritize tasks, time-block the schedule, and set realistic, achievable goals. You keep plans concise and actionable."},
			{Key: "coach", Name: "Koç", Avatar: "🎯", Color: "#10b981",
				Soul: "You are a productivity and habit coach. You optimize routines, suggest small habit improvements, remove friction, and keep motivation high with encouraging, practical advice."},
			{Key: "reminder", Name: "Hatırlatıcı", Avatar: "⏰", Color: "#f59e0b",
				Soul: "You are a reminder assistant. You track commitments and surface timely, friendly reminders and follow-ups so nothing important slips."},
		},
		Flow: &tmplFlow{
			Name:        "Günlük Optimizasyon",
			Description: "Plan → Koçluk: günü planla, ardından rutini iyileştir",
			Steps: []tmplStep{
				{ID: "plan", Title: "Günü Planla", AgentKey: "planner",
					Prompt: "Today's context, goals and tasks:\n{{input}}\n\nProduce a prioritized, time-blocked plan for the day."},
				{ID: "coach", Title: "Rutini İyileştir", AgentKey: "coach",
					Prompt: "Given today's plan below, suggest concrete improvements to the routine and habits to make the day smoother and more productive.\n\n{{last}}"},
			},
		},
		Schedules: []tmplSchedule{
			{AgentKey: "planner", CronExpr: "0 8 * * *", Prompt: "Plan today: review priorities and propose a prioritized, time-blocked schedule, then send it as a notification."},
			{AgentKey: "coach", CronExpr: "0 20 * * *", Prompt: "Reflect on today: review what was done, note wins and friction points, and suggest one improvement for tomorrow."},
		},
	},
}

// templateByID returns the named template, falling back to the "blank" default.
func templateByID(id string) workspaceTemplate {
	for _, t := range workspaceTemplates {
		if t.ID == id {
			return t
		}
	}
	return workspaceTemplates[0]
}

// templateListItem is the catalog view sent to the frontend.
type templateListItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	AgentCount  int    `json:"agentCount"`
	HasFlow     bool   `json:"hasFlow"`
}

// handleListWorkspaceTemplates returns the available workspace templates.
func (s *Server) handleListWorkspaceTemplates(w http.ResponseWriter, _ *http.Request) {
	out := make([]templateListItem, 0, len(workspaceTemplates))
	for _, t := range workspaceTemplates {
		out = append(out, templateListItem{
			ID: t.ID, Name: t.Name, Description: t.Description, Icon: t.Icon,
			AgentCount: len(t.Agents), HasFlow: t.Flow != nil,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// seedTemplate populates a freshly created workspace from a template: it creates
// the agents, wires and stores the flow (if any), and adds the disabled starter
// schedules. The first created agent is treated as the workspace default. All
// failures are logged but non-fatal so the workspace is still usable.
func (s *Server) seedTemplate(ctx context.Context, wsNew *workspace.Workspace, tmpl workspaceTemplate) {
	provider, model := s.defaultProviderModel(wsNew)

	// Create agents, recording key → real ID for flow/schedule wiring.
	ids := make(map[string]string, len(tmpl.Agents))
	for _, ta := range tmpl.Agents {
		agent, err := wsNew.DB.CreateAgent(ctx, db.Agent{
			Name:     ta.Name,
			Soul:     ta.Soul,
			Avatar:   ta.Avatar,
			Color:    ta.Color,
			Provider: provider,
			Model:    model,
		})
		if err != nil {
			s.logger.Warn("seed template agent failed", "workspace", wsNew.ID, "agent", ta.Name, "error", err)
			continue
		}
		ids[ta.Key] = agent.ID
	}

	// Wire and store the flow, if all referenced agents were created.
	if tmpl.Flow != nil && len(tmpl.Flow.Steps) > 0 {
		s.seedTemplateFlow(ctx, wsNew, *tmpl.Flow, ids)
	}

	// Starter schedules (always disabled).
	for _, ts := range tmpl.Schedules {
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
}

// seedTemplateFlow builds a linear orchestration graph from the template steps,
// resolving agent keys to real IDs, and persists it as a flow.
func (s *Server) seedTemplateFlow(ctx context.Context, wsNew *workspace.Workspace, tf tmplFlow, ids map[string]string) {
	nodes := make([]orchestration.Node, 0, len(tf.Steps))
	for i, st := range tf.Steps {
		agentID, ok := ids[st.AgentKey]
		if !ok {
			s.logger.Warn("seed flow skipped: missing agent", "workspace", wsNew.ID, "flow", tf.Name, "agentKey", st.AgentKey)
			return
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
	if err := graph.Validate(); err != nil {
		s.logger.Warn("seed flow invalid graph", "workspace", wsNew.ID, "flow", tf.Name, "error", err)
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
