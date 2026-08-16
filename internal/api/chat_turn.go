package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
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
// toolRecap (may be "") is the <recent_tool_activity> block rendered from the
// recent assistant turns' Steps traces — injected here on the volatile side so
// the history messages themselves stay byte-stable for the rolling cache
// breakpoint (see recentToolActivityBlock).
func (s *Server) composeTurnRequest(ctx context.Context, wsp *workspace.Workspace, session db.Session, agentRow db.Agent, turnAgents []db.Agent, message string, prep conversation.Prepared, freshSession, multiAgent bool, toolRecap, feedbackRecap, lifecycleContext string) providers.Request {
	// The static prefix is served through the prompt epoch (frozen snapshot,
	// promptepoch.go): the builder below composes it from LIVE state, but between
	// adopt points the frozen session-start bytes ship instead, so mid-session
	// config drift (skill installs, settings edits, capability toggles, a new
	// participant) cannot bust the prompt cache. prep.Compacted marks a fold —
	// the history cache is busted anyway, so pending changes adopt for free.
	system, _ := wsp.Runtime.EpochStaticSystem(ctx, session.ID, agentRow, multiAgent, prep.Compacted,
		strings.TrimSpace(session.WorkingDir), func() string {
			return s.buildStaticPrefix(ctx, wsp, session, agentRow, multiAgent)
		})
	// Best-effort: ensure the session's repo is indexed in this workspace's isolated
	// store (guarded to run at most once per cwd per process; no-op without a cwd or
	// an enabled codebase-memory server). Deliberately OUTSIDE the static builder:
	// the builder must stay pure (a frozen turn still calls it for drift detection,
	// and side effects must run regardless of whether the snapshot serves).
	wsp.Runtime.EnsureCodebaseIndexed(ctx, session.WorkingDir)
	// Wall-clock awareness: a single date/time line so the agent always knows
	// "now" without a tool round-trip (there is no get_current_time tool). Volatile
	// by nature, so it leads the dynamic suffix and never invalidates the cache.
	dynamic := dateTimeContextBlock()
	// Session + workspace identity (the external agent project session_state parity): which session
	// and workspace the agent runs in, plus its permission mode so it knows what it
	// may do (read-only vs. auto) instead of attempting an edit that will be denied.
	// Volatile side because the mode can change mid-session (Shift+Tab).
	dynamic = strings.TrimSpace(dynamic + "\n\n" + sessionStateBlock(session, agentRow, wsp.ID, wsp.Name, wsp.DataDir))
	// Recap of recent turns' tool I/O ("what did you just run / what did it
	// return"). Volatile by design: injecting it here instead of into the history
	// messages keeps those messages byte-stable for the rolling cache breakpoint.
	if strings.TrimSpace(toolRecap) != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + toolRecap)
	}
	// The user's 👍/👎 on earlier replies — what landed and what did not. Volatile
	// for the same reason as the recap above: ratings change independently of the
	// turns they annotate, so they must never rewrite a cached history message.
	if strings.TrimSpace(feedbackRecap) != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + feedbackRecap)
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
	// Machine-environment marker (OS/arch/native shell) so the agent writes shell
	// commands in the right syntax without guessing. Shares ONE source with the
	// headless path (agent.autonomousSystemPrompt). Volatile side, never cached.
	dynamic = strings.TrimSpace(dynamic + "\n\n" + agent.EnvironmentContextBlock())
	// Shell-execution capability, single-sourced: when the gate is on + a shell
	// backs it + THIS agent's tool filter offers it, this advertises the registered
	// Bash/PowerShell tools; otherwise it states shell is disabled and gives the
	// dead-tool rule so a bare `PowerShell` call (which hits "not enabled in this
	// context") is not looped on. Volatile side: the gate can toggle mid-session.
	// agentRow is passed because the allowlist is per-agent, not per-workspace.
	if sh := wsp.Runtime.ShellToolsContextBlock(ctx, agentRow, false); sh != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + sh)
	}
	// Coordination scratchpad (M2/M3): a shared folder the coordinator and ALL its
	// workers can read/write, for durable cross-worker knowledge that shouldn't ride
	// in every prompt. Injected for a coordinator session and for its workers so
	// they converge on the SAME absolute path.
	if sb := coordinationScratchpadBlock(wsp, session); sb != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + sb)
	}
	// The rolling compaction summary is NOT folded into the volatile dynamic here
	// anymore (P2, _Docs/50): it is stable between two folds, so it travels in
	// req.Summary and cache-capable providers place it as a synthetic head message
	// INSIDE the cached prefix (a cache READ turn-to-turn) instead of re-shipping it
	// every turn. Providers without caching fold it back into the system prompt.
	summary := conversationSummaryBlock(prep.Summary)
	// Surface the session's existing artifacts so the agent revises them
	// (update_artifact by id) instead of creating duplicates.
	if ab := artifactsContextBlock(ctx, wsp.DB, session.ID); ab != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + ab)
	}
	// Failure lessons (hata→ders döngüsü): the newest distilled lessons from
	// past failed turns, workspace-wide, so known failure shapes are not
	// repeated. Volatile side (the set accrues over time), never cached.
	if lb := wsp.Runtime.LessonsContextBlock(ctx, agentRow.ID); lb != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + lb)
	}
	// Surface the active todo checklist so the agent keeps tracking it even after
	// the original todo_write message scrolls out of context / is compacted away.
	// On a fresh session it falls back to the durable progress file from a previous
	// session (persistent-progress / claude-progress convention), keyed to the cwd.
	if tb := todoContextBlock(ctx, wsp.DB, session.ID, wsp.Runtime.ProgressDir(session.ID), s.tun.ProgressResume()); tb != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + tb)
	}
	// Cross-session awareness: a short summary of the workspace's recent PAST
	// sessions (active/live sessions are NOT auto-sent — the agent lists them on
	// demand via list_sessions, which pages through ALL of them). Always on;
	// injected only on a session's first turn (its "start").
	if freshSession {
		if sb := sessionsContextBlock(ctx, wsp.DB, session.ID); sb != "" {
			dynamic = strings.TrimSpace(dynamic + "\n\n" + sb)
		}
	}

	// Context injected by SessionStart / UserPromptSubmit lifecycle hooks (e.g. a
	// caveman-style "respond terse" ruleset). Volatile per turn, so it rides the
	// dynamic suffix and never invalidates the cached static prefix. Leads the
	// suffix so a style directive is read before the rest of the volatile context.
	if lc := strings.TrimSpace(lifecycleContext); lc != "" {
		dynamic = strings.TrimSpace(lc + "\n\n" + dynamic)
	}

	// Prompt-epoch drift notice: the frozen snapshot is holding back a live
	// change. A compact diff of WHAT changed (persona/instructions/skills/tools)
	// on the VOLATILE side, so telling the agent about the drift never causes the
	// very cache bust the snapshot exists to prevent. Empty when in sync; falls
	// back to the generic one-liner when the diff could not be itemised.
	if note := wsp.Runtime.PromptEpochContextNote(session.ID, agentRow.ID); note != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + note)
	}

	return providers.Request{
		Model:         agentRow.Model,
		System:        system,
		SystemDynamic: dynamic,
		Summary:       summary,
		Messages:      prep.Messages,
	}
}

// compactionLeadStep builds the head-of-turn step that surfaces an auto-compaction
// fold on screen (same 🗜 framing as the manual /compact report). foldedMsgs comes
// from conversation.Prepared.FoldedMsgs. Shared by the interactive (chat_stream)
// and autonomous (wake/coordinator/worker via wake_turn) turn paths so a budgeted
// fold renders identically no matter which kind of turn triggered it — the fix for
// "compaction ran on a spawned/coordinator turn but I can't see it".
func compactionLeadStep(foldedMsgs int) agent.TurnStep {
	return agent.TurnStep{
		Kind: agent.StepText,
		Text: fmt.Sprintf("🗜 Bağlam otomatik sıkıştırıldı — %d mesaj kalıcı özete katlandı.", foldedMsgs),
	}
}

// consumeContextChangeLead returns a one-element lead trace (a context_change
// step) when the session's frozen static context drifted this episode, else
// nil. Consumes the one-shot so it fires once per drift episode. Lives here (not
// chat.go) because the db.Agent variable `agent` shadows the agent package name
// in that handler; returning the slice lets callers use type inference.
func consumeContextChangeLead(rt *agent.Runtime, sessionID, agentID string) []agent.TurnStep {
	if cc := rt.ConsumeContextChange(sessionID, agentID); cc != nil {
		return []agent.TurnStep{agent.ContextChangeStep(cc)}
	}
	return nil
}

// consumeCacheBreakLead returns a one-element trace (a cache_break step) when the
// turn that just ran lost the session's warm prompt-cache prefix for an
// attributable "something changed" reason, else nil. Consumes the one-shot so the
// break is carded once.
//
// Unlike the context_change lead this is consumed AFTER the completion, not
// before: the break is only knowable from the provider's usage reply. It is still
// prepended to the trace (the cold prefix was paid at the head of the turn) and
// deliberately NOT emitted as a live SSE step — emitting it late would paint it
// below the streamed answer, then jump to the top on reload.
func consumeCacheBreakLead(rt *agent.Runtime, sessionID string) []agent.TurnStep {
	if cb := rt.ConsumeCacheBreak(sessionID); cb != nil {
		if st := agent.CacheBreakStep(cb); st.Kind != "" {
			return []agent.TurnStep{st}
		}
	}
	return nil
}

// buildStaticPrefix composes the STATIC system prefix for one chat turn from
// LIVE state: persona, coordinator manual, agent-name note, multi-agent history
// note, user context, workspace instructions, artifact guidance, and the skills/
// lazy-tools/capability catalog blocks. It is the single source the prompt epoch
// freezes (EpochStaticSystem) — MUST stay pure (no side effects) and cheap: on a
// frozen turn its output is only compared against the snapshot for drift
// detection, not shipped.
func (s *Server) buildStaticPrefix(ctx context.Context, wsp *workspace.Workspace, session db.Session, agentRow db.Agent, multiAgent bool) string {
	system := buildSystemPrompt(agentRow)
	// Coordinator sessions (M2, _Docs/47) lead with the coordinator operating manual
	// so the agent drives workers, synthesizes their notifications itself, and runs
	// the research→synthesis→implementation→verification loop. Role is stable, so
	// this sits in the cached static prefix. Only a coordinator session gets it (and
	// only a coordinator session gets the spawn_worker/send_to_worker/... tools).
	// Includes a MID-LEVEL node of a coordinator tree (a worker with coordinator
	// mode on): it needs the manual for its own workers plus the upward-reporting
	// rules for its parent.
	if session.IsCoordinator() {
		// Registry prompt "coordinator" (workspace override → embedded default).
		manual := agent.WorkspacePrompt(wsp.DataDir, "coordinator")
		lead := coordinatorLeadBlock(manual, agentRow.CoordinatorPrompt,
			coordinatorRecipeBlock(wsp, session), coordinatorSubordinateBlock(session))
		system = strings.TrimSpace(lead + "\n\n" + system)
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
	// Terse ("caveman") reply style: workspace toggle + registry prompt "terse".
	// After the instructions so a workspace rule can be phrased to override it.
	if tb := wsp.Runtime.TerseModeBlock(); tb != "" {
		system = strings.TrimSpace(system + "\n\n" + tb)
	}
	// Always-on: deliverables (files/documents) surface as artifacts only when the
	// agent registers them deliberately (create_artifact / artifacts API) — there is
	// no automatic capture of written files.
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
	// Advertise optional external-tool capabilities (e.g. codebase-memory) present in
	// this workspace so the agent reaches for them, with the cwd-derived project id.
	// Presence is stable per workspace/session, so it rides the cached static prefix.
	// Shares ONE source with the headless path (agent.autonomousSystemPrompt).
	if cb := wsp.Runtime.CapabilityContext(ctx, strings.TrimSpace(session.WorkingDir)); cb != "" {
		system = strings.TrimSpace(system + "\n\n" + cb)
	}
	return system
}

// dateTimeContextBlock renders the current server-local date/time as a single
// system-prompt line. It replaces the removed get_current_time tool: the agent
// reads "now" straight from its context instead of spending a tool round-trip
// on it. Wording lives in agent.DateTimeContextBlock so the chat and headless
// paths share ONE source.
func dateTimeContextBlock() string {
	return agent.DateTimeContextBlock()
}

// sessionStateBlock renders the session + workspace identity as a compact
// <session_state> block (the external agent project parity), so the agent knows which session and
// workspace it runs in, the workspace's data dir, and its permission mode — the
// last so it does not attempt an edit/command that read-only mode will deny.
// permissionMode is per-agent ("read-only" | "ask" | "auto"; empty → auto).
func sessionStateBlock(session db.Session, agentRow db.Agent, wsID, wsName, wsPath string) string {
	mode := strings.TrimSpace(agentRow.PermissionMode)
	if mode == "" {
		mode = "auto"
	}
	var b strings.Builder
	b.WriteString("<session_state>\n")
	b.WriteString("sessionId: " + session.ID + "\n")
	b.WriteString("permissionMode: " + mode + " (read-only = no edits/commands · ask = confirm first · auto = full autonomy)\n")
	b.WriteString("workspace: " + wsID + " \"" + wsName + "\"\n")
	b.WriteString("workspacePath: " + wsPath + "\n")
	b.WriteString("</session_state>")
	return b.String()
}

// conversationSummaryBlock renders the rolling compaction summary for the dynamic
// system prompt, wrapped with a post-compaction recovery note. The turns that
// preceded this summary were folded into it (their verbatim text — code, tool
// output, file contents — is no longer in context), so the agent is told how to
// recover exact pre-compaction detail when it actually needs it rather than
// guessing from the digest: full-text search the past messages (conversation_search)
// or simply re-open the relevant files (the fs tools are unlocked). This is
// TionSwarm's equivalent of Claude Code's post-compaction transcript pointer,
// adapted to the recovery tools TionSwarm already ships — no readFileState tracker
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

// maybeSnippetTitle gives a freshly-enqueued chat its first visible title the
// instant the user message lands — a short snippet of the prompt — so the UI drops
// its "new chat" placeholder immediately instead of waiting on the LLM auto-title
// round-trip that runs during the first turn. It only names an as-yet-untitled
// session; the later LLM auto-title (maybeAutoTitle) refines this snippet into a
// cleaner title. Best-effort throughout: any failure just leaves the title
// untouched and never disturbs the enqueue/reply path.
func (s *Server) maybeSnippetTitle(ctx context.Context, wsp *workspace.Workspace, sessionID, message string) {
	if wsp == nil || wsp.DB == nil {
		return
	}
	session, err := wsp.DB.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	// Already named (manual, snippet from a prior message, or a completed LLM
	// title) — never clobber an existing title.
	if strings.TrimSpace(session.Title) != "" {
		return
	}
	snip := titleSnippet(message)
	if snip == "" {
		return
	}
	if err := wsp.DB.SetSessionTitle(ctx, sessionID, snip); err != nil {
		s.logger.Warn("snippet title failed", "session", sessionID, "error", err)
		return
	}
	emitSessionChange(wsp, sessionID, "title")
}

// titleSnippet reduces a user message to a short single-line title fragment: its
// first line only, inner whitespace collapsed to single spaces, trimmed to ~48
// runes with a trailing ellipsis when it was cut. Rune-safe so multi-byte (e.g.
// Turkish) characters are never split mid-character. Returns "" for a blank message.
func titleSnippet(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if msg == "" {
		return ""
	}
	const maxRunes = 48
	r := []rune(msg)
	if len(r) > maxRunes {
		return strings.TrimSpace(string(r[:maxRunes])) + "…"
	}
	return msg
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
