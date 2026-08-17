package api

import (
	"context"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
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
	// Context-reset lineage: the session this one continues (if born from a
	// /handoff) and the handoff artifact written into this session at reset.
	ParentSessionID   string `json:"parentSessionId,omitempty"`
	HandoffArtifactID string `json:"handoffArtifactId,omitempty"`
	// Coordination (M2). Role is LINEAGE ("worker" = spawned by a coordinator, or
	// ""); CoordinatorMode is the CAPABILITY (drives workers, gets the coordinator
	// prompt + spawn_worker/... tools). They are independent — a mid-level node of
	// a deep tree has both. CoordinatorSessionID is the back-link to the
	// coordinator above; RootCoordinatorSessionID/CoordinatorDepth address this
	// session inside its tree so the UI renders the hierarchy without walking
	// parent links one request at a time.
	Role                     string `json:"role,omitempty"`
	CoordinatorMode          bool   `json:"coordinatorMode,omitempty"`
	CoordinatorSessionID     string `json:"coordinatorSessionId,omitempty"`
	RootCoordinatorSessionID string `json:"rootCoordinatorSessionId,omitempty"`
	CoordinatorDepth         int    `json:"coordinatorDepth,omitempty"`
	// CoordinatorWorkflow is the selected coordinator recipe slug (M5), if any.
	CoordinatorWorkflow string `json:"coordinatorWorkflow,omitempty"`
	// CoordinatorStallHalted is true when the phantom-spawn stall guard has hard-halted
	// this coordinator's auto-turns (see coordination_stall.go). Drives the persistent
	// "durduruldu" badge + resume CTA in the coordination panel.
	CoordinatorStallHalted bool  `json:"coordinatorStallHalted,omitempty"`
	CreatedAt              int64 `json:"createdAt"`
	UpdatedAt              int64 `json:"updatedAt"`

	// Tags are the session's free-form labels (also drive tag-triggered automations).
	Tags []string `json:"tags,omitempty"`

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

	// Running describes an in-flight turn (background claude-cli/provider process)
	// for this session, or nil when idle. Lets the panel show + stop/restart it.
	Running *runningTurnDTO `json:"running,omitempty"`

	// WarmCLIProcess is true when a persistent-pool claude-cli process is kept warm
	// between turns for this session (only in persistent-pool mode). The panel offers
	// to recycle it so the next turn cold-restarts fresh.
	WarmCLIProcess bool `json:"warmCliProcess"`
}

// runningTurnDTO is the client view of an in-flight turn behind the Session Info
// panel's "running process" card.
type runningTurnDTO struct {
	RunID      string `json:"runId"`
	StartedAt  int64  `json:"startedAt"` // unix seconds
	Autonomous bool   `json:"autonomous"`
	Provider   string `json:"provider,omitempty"`

	// Liveness, from the same signal the queue watchdog judges on (see
	// Server.turnIdleFor). Elapsed time alone cannot answer "is this turn working
	// or wedged?" — a 40-minute turn emitting steps is fine, a 3-minute one that
	// has gone silent may not be.
	//
	// This is an ABSOLUTE timestamp, not a precomputed idle duration: the panel
	// refetches when the conversation changes, so a silent session would freeze a
	// duration at its last value — understating idleness exactly when it matters.
	// The client ticks it against the server clock instead. Limits are the two
	// bounds that will cut the turn, so the panel can show how close either is.
	LastActivityAt int64 `json:"lastActivityAt"` // unix seconds
	IdleLimitSec   int64 `json:"idleLimitSec"`
	HardLimitSec   int64 `json:"hardLimitSec"`
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
	case fillerRoleWorkerNote:
		return "Worker sonuçları"
	case fillerRoleAutoPrompt:
		return "Otomatik dürtme"
	default:
		return role
	}
}

// Synthetic filler "roles" that split the user bucket. They are NOT message
// roles — on the wire these messages carry role "user" (the model must replay
// them as user turns) and are told apart by db.Message.Origin. Without the split
// a coordinator session reads as "the user wrote 400 KB", when in truth the user
// typed a few lines and the rest is machine-injected worker output.
const (
	fillerRoleWorkerNote = "worker-note" // <task-notification> / <coordination-status> injections
	fillerRoleAutoPrompt = "auto-prompt" // schedule_wake resumes + scheduled routine prompts
)

// fillerRoleFor returns the bucket a message belongs to: its role, except that
// user turns are split by Origin so machine-injected prompts do not masquerade
// as the human's own input. Unknown origins fall back to the plain role, so a
// new Origin value shows up as "Kullanıcı" rather than an unlabelled bucket.
func fillerRoleFor(m db.Message) string {
	if m.Role != "user" {
		return m.Role
	}
	switch m.Origin {
	case "worker-note":
		return fillerRoleWorkerNote
	case "wake", "schedule":
		return fillerRoleAutoPrompt
	default:
		return m.Role
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
		ID:                       session.ID,
		Title:                    session.Title,
		Kind:                     session.Kind,
		State:                    session.State,
		AgentID:                  session.AgentID,
		MessageCount:             session.MessageCount,
		Unread:                   session.Unread,
		Tags:                     session.Tags,
		ParentSessionID:          session.ParentSessionID,
		HandoffArtifactID:        session.HandoffArtifactID,
		Role:                     session.Role,
		CoordinatorMode:          session.IsCoordinator(),
		CoordinatorSessionID:     session.CoordinatorSessionID,
		RootCoordinatorSessionID: session.RootCoordinator(),
		CoordinatorDepth:         session.CoordinatorDepth,
		CoordinatorWorkflow:      session.CoordinatorWorkflow,
		CoordinatorStallHalted:   wsp.Runtime.CoordinatorStallHalted(id),
		CreatedAt:                session.CreatedAt,
		UpdatedAt:                session.UpdatedAt,
		HasSummary:               session.Summary != "",
		SummaryMsgCount:          session.SummaryMsgCount,
		SummaryTokens:            conversation.EstimateText(session.Summary),
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
	// just the visible transcript. multiAgent changes the static prefix (a
	// history-annotation note is prepended), so resolve it the way the real turn
	// does; only the flag is used here, the labelled copy is the turn's business.
	_, multiAgent := s.labelMultiAgentHistory(ctx, wsp.DB, session.AgentID, history)
	extra := s.systemFillers(ctx, wsp, session, multiAgent)

	// Context fillers: summary + per-role message buckets PLUS the non-message
	// buckets (system/tools/artifacts), all sorted by token weight descending.
	resp.Fillers = append(buildFillers(session.Summary, pending), extra...)
	sort.SliceStable(resp.Fillers, func(i, j int) bool { return resp.Fillers[i].Tokens > resp.Fillers[j].Tokens })

	// Derive the "used" total from the SAME buckets the bar renders, so the header
	// figure equals the sum of the visible segments exactly (buildFillers already
	// folds in MsgOverhead per message, matching EstimateTokens). Previously the
	// header used EstimateTokens directly while the segments omitted the overhead,
	// so the bar never quite reached the reported percentage.
	ctxUsed := 0
	for _, f := range resp.Fillers {
		ctxUsed += f.Tokens
	}
	resp.ContextTokens = ctxUsed
	// Effective window = the compaction threshold the conversation manager ACTUALLY
	// uses, i.e. EffectiveBudget (the model-aware lift of the configured floor toward
	// window*fraction, capped by the ceiling) — NOT the raw MaxContextTokens floor.
	// The old code reported the floor, so for a big-window model (e.g. a claude-cli
	// opus agent with a 1M window and a 512K ceil) the meter compared usage against
	// the 63K floor and read 130%, promising a fold the engine had no intention of
	// running until ~512K. Mirror Prepare's budget math so the meter and the engine
	// agree. Unknown agent → fall back to the raw floor.
	cur := s.settings.Get()
	resp.ContextWindow = cur.MaxContextTokens
	if ag, aerr := wsp.DB.GetAgent(ctx, session.AgentID); aerr == nil {
		resp.ContextWindow = conversation.EffectiveBudget(ag.Provider, ag.Model, cur.MaxContextTokens, cur.ContextBudgetFraction, cur.ContextBudgetCeil)
	}

	// Participating agents: distinct agent per assistant turn (falling back to the
	// session's default agent), with the default agent always present.
	resp.Agents = buildAgentStats(ctx, wsp.DB, session, history)
	for i := range resp.Agents {
		if resp.Agents[i].AgentID == session.AgentID {
			resp.AgentName = resp.Agents[i].Name
		}
	}

	// Live background process: an in-flight turn (chat-streaming or autonomous) and,
	// in persistent-pool mode, a warm claude-cli process kept between turns.
	if info, ok := s.runs.sessionRunInfo(wsp.ID, id); ok {
		resp.Running = &runningTurnDTO{
			RunID:      info.RunID,
			StartedAt:  info.StartedAt.Unix(),
			Autonomous: info.Autonomous,
			Provider:   info.Provider,

			// Fall back to the turn's start: before the first event lands, the turn has
			// been silent since it began — which is precisely the setup-wedge case.
			LastActivityAt: info.StartedAt.Unix(),
			IdleLimitSec:   int64(s.inboxTurnIdleWatchdog().Seconds()),
			HardLimitSec:   int64(s.inboxTurnWatchdog().Seconds()),
		}
		if last, ok := s.hub.LastActivity(wsp.ID, id); ok && last.After(info.StartedAt) {
			resp.Running.LastActivityAt = last.Unix()
		}
	}
	resp.WarmCLIProcess = wsp.Runtime.HasWarmCLISession(id)

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
		role := fillerRoleFor(m)
		f := byRole[role]
		if f == nil {
			f = &contextFiller{Label: roleLabel(role), Role: role}
			byRole[role] = f
			order = append(order, role)
		}
		// Include the per-message framing cost so the sum of the role buckets matches
		// the aggregate EstimateTokens (which also adds MsgOverhead per message);
		// otherwise the usage bar's segments under-fill by 4×msgCount.
		f.Tokens += conversation.EstimateText(m.Text) + conversation.MsgOverhead
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
// appears as a chat message: the static prefix (persona + user profile +
// workspace instructions + the skills / load-on-demand-tools catalogs), the
// schemas actually shipped at turn start, and the session's artifact block.
// Without these the meter under-reports how full the model's context is.
//
// The prefix is NOT re-derived here: it comes from buildStaticPrefix, the very
// builder composeTurnRequest freezes into the prompt epoch. Hand-mirroring it
// (as this function used to) silently drifts — that is how the skills catalog,
// the lazy-tool catalog, the artifact guidance and the capability block all went
// uncounted. Deliberately buildStaticPrefix and NOT Runtime.EpochStaticSystem:
// the latter MUTATES (freezes + persists an epoch, emits a debug event) and this
// is a read-only panel; the live prefix is what the next adopt point ships anyway.
func (s *Server) systemFillers(ctx context.Context, wsp *workspace.Workspace, session db.Session, multiAgent bool) []contextFiller {
	agentRow, err := wsp.DB.GetAgent(ctx, session.AgentID)
	if err != nil {
		return nil
	}

	out := make([]contextFiller, 0, 5)
	system := s.buildStaticPrefix(ctx, wsp, session, agentRow, multiAgent)

	// Carve the two self-contained catalog blocks out of the prefix into their own
	// buckets. Both are costs the user can act on INDEPENDENTLY of the prompt text
	// (unassign a skill / drop a tool to a leaner visibility tier), which is exactly
	// what a single "system" bucket hides. Each block was appended verbatim, so it
	// is removed by exact match; if a block is not found the tokens stay inside the
	// system bucket rather than being counted twice.
	carve := func(block, label, role string, count int) {
		block = strings.TrimSpace(block)
		if block == "" {
			return
		}
		stripped, ok := stripBlock(system, block)
		if !ok {
			return
		}
		system = stripped
		out = append(out, contextFiller{Label: label, Role: role, Tokens: conversation.EstimateText(block), Count: count})
	}

	// Available Skills: slug + summary per advertised skill. Only the CATALOG lives
	// in the window — a skill BODY arrives via use_skill as a tool result, which is
	// within-turn only and never persists into the next turn, so it cannot show up
	// in this between-turns snapshot.
	skills := wsp.Runtime.SkillsCatalogBlockForAgent(agentRow)
	carve(skills, "Skill kataloğu", "skills", countCatalogSkills(skills))

	// Load-on-demand tool catalog: names (+ tiered descriptions) of the lazy tools.
	// Their SCHEMAS are not shipped — those arrive only after activate_tools — so
	// this block is their entire standing cost and belongs beside "Araçlar", not
	// inside it.
	carve(wsp.Runtime.LazyToolsCatalogBlock(ctx, agentRow), "Araç kataloğu (talep üzerine)", "lazy-tools",
		len(wsp.Runtime.LazyToolCatalog(ctx, agentRow)))

	if strings.TrimSpace(system) != "" {
		out = append(out, contextFiller{Label: "Sistem promptu", Role: "system", Tokens: conversation.EstimateText(system), Count: 1})
	}

	// Tool schemas SHIPPED at turn start — the eager tier only. ToolCatalog (every
	// allowed tool) was over-counting here by billing lazy tools for schemas that
	// never leave the server; those are already covered by the catalog block above.
	if cat := wsp.Runtime.ShippedToolCatalog(ctx, agentRow); len(cat) > 0 {
		out = append(out, contextFiller{Label: "Araçlar", Role: "tools", Tokens: estimateToolCatalog(cat), Count: len(cat)})
	}

	// Session artifact context block (dynamic suffix).
	if ab := artifactsContextBlock(ctx, wsp.DB, session.ID); strings.TrimSpace(ab) != "" {
		out = append(out, contextFiller{Label: "Artifactlar", Role: "artifacts", Tokens: conversation.EstimateText(ab), Count: 1})
	}

	return out
}

// contextOverheadTokens estimates the non-message context shipped every turn —
// static system prefix, skill/tool catalogs, eager tool schemas and the artifact
// block — by summing the SAME buckets the context meter renders (systemFillers).
// Fed into Prepare via conversation.WithContextOverhead so the budgeted fold gates
// on the true footprint (messages + this), not on messages alone: reusing
// systemFillers guarantees the meter and the fold engine agree on the overhead.
func (s *Server) contextOverheadTokens(ctx context.Context, wsp *workspace.Workspace, session db.Session, multiAgent bool) int {
	total := 0
	for _, f := range s.systemFillers(ctx, wsp, session, multiAgent) {
		total += f.Tokens
	}
	return total
}

// countCatalogSkills counts the entries in a rendered "# Available Skills" block.
// renderCatalog writes exactly one "- `slug`…" line per advertised skill, so the
// line prefix is the entry marker; surrounding prose never uses it.
func countCatalogSkills(block string) int {
	n := 0
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- `") {
			n++
		}
	}
	return n
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
