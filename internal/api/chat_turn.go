package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/swarmgo/internal/agent"
	"github.com/bilal-arikan/swarmgo/internal/conversation"
	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/memory"
	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/workspace"
)

// isFirstUntitledTurn reports whether this is the opening message of a fresh chat
// session that should auto-generate a title (only when auto-titling is enabled).
// Must be evaluated BEFORE the user message is appended.
func (s *Server) isFirstUntitledTurn(session db.Session) bool {
	return session.Kind == "chat" &&
		strings.TrimSpace(session.Title) == "" && session.MessageCount == 0 &&
		s.settings.Get().AutoTitleEnabled
}

// composeTurnRequest builds the provider request for one chat turn. The system
// prompt is split into a STATIC prefix (user profile + persona + workspace
// instructions) that stays stable across turns so prompt caching remains
// effective, and a DYNAMIC suffix (recalled memory + running summary + existing
// artifacts) that changes every turn and is kept outside the cached prefix.
//
// Shared by both the blocking (chat.go) and streaming (chat_stream.go) handlers.
func (s *Server) composeTurnRequest(ctx context.Context, wsp *workspace.Workspace, session db.Session, agentRow db.Agent, turnAgents []db.Agent, message string, prep conversation.Prepared, freshSession, multiAgent bool) providers.Request {
	system := buildSystemPrompt(agentRow)
	// Coordinator sessions (M2, _Docs/47) lead with the coordinator operating manual
	// so the agent drives workers, synthesizes their notifications itself, and runs
	// the research→synthesis→implementation→verification loop. Role is stable, so
	// this sits in the cached static prefix. Only a coordinator session gets it (and
	// only a coordinator session gets the spawn_worker/send_to_worker/... tools).
	if session.Role == "coordinator" {
		system = strings.TrimSpace(coordinatorSystemPrompt() + "\n\n" + system)
	}
	// Tell the agent its own name and how "@name" references work. The message is
	// addressed to THIS agent (chosen from the UI dropdown). An "@name" inside the
	// message is just a NAME REFERENCE — the user pointing at who they mean — NOT a
	// handoff or a command to invoke that agent, and NOT a file/skill/entity to look
	// up. You answer the message yourself; if it helps you may address or relay to
	// the referenced agent in your reply, but nothing is routed automatically.
	if n := strings.TrimSpace(agentRow.Name); n != "" {
		note := "You are the agent \"" + n + "\", and this message is addressed to you. It may contain \"@name\" references to other agents — treat each as a plain name reference (the user pointing at who they mean), not a handoff, a command to call that agent, or a file/skill to look up. Answer the message yourself; if useful you may address or relay to a referenced agent in your reply, but there is no automatic routing. If you hand work to another agent with spawn_session, its result runs in a SEPARATE session and does NOT come back to this conversation — do not promise to relay it here; instead tell the user it is running and where to find it (the activity feed)."
		system = strings.TrimSpace(note + "\n\n" + system)
	}
	// In a session shared by several agents, the history is annotated with each
	// assistant turn's author (see labelMultiAgentHistory). Tell the agent how to
	// read those "[Name]:" tags so it can answer "who said what" — and not copy
	// the tags into its own reply.
	if multiAgent {
		system = strings.TrimSpace(multiAgentHistoryNote + "\n\n" + system)
	}
	if uc := userContextBlock(s.settings.Get()); uc != "" {
		system = strings.TrimSpace(uc + "\n\n" + system)
	}
	if ins := strings.TrimSpace(wsp.Settings().Instructions); ins != "" {
		system = strings.TrimSpace(system + "\n\n# Workspace Instructions\n" + ins)
	}
	// Always-on: deliverables (files/documents) should surface as artifacts.
	system = strings.TrimSpace(system + "\n\n" + artifactDeliverableGuidance)
	// Advertise the skills THIS agent has selected (slug + summary only, in the
	// agent's chosen order). The full body is loaded lazily via use_skill. Part of
	// the cached static prefix since an agent's skill selection changes rarely.
	if sb := wsp.Runtime.SkillsCatalogBlockForAgent(agentRow); sb != "" {
		system = strings.TrimSpace(system + "\n\n" + sb)
	}
	// Advertise the agent's LAZY tools (self-management + MCP) as a lightweight
	// load-on-demand catalog; full schemas are pulled via activate_tools. Part of
	// the cached static prefix since the lazy set is stable per agent/workspace.
	if tb := wsp.Runtime.LazyToolsCatalogBlock(ctx, agentRow); tb != "" {
		system = strings.TrimSpace(system + "\n\n" + tb)
	}

	// Wall-clock awareness: a single date/time line so the agent always knows
	// "now" without a tool round-trip (there is no get_current_time tool). Volatile
	// by nature, so it leads the dynamic suffix and never invalidates the cache.
	dynamic := dateTimeContextBlock()
	// The session's persistent goal leads the dynamic context — it is the agent's
	// north star and should be the first thing it reads after the static persona.
	if gb := goalContextBlock(session.Goal, session.GoalDone); gb != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + gb)
	}
	// Tell the agent its working directory (cwd) + git branch, so it knows where
	// its file/shell tools operate. The session override wins; else the workspace
	// default. Kept in the dynamic suffix because the branch can change.
	cwd := strings.TrimSpace(session.WorkingDir)
	if cwd == "" {
		cwd = wsp.Runtime.WorkspaceDefaultDir()
	}
	if wb := workdirContextBlock(cwd); wb != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + wb)
	}
	// Coordination scratchpad (M2/M3): a shared folder the coordinator and ALL its
	// workers can read/write, for durable cross-worker knowledge that shouldn't ride
	// in every prompt. Injected for a coordinator session and for its workers so
	// they converge on the SAME absolute path.
	if sb := coordinationScratchpadBlock(wsp, session); sb != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + sb)
	}
	// Core memory (MemGPT-style): the agent's self-maintained named working-memory
	// blocks, re-injected verbatim every turn (edited via core_memory_replace/
	// append). Default blocks are persona (about itself) + human (about the user);
	// an agent may define more. Sits above recall because it is the agent's own
	// durable context, not a similarity hit; recall excludes core kinds, so it
	// never appears twice.
	if blocks, err := wsp.Runtime.Memory().ReadCoreBlocks(ctx, agentRow.ID); err == nil {
		if cb := coreMemoryBlock(blocks); cb != "" {
			dynamic = strings.TrimSpace(dynamic + "\n\n" + cb)
		}
	}
	if block := wsp.Runtime.Memory().ContextBlock(ctx, agentRow.ID, message, 5); block != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + block)
	}
	if sb := conversationSummaryBlock(prep.Summary); sb != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + sb)
	}
	// Surface the session's existing artifacts so the agent revises them
	// (update_artifact by id) instead of creating duplicates.
	if ab := artifactsContextBlock(ctx, wsp.DB, session.ID); ab != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + ab)
	}
	// Surface the active todo checklist so the agent keeps tracking it even after
	// the original todo_write message scrolls out of context / is compacted away.
	// On a fresh session it falls back to the durable progress file from a previous
	// session (persistent-progress / claude-progress convention), keyed to the cwd.
	if tb := todoContextBlock(ctx, wsp.DB, session.ID, wsp.Runtime.ProgressDir(session.ID), s.tun.ProgressResume()); tb != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + tb)
	}
	// Cross-session awareness: a short summary of the workspace's active + recent
	// sessions. Configured PER WORKSPACE; injected every turn or only on a
	// session's first turn (its "start") depending on the toggle.
	if sc := wsp.Settings(); sc.SessionContextEnabled && (sc.SessionContextEveryTurn || freshSession) {
		recent := sc.SessionContextRecentCount
		if recent <= 0 {
			recent = 5
		}
		if sb := sessionsContextBlock(ctx, wsp.DB, session.ID, recent); sb != "" {
			dynamic = strings.TrimSpace(dynamic + "\n\n" + sb)
		}
	}

	// Memory-pressure signal (MemGPT-style paging hint): when the context budget is
	// nearly full, warn the agent — BEFORE the next silent compaction — to persist
	// anything that must survive. Volatile (recomputed each turn) so it leads the
	// dynamic suffix without disturbing the cached static prefix. Threshold 0 = off.
	if warn := s.tun.MemoryPressureWarn(); warn > 0 && prep.Pressure >= warn {
		note := fmt.Sprintf(
			"⚠️ Context is %d%% full and older turns will soon be compacted into a summary. "+
				"If any fact, decision, or detail here must survive, persist it now "+
				"(memory_add for long-term recall, or core_memory_replace/append for working memory).",
			int(prep.Pressure*100))
		dynamic = strings.TrimSpace(note + "\n\n" + dynamic)
	}

	return providers.Request{
		Model:         agentRow.Model,
		System:        system,
		SystemDynamic: dynamic,
		Messages:      prep.Messages,
	}
}

// dateTimeContextBlock renders the current server-local date/time as a single
// system-prompt line, e.g. "Current date and time: Monday, 2026-06-22 15:31:08
// (+03:00)". It replaces the removed get_current_time tool: the agent reads
// "now" straight from its context instead of spending a tool round-trip on it.
// Includes seconds and is labelled as the turn-start instant so an agent doing a
// timing task uses this exact value as a baseline instead of fabricating one — it
// is captured once per turn and does NOT advance mid-turn.
func dateTimeContextBlock() string {
	return "Current date and time (captured at the start of this turn; seconds-precise, does not tick mid-turn): " +
		time.Now().Format("Monday, 2006-01-02 15:04:05 (-07:00)")
}

// conversationSummaryBlock renders the rolling compaction summary for the dynamic
// system prompt, wrapped with a post-compaction recovery note. The turns that
// preceded this summary were folded into it (their verbatim text — code, tool
// output, file contents — is no longer in context), so the agent is told how to
// recover exact pre-compaction detail when it actually needs it rather than
// guessing from the digest: full-text search the past messages (conversation_search)
// or simply re-open the relevant files (the fs tools are unlocked). This is
// SwarmGo's equivalent of Claude Code's post-compaction transcript pointer,
// adapted to the recovery tools SwarmGo already ships — no readFileState tracker
// is needed because file contents are never cross-turn context here anyway, so a
// re-read on demand fully restores them. Returns "" for an empty summary so the
// caller can append it unconditionally.
func conversationSummaryBlock(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	const recovery = "\n\nNote: the turns before this summary were compacted and their full text is no longer in context. " +
		"If you need exact pre-compaction detail — a code snippet, an error message, file contents, or a specific decision — " +
		"recover it instead of guessing: use conversation_search to find what was said, or re-open the relevant files with your file tools."
	return "## Conversation summary so far\n" + summary + recovery
}

// coreMemoryBlock formats the agent's named core-memory blocks for prompt
// injection, emitting only the non-empty ones under a shared header. Each block
// shows its description (as guidance) and a usage counter against its limit, so
// the agent sees how full it is and which label to target. Returns "" when every
// block is blank so the caller can append it unconditionally.
func coreMemoryBlock(blocks []memory.BlockView) string {
	var body strings.Builder
	for _, blk := range blocks {
		content := strings.TrimSpace(blk.Content)
		if content == "" {
			continue
		}
		fmt.Fprintf(&body, "\n### %s (%d/%d chars)", blk.Label, len([]rune(content)), blk.CharLimit)
		if d := strings.TrimSpace(blk.Description); d != "" {
			fmt.Fprintf(&body, " — %s", d)
		}
		body.WriteString("\n")
		body.WriteString(content)
	}
	if body.Len() == 0 {
		return ""
	}
	return "## Core memory (you maintain this; edit with core_memory_replace/append using the block label)" + body.String()
}

// adoptMentionedAgent makes the first @mentioned agent the session's default
// (main) agent — but only on a brand-new session (no prior messages), so opening
// a chat by mentioning @X pins the whole thread to X. mentionIDs are the raw
// requested agent ids; agents is the resolved, ordered list (agents[0] is the
// first valid mention). Returns the possibly-updated session.
func (s *Server) adoptMentionedAgent(ctx context.Context, database *db.DB, session db.Session, mentionIDs []string, agents []db.Agent) db.Session {
	if session.MessageCount != 0 || len(mentionIDs) == 0 || len(agents) == 0 {
		return session
	}
	want := agents[0].ID
	if want == "" || want == session.AgentID {
		return session
	}
	if err := database.SetSessionAgent(ctx, session.ID, want); err != nil {
		s.logger.Warn("adopt mentioned agent failed", "session", session.ID, "error", err)
		return session
	}
	session.AgentID = want
	return session
}

// marshalSteps serialises the activity trace for persistence. Best-effort: an
// encode error must never fail the reply, so it falls back to an empty trace.
func marshalSteps(steps []agent.TurnStep) string {
	if len(steps) == 0 {
		return "[]"
	}
	if b, err := json.Marshal(steps); err == nil {
		return string(b)
	}
	return "[]"
}

// maybeAutoTitle generates and persists a session title from the opening message
// when firstTurn is set. Best-effort: a failure never breaks the reply. Returns
// the new title, or "" when none was generated.
func (s *Server) maybeAutoTitle(ctx context.Context, wsp *workspace.Workspace, firstTurn bool, agentID, sessionID, message string) string {
	if !firstTurn {
		return ""
	}
	title, err := wsp.Runtime.TitleFor(ctx, agentID, message)
	if err != nil {
		s.logger.Warn("auto title failed", "session", sessionID, "error", err)
		return ""
	}
	if title == "" {
		return ""
	}
	if err := wsp.DB.SetSessionTitle(ctx, sessionID, title); err != nil {
		s.logger.Warn("set session title failed", "session", sessionID, "error", err)
		return ""
	}
	return title
}
