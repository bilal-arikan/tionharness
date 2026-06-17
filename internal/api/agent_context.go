package api

import (
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/conversation"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/workspace"
)

// agentContextPreview is the context an agent receives on a FRESH turn (no
// session history): the static system prompt plus the tool catalog it is given.
// The dynamic suffix (recalled memory, running summary, session artifacts, todo
// list, cross-session context) is added per-turn and depends on the message, so
// it is intentionally not part of this "from scratch" preview.
type agentContextPreview struct {
	System       string         `json:"system"`
	SystemTokens int            `json:"systemTokens"`
	Tools        []toolSummary  `json:"tools"`
	ToolTokens   int            `json:"toolTokens"`
	TotalTokens  int            `json:"totalTokens"`
}

// toolSummary is one offered tool, name + description (schema omitted for the
// preview but counted in ToolTokens).
type toolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// handleAgentContext returns the assembled fresh-start context for an agent so
// the user can preview exactly what it begins each turn with.
func (s *Server) handleAgentContext(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()
	agent, err := wsp.DB.GetAgent(ctx, r.PathValue("id"))
	if writeDBError(w, err, "agent not found") {
		return
	}
	system := s.buildAgentStaticPrompt(wsp, agent)
	defs := wsp.Runtime.ToolCatalog(ctx, agent)
	tools := make([]toolSummary, 0, len(defs))
	for _, d := range defs {
		tools = append(tools, toolSummary{Name: d.Name, Description: d.Description})
	}
	sysTok := conversation.EstimateText(system)
	toolTok := estimateToolCatalog(defs)
	writeJSON(w, http.StatusOK, agentContextPreview{
		System:       system,
		SystemTokens: sysTok,
		Tools:        tools,
		ToolTokens:   toolTok,
		TotalTokens:  sysTok + toolTok,
	})
}

// buildAgentStaticPrompt mirrors the STATIC half of composeTurnRequest for a
// single agent with no session, so the preview matches what is actually sent.
// Keep in sync with composeTurnRequest's static-prefix assembly.
func (s *Server) buildAgentStaticPrompt(wsp *workspace.Workspace, agent db.Agent) string {
	system := buildSystemPrompt(agent)
	if n := strings.TrimSpace(agent.Name); n != "" {
		note := "You are the agent named \"" + n + "\". In this chat, the user picks which agent should answer by starting a message with \"@<AgentName>\". So an \"@" + n + "\" at the start of a message means the user is addressing you by name — treat it as being called, not as a file, skill, or entity to look up; just answer the rest of the message."
		system = strings.TrimSpace(note + "\n\n" + system)
	}
	if uc := userContextBlock(s.settings.Get()); uc != "" {
		system = strings.TrimSpace(uc + "\n\n" + system)
	}
	if ins := strings.TrimSpace(wsp.Settings().Instructions); ins != "" {
		system = strings.TrimSpace(system + "\n\n# Workspace Instructions\n" + ins)
	}
	system = strings.TrimSpace(system + "\n\n" + artifactDeliverableGuidance)
	if sb := wsp.Runtime.SkillsCatalogBlockForAgent(agent); sb != "" {
		system = strings.TrimSpace(system + "\n\n" + sb)
	}
	return system
}
