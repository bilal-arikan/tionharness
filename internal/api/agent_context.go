package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/conversation"
	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/workspace"
)

// agentContextPreview is the context an agent receives on a FRESH turn (no
// session history): the static system prompt plus the tool catalog it is given.
// The dynamic suffix (recalled memory, running summary, session artifacts, todo
// list, cross-session context) is added per-turn and depends on the message, so
// it is intentionally not part of this "from scratch" preview.
type agentContextPreview struct {
	// Provider drives provider-aware UI notes (e.g. claude-cli weaves the dynamic
	// suffix into the last user message rather than a separate system block).
	Provider     string        `json:"provider"`
	System       string        `json:"system"`
	SystemTokens int           `json:"systemTokens"`
	// Skills is the agent's selected-skills catalog block, split out of the system
	// prompt so the preview UI can fold it as its own segment (it still lives inside
	// the cached static prefix). Empty when the agent has no skills selected.
	Skills       string        `json:"skills"`
	SkillsTokens int           `json:"skillsTokens"`
	Tools        []toolSummary `json:"tools"`
	ToolTokens   int           `json:"toolTokens"`
	// LazyTools are the on-demand tools whose schemas are NOT shipped at turn start.
	// Their names+descriptions live in the system prompt's load-on-demand catalog
	// block (counted under SystemTokens). Activated via activate_tools.
	LazyTools []toolSummary `json:"lazyTools"`
	// Dynamic is the per-turn suffix simulated for the optional ?message= sample:
	// recalled memory (for that message) + the cross-session block (when enabled).
	// Session-only parts (summary, artifacts, todos) need a live session and are
	// omitted. Empty when no message was given and cross-session is off.
	Dynamic       string `json:"dynamic"`
	DynamicTokens int    `json:"dynamicTokens"`
	TotalTokens   int    `json:"totalTokens"`
}

// toolSummary is one offered tool: name + full description, plus the full JSON
// input schema for EAGER tools (so the preview can show exactly what is shipped
// every turn, expandable per tool). InputSchema is empty for lazy tools, whose
// schemas are not shipped at turn start. ToolTokens still counts the eager schemas.
type toolSummary struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
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
	system := s.buildAgentStaticPrompt(ctx, wsp, agent)
	// Eager tools only: these are the schemas actually shipped at turn start. Lazy
	// (on-demand) tools live in the system prompt's load-on-demand catalog block
	// and are counted under SystemTokens, not here.
	defs := wsp.Runtime.ShippedToolCatalog(ctx, agent)
	tools := make([]toolSummary, 0, len(defs))
	for _, d := range defs {
		// Eager tools ship their FULL schema every turn — include it so the preview
		// can expand the exact payload (description + input schema + folded examples).
		tools = append(tools, toolSummary{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema})
	}
	// Lazy tools: name+description only (schemas not shipped; tokens already in sysTok).
	lazyDefs := wsp.Runtime.LazyToolCatalog(ctx, agent)
	lazyTools := make([]toolSummary, 0, len(lazyDefs))
	for _, d := range lazyDefs {
		lazyTools = append(lazyTools, toolSummary{Name: d.Name, Description: d.Description})
	}
	// Optional sample message → simulate the message-dependent dynamic suffix.
	dynamic := buildAgentDynamicPrompt(ctx, wsp, agent, r.URL.Query().Get("message"))

	// Split the skills catalog block out of the composed system prompt (same block
	// composeTurnRequest appends) so the preview can fold it as its own segment.
	skillsText := strings.TrimSpace(wsp.Runtime.SkillsCatalogBlockForAgent(agent))
	if skillsText != "" {
		if stripped, ok := stripBlock(system, skillsText); ok {
			system = stripped
		} else {
			skillsText = ""
		}
	}

	sysTok := conversation.EstimateText(system)
	skillsTok := conversation.EstimateText(skillsText)
	toolTok := estimateToolCatalog(defs)
	dynTok := conversation.EstimateText(dynamic)
	writeJSON(w, http.StatusOK, agentContextPreview{
		Provider:      agent.Provider,
		System:        system,
		SystemTokens:  sysTok,
		Skills:        skillsText,
		SkillsTokens:  skillsTok,
		Tools:         tools,
		ToolTokens:    toolTok,
		LazyTools:     lazyTools,
		Dynamic:       dynamic,
		DynamicTokens: dynTok,
		TotalTokens:   sysTok + skillsTok + toolTok + dynTok,
	})
}

// buildAgentDynamicPrompt simulates the per-turn dynamic suffix for a fresh
// agent: memory recalled for the sample message (when given) plus the
// cross-session block (when that feature is enabled). Mirrors the message-
// independent half of composeTurnRequest's dynamic assembly; session-scoped
// parts (summary/artifacts/todos) are intentionally excluded (no live session).
func buildAgentDynamicPrompt(ctx context.Context, wsp *workspace.Workspace, agent db.Agent, message string) string {
	var dynamic string
	if message = strings.TrimSpace(message); message != "" {
		if block := wsp.Runtime.Memory().ContextBlock(ctx, agent.ID, message, 5); block != "" {
			dynamic = block
		}
	}
	if wsp.Runtime.SessionContextEnabled() {
		if sb := sessionsContextBlock(ctx, wsp.DB, "", wsp.Runtime.SessionContextRecentCount()); sb != "" {
			dynamic = strings.TrimSpace(dynamic + "\n\n" + sb)
		}
	}
	return strings.TrimSpace(dynamic)
}

// buildAgentStaticPrompt mirrors the STATIC half of composeTurnRequest for a
// single agent with no session, so the preview matches what is actually sent.
// Keep in sync with composeTurnRequest's static-prefix assembly.
func (s *Server) buildAgentStaticPrompt(ctx context.Context, wsp *workspace.Workspace, agent db.Agent) string {
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
	if tb := wsp.Runtime.LazyToolsCatalogBlock(ctx, agent); tb != "" {
		system = strings.TrimSpace(system + "\n\n" + tb)
	}
	return system
}
