package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// agentContextPreview is the context an agent receives on a FRESH turn (no
// session history): the static system prompt plus the tool catalog it is given.
// The dynamic suffix (recalled memory, running summary, session artifacts, todo
// list, cross-session context) is added per-turn and depends on the message, so
// it is intentionally not part of this "from scratch" preview.
type agentContextPreview struct {
	// Provider drives provider-aware UI notes (e.g. claude-cli weaves the dynamic
	// suffix into the last user message rather than a separate system block).
	Provider     string `json:"provider"`
	System       string `json:"system"`
	SystemTokens int    `json:"systemTokens"`
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
	// CLIOverhead is set only for CLI-wrapper providers (claude-cli): the projected
	// harness cost (base system + built-ins + eager bridged tools) that TotalTokens
	// does NOT include. Predicted-only here — an agent preview has no session, so
	// there is no measured figure (MeasuredTokens/Calls are 0).
	CLIOverhead *cliOverheadPreview `json:"cliOverhead,omitempty"`
}

// toolSummary is one offered tool: name + full description, plus the full JSON
// input schema for EAGER tools (so the preview can show exactly what is shipped
// every turn, expandable per tool). InputSchema is empty for lazy tools, whose
// schemas are not shipped at turn start. ToolTokens still counts the eager schemas.
type toolSummary struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	// Visibility is the tool's effective tier ("full" | "summary" | "name-only" |
	// "hidden"), set only for lazy (on-demand) entries so the UI can badge each and
	// render it like the load-on-demand catalog block does: summary keeps its
	// description, name-only/hidden show the name alone. Empty for eager tools.
	Visibility string `json:"visibility,omitempty"`
}

// tieredLazyTools shapes the on-demand (lazy) tool list for a context preview so
// each entry mirrors what the "Available Tools (load on demand)" block actually
// renders to the model: summary keeps its (truncated) description, name-only and
// hidden shed it (name alone). The visibility tier rides along so the UI can badge
// each row (Özet / İsim / Gizli). lazyDefs come from Runtime.LazyToolCatalog (bare
// built-in names, namespaced MCP names) and visOf resolves each tool's tier from
// the same per-agent registry the real turn uses.
func tieredLazyTools(visOf func(string) string, lazyDefs []providers.ToolDef) []toolSummary {
	out := make([]toolSummary, 0, len(lazyDefs))
	for _, d := range lazyDefs {
		vis := visOf(d.Name)
		desc := d.Description
		if vis == tools.VisibilityNameOnly || vis == tools.VisibilityHidden {
			desc = "" // name alone — matches the rendered catalog block
		}
		out = append(out, toolSummary{Name: d.Name, Description: desc, Visibility: vis})
	}
	return out
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
	// Tier-aware so each row mirrors the load-on-demand catalog block (summary keeps
	// its description, name-only/hidden show the name alone) and carries its chip.
	lazyDefs := wsp.Runtime.LazyToolCatalog(ctx, agent)
	lazyTools := tieredLazyTools(wsp.Runtime.ToolVisibilityFunc(ctx, agent), lazyDefs)
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
	total := sysTok + skillsTok + toolTok + dynTok
	// Predicted-only CLI overhead (empty sessionID → no measured turn): count the
	// eager (core-tier) bridged tools that carry a full schema up front.
	eagerTools := 0
	for _, d := range defs {
		if interactionTier(d.Name) == "core" {
			eagerTools++
		}
	}
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
		TotalTokens:   total,
		CLIOverhead:   computeCLIOverhead(ctx, wsp, agent.Provider, "", total, eagerTools),
	})
}

// buildAgentDynamicPrompt simulates the per-turn dynamic suffix for a fresh
// agent: the cross-session block (always on). Mirrors the
// message-independent half of composeTurnRequest's dynamic assembly; session-
// scoped parts (summary/artifacts/todos) are intentionally excluded (no live
// session).
func buildAgentDynamicPrompt(ctx context.Context, wsp *workspace.Workspace, agent db.Agent, message string) string {
	var dynamic string
	if sb := sessionsContextBlock(ctx, wsp.DB, ""); sb != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + sb)
	}
	return strings.TrimSpace(dynamic)
}

// buildAgentStaticPrompt returns the STATIC prefix an agent starts a fresh turn
// with, for the "what does this agent begin with" preview.
//
// It does NOT mirror composeTurnRequest by hand — it CALLS the one builder the
// real turn uses, with a zero db.Session standing in for "no session yet". The
// hand-written mirror it replaced had drifted: it still carried the old
// "@<AgentName> picks who answers" note (rewritten in buildStaticPrefix) and
// never learned about the capability block, so the preview under-reported the
// prefix and quoted text the agent no longer receives.
//
// The zero session is not a fudge — it is exactly a session with no coordinator
// role and no working directory, which is what buildStaticPrefix reads off it
// (IsCoordinator, WorkingDir). A real session without an explicit cwd produces
// the same prefix. multiAgent is false: a fresh session has no other authors yet.
func (s *Server) buildAgentStaticPrompt(ctx context.Context, wsp *workspace.Workspace, agent db.Agent) string {
	return s.buildStaticPrefix(ctx, wsp, db.Session{}, agent, false)
}
