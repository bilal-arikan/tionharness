package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/awareness"
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
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
// instructions + the session briefing) that stays stable across turns so prompt
// caching remains effective, and a DYNAMIC suffix (clock, identity, recap,
// checklist, artifacts, pulse — composed and metered by the awareness layer,
// _Docs/94) that changes every turn and is kept outside the cached prefix.
//
// Shared by both the blocking (chat.go) and streaming (chat_stream.go) handlers.
// toolRecap (may be "") is the <recent_tool_activity> block rendered from the
// recent assistant turns' Steps traces — injected here on the volatile side so
// the history messages themselves stay byte-stable for the rolling cache
// breakpoint (see recentToolActivityBlock).
func (s *Server) composeTurnRequest(ctx context.Context, wsp *workspace.Workspace, session db.Session, agentRow db.Agent, turnAgents []db.Agent, message string, prep conversation.Prepared, freshSession, multiAgent bool, toolRecap, feedbackRecap, lifecycleContext string) providers.Request {
	// The session briefing is frozen into the static prefix (_Docs/94). It is
	// recomposed exactly where the prompt epoch adopts live state anyway: the
	// first turn and a compaction fold. Elsewhere it ships byte-stable, so the
	// drift check never sees it as a change.
	if freshSession || prep.Compacted {
		wsp.Runtime.InvalidateBrief(session.ID)
	}
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
	// Best-effort: ensure the session's repo is indexed by codebase-memory and by
	// zvec-grep (each guarded to run at most once per repo per process; no-op
	// without a cwd or the matching enabled server). Deliberately OUTSIDE the static
	// builder: the builder must stay pure (a frozen turn still calls it for drift
	// detection, and side effects must run regardless of whether the snapshot serves).
	wsp.Runtime.EnsureCodebaseIndexed(ctx, session.WorkingDir)
	wsp.Runtime.EnsureZvecGrepIndexed(ctx, session.WorkingDir)

	// The VOLATILE suffix is a list of sections the awareness layer fits to the
	// turn budget and meters (_Docs/94): pinned sections (identity, clock, the
	// environment, hook context) never degrade; the rest carry a pointer form
	// and degrade in priority order when a turn would otherwise run over.
	pin := func(key, text string) awareness.Section {
		return awareness.Section{Key: key, Text: text, Priority: awareness.PriorityPinned, Volatile: true}
	}
	var lead []awareness.Section
	// Context injected by SessionStart / UserPromptSubmit lifecycle hooks (e.g. a
	// caveman-style "respond terse" ruleset). Leads the suffix so a style
	// directive is read before the rest of the volatile context.
	if lc := strings.TrimSpace(lifecycleContext); lc != "" {
		lead = append(lead, pin("hooks", lc))
	}
	// Wall-clock awareness: a single date/time line so the agent always knows
	// "now" without a tool round-trip (there is no get_current_time tool).
	lead = append(lead, pin("clock", dateTimeContextBlock()))
	// Session + workspace identity (session_state parity): which session and
	// workspace the agent runs in, plus its permission mode so it knows what it
	// may do (read-only vs. auto) instead of attempting an edit that will be
	// denied. Volatile because the mode can change mid-session (Shift+Tab).
	lead = append(lead, pin("session", sessionStateBlock(session, agentRow, wsp.ID, wsp.Name, wsp.DataDir)))
	// Recap of recent turns' tool I/O ("what did you just run / what did it
	// return"). Injected here instead of into the history messages so those stay
	// byte-stable for the rolling cache breakpoint. The single most droppable
	// section: the transcript still holds every line of it.
	if strings.TrimSpace(toolRecap) != "" {
		lead = append(lead, awareness.Section{Key: "tool-recap", Text: toolRecap, Priority: 7, Volatile: true,
			Pointer: "<recent_tool_activity>omitted to fit the context budget; the transcript above holds every call and result</recent_tool_activity>"})
	}
	// The user's 👍/👎 on earlier replies — what landed and what did not.
	if strings.TrimSpace(feedbackRecap) != "" {
		lead = append(lead, awareness.Section{Key: "feedback", Text: feedbackRecap, Priority: 6, Volatile: true})
	}
	// Tell the agent its working directory (cwd) + git branch, so it knows where
	// its file/shell tools operate. The session override wins; else the workspace
	// default. Volatile because the branch can change.
	cwd := strings.TrimSpace(session.WorkingDir)
	if cwd == "" {
		cwd = wsp.Runtime.WorkspaceDefaultDir()
	}
	if wb := workdirContextBlock(cwd); wb != "" {
		lead = append(lead, pin("workdir", wb))
	}
	// Machine-environment marker (OS/arch/native shell) so the agent writes shell
	// commands in the right syntax without guessing. Shares ONE source with the
	// headless path (agent.autonomousSystemPrompt).
	lead = append(lead, pin("environment", agent.EnvironmentContextBlock()))
	// Shell-execution capability, single-sourced: when the gate is on + a shell
	// backs it + THIS agent's tool filter offers it, this advertises the registered
	// Bash/PowerShell tools; otherwise it states shell is disabled and gives the
	// dead-tool rule so a bare `PowerShell` call is not looped on.
	if sh := wsp.Runtime.ShellToolsContextBlock(ctx, agentRow, false); sh != "" {
		lead = append(lead, pin("shell", sh))
	}
	// Coordination scratchpad (M2/M3): a shared folder the coordinator and ALL its
	// workers can read/write, injected for both so they converge on the SAME path.
	if sb := coordinationScratchpadBlock(wsp, session); sb != "" {
		lead = append(lead, pin("scratchpad", sb))
	}
	// Coordinator situation snapshot: live fleet state, the spawnable agent roster
	// with each agent's write capability, and the board — authoritative and
	// refreshed every turn, so the coordinator never re-reads it with tool calls.
	if cb := wsp.Runtime.CoordinatorSituationBlock(ctx, session); cb != "" {
		lead = append(lead, awareness.Section{Key: "situation", Text: cb, Priority: 1, Volatile: true,
			Pointer: "## Coordinator situation\nThe fleet/board snapshot was omitted to fit the context budget; list_workers and get_view board carry it."})
	}
	// Prompt-epoch drift notice: the frozen snapshot is holding back a live
	// change. A compact diff of WHAT changed on the volatile side, so telling the
	// agent about the drift never causes the very cache bust the snapshot exists
	// to prevent. Empty when in sync. Trails the awareness sections so it is the
	// last thing before the meter.
	var trail []awareness.Section
	if note := wsp.Runtime.PromptEpochContextNote(session.ID, agentRow.ID); note != "" {
		trail = append(trail, pin("epoch", note))
	}
	// The awareness layer appends the live checklist (or the resumed progress
	// file), the session's artifacts and the de-duplicated workspace pulse, fits
	// the whole suffix to the turn budget and closes with the meter.
	dynamic := wsp.Runtime.TurnBlock(ctx, session, agentRow, freshSession, lead, trail...).Text

	// The rolling compaction summary is NOT folded into the volatile dynamic here
	// anymore (P2, _Docs/50): it is stable between two folds, so it travels in
	// req.Summary and cache-capable providers place it as a synthetic head message
	// INSIDE the cached prefix (a cache READ turn-to-turn) instead of re-shipping it
	// every turn. Providers without caching fold it back into the system prompt.
	summary := conversationSummaryBlock(prep.Summary)

	return providers.Request{
		Model:         agentRow.Model,
		System:        system,
		SystemDynamic: dynamic,
		Summary:       summary,
		Messages:      prep.Messages,
	}
}

// compactionLeadStep builds the head-of-turn step that surfaces an auto-compaction
// fold on screen (same 🗜 framing as the manual /compact report). fold comes from
// conversation.Prepared.Fold. Shared by the interactive (chat_stream) and
// autonomous (wake/coordinator/worker via wake_turn) turn paths so a budgeted fold
// renders identically no matter which kind of turn triggered it — the fix for
// "compaction ran on a spawned/coordinator turn but I can't see it".
//
// The step carries the fold's figures as structured fields so the UI can render a
// typed compaction card; Text keeps the original human-readable line so a client
// that does not know the compaction kind (and every already-persisted session)
// still shows something sensible.
func compactionLeadStep(fold conversation.Compaction, provider providers.Provider) agent.TurnStep {
	step := agent.TurnStep{
		Kind:         agent.StepCompaction,
		Text:         fmt.Sprintf("🗜 Bağlam otomatik sıkıştırıldı — %d mesaj kalıcı özete katlandı.", fold.FoldedMsgs),
		FoldedMsgs:   fold.FoldedMsgs,
		BeforeTokens: fold.BeforeTokens,
		AfterTokens:  fold.AfterTokens,
		Trigger:      fold.Trigger,
		Source:       "tionharness",
	}
	if provider != nil {
		if _, ok := providers.AsCLI(provider); ok {
			step.Provider = provider.Name()
			// Neither supported CLI currently exposes a documented, reliable native
			// compaction event. A TionHarness fold therefore invalidates the warm CLI
			// transcript and the request starts fresh from summary + recent tail.
			step.SessionAction = "restart-summary"
		}
	}
	return step
}

// foldFailedLeadStep announces a fold that was needed but could not be produced:
// the turn is over budget and runs with its full, uncompacted history.
//
// It reuses StepCompaction rather than StepError on purpose — this is a
// compaction event, and StepError reads as "the turn failed", which is exactly
// the impression to avoid: the turn continues normally. FoldedMsgs 0 is what
// distinguishes it from a successful fold for clients that only read the
// structured fields.
func foldFailedLeadStep(reason string) agent.TurnStep {
	text := "⚠ Bağlam sıkıştırılamadı — bu tur tam geçmişle çalışıyor."
	if reason != "" {
		text += " (" + reason + ")"
	}
	return agent.TurnStep{
		Kind:    agent.StepCompaction,
		Text:    text,
		Trigger: conversation.TriggerAuto,
		Source:  "tionharness",
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
	if cb := wsp.Runtime.CapabilityContext(ctx, agentRow, strings.TrimSpace(session.WorkingDir)); cb != "" {
		system = strings.TrimSpace(system + "\n\n" + cb)
	}
	// Session briefing (_Docs/94): what the workspace looks like, the notes that
	// reach this session, recently finished work, open loops. Composed on the
	// session's first turn and frozen with the rest of the prefix; recomposed on a
	// compaction fold (composeTurnRequest invalidates it there). Shares ONE source
	// with the headless path (agent.autonomousSystemPrompt).
	if bb := wsp.Runtime.BriefBlock(ctx, session, agentRow); bb != "" {
		system = strings.TrimSpace(system + "\n\n" + bb)
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
// TionHarness's equivalent of Claude Code's post-compaction transcript pointer,
// adapted to the recovery tools TionHarness already ships — no readFileState tracker
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
// session; the later LLM auto-title candidate refines this snippet into a
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

// autoTitleCandidate generates a title from the opening message without mutating
// session state. The provider call intentionally runs outside the session
// generation gate; the caller fences the later persist and terminal events.
func (s *Server) autoTitleCandidate(ctx context.Context, wsp *workspace.Workspace, firstTurn bool, agentID, sessionID, message string) string {
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
	return title
}

// persistAutoTitle commits a previously generated title. Callers hold the
// session generation gate so a stale run cannot rename the session.
func (s *Server) persistAutoTitle(ctx context.Context, wsp *workspace.Workspace, sessionID, title string) string {
	if title == "" {
		return ""
	}
	if err := wsp.DB.SetSessionTitle(ctx, sessionID, title); err != nil {
		s.logger.Warn("set session title failed", "session", sessionID, "error", err)
		return ""
	}
	return title
}
