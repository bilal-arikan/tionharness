package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// handleChatStream runs one chat turn over Server-Sent Events, emitting each
// activity step the moment it occurs. A turn may involve several agents (via
// "@mention" routing); each answers in order, seeing the prior replies. Events:
//
//	meta  → { userMessage, contextTokens }      (once)
//	agent → { agentId, index }                  (before each agent's steps)
//	step  → a single TurnStep                   (zero or more, current agent)
//	reply → { replyMessage }                    (after each agent finishes)
//	done  → { sessionTitle }                    (terminal, success)
//	error → { error }                           (terminal, failure)
//
// runChatTurn runs one chat turn to completion, publishing every UI event to the
// session hub (the authoritative render path for all windows). write is an
// optional legacy SSE sink: the direct /chat/stream handler passes one, the queue
// worker passes nil (the hub carries the UI either way). clientGone signals the
// submitting client navigated away so an interactive ask_user unblocks instead of
// pinning the detached turn; pass a never-done context for a server-driven queued
// turn that has no single owning client.
func (s *Server) runChatTurn(clientGone context.Context, wsp *workspace.Workspace, req chatReq, write func(event string, data any)) {
	// Register this turn so it can be stopped or steered while running.
	runID := uuid.NewString()
	// clientMsgID keys this turn's terminal hub events (turn_done / turn_error) so a
	// queue observer — the legacy /chat/stream + /chat handlers that now enqueue and
	// relay the hub — can tell its own turn's completion from another queued turn's.
	// Empty for the frontend's own /messages turns (harmless: it reads sessionTitle).
	clientMsgID := req.ClientMsgID
	// Detach the turn from the client connection so a page refresh/navigation never
	// cancels generation; only an explicit "stop" control cancels it.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(clientGone))
	defer cancel()
	// Inactivity watchdog for the interactive turn: a provider whose stream dies
	// silently (half-open socket, CLI subprocess that never exits) otherwise pins
	// this session forever and its queued inbox messages are never delivered. The
	// chat wrapper installs the heartbeat interval too, so a long step-less
	// operation is kept alive while a truly stalled stream is still reclaimed.
	ctx, stopTimeout := agent.WithChatActivityTimeout(runCtx, s.tun.ChatTurnTimeout(), s.chatTurnIdle())
	defer stopTimeout()
	run := s.runs.register(runID, req.SessionID, wsp.ID, cancel)
	defer s.runs.unregister(runID)
	// steer_undelivered fallback (Doc 59): a claude-cli steer is delivered at the
	// next tool boundary via the Interaction MCP permission tool's additionalContext.
	// If the turn ends (any path) with a steer message that never reached a tool
	// boundary (a tool-less, text-only turn), enqueue it as the next message so the
	// user's intent is not lost. takeSteer returns "" on a normal turn (or once the
	// message was already delivered), so this is a no-op in the common case.
	defer func() {
		if msg := run.takeSteer(); msg != "" {
			// To the FRONT of the queue: the user typed this steer to redirect THIS
			// turn, before anything they queued afterwards, so appending it to the tail
			// would deliver their oldest intent last.
			s.enqueueMessageFront(wsp.ID, chatReq{SessionID: req.SessionID, Message: msg, AgentIDs: req.AgentIDs})
		}
	}()
	ctx = agent.WithSteer(ctx, run.steer)
	// Point any CLI subprocess (claude-cli, ...) at the in-process Interaction MCP
	// endpoint for this turn, carrying the per-run token. No-op when unknown.
	if url := s.interactionURL(); url != "" {
		coreNames, extNames := splitInteractionTiers(interactionAdvertisedNames(s.tun, false), nil, nil)
		ctx = tools.WithInteractionEndpoint(ctx, url, run.token, coreNames, extNames)
	}
	// Install the legacy SSE writer only for the direct HTTP path; the queue worker
	// passes nil and run.emit becomes a no-op (the hub carries all UI).
	if write != nil {
		run.setWrite(write)
		defer run.clearWrite()
	}
	// fail reports a pre-flight failure (before the turn commits) to the legacy SSE
	// sink AND the hub, so every window clears its "thinking" state and shows why.
	fail := func(reason, detail string) {
		s.logger.Error("chat turn preflight failed", "session", req.SessionID, "reason", reason, "detail", detail)
		payload := map[string]any{"error": detail, "reason": reason, "clientMsgId": clientMsgID}
		run.emit("error", payload)
		s.publishHub(wsp.ID, req.SessionID, sessionhub.KindTurnError, payload, false)
		s.hub.Commit(wsp.ID, req.SessionID)
	}

	database := wsp.DB
	// Drop any crash-recovery sidecar when the turn returns by any normal path
	// (success, handled failure, client abort): only a true mid-turn process
	// death must leave it behind for the next boot to reclaim.
	defer database.ClearInflight(req.SessionID)

	// Ensure a terminal chat event fires even when generation fails after the
	// turn has begun: the frontend uses it to clear the post-reload "thinking"
	// indicator and reload the transcript. The success path sets emitted=true
	// and publishes its own richer event below.
	turnStarted := false
	emitted := false
	defer func() {
		if !turnStarted || emitted {
			return
		}
		wsp.Runtime.Emit(events.Event{
			Type:   "chat",
			Level:  "error",
			Title:  "Sohbet turu sonlandı",
			Target: map[string]string{"view": "chat", "sessionId": req.SessionID},
		})
	}()

	session, err := database.GetSession(ctx, req.SessionID)
	if err != nil {
		fail("session_not_found", "session not found")
		return
	}
	// Every session runs at most ONE turn at a time: claim the per-session turn slot
	// (blocking until any in-flight turn finishes) so this turn never overlaps a
	// concurrent direct /chat/stream call, a queued inbox turn, a scheduler wake, a
	// peer delivery, or (for a coordinator) an auto turn. Worker notifications
	// arriving mid-turn coalesce and trigger one auto turn on release. Claiming for
	// ALL sessions — not just coordinators — is what closes the plain-session
	// concurrent-turn race (_Docs/58).
	//
	// The send-queue worker claims the slot BEFORE it pops this message (so it stays
	// visible in the queue tray while it waits) and hands ownership over via
	// turnSlotHeld; claiming again here would deadlock the turn behind itself.
	if !req.turnSlotHeld {
		release := wsp.Runtime.BeginSessionUserTurn(session.ID)
		defer release()
	}
	firstTurn := s.isFirstUntitledTurn(session)
	// Captured before the user message is appended: primes cross-session context
	// on a fresh session's first turn.
	freshSession := session.MessageCount == 0

	// Resolve the ordered list of responding agents (default → session agent).
	agents := s.resolveTurnAgents(ctx, database, session, req.AgentIDs)
	if len(agents) == 0 {
		fail("agent_not_found", "agent not found")
		return
	}
	// A fresh session opened by @mentioning an agent adopts it as the main agent.
	session = s.adoptMentionedAgent(ctx, database, session, req.AgentIDs, agents)
	// Label the run with the responding agent's provider so the Session Info panel
	// can show which kind of background process is running (e.g. "claude-cli").
	run.setProvider(agents[0].Provider)
	// Record whether "Yönlendir" (mid-turn steer) can actually reach this turn, so
	// the control endpoint can tell the client to queue instead of silently
	// dropping it. claude-cli delivers a steer only at a permission-prompt tool
	// boundary, which exists solely in "ask"/"read-only" modes; "auto" runs the CLI
	// with --dangerously-skip-permissions (no such boundary). Native providers drain
	// the steer channel in the tool loop regardless of mode. Effective mode = the
	// request override when set, else the agent's own mode.
	effMode := agents[0].PermissionMode
	if req.PermissionMode != "" {
		effMode = req.PermissionMode
	}
	run.setSteerable(steerableForTurn(agents[0].Provider, effMode))

	// Persist the incoming user message once. Stamp the routed recipient agent
	// (agents[0]) so a multi-agent thread's history can show which agent each
	// question was directed at — the "@name" in the text is only informational and
	// does not route. Harmless in a 1:1 session (labelling only kicks in with 2+
	// agents).
	userMsg, err := database.AddMessage(ctx, db.Message{
		SessionID:   session.ID,
		Role:        providers.RoleUser,
		AgentID:     agents[0].ID,
		Text:        req.Message,
		Attachments: req.Attachments,
	})
	if err != nil {
		fail("persist_error", err.Error())
		return
	}
	// The turn is now committed (user message persisted); arm the terminal-event
	// guard so a later failure still notifies the frontend.
	turnStarted = true
	// Every file attached to a chat turn becomes a session artifact (origin chat).
	s.captureAttachmentArtifacts(ctx, database, session.ID, agents[0].ID, req.Attachments)

	// The legacy SSE writer (if any) was installed at the top; sse is a no-op when
	// none is set (queue worker path). All UI rides the hub regardless.
	sse := run.emit

	sse("meta", map[string]any{"userMessage": userMsg, "runId": runID})
	// Put the user message onto the session hub so EVERY window watching this
	// session (not just the one that submitted) renders it live, in order.
	s.publishHub(wsp.ID, session.ID, sessionhub.KindUserMessage, userMsg, false)

	// Wire the interactive asker: the ask_user tool emits a transient "ask" step
	// and blocks here until the client POSTs an answer (or the turn is stopped).
	// The tool loop runs in this same goroutine, so emitting via sse is safe.
	ctx = tools.WithAsker(ctx, func(ctx context.Context, question string, options []string) (string, error) {
		// Resolve-once interaction on the session hub: EVERY window renders the card
		// and the first to answer wins (CAS). clientGone still aborts a detached turn
		// whose user navigated away so the goroutine never leaks.
		pi := s.openInteraction(wsp.ID, session.ID, "ask", map[string]any{"question": question, "options": options})
		return s.waitInteraction(ctx, clientGone, pi)
	})

	// Multi-question asker: ask_user with several questions emits ONE interaction
	// carrying all of them; every window renders a combined form and any POSTs a JSON
	// array of answers, which FormatMultiAnswer folds into a single labeled block.
	ctx = tools.WithMultiAsker(ctx, func(ctx context.Context, questions []tools.AskQuestion) (string, error) {
		pi := s.openInteraction(wsp.ID, session.ID, "ask", map[string]any{"questions": questions})
		ans, err := s.waitInteraction(ctx, clientGone, pi)
		if err != nil {
			return "", err
		}
		return tools.FormatMultiAnswer(questions, ans), nil
	})

	// Session-scoped permission grants + the approval prompter for write/exec
	// tools under "ask" mode. The prompter emits a dedicated StepPermission card
	// and blocks on the same answer channel as ask_user; "Always allow" is
	// recorded in the session grants so it isn't re-asked. Also wired onto the run
	// so the CLI permission-prompt tool shares the same grant set.
	grants := s.grants.forSession(wsp.ID, session.ID)
	run.setGrants(grants)
	ctx = tools.WithGrants(ctx, grants)
	ctx = tools.WithPermissionPrompter(ctx, func(ctx context.Context, tool, risk, arg string, options []string) (string, error) {
		pi := s.openInteraction(wsp.ID, session.ID, "permission", map[string]any{
			"tool": tool, "reason": risk, "text": arg, "options": options,
		})
		return s.waitInteraction(ctx, clientGone, pi)
	})

	// Lifecycle hooks (Claude Code parity), fired once per user turn BEFORE any
	// agent runs. SessionStart primes a fresh session; UserPromptSubmit sees the
	// submitted prompt and may inject context (e.g. a caveman "respond terse"
	// ruleset) or BLOCK the turn entirely. Injected context is folded into every
	// responding agent's dynamic system prompt below. Fail-open by construction.
	var lifecycleContext string
	if freshSession {
		ss := wsp.Runtime.RunLifecycleHooks(ctx, session.ID, db.HookSessionStart, agent.LifecycleExtras{Source: "startup"})
		for _, st := range ss.Steps {
			sse("step", st)
		}
		lifecycleContext = ss.Context
	}
	ups := wsp.Runtime.RunLifecycleHooks(ctx, session.ID, db.HookUserPromptSubmit, agent.LifecycleExtras{Prompt: req.Message})
	for _, st := range ups.Steps {
		sse("step", st)
	}
	if c := strings.TrimSpace(ups.Context); c != "" {
		lifecycleContext = strings.TrimSpace(lifecycleContext + "\n\n" + c)
	}
	if ups.Block {
		// A UserPromptSubmit hook vetoed this prompt: persist the reason as the
		// assistant reply (so it survives reload) and end the turn without calling
		// any model. Not a failure — an intentional, hook-driven stop.
		reason := strings.TrimSpace(ups.Reason)
		if reason == "" {
			reason = "Prompt bir hook tarafından engellendi."
		}
		blockMsg, berr := database.AddMessage(ctx, db.Message{
			ID:        uuid.NewString(),
			SessionID: session.ID,
			Role:      providers.RoleAssistant,
			AgentID:   agents[0].ID,
			Text:      reason,
			Steps:     marshalSteps(ups.Steps),
		})
		if berr != nil {
			s.failTurn(ctx, wsp, sse, session.ID, agents[0].ID, clientMsgID, "hook_block_persist", berr.Error())
			return
		}
		sse("reply", map[string]any{"replyMessage": blockMsg})
		sse("done", map[string]any{"sessionTitle": strings.TrimSpace(session.Title)})
		emitted = true
		return
	}

	// Stop-hook continuation loop (Claude Code parity): after the agents answer, a
	// Stop hook may block to force another pass (e.g. "you forgot to run the tests").
	// stopContinue carries the block reason into the next pass as injected context;
	// bounded by maxStopPasses, and StopHookActive lets the hook detect it is already
	// in a forced continuation and relent.
	const maxStopPasses = 3
	stopContinue := ""
	for stopPass := 0; ; stopPass++ {
		passContext := lifecycleContext
		if stopContinue != "" {
			passContext = strings.TrimSpace(lifecycleContext + "\n\n[A Stop hook asked you to keep working before ending the turn]\n" + stopContinue)
		}

		// Each agent answers in turn, re-reading the (growing) history so later
		// agents see the earlier replies.
		for i, agentRow := range agents {
			// Per-turn reasoning override (local copy only — never persisted).
			if req.ThinkingLevel != "" {
				agentRow.ThinkingLevel = req.ThinkingLevel
			}
			if req.PermissionMode != "" {
				agentRow.PermissionMode = req.PermissionMode
			}
			provider, perr := s.providers.Get(agentRow.ProviderRef())
			if perr != nil {
				s.failTurn(ctx, wsp, sse, session.ID, agentRow.ID, clientMsgID, "provider_unavailable", perr.Error())
				return
			}
			// Pin this workspace's claude-home before Prepare's rolling compaction,
			// which folds via a direct provider.Complete (summarizeRendered) that
			// bypasses guardedComplete. Without this the claude-cli provider falls
			// back to the global claude-home and fails auth even when the workspace
			// is logged in (mirrors the manual /compact path in summary.go).
			if herr := wsp.Runtime.PinCLIHome(provider); herr != nil {
				s.failTurn(ctx, wsp, sse, session.ID, agentRow.ID, clientMsgID, "provider_unavailable", herr.Error())
				return
			}

			sse("agent", map[string]any{"agentId": agentRow.ID, "index": i})
			// Mirror agent-start onto the hub so late-joining windows know which
			// agent is answering (multi-agent threads render each turn's author).
			s.publishHub(wsp.ID, session.ID, sessionhub.KindAgentStart, map[string]any{"agentId": agentRow.ID, "index": i}, false)

			history, herr := database.ListMessages(ctx, session.ID)
			if herr != nil {
				s.failTurn(ctx, wsp, sse, session.ID, agentRow.ID, clientMsgID, "history_error", herr.Error())
				return
			}
			session, _ = database.GetSession(ctx, session.ID)
			// Raw (un-annotated) message list — the basis for the claude-cli resume delta
			// (stable indices, unlike the annotated history below). Includes this turn's
			// just-added user message.
			rawHistory := history
			// Annotate the history with each assistant turn's author so this agent can
			// tell who said what in a thread shared by several agents (no-op for a
			// single-agent session). multiAgent gates the explanatory system note.
			history, multiAgent := s.labelMultiAgentHistory(ctx, database, agentRow.ID, history)
			// Recap of recent turns' tool I/O (the trace is otherwise dropped when
			// history → provider messages). Rendered as a volatile dynamic block —
			// NOT folded into the history — so the history messages stay byte-stable
			// for the rolling prompt-cache breakpoint.
			toolRecap := recentToolActivityBlock(history)
			// The user's 👍/👎 on earlier turns. Same volatile-block treatment as the
			// tool recap (never folded into the history): a rating can be added, flipped
			// or cleared at any moment, and rewriting a cached history message for it
			// would bust the rolling prompt-cache breakpoint.
			feedbackRecap := recentFeedbackBlock(history)
			// Carry this workspace's editable compaction prompt onto the turn context.
			ctx = conversation.WithCompactPrompt(ctx, wsp.Runtime.CompactPromptTemplate())
			ctx = conversation.WithAttachmentRoot(ctx, wsp.SandboxRoot())
			ctx = conversation.WithClaudeHome(ctx, wsp.Runtime.ClaudeHomeDir())
			// Budget the fold against the TRUE per-turn footprint: the non-message
			// context (static prefix + tool/skill catalogs + eager schemas + artifacts)
			// ships every turn but Prepare only folds messages, so without this a large
			// static prefix keeps the message-only estimate under budget while the real
			// footprint runs over — the fold never fires (the 127%-but-never-compacted
			// coordinator case). systemFillers is the same basis the context meter uses.
			ctx = conversation.WithContextOverhead(ctx, s.contextOverheadTokens(ctx, wsp, session, history, multiAgent))
			// PreCompact lifecycle hook (Claude Code parity): Prepare invokes this just
			// before it folds older turns into the rolling summary. Fire-and-forget audit.
			ctx = conversation.WithPreCompact(ctx, func(trigger string) {
				pc := wsp.Runtime.RunLifecycleHooks(ctx, session.ID, db.HookPreCompact, agent.LifecycleExtras{Trigger: trigger})
				for _, st := range pc.Steps {
					sse("step", st)
				}
			})
			prep, cerr := s.convo.Prepare(ctx, database, provider, session, agentRow, history)
			if cerr != nil {
				s.failTurn(ctx, wsp, sse, session.ID, agentRow.ID, clientMsgID, "compaction_failed", "compaction failed: "+cerr.Error())
				return
			}

			llmReq := s.composeTurnRequest(ctx, wsp, session, agentRow, agents, req.Message, prep, freshSession, multiAgent, toolRecap, feedbackRecap, passContext)
			// Prompt-epoch drift step: if the static context changed since the frozen
			// snapshot, surface a context_change step at the head of the turn (once per
			// drift episode). Emitted live and prepended to the persisted trace so the
			// chat history shows when the change landed. The agent already read the diff
			// via the dynamic-suffix note composeTurnRequest injected.
			leadSteps := consumeContextChangeLead(wsp.Runtime, session.ID, agentRow.ID)
			// Auto-compaction visibility: Prepare folds older history into the rolling
			// summary silently, inside this already-serialized turn (it holds the inbox
			// slot, so it cannot and must not re-enter the send-queue like the manual
			// /compact command — that would self-deadlock on the serial slot). What it
			// lacked was on-screen presence. Surface it as a lead step on the SAME hub
			// channel the manual command uses (live SSE + cross-window publish + persisted
			// trace via leadSteps below) so the fold shows up like any other turn event.
			if prep.Compacted {
				leadSteps = append([]agent.TurnStep{compactionLeadStep(prep.FoldedMsgs)}, leadSteps...)
			}
			for _, st := range leadSteps {
				sse("step", st)
				wsp.Runtime.EmitSessionStep(session.ID, st)
				s.publishHub(wsp.ID, session.ID, sessionhub.KindStep, st, false)
			}
			// claude-cli session resume (opt-in): when engaged, this trims llmReq to the
			// unseen delta and sets ResumeSessionID so the CLI reuses its warm cache.
			resumePlan := s.planClaudeResume(ctx, provider, len(agents), session, rawHistory, prep.Compacted, &llmReq)

			// Attach a per-agent artifact sink so create_artifact / update_artifact
			// persist content stamped with this session + agent — both on the native
			// tool path (via context) and the CLI path (via the run, used by the
			// Interaction MCP backend).
			sink := newArtifactSink(database, session.ID, agentRow.ID, wsp.Runtime.Emit)
			run.setArtifacts(sink)
			turnCtx := tools.WithArtifacts(ctx, sink)
			// Attach a per-agent notify sink so the notify tool can raise a desktop
			// notification on both tool paths (native via context, CLI via the run, used
			// by the Interaction MCP backend). It publishes an "agent" event onto the
			// workspace bus → SSE → OS toast (per device prefs).
			nsink := newNotifySink(session.ID, agentRow.ID, wsp.Runtime.Emit)
			// Notification lifecycle hook (Claude Code parity): fire for every notify call
			// so an audit/relay hook (e.g. forward to Slack) sees agent notifications.
			nsink.onNotify = func(spec tools.NotifySpec) {
				wsp.Runtime.RunLifecycleHooks(turnCtx, session.ID, db.HookNotification,
					agent.LifecycleExtras{Message: strings.TrimSpace(spec.Title + " " + spec.Body)})
			}
			run.setNotify(nsink)
			turnCtx = tools.WithNotify(turnCtx, nsink)
			// focus_view shares the same sink (it implements NavigateSink too): an agent
			// can drive the UI to a view/entity on both tool paths.
			run.setNav(nsink)
			turnCtx = tools.WithNavigate(turnCtx, nsink)
			// Session sink: one sink for every session-scoped mutation — title, working
			// dir, tags, archive — on both tool
			// tools and the session-edit tools alike; each mutation emits a "session"
			// event so open windows refresh live.
			ssink := wsp.Runtime.NewSessionSink(session.ID)
			run.setSession(ssink)
			turnCtx = tools.WithSession(turnCtx, ssink)
			// Persistent progress: bind a todo sink so todo_write persists the checklist
			// to the project's progress file on both tool paths (native via context, CLI
			// via the run). Keyed to this session's working dir. Gated by ProgressPersist.
			if s.tun.ProgressPersist() {
				todoSink := wsp.Runtime.NewTodoSink(session.ID, agentRow.ID)
				run.setTodoSink(todoSink)
				turnCtx = tools.WithTodoSink(turnCtx, todoSink)
			}
			// Stamp the session id so the runtime can resolve this session's WorkingDir
			// (cwd) for the fs/shell sandbox and provider cwd.
			turnCtx = agent.WithSessionID(turnCtx, session.ID)

			// Wire the self-wake scheduler for THIS agent + session, on both tool paths:
			// the native built-in reads it from the context; the CLI path reaches it via
			// the run (used by the Interaction MCP schedule_wake dispatch). A wake arms a
			// one-shot schedule that re-delivers a prompt into this session as a fresh
			// turn, so the conversation continues on its own.
			respondingID := agentRow.ID
			wakeFn := func(wctx context.Context, delaySeconds int, prompt, reason string) (string, error) {
				return wsp.Runtime.ScheduleWake(wctx, session.ID, respondingID, prompt, reason, delaySeconds)
			}
			turnCtx = tools.WithWakeScheduler(turnCtx, wakeFn)
			run.setWakeScheduler(wakeFn)

			// use_skill (CLI path): mirror the native built-in for claude-cli agents,
			// which reach TionHarness skills only through the Interaction MCP bridge. The
			// loader enforces the same per-agent allowlist as the native use_skill tool,
			// so a restricted skill stays unreachable unless assigned/shared.
			skillAgent := agentRow
			run.setSkillLoader(func(slug string) (string, error) {
				return wsp.Runtime.LoadSkillForAgent(skillAgent, slug)
			})
			// skill_search (CLI path): same per-agent allowlist; lets a claude-cli agent
			// discover on-demand/conditional skills not in its appended catalog. (SK-2)
			run.setSkillSearcher(func(query string, limit int) []tools.SkillHit {
				return wsp.Runtime.SearchSkillsForAgent(skillAgent, query, limit)
			})
			// SK-3 (CLI path): loading a skill auto-grants its declared allowed-tools.
			run.setSkillAllowed(func(slug string) []string {
				return wsp.Runtime.SkillAllowedToolsForAgent(skillAgent, slug)
			})

			// shell (CLI path): mirror the native built-in for claude-cli agents, which
			// reach TionHarness tools only through the Interaction MCP bridge. When shell is
			// enabled, install a sandboxed runner so the bridge's shell dispatch runs
			// commands through TionHarness's PowerShell shell — letting the CLI's own POSIX
			// Bash be safely disallowed. nil when shell is off (then Bash stays allowed).
			run.setShellRunner(wsp.Runtime.NewShellRunner())

			// Spawn (CLI path): mirror the native built-in for claude-cli agents, which
			// reach TionHarness tools only through the Interaction MCP bridge. Install a
			// per-agent spawn tool on the run so the bridge's spawn_session dispatch can
			// launch independent sessions. Self-management is always on now; a fresh
			// instance per turn resets the per-turn spawn budget.
			run.setSpawnTool(tools.NewSpawnSessionTool(respondingID, s.tun.SpawnMaxPerTurn(),
				func(sctx context.Context, target, prompt, modelOverride string) (tools.SpawnResult, error) {
					// Inherit this chat session's working directory, so a spawn from a
					// session pinned to repo A does not silently open in the workspace
					// default directory.
					res, err := wsp.Runtime.SpawnSession(sctx, target, prompt, agent.SpawnOptions{
						ModelOverride: modelOverride,
						CreatedBy:     respondingID,
						WorkingDir:    wsp.Runtime.SessionWorkdir(session.ID),
					})
					return tools.SpawnResult{SessionID: res.SessionID, AgentName: res.AgentName, Queued: res.Queued, QueuePosition: res.QueuePosition}, err
				}))

			// run_subagent (CLI path): mirror the native delegation built-in so a
			// claude-cli agent can hand a self-contained sub-task to another agent and
			// get the answer back IN THIS turn (unlike spawn_session's fire-and-forget
			// into a separate session). nil when delegation is off. Bound to this agent
			// so depth / cycle / per-turn budget guards apply.
			run.setRunAgent(wsp.Runtime.RunSubagentRunner(agentRow, run.autonomous))

			// Self-management bridge (CLI path, CLI-3): claude-cli has no native
			// activate_tools loop, so advertise the responding agent's lazy
			// self-management tools up front through the Interaction MCP and dispatch
			// them through the same registry the native loop uses. Empty when
			// self-manage is off. Re-point the CLI MCP endpoint for THIS agent turn so
			// its allowlist carries the static interaction tools + the bridged tools.
			bridgeDefs, bridgeCall := wsp.Runtime.BridgeTools(turnCtx, agentRow)
			run.setBridge(bridgeDefs, bridgeCall)
			// Visibility-aware CLI wire split: full→core (eager), summary/name-only→
			// extended (deferred), hidden→neither. Installed on the run so tools/list
			// (Tools) classifies identically to the allowlist built here.
			visOf := wsp.Runtime.ToolVisibilityFunc(turnCtx, agentRow)
			run.setTierVis(visOf)
			// Effective tool filter (workspace DisabledTools + agent denylist), installed
			// so the Interaction bridge's tools/list and the allowlist drop tools the
			// native ToolCatalog would (e.g. a workspace disabling PowerShell to force Bash).
			allowOf := wsp.Runtime.ToolAllowedFunc(turnCtx, agentRow)
			run.setToolAllowed(allowOf)
			if url := s.interactionURL(); url != "" {
				names := filterAllowedNames(interactionAdvertisedNames(s.tun, false), allowOf)
				coreNames, extNames := splitInteractionTiers(names, bridgeDefs, visOf)
				// Stable per-(session,agent) Bearer token (not run.token): keeps the CLI
				// mcp-config byte-identical across turns so a persistent process stays warm
				// (Doc 52 §3-D). bindActive resolves it to this in-flight run.
				tok := s.runs.interactionToken(wsp.ID, req.SessionID, agentRow.ID)
				s.runs.bindActive(tok, run)
				turnCtx = tools.WithInteractionEndpoint(turnCtx, url, tok, coreNames, extNames)
			}

			agentStart := time.Now()
			// Pre-allocate the reply id so the streaming crash sidecar and the final
			// persisted message share one identity (recovery is then idempotent).
			replyID := uuid.NewString()
			// Tag every debug event emitted during this turn with the reply id, so the
			// per-message debug panel can fetch exactly this message's spend/latency.
			turnCtx = agent.WithTurnID(turnCtx, replyID)
			// Durable Ask (MVP): opt this interactive turn into durable ask_user
			// suspend/resume. At a clean suspend point the native loop returns an
			// *askSuspend sentinel instead of blocking the asker; the intercept below
			// parks it to disk + opens a durable card. The blocking asker (wired above)
			// still handles non-clean asks (parallel/PTC) as the fallback.
			turnCtx = agent.WithDurableAsk(turnCtx)
			// Snapshot the in-flight reply to disk on a throttle: the partial answer
			// text (accumulated from streaming deltas) plus the persistable trace so
			// far. A mid-turn process death leaves this sidecar for boot to reclaim.
			var partial strings.Builder
			var kept []agent.TurnStep
			var lastSnap time.Time
			snapshot := func() {
				if time.Since(lastSnap) < 600*time.Millisecond {
					return
				}
				lastSnap = time.Now()
				_ = database.WriteInflight(db.InflightTurn{
					MessageID: replyID,
					SessionID: session.ID,
					AgentID:   agentRow.ID,
					StartedAt: agentStart.Unix(),
					Text:      partial.String(),
					Steps:     marshalSteps(kept),
				})
			}
			resp, steps, cerr := wsp.Runtime.CompleteWithToolsStream(turnCtx, agentRow, provider, llmReq, false,
				func(st agent.TurnStep) {
					agent.TouchActivity(turnCtx)
					sse("step", st)
					// Mirror the step onto the process-wide bus so OTHER windows viewing
					// this session render it live too. The originating window ignores the
					// bus copy (it owns the run and streams over its own per-request SSE);
					// the emitter drops high-frequency/interactive kinds itself.
					wsp.Runtime.EmitSessionStep(session.ID, st)
					switch st.Kind {
					case agent.StepDelta:
						partial.WriteString(st.Text)
						// Ephemeral token growth: best-effort live, seq 0, not retained.
						s.publishHub(wsp.ID, session.ID, sessionhub.KindDelta, st, true)
					case agent.StepToolDelta:
						s.publishHub(wsp.ID, session.ID, sessionhub.KindToolDelta, st, true)
					case agent.StepTombstone:
						s.publishHub(wsp.ID, session.ID, sessionhub.KindTombstone, st, true)
					case agent.StepAsk, agent.StepPermission, agent.StepPlan:
						// Interactive prompts are handled by the Phase 2 interaction CAS
						// (interaction_open/resolved), not broadcast as plain hub steps —
						// otherwise a passive window would show a card it cannot resolve.
					default:
						kept = append(kept, st)
						// Durable activity (thinking/tool/todo/diff/recovery/error/…) →
						// seq'd on the hub so every window renders it live and a reconnect
						// gap-fills it from the ring.
						s.publishHub(wsp.ID, session.ID, sessionhub.KindStep, st, false)
					}
					snapshot()
				},
			)
			// Prompt-cache break card: the completion's usage revealed that this turn
			// re-paid the whole cached prefix because the model or the prompt/tool
			// schemas changed. Appended to the lead so it sits at the head of the
			// persisted trace (that is where the cold prefix was paid) on every exit
			// path below — success, provider error and durable-ask suspend alike.
			leadSteps = append(leadSteps, consumeCacheBreakLead(wsp.Runtime, session.ID)...)
			// Durable Ask suspend: the native loop parked at a clean ask_user point.
			// This is NOT an error — persist the suspend snapshot (lead + loop trace),
			// open a durable card keyed to the ask id, clear the crash sidecar (the wait
			// is now durable, not a mid-turn orphan), and return the goroutine cleanly.
			// The answer endpoint re-drives the turn via ResumeAsk.
			if cerr != nil && !errors.Is(context.Cause(ctx), agent.ErrTurnHardTimeout) && !errors.Is(context.Cause(ctx), agent.ErrTurnIdleTimeout) {
				snapSteps := append(append([]agent.TurnStep{}, leadSteps...), steps...)
				if ask, suspended, perr := wsp.Runtime.SuspendAskFromError(context.WithoutCancel(ctx), agentRow, session.ID, llmReq, snapSteps, cerr); suspended {
					if perr != nil {
						s.logger.Error("durable ask: persist suspend failed", "session", session.ID, "error", perr)
					} else {
						_ = database.ClearInflight(session.ID)
						s.openDurableAskCard(wsp.ID, session.ID, ask, false)
						s.hub.Commit(wsp.ID, session.ID)
						s.logger.Info("durable ask: turn suspended", "session", session.ID, "ask", ask.ID)
						sse("ask_suspended", map[string]any{"askId": ask.ID})
						return
					}
				}
			}
			if cerr != nil {
				// Distinguish a manual Stop (run.cancel cancelled ctx) from a genuine
				// provider failure. Either way, PRESERVE the partial trace accumulated so
				// far (kept) so the tools/text the agent already produced stay visible
				// instead of vanishing — append an error/stopped step at the end.
				cause := context.Cause(ctx)
				stopped := errors.Is(cause, context.Canceled)
				detail, reason := chatTurnFailure(cause, cerr)
				switch reason {
				case reasonTurnHardTimeout:
					s.logger.Info("chat turn hit hard timeout", "session", session.ID, "agent", agentRow.ID)
				case reasonTurnIdleTimeout:
					s.logger.Info("chat turn stalled: no step within the inactivity window",
						"session", session.ID, "agent", agentRow.ID, "idle", s.chatTurnIdle().String())
				case reasonStopped:
					s.logger.Info("chat turn stopped by user", "session", session.ID, "agent", agentRow.ID)
				default:
					s.logger.Error("stream completion failed", "error", cerr, "agent", agentRow.ID)
				}
				trace := append(append(leadSteps, kept...), agent.TurnStep{Kind: agent.StepError, Text: detail, Reason: reason})
				// Persist with a detached context so a cancelled (stopped) ctx still saves.
				persistCtx := context.WithoutCancel(ctx)
				payload := map[string]any{"error": detail, "reason": reason, "clientMsgId": clientMsgID}
				if msg, aerr := database.AddMessage(persistCtx, db.Message{
					ID:        replyID,
					SessionID: session.ID,
					Role:      providers.RoleAssistant,
					AgentID:   agentRow.ID,
					Text:      partial.String(),
					Steps:     marshalSteps(trace),
					// A user Stop is Cancelled (clean, intentional); a provider failure
					// keeps Interrupted (the "cut off" banner) as before.
					Cancelled:   stopped,
					Interrupted: !stopped,
					DurationMs:  time.Since(agentStart).Milliseconds(),
				}); aerr != nil {
					s.logger.Error("persist interrupted turn failed", "session", session.ID, "error", aerr)
				} else {
					payload["replyMessage"] = msg
					// Push the interrupted/stopped reply onto the hub so other windows
					// render the preserved partial instead of a dangling live bubble.
					s.publishHub(wsp.ID, session.ID, sessionhub.KindReply, msg, false)
					s.hub.Commit(wsp.ID, session.ID)
				}
				_ = database.ClearInflight(session.ID)
				// Auto-tag the turn failure (skips a clean user "stopped"), plus any real
				// tool error captured before the failure.
				wsp.Runtime.AutoTagTurn(context.WithoutCancel(ctx), session.ID, trace, reason)
				sse("error", payload)
				// Terminal error onto the hub so every window clears its "thinking"
				// indicator and shows the failure, not just the submitting window.
				s.publishHub(wsp.ID, session.ID, sessionhub.KindTurnError, payload, false)
				s.hub.Commit(wsp.ID, session.ID)
				return
			}

			replyMsg, aerr := database.AddMessage(ctx, db.Message{
				ID:         replyID,
				SessionID:  session.ID,
				Role:       providers.RoleAssistant,
				AgentID:    agentRow.ID,
				Text:       resp.Text,
				Steps:      marshalSteps(append(leadSteps, steps...)),
				Model:      resp.Model,
				StopReason: resp.StopReason,
				Usage:      messageUsage(resp.Usage),
				DurationMs: time.Since(agentStart).Milliseconds(),
			})
			if aerr != nil {
				s.failTurn(ctx, wsp, sse, session.ID, agentRow.ID, clientMsgID, "persist_error", aerr.Error())
				return
			}
			// Persist the rotated claude-cli session id so the NEXT turn resumes it and
			// sends only the new delta. sentCount+1 accounts for this turn's assistant
			// reply, which the CLI already holds server-side (no need to resend it).
			if resumePlan.active && resp.SessionID != "" {
				if rerr := database.SetSessionCLIResume(ctx, session.ID, resp.SessionID, resumePlan.sentCount+1); rerr != nil {
					s.logger.Warn("persist cli resume state failed", "session", session.ID, "error", rerr)
				}
				// P1.4: claude-cli resume model mismatch — the CLI may have served the
				// response with a different model than the agent's current configuration
				// (e.g. agent was reconfigured but the warm CLI session still runs the old
				// model). Log a debug event so the discrepancy is diagnosable.
				if resp.Model != "" && resp.Model != agentRow.Model {
					s.logger.Info("cli-resume-model-mismatch",
						"session", session.ID, "agent", agentRow.ID,
						"agent_model", agentRow.Model, "cli_model", resp.Model,
						"cli_session", resp.SessionID)
				}
			}
			// P1.1: update the session header's model snapshot when the actual
			// response model differs — keeps the header's O(1) answer current.
			if resp.Model != "" && resp.Model != session.Model {
				if merr := database.SetSessionModel(ctx, session.ID, resp.Model); merr != nil {
					s.logger.Warn("set session model failed", "session", session.ID, "error", merr)
				}
			}
			// Reply is durable now; drop this agent's sidecar before the next agent
			// (the top-level defer is the catch-all for early-return paths).
			_ = database.ClearInflight(session.ID)
			sse("reply", map[string]any{"replyMessage": replyMsg})
			// Canonical reply onto the hub: every window replaces its live-accumulated
			// bubble with this persisted, authoritative message (steps + usage + model).
			s.publishHub(wsp.ID, session.ID, sessionhub.KindReply, replyMsg, false)
			// This agent's turn is now in the persisted transcript → a fresh
			// subscriber need not replay it (only the next agent's in-flight tail).
			s.hub.Commit(wsp.ID, session.ID)

			s.logger.Info("chat turn completed",
				"session", session.ID, "agent", agentRow.Name, "provider", agentRow.Provider,
				"model", resp.Model, "in", resp.Usage.InputTokens, "out", resp.Usage.OutputTokens,
				"steps", len(steps), "stream", true,
				"dur", time.Since(agentStart).Round(time.Millisecond).String())

			// Auto-tag: derive session tags from this turn (tool-error / error /
			// archived) so an automation can later scan + repair them.
			wsp.Runtime.AutoTagTurn(ctx, session.ID, steps, "")

			// Tag-triggered automations: signal that this session finished a turn. The
			// runtime dispatches it detached, so a tagged session completing can spawn a
			// follow-up (the automation loop) without blocking this turn. Fired per
			// responding agent so a multi-agent turn's last reply carries the result.
			wsp.Runtime.FireTurnFinished(session.ID, agentRow.ID, resp.Text)
		}

		// Stop lifecycle hook (Claude Code parity): the main agent(s) finished this
		// pass. The hook may inject audit context and — via decision:"block" — force
		// one more pass with the reason as guidance (StopHookActive marks a forced
		// continuation so the hook can relent; maxStopPasses is the hard backstop).
		stop := wsp.Runtime.RunLifecycleHooks(ctx, session.ID, db.HookStop,
			agent.LifecycleExtras{StopHookActive: stopPass > 0})
		for _, st := range stop.Steps {
			sse("step", st)
		}
		if !stop.Block || stopPass >= maxStopPasses {
			break
		}
		stopContinue = stop.Reason
	}

	// Auto-title once, after the turn, using the first responding agent.
	sessionTitle := s.maybeAutoTitle(ctx, wsp, firstTurn, agents[0].ID, session.ID, req.Message)

	sse("done", map[string]any{"sessionTitle": sessionTitle})
	// Terminal success onto the hub: every window stops its live indicator and
	// picks up the (possibly new) session title. clientMsgId lets a queue observer
	// (legacy /chat + /chat/stream) recognise its own turn's completion.
	s.publishHub(wsp.ID, session.ID, sessionhub.KindTurnDone, map[string]any{"sessionTitle": sessionTitle, "clientMsgId": clientMsgID}, false)
	s.hub.Commit(wsp.ID, session.ID)

	// Publish a chat-completion event so other workspaces can flag activity with
	// a badge when the user is viewing a different workspace. The frontend uses
	// chat events only for the badge (not a duplicate desktop notification).
	title := strings.TrimSpace(session.Title)
	if sessionTitle != "" {
		title = sessionTitle
	}
	if title == "" {
		title = "Sohbet"
	}
	emitted = true
	wsp.Runtime.Emit(events.Event{
		Type:   "chat",
		Level:  "success",
		Title:  "Yanıt hazır: " + title,
		Body:   req.Message,
		Target: map[string]string{"view": "chat", "sessionId": session.ID},
	})
}

// resolveTurnAgents turns the requested agent ids (from "@mention" routing) into
// an ordered, de-duplicated list of existing agents. Falls back to the session's
// default agent when none are given or valid.
func (s *Server) resolveTurnAgents(ctx context.Context, database *db.DB, session db.Session, ids []string) []db.Agent {
	var out []db.Agent
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		if a, err := database.GetAgent(ctx, id); err == nil {
			seen[id] = true
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		if a, err := database.GetAgent(ctx, session.AgentID); err == nil {
			out = append(out, a)
		}
	}
	return out
}

// failTurn records a turn-level failure as a persisted assistant message so the
// error shows up in the chat hierarchy (with its detail) and survives a reload,
// then notifies the client via the SSE `error` event carrying that message.
// agentID is the responding agent (may be "" if none was selected yet); reason
// is a stable machine tag (provider_error, compaction_failed, …) shown as a
// badge; detail is the human-readable message.
func (s *Server) failTurn(ctx context.Context, wsp *workspace.Workspace, sse func(string, any), sessionID, agentID, clientMsgID, reason, detail string) {
	database := wsp.DB
	// Surface the failure in the server log too — without this a turn that dies
	// before producing output (provider unavailable, compaction failure, …) is
	// invisible server-side and only visible as a red card in the UI.
	s.logger.Error("turn failed", "session", sessionID, "agent", agentID, "reason", reason, "detail", detail)
	step := agent.TurnStep{Kind: agent.StepError, Text: detail, Reason: reason}
	payload := map[string]any{"error": detail, "reason": reason, "clientMsgId": clientMsgID}
	if msg, err := database.AddMessage(ctx, db.Message{
		SessionID: sessionID,
		Role:      providers.RoleAssistant,
		AgentID:   agentID,
		Steps:     marshalSteps([]agent.TurnStep{step}),
	}); err != nil {
		s.logger.Error("persist turn error failed", "session", sessionID, "error", err)
	} else {
		payload["replyMessage"] = msg
	}
	sse("error", payload)
	// Terminal error onto the hub too, so every window (and a queue observer waiting
	// on this turn) clears its "thinking" state and sees the failure — previously
	// failTurn only wrote the legacy SSE sink, leaving hub clients to discover it on
	// reload and the /chat + /chat/stream queue observers hanging.
	s.publishHub(wsp.ID, sessionID, sessionhub.KindTurnError, payload, false)
	s.hub.Commit(wsp.ID, sessionID)
}
