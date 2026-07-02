package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/conversation"
	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/workspace"
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
	Cache         cachePreview     `json:"cache"`
	// CLIOverhead is set only for CLI-wrapper providers (claude-cli),
	// where TotalTokens above under-reports the real billed input — see the type doc.
	CLIOverhead *cliOverheadPreview `json:"cliOverhead,omitempty"`
}

// cliOverheadPreview surfaces, for CLI-wrapper providers (claude-cli),
// the gap between SwarmGo's own segment estimate (TotalTokens) and the real prompt
// the underlying CLI actually sends to the model. The CLI injects its OWN system
// prompt + tool schemas + MCP bridge that SwarmGo never composes or sees, so for
// these providers TotalTokens under-reports the billed input. MeasuredTokens is the
// real model input of ONE call (input + cacheRead + cacheWrite): claude-cli reports
// those counters CUMULATIVELY across its internal tool-loop round-trips within a
// single turn (cache_read especially — a turn's value can be several × the single-
// pass context because each internal call re-reads the same warm prefix), so the
// recorded turn total is divided by the round-trip count (result num_turns, carried
// as DebugEvent.Calls) to recover the per-call figure. It is 0 until the first turn
// has been sent.
type cliOverheadPreview struct {
	Note            string `json:"note"`
	EstimatedTokens int    `json:"estimatedTokens"` // SwarmGo segment sum (== TotalTokens)
	MeasuredTokens  int    `json:"measuredTokens"`  // real model input per call (turn total ÷ num_turns)
	OverheadTokens  int    `json:"overheadTokens"`  // max(0, measured - estimated)
	Calls           int    `json:"calls"`           // CLI internal round-trips behind measuredTokens (num_turns)
}

// cachePreview tells the UI which segments of the next request are served from a
// warm prompt cache vs sent fresh, so it can dim/colour the uncached parts and
// draw the cache boundary. The model is provider-specific: anthropic places a
// cache_control breakpoint on the static System prefix (Tools + System cached,
// Dynamic + messages fresh) only when ExtendedPromptCache is on; claude-cli has
// no breakpoint of its own, but --resume (ClaudeResume, warm) keeps the system +
// the first CachedMsgCount messages server-side, sending only the newest delta.
type cachePreview struct {
	Mode           string `json:"mode"` // "anthropic" | "claude-resume" | "none"
	Note           string `json:"note"` // one-line human explanation
	SystemCached   bool   `json:"systemCached"`
	DynamicCached  bool   `json:"dynamicCached"`
	ToolsCached    bool   `json:"toolsCached"`
	CachedMsgCount int    `json:"cachedMsgCount"` // leading messages served warm
}

// computeCachePreview derives the per-segment cache map from the provider, the
// relevant settings, and (for claude-cli resume) the session's recorded warm
// boundary. msgCount is the number of transcript turns the preview will show.
func (s *Server) computeCachePreview(provider string, session db.Session, msgCount int, hasSystem, hasDynamic, hasTools bool) cachePreview {
	set := s.settings.Get()
	switch provider {
	case "anthropic":
		if !set.ExtendedPromptCache {
			return cachePreview{Mode: "none", Note: "Genişletilmiş prompt-cache kapalı → cache breakpoint yok; tüm istek her tur taze gönderilir."}
		}
		// Breakpoint sits on the static System prefix (and the tool defs that
		// precede it). If there is no static System, it falls to Dynamic.
		c := cachePreview{Mode: "anthropic", ToolsCached: hasTools}
		if hasSystem {
			c.SystemCached = true
			c.Note = "Anthropic genişletilmiş cache: Araçlar + Sistem promptu cache'li (1s TTL); Dinamik bağlam + mesajlar cache dışı, her tur yeniden gönderilir."
		} else {
			c.DynamicCached = hasDynamic
			c.Note = "Anthropic genişletilmiş cache: statik Sistem promptu yok → breakpoint Dinamik bloğa düştü; mesajlar cache dışı."
		}
		return c
	case "openrouter":
		// SwarmGo sends an Anthropic-style cache_control breakpoint on the static
		// System prefix for OpenRouter. It is honoured by Anthropic/Gemini backends;
		// OpenAI/DeepSeek models cache implicitly anyway. Either way the Tools +
		// System prefix is the warm part; Dynamic + messages go fresh.
		c := cachePreview{Mode: "openrouter", ToolsCached: hasTools}
		// Two breakpoints: one on the static System prefix, one on the tail of the
		// transcript → in steady state the whole history except the newest message
		// is a cache hit (the first turn pays a cache write).
		if msgCount > 1 {
			c.CachedMsgCount = msgCount - 1
		}
		if hasSystem {
			c.SystemCached = true
			c.Note = "OpenRouter prompt-cache: Araçlar + Sistem + mesaj geçmişi cache breakpoint'li (Anthropic/Gemini'de cache'li, OpenAI/DeepSeek'te otomatik); yalnız en yeni mesaj taze (ilk turda cache yazılır)."
		} else {
			c.DynamicCached = hasDynamic
			c.Note = "OpenRouter prompt-cache: statik Sistem yok → breakpoint Dinamik + mesaj geçmişine düştü; yalnız en yeni mesaj taze."
		}
		return c
	case "claude-cli":
		warm := set.ClaudeResume && session.CLISessionID != "" && session.CLISentMsgCount > 0
		if !warm {
			return cachePreview{Mode: "none", Note: "claude-cli ilk/soğuk tur: bu tur cache yazılır. Sistem promptu artık statik (dinamik bağlam mesaj kuyruğuna taşındı) → Claude Code auto-cache'i sonraki turda sıcak prefix'i yeniden kullanır."}
		}
		cached := session.CLISentMsgCount
		if cached > msgCount {
			cached = msgCount
		}
		return cachePreview{
			Mode:           "claude-resume",
			SystemCached:   true,
			ToolsCached:    hasTools,
			DynamicCached:  false, // volatile (bellek+özet) — mesaj kuyruğunda, her tur taze
			CachedMsgCount: cached,
			Note:           "claude-cli --resume (sıcak): statik Sistem + ilk " + strconv.Itoa(cached) + " mesaj CLI'da server-side sıcak; dinamik bağlam mesaj kuyruğunda taze gider, cached prefix'i bozmaz.",
		}
	default:
		return cachePreview{Mode: "none", Note: "Bu sağlayıcı için SwarmGo cache breakpoint göndermez → istek her tur taze."}
	}
}

// previewMessage is one transcript turn as the model would receive it (role + the
// final text, with author labels / tool recap already folded in). Author/Self
// expose WHO authored the turn so the preview UI can show a per-message badge
// even in a single-agent session (where the text carries no "[Name]:" prefix):
// for an assistant turn Author is the authoring agent; for a user turn it is the
// agent the message was directed at (may be empty). Self marks the responding
// agent's own turns.
type previewMessage struct {
	Role   string `json:"role"`
	Text   string `json:"text"`
	Author string `json:"author,omitempty"`
	Self   bool   `json:"self,omitempty"`
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

	// Per-message authorship, aligned 1:1 with the user/assistant turns that go on
	// the wire (composeTurnRequest ships prep.Messages = these turns, same order).
	// Resolve agent ids to display names once, cached.
	names := map[string]string{}
	nameOf := func(id string) string {
		if id == "" {
			return ""
		}
		if n, ok := names[id]; ok {
			return n
		}
		name := id
		if a, err := wsp.DB.GetAgent(ctx, id); err == nil {
			if n := strings.TrimSpace(a.Name); n != "" {
				name = n
			}
		}
		names[id] = name
		return name
	}
	type authorInfo struct {
		name string
		self bool
	}
	authorsSeq := make([]authorInfo, 0, len(history))
	for _, m := range history {
		if m.Role != providers.RoleUser && m.Role != providers.RoleAssistant {
			continue
		}
		authorsSeq = append(authorsSeq, authorInfo{
			name: nameOf(m.AgentID),
			self: m.AgentID != "" && m.AgentID == agent.ID,
		})
	}

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
		// Full schema for eager tools so the preview can expand the exact payload.
		toolList = append(toolList, toolSummary{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema})
	}

	msgs := make([]previewMessage, 0, len(req.Messages))
	msgTok := 0
	for i, m := range req.Messages {
		pm := previewMessage{Role: m.Role, Text: m.Text}
		if i < len(authorsSeq) {
			pm.Author = authorsSeq[i].name
			pm.Self = authorsSeq[i].self
		}
		msgs = append(msgs, pm)
		msgTok += conversation.EstimateText(m.Text)
	}

	sysTok := conversation.EstimateText(req.System)
	dynTok := conversation.EstimateText(req.SystemDynamic)
	toolTok := estimateToolCatalog(defs)

	cache := s.computeCachePreview(
		agent.Provider, session, len(msgs),
		strings.TrimSpace(req.System) != "",
		strings.TrimSpace(req.SystemDynamic) != "",
		len(defs) > 0,
	)

	totalTok := sysTok + dynTok + msgTok + toolTok
	cliOver := computeCLIOverhead(ctx, wsp, agent.Provider, session.ID, totalTok)

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
		TotalTokens:   totalTok,
		Cache:         cache,
		CLIOverhead:   cliOver,
	})
}

// computeCLIOverhead derives the CLI-wrapper overhead preview for claude-cli
// agents (nil for native providers). The real billed input is measured
// from the session's recorded lifetime usage (input + cacheRead + cacheWrite,
// averaged per call) and compared against SwarmGo's own segment estimate, so the
// UI can warn that TotalTokens excludes the CLI's injected prompt + tools + MCP
// bridge. Returns a populated (overhead-0) preview with a "not measured yet" note
// when the session has no recorded calls.
func computeCLIOverhead(ctx context.Context, wsp *workspace.Workspace, provider, sessionID string, estimated int) *cliOverheadPreview {
	if provider != "claude-cli" {
		return nil
	}
	name := "claude-cli (Claude Code)"

	measured, calls := 0, 0
	// Most accurate next-turn projection: the REAL input of the most recent llm_call
	// from the debug journal (steady-state), rather than a cold+warm lifetime
	// average that under/over-states what the next turn will actually cost.
	if evs, derr := wsp.DB.ReadDebugEvents(ctx, sessionID, db.DebugLLMCall, 1); derr == nil && len(evs) > 0 {
		e := evs[len(evs)-1]
		// claude-cli bills in/out/cache CUMULATIVELY across its internal tool-loop
		// round-trips (e.Calls == result num_turns; verified: result cacheRead ==
		// Σ per-assistant cacheRead). Divide by the round-trip count to recover ONE
		// call's single-pass context. Native providers / single-call turns record
		// e.Calls 0 or 1 → no division.
		n := e.Calls
		if n < 1 {
			n = 1
		}
		measured = (e.In + e.CacheRead + e.CacheWrite) / n
		calls = n
	}
	// Fallback (debug journal off / no llm_call yet): lifetime average per call. The
	// session rollup tracks ProviderCalls (Σ num_turns), so dividing the cumulative
	// token totals by it recovers the per-call (single-pass) context just like the
	// debug path — not merely a per-SwarmGo-turn average. Sessions recorded before
	// the counter existed have ProviderCalls 0 → fall back to the turn count.
	if measured == 0 {
		if u, err := wsp.DB.GetSessionUsage(ctx, sessionID); err == nil && u.Calls > 0 {
			n := u.ProviderCalls
			if n < 1 {
				n = u.Calls
			}
			calls = n
			measured = (u.InputTokens + u.CacheReadTokens + u.CacheWriteTokens) / n
		}
	}

	over := measured - estimated
	if over < 0 {
		over = 0
	}

	note := name + " kendi sistem promptu + araç şemaları + MCP köprüsünü modele ekler; bu yük yukarıdaki segment tahminine (TotalTokens) DAHİL DEĞİL. 'Gerçek' = bir çağrının tek-geçiş girdisi (input+cacheRead+cacheWrite). Not: claude-cli in/out/cache'i tek tur içindeki iç tool-loop adımları (num_turns) boyunca KÜMÜLATİF raporlar → çağrı başına bağlamı bulmak için num_turns'e bölünür."
	if measured == 0 {
		note = name + " kendi sistem promptu + araçlarını ekler (segment tahmini bunu saymaz). Henüz tur gönderilmedi → gerçek girdi ilk turdan sonra ölçülür."
	}

	return &cliOverheadPreview{
		Note:            note,
		EstimatedTokens: estimated,
		MeasuredTokens:  measured,
		OverheadTokens:  over,
		Calls:           calls,
	}
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
