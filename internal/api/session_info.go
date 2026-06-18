package api

import (
	"context"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bilal/swarmgo/internal/conversation"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/workspace"
)

// sessionInfoResp is the rich detail payload behind the session detail panel:
// on-disk footprint, context composition and the agents that took part.
type sessionInfoResp struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Kind         string `json:"kind"`
	State        string `json:"state"`
	AgentID      string `json:"agentId"`
	AgentName    string `json:"agentName"`
	MessageCount int    `json:"messageCount"`
	Unread       bool   `json:"unread"`
	Goal         string `json:"goal"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`

	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	FileCount int    `json:"fileCount"`

	ContextTokens   int  `json:"contextTokens"`
	ContextWindow   int  `json:"contextWindow"` // compaction threshold (effective window)
	HasSummary      bool `json:"hasSummary"`
	SummaryMsgCount int  `json:"summaryMsgCount"`
	SummaryTokens   int  `json:"summaryTokens"`

	// Fillers breaks the live context window (summary + pending messages) into
	// labelled buckets so the user sees what actually fills the model's context.
	Fillers []contextFiller `json:"fillers"`

	// Agents lists every agent that produced a turn in this session, with the
	// session's default agent always included even with zero turns.
	Agents []sessionAgentStat `json:"agents"`
}

type contextFiller struct {
	Label  string `json:"label"`
	Role   string `json:"role"`
	Tokens int    `json:"tokens"`
	Count  int    `json:"count"`
}

type sessionAgentStat struct {
	AgentID  string `json:"agentId"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Color    string `json:"color"`
	Turns    int    `json:"turns"`
	Tokens   int    `json:"tokens"`
	IsOwner  bool   `json:"isOwner"`
	Disabled bool   `json:"disabled"` // agent no longer exists
}

// roleLabel maps a message role to a Turkish display label for the filler list.
func roleLabel(role string) string {
	switch role {
	case "user":
		return "Kullanıcı"
	case "assistant":
		return "Asistan"
	case "tool":
		return "Araç"
	case "system":
		return "Sistem"
	default:
		return role
	}
}

// handleSessionInfo returns a session's full detail payload (disk size, context
// composition, participating agents) for the session detail panel.
func (s *Server) handleSessionInfo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()

	session, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}
	history, err := wsp.DB.ListMessages(ctx, id)
	if writeDBError(w, err, "") {
		return
	}

	resp := sessionInfoResp{
		ID:              session.ID,
		Title:           session.Title,
		Kind:            session.Kind,
		State:           session.State,
		AgentID:         session.AgentID,
		MessageCount:    session.MessageCount,
		Unread:          session.Unread,
		Goal:            session.Goal,
		CreatedAt:       session.CreatedAt,
		UpdatedAt:       session.UpdatedAt,
		HasSummary:      session.Summary != "",
		SummaryMsgCount: session.SummaryMsgCount,
		SummaryTokens:   conversation.EstimateText(session.Summary),
	}

	// On-disk footprint: walk the session's folder.
	if dir, err := wsp.DB.SessionDir(id); err == nil {
		resp.Path = dir
		resp.SizeBytes, resp.FileCount = dirSize(dir)
	}

	// Pending window = messages not yet folded into the summary (what is actually
	// sent to the model). Mirrors handleSessionContext.
	pending := history
	if session.SummaryMsgCount <= len(history) {
		pending = history[session.SummaryMsgCount:]
	}
	// Non-message context sent on every turn (system prompt, tool/MCP schemas,
	// artifact block) — estimated so the meter reflects the real footprint, not
	// just the visible transcript.
	extra := s.systemFillers(ctx, wsp, session)
	extraTokens := 0
	for _, f := range extra {
		extraTokens += f.Tokens
	}

	resp.ContextTokens = conversation.EstimateTokens(session.Summary, pending) + extraTokens
	// Effective window = the compaction threshold pushed into the conversation
	// manager; once the pending window exceeds it, older turns fold into summary.
	resp.ContextWindow = s.settings.Get().MaxContextTokens

	// Context fillers: summary + per-role message buckets PLUS the non-message
	// buckets (system/tools/artifacts), all sorted by token weight descending.
	resp.Fillers = append(buildFillers(session.Summary, pending), extra...)
	sort.SliceStable(resp.Fillers, func(i, j int) bool { return resp.Fillers[i].Tokens > resp.Fillers[j].Tokens })

	// Participating agents: distinct agent per assistant turn (falling back to the
	// session's default agent), with the default agent always present.
	resp.Agents = buildAgentStats(ctx, wsp.DB, session, history)
	for i := range resp.Agents {
		if resp.Agents[i].AgentID == session.AgentID {
			resp.AgentName = resp.Agents[i].Name
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// dirSize walks a folder, returning total bytes and regular-file count.
func dirSize(dir string) (int64, int) {
	var total int64
	var count int
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil {
			total += info.Size()
			count++
		}
		return nil
	})
	return total, count
}

// buildFillers turns the live context window into labelled, token-weighted buckets.
func buildFillers(summary string, pending []db.Message) []contextFiller {
	byRole := map[string]*contextFiller{}
	order := []string{}
	for _, m := range pending {
		f := byRole[m.Role]
		if f == nil {
			f = &contextFiller{Label: roleLabel(m.Role), Role: m.Role}
			byRole[m.Role] = f
			order = append(order, m.Role)
		}
		f.Tokens += conversation.EstimateText(m.Text)
		f.Count++
	}

	fillers := make([]contextFiller, 0, len(order)+1)
	if summary != "" {
		fillers = append(fillers, contextFiller{
			Label:  "Özet",
			Role:   "summary",
			Tokens: conversation.EstimateText(summary),
			Count:  1,
		})
	}
	for _, role := range order {
		fillers = append(fillers, *byRole[role])
	}
	sort.SliceStable(fillers, func(i, j int) bool { return fillers[i].Tokens > fillers[j].Tokens })
	return fillers
}

// systemFillers estimates the context that is sent on every turn but never
// appears as a chat message: the static system prompt (persona + user profile +
// workspace instructions), the agent's effective tool catalog (built-in + MCP
// schemas) and the session's artifact context block. Without these the meter
// under-reports how full the model's context actually is. Mirrors the request
// assembled by composeTurnRequest.
func (s *Server) systemFillers(ctx context.Context, wsp *workspace.Workspace, session db.Session) []contextFiller {
	agentRow, err := wsp.DB.GetAgent(ctx, session.AgentID)
	if err != nil {
		return nil
	}

	out := make([]contextFiller, 0, 3)

	// System prompt (static prefix): persona + user profile + workspace instructions.
	system := buildSystemPrompt(agentRow)
	if uc := userContextBlock(s.settings.Get()); uc != "" {
		system = strings.TrimSpace(uc + "\n\n" + system)
	}
	if ins := strings.TrimSpace(wsp.Settings().Instructions); ins != "" {
		system = strings.TrimSpace(system + "\n\n# Workspace Instructions\n" + ins)
	}
	if strings.TrimSpace(system) != "" {
		out = append(out, contextFiller{Label: "Sistem promptu", Role: "system", Tokens: conversation.EstimateText(system), Count: 1})
	}

	// Tool catalog (built-in + MCP) exactly as the agent receives it.
	if cat := wsp.Runtime.ToolCatalog(ctx, agentRow); len(cat) > 0 {
		out = append(out, contextFiller{Label: "Araçlar", Role: "tools", Tokens: estimateToolCatalog(cat), Count: len(cat)})
	}

	// Session goal block (dynamic suffix) — the persistent objective injected on
	// every turn.
	if gb := goalContextBlock(session.Goal); gb != "" {
		out = append(out, contextFiller{Label: "Hedef", Role: "goal", Tokens: conversation.EstimateText(gb), Count: 1})
	}

	// Session artifact context block (dynamic suffix).
	if ab := artifactsContextBlock(ctx, wsp.DB, session.ID); strings.TrimSpace(ab) != "" {
		out = append(out, contextFiller{Label: "Artifactlar", Role: "artifacts", Tokens: conversation.EstimateText(ab), Count: 1})
	}

	return out
}

// estimateToolCatalog approximates the token cost of a tool catalog as it is
// serialised into the request: name + description + JSON input schema per tool,
// plus a small per-tool framing overhead.
func estimateToolCatalog(defs []providers.ToolDef) int {
	total := 0
	for _, d := range defs {
		total += conversation.EstimateText(d.Name)
		total += conversation.EstimateText(d.Description)
		total += conversation.EstimateText(string(d.InputSchema))
		total += 8 // JSON framing per tool
	}
	return total
}

// buildAgentStats collects per-agent turn/token counts from a session's history.
func buildAgentStats(ctx context.Context, database *db.DB, session db.Session, history []db.Message) []sessionAgentStat {
	stats := map[string]*sessionAgentStat{}
	order := []string{}
	touch := func(agentID string) *sessionAgentStat {
		st := stats[agentID]
		if st == nil {
			st = &sessionAgentStat{AgentID: agentID, IsOwner: agentID == session.AgentID}
			stats[agentID] = st
			order = append(order, agentID)
		}
		return st
	}
	// Always surface the session's default agent, even with no assistant turns.
	touch(session.AgentID)

	for _, m := range history {
		if m.Role != "assistant" {
			continue
		}
		aid := m.AgentID
		if aid == "" {
			aid = session.AgentID
		}
		st := touch(aid)
		st.Turns++
		st.Tokens += conversation.EstimateText(m.Text)
	}

	out := make([]sessionAgentStat, 0, len(order))
	for _, aid := range order {
		st := stats[aid]
		if ag, err := database.GetAgent(ctx, aid); err == nil {
			st.Name = ag.Name
			st.Avatar = ag.Avatar
			st.Color = ag.Color
		} else {
			st.Name = "Silinmiş ajan"
			st.Disabled = true
		}
		out = append(out, *st)
	}
	// Owner first, then by turn count descending.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsOwner != out[j].IsOwner {
			return out[i].IsOwner
		}
		return out[i].Turns > out[j].Turns
	})
	return out
}
