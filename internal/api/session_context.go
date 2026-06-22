package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/conversation"
	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// sessionContextPreview is the EXACT next-turn context a session's agent would be
// sent: the composed system prompt + dynamic suffix, the message transcript that
// would go on the wire, and the shipped tool catalog — each with a token estimate.
// A debug view mirroring the agent context preview, but for a live session (real
// history, author labels, tool recap, running summary, memory, goal, cwd).
type sessionContextPreview struct {
	AgentName     string           `json:"agentName"`
	MultiAgent    bool             `json:"multiAgent"`
	System        string           `json:"system"`
	SystemTokens  int              `json:"systemTokens"`
	Dynamic       string           `json:"dynamic"`
	DynamicTokens int              `json:"dynamicTokens"`
	Messages      []previewMessage `json:"messages"`
	MessageTokens int              `json:"messageTokens"`
	Tools         []toolSummary    `json:"tools"`
	ToolTokens    int              `json:"toolTokens"`
	TotalTokens   int              `json:"totalTokens"`
}

// previewMessage is one transcript turn as the model would receive it (role + the
// final text, with author labels / tool recap already folded in).
type previewMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// handleSessionContextPreview assembles and returns the next-turn context for a
// session WITHOUT side effects: it never compacts/persists a summary and never
// calls a provider (it builds the Prepared bundle by hand from the current history
// + the session's existing summary). An optional ?message= is appended as a
// pending user turn so the preview shows "what the agent would see if I send this".
func (s *Server) handleSessionContextPreview(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()

	session, err := wsp.DB.GetSession(ctx, r.PathValue("id"))
	if writeDBError(w, err, "session not found") {
		return
	}
	agent, err := wsp.DB.GetAgent(ctx, session.AgentID)
	if writeDBError(w, err, "agent not found") {
		return
	}

	history, err := wsp.DB.ListMessages(ctx, session.ID)
	if writeDBError(w, err, "") {
		return
	}
	// Optional sample "next" user message → preview the context for that message.
	sample := strings.TrimSpace(r.URL.Query().Get("message"))
	if sample != "" {
		history = append(history, db.Message{
			SessionID: session.ID,
			Role:      providers.RoleUser,
			AgentID:   agent.ID,
			Text:      sample,
		})
	}

	// Same history shaping the real turn does — author labels + recent tool recap.
	history, multiAgent := s.labelMultiAgentHistory(ctx, wsp.DB, agent.ID, history)
	history = appendRecentToolSummaries(history)

	// Build the Prepared bundle by hand (no compaction, no provider call): the
	// transcript as-is plus the session's existing rolling summary.
	prep := conversation.Prepared{
		Summary:  session.Summary,
		Messages: historyToPreviewMessages(history),
	}
	req := s.composeTurnRequest(ctx, wsp, session, agent, []db.Agent{agent}, sample, prep, false, multiAgent)

	// Shipped (eager) tool catalog — schemas actually sent each turn.
	defs := wsp.Runtime.ShippedToolCatalog(ctx, agent)
	toolList := make([]toolSummary, 0, len(defs))
	for _, d := range defs {
		toolList = append(toolList, toolSummary{Name: d.Name, Description: d.Description})
	}

	msgs := make([]previewMessage, 0, len(req.Messages))
	msgTok := 0
	for _, m := range req.Messages {
		msgs = append(msgs, previewMessage{Role: m.Role, Text: m.Text})
		msgTok += conversation.EstimateText(m.Text)
	}

	sysTok := conversation.EstimateText(req.System)
	dynTok := conversation.EstimateText(req.SystemDynamic)
	toolTok := estimateToolCatalog(defs)

	writeJSON(w, http.StatusOK, sessionContextPreview{
		AgentName:     agent.Name,
		MultiAgent:    multiAgent,
		System:        req.System,
		SystemTokens:  sysTok,
		Dynamic:       req.SystemDynamic,
		DynamicTokens: dynTok,
		Messages:      msgs,
		MessageTokens: msgTok,
		Tools:         toolList,
		ToolTokens:    toolTok,
		TotalTokens:   sysTok + dynTok + msgTok + toolTok,
	})
}

// historyToPreviewMessages maps stored user/assistant turns to provider messages
// (text only), mirroring the conversation package's own mapping for the preview —
// without importing its unexported helper.
func historyToPreviewMessages(history []db.Message) []providers.Message {
	out := make([]providers.Message, 0, len(history))
	for _, m := range history {
		if m.Role == providers.RoleUser || m.Role == providers.RoleAssistant {
			out = append(out, providers.Message{Role: m.Role, Text: m.Text})
		}
	}
	return out
}
